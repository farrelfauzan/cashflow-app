// Package tenantimport loads an organization's existing properties, units, tenants and leases
// from a CSV file, so a new organization does not retype its notebook.
//
// Dry run and commit share one code path: the whole file runs in one transaction, each row in
// its own savepoint so one bad row does not hide the next. A dry run always rolls back. A commit
// keeps the transaction only when no row has an error, so it saves all rows or none.
package tenantimport

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Action says what a row did, or would do, to a record.
type Action string

// The actions.
const (
	Created Action = "created"
	Matched Action = "matched"
)

// RowError is one problem in a row. Column is the canonical column name.
type RowError struct {
	Column  string
	Message string
}

// Row is the outcome of one data row.
type Row struct {
	Line   int
	Errors []RowError
	Notes  []string
	// What happened to each record; empty when the row stopped before reaching it.
	Property, Unit, Tenant, Lease Action
}

// OK reports whether the row has no errors.
func (r Row) OK() bool { return len(r.Errors) == 0 }

// Summary counts the rows and what they created.
type Summary struct {
	Rows, RowsWithErrors                                           int
	PropertiesCreated, UnitsCreated, TenantsCreated, LeasesCreated int
	LeasesMatched                                                  int
}

// Result is the outcome of an import.
type Result struct {
	DryRun    bool
	Committed bool
	Summary   Summary
	Rows      []Row
}

// errDiscard rolls back the import transaction without it being a failure.
var errDiscard = errors.New("tenantimport: discard")

// Service runs imports through the property and tenant services, so every rule and permission
// check is theirs.
type Service struct {
	store      *store.Store
	authz      *authz.Service
	properties *property.Service
	tenants    *tenant.Service
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service, props *property.Service, tenants *tenant.Service) *Service {
	return &Service{store: st, authz: az, properties: props, tenants: tenants}
}

// Run imports the CSV in data. It needs tenant:manage and lease:manage for the whole
// organization. A file that cannot be read returns a *FileError and no rows.
func (s *Service) Run(ctx context.Context, p authz.Principal, data []byte, dryRun bool) (Result, error) {
	records, err := parseFile(data)
	if err != nil {
		return Result{}, err
	}
	res := Result{DryRun: dryRun, Rows: make([]Row, len(records))}
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		for _, action := range []string{authz.ActionTenantManage, authz.ActionLeaseManage} {
			req := authz.Request{Action: action, Resource: authz.Resource{Type: strings.TrimSuffix(action, ":manage")}}
			if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
				return err
			}
		}
		for i, rec := range records {
			row, err := s.importRow(ctx, tx, p, rec)
			if err != nil {
				return fmt.Errorf("line %d: %w", rec.Line, err)
			}
			res.Rows[i] = row
			res.Summary.add(row)
		}
		if dryRun || res.Summary.RowsWithErrors > 0 {
			return errDiscard
		}
		return nil
	})
	if errors.Is(err, errDiscard) {
		return res, nil
	}
	if err != nil {
		return Result{}, err
	}
	res.Committed = true
	return res, nil
}

// importRow checks one record and, when it is valid, finds or creates its property, unit, tenant
// and lease inside a savepoint. Row problems come back in the row; only unexpected failures and
// permission denials are returned as errors, and they stop the whole import.
func (s *Service) importRow(ctx context.Context, tx pgx.Tx, p authz.Principal, rec record) (Row, error) {
	row := Row{Line: rec.Line}
	in := check(rec, &row)
	if !row.OK() {
		return row, nil
	}
	sp, err := tx.Begin(ctx) // a savepoint: a failed row leaves the rest of the import usable
	if err != nil {
		return row, fmt.Errorf("opening savepoint: %w", err)
	}
	if err := s.write(ctx, sp, p, in, &row); err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return row, fmt.Errorf("rolling back savepoint: %w", rbErr)
		}
		if rowErr, ok := asRowError(err); ok {
			row.Errors = append(row.Errors, rowErr)
			row.Property, row.Unit, row.Tenant, row.Lease = "", "", "", ""
			return row, nil
		}
		return row, err
	}
	if err := sp.Commit(ctx); err != nil {
		return row, fmt.Errorf("releasing savepoint: %w", err)
	}
	return row, nil
}

// input is a record turned into the services' inputs, already validated by them.
type input struct {
	property property.PropertyInput
	unit     property.UnitInput
	tenant   tenant.Input
	terms    tenant.Terms
}

// check parses a record and validates it with the property and tenant rules, adding every
// problem it finds to row.
func check(rec record, row *Row) input {
	fail := func(col, msg string) { row.Errors = append(row.Errors, RowError{Column: col, Message: msg}) }

	rent, ok := parseRupiah(rec.get(colRent))
	if !ok {
		fail(colRent, "Harga sewa harus angka rupiah, contoh 850000 atau 850.000.")
	}
	deposit, ok := parseRupiah(rec.get(colDeposit))
	if !ok {
		fail(colDeposit, "Jaminan harus angka rupiah, contoh 850000 atau 850.000.")
	}
	dueDay, ok := parseDay(rec.get(colDueDay))
	if !ok {
		fail(colDueDay, "Tanggal jatuh tempo harus angka 1 sampai 28.")
	}
	start, ok := parseDate(rec.get(colStartDate))
	if !ok {
		fail(colStartDate, "Tanggal mulai harus seperti 2026-10-01 atau 01/10/2026.")
	}
	parsed := len(row.Errors) == 0

	var in input
	var err error
	if in.property, err = property.ValidProperty(property.PropertyInput{
		Name: rec.get(colProperty), Kind: parseKind(rec.get(colPropertyKind)),
	}); err != nil {
		addInputError(row, err)
	}
	unit := property.UnitInput{Label: rec.get(colUnit)}
	if rent > 0 {
		unit.DefaultRent = &rent // a new unit takes the lease's rent as its usual rent
	}
	if in.unit, err = property.ValidUnit(unit); err != nil {
		addInputError(row, err)
	}
	if in.tenant, err = tenant.ValidTenant(tenant.Input{Name: rec.get(colTenantName), Phone: rec.get(colPhone)}); err != nil {
		addInputError(row, err)
	}
	if parsed {
		if in.terms, err = tenant.ValidTerms(tenant.Terms{
			StartDate: start, RentAmount: rent, DueDay: dueDay, DepositAmount: deposit,
		}); err != nil {
			addInputError(row, err)
		}
	}
	return in
}

// write finds or creates the row's records through the services, inside the savepoint tx.
func (s *Service) write(ctx context.Context, tx pgx.Tx, p authz.Principal, in input, row *Row) error {
	org := p.OrganizationID

	prop, err := property.FindByName(ctx, tx, org, in.property.Name)
	row.Property = Matched
	if errors.Is(err, property.ErrNotFound) {
		prop, err = s.properties.CreateTx(ctx, tx, p, in.property)
		row.Property = Created
	}
	if err != nil {
		return err
	}

	unit, err := property.FindUnit(ctx, tx, org, prop.ID, in.unit.Label)
	row.Unit = Matched
	if errors.Is(err, property.ErrNotFound) {
		unit, err = s.properties.CreateUnitTx(ctx, tx, p, prop.ID, in.unit)
		row.Unit = Created
	}
	if err != nil {
		return err
	}

	tn, err := tenant.FindByPhone(ctx, tx, org, in.tenant.Phone)
	row.Tenant = Matched
	if errors.Is(err, tenant.ErrNotFound) {
		tn, err = s.tenants.CreateTx(ctx, tx, p, in.tenant)
		row.Tenant = Created
	}
	if err != nil {
		return err
	}
	if row.Tenant == Matched && !strings.EqualFold(tn.Name, in.tenant.Name) {
		row.Notes = append(row.Notes, fmt.Sprintf("Nomor ini sudah terdaftar atas nama %s; nama itu yang dipakai.", tn.Name))
	}

	current, err := tenant.ActiveLeaseForUnit(ctx, tx, org, unit.ID)
	switch {
	case err == nil && current.TenantID == tn.ID:
		row.Lease = Matched
		if current.RentAmount != in.terms.RentAmount || current.DueDay != in.terms.DueDay || !current.StartDate.Equal(in.terms.StartDate) {
			row.Notes = append(row.Notes, "Sewa ini sudah ada dengan isian lain; isian baris ini tidak mengubahnya.")
		}
		return nil
	case err == nil:
		return &rowError{RowError{Column: colUnit, Message: "Unit ini masih disewa penyewa lain."}}
	case !errors.Is(err, tenant.ErrNotFound):
		return err
	}
	_, err = s.tenants.CreateLeaseTx(ctx, tx, p, tenant.NewLease{TenantID: tn.ID, UnitID: unit.ID, Terms: in.terms})
	row.Lease = Created
	return err
}

// rowError carries a row problem found while writing.
type rowError struct{ RowError }

func (e *rowError) Error() string { return "tenantimport: " + e.Column + ": " + e.Message }

// asRowError turns the errors a row can cause into a RowError. Anything else stops the import.
func asRowError(err error) (RowError, bool) {
	var re *rowError
	var propertyInput *property.InputError
	var tenantInput *tenant.InputError
	switch {
	case errors.As(err, &re):
		return re.RowError, true
	case errors.As(err, &propertyInput):
		return RowError{Column: propertyColumns[propertyInput.Field], Message: propertyInput.Message}, true
	case errors.As(err, &tenantInput):
		return RowError{Column: tenantColumns[tenantInput.Field], Message: tenantInput.Message}, true
	case errors.Is(err, tenant.ErrUnitOccupied):
		return RowError{Column: colUnit, Message: "Unit ini masih disewa penyewa lain."}, true
	case errors.Is(err, tenant.ErrPhoneTaken):
		return RowError{Column: colPhone, Message: "Nomor ini sudah dipakai penyewa lain."}, true
	case errors.Is(err, property.ErrNameTaken):
		return RowError{Column: colProperty, Message: "Nama properti ini sudah dipakai."}, true
	case errors.Is(err, property.ErrLabelTaken):
		return RowError{Column: colUnit, Message: "Nama unit ini sudah dipakai di properti ini."}, true
	}
	return RowError{}, false
}

// The services name fields as their own inputs do; the import names the CSV columns.
var (
	propertyColumns = map[string]string{"name": colProperty, "kind": colPropertyKind, "label": colUnit, "defaultRent": colRent}
	tenantColumns   = map[string]string{
		"name": colTenantName, "phone": colPhone, "startDate": colStartDate, "rentAmount": colRent,
		"dueDay": colDueDay, "depositAmount": colDeposit,
	}
)

func addInputError(row *Row, err error) {
	if re, ok := asRowError(err); ok {
		// The rent column feeds both the unit's usual rent and the lease; report it once.
		for _, e := range row.Errors {
			if e.Column == re.Column {
				return
			}
		}
		row.Errors = append(row.Errors, re)
		return
	}
	row.Errors = append(row.Errors, RowError{Column: "", Message: "Baris ini tidak bisa diperiksa."})
}

func (s *Summary) add(row Row) {
	s.Rows++
	if !row.OK() {
		s.RowsWithErrors++
		return
	}
	count := func(a Action, n *int) {
		if a == Created {
			*n++
		}
	}
	count(row.Property, &s.PropertiesCreated)
	count(row.Unit, &s.UnitsCreated)
	count(row.Tenant, &s.TenantsCreated)
	count(row.Lease, &s.LeasesCreated)
	if row.Lease == Matched {
		s.LeasesMatched++
	}
}
