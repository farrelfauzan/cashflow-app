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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
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
	// Values are the lease terms as read from the row; nil when any of them could not be read.
	Values *Values
	// The property, unit and tenant names as read from the file, trimmed, whatever else the
	// row got wrong.
	PropertyName, UnitLabel, TenantName string
	// InvoiceCount and InvoiceTotal are the invoices the lease this row creates gets once it is
	// saved, as invoice generation makes them today (billing.PreviewLease): every period from
	// "Mulai tagih dari" due within the lead time. Zero when the row creates no lease.
	InvoiceCount int
	InvoiceTotal int64
}

// Values are the lease terms read from a row, shown so a dry run says exactly what is saved.
type Values struct {
	StartDate time.Time
	// BilledFrom is "Mulai tagih dari"; the start date when the cell is empty.
	BilledFrom time.Time
	// TermMonths is the jangka_bulan cell, or the term resolved from the unit when it is empty.
	TermMonths int
	// Rent is the sewa cell, the unit's price for one whole term; nil when the cell is empty.
	Rent    *int64
	Deposit int64
}

// OK reports whether the row has no errors.
func (r Row) OK() bool { return len(r.Errors) == 0 }

// Summary counts the rows and what they created.
type Summary struct {
	Rows, RowsWithErrors                                           int
	PropertiesCreated, UnitsCreated, TenantsCreated, LeasesCreated int
	LeasesMatched                                                  int
	// InvoiceCount and InvoiceTotal add up the rows' invoices.
	InvoiceCount int
	InvoiceTotal int64
}

// Result is the outcome of an import.
type Result struct {
	DryRun    bool
	Committed bool
	Summary   Summary
	// Warnings are about the file as a whole; never nil, empty when there are none.
	Warnings []Warning
	Rows     []Row
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
	clock      clock.Clock
}

// NewService returns a Service. c decides the import day, in Asia/Jakarta: the day unit prices
// are checked on and new ones take effect. Once an import that created leases commits, it asks
// for the organization's invoices through the tenant service's queue (tenant.Service.LeaseSaved).
func NewService(st *store.Store, az *authz.Service, props *property.Service, tenants *tenant.Service, c clock.Clock) *Service {
	return &Service{store: st, authz: az, properties: props, tenants: tenants, clock: c}
}

// Run imports the CSV in data. It needs tenant:manage and lease:manage for the whole
// organization. A file that cannot be read returns a *FileError and no rows.
func (s *Service) Run(ctx context.Context, p authz.Principal, data []byte, dryRun bool) (Result, error) {
	records, warnings, err := parseFile(data)
	if err != nil {
		return Result{}, err
	}
	if warnings == nil {
		warnings = []Warning{}
	}
	res := Result{DryRun: dryRun, Warnings: warnings, Rows: make([]Row, len(records))}
	var firstLease uuid.UUID // the first lease the import created, which names its generate task
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		for _, req := range importRequests() {
			if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
				return err
			}
		}
		for i, rec := range records {
			row, leaseID, err := s.importRow(ctx, tx, p, rec)
			if err != nil {
				return fmt.Errorf("line %d: %w", rec.Line, err)
			}
			if firstLease == uuid.Nil {
				firstLease = leaseID
			}
			if row.Values != nil && row.Values.TermMonths == 0 {
				row.Values = nil // the term comes from the unit, which the row never reached
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
	// One generate task covers the whole organization, so one is enough for the whole file.
	if firstLease != uuid.Nil {
		s.tenants.LeaseSaved(ctx, p.OrganizationID, firstLease)
	}
	return res, nil
}

// importRequests are the checks an import needs: tenant:manage and lease:manage for the whole
// organization. The template download asks for the same.
func importRequests() []authz.Request {
	var reqs []authz.Request
	for _, action := range []string{authz.ActionTenantManage, authz.ActionLeaseManage} {
		reqs = append(reqs, authz.Request{Action: action, Resource: authz.Resource{Type: strings.TrimSuffix(action, ":manage")}})
	}
	return reqs
}

// importRow checks one record and, when it is valid, finds or creates its property, unit, tenant
// and lease inside a savepoint. Row problems come back in the row; only unexpected failures and
// permission denials are returned as errors, and they stop the whole import. It also returns the
// id of the lease it created, uuid.Nil when it created none.
func (s *Service) importRow(ctx context.Context, tx pgx.Tx, p authz.Principal, rec record) (Row, uuid.UUID, error) {
	row := Row{
		Line: rec.Line, PropertyName: rec.get(colProperty), UnitLabel: rec.get(colUnit), TenantName: rec.get(colTenantName),
	}
	in := check(rec, &row)
	if !row.OK() {
		return row, uuid.Nil, nil
	}
	sp, err := tx.Begin(ctx) // a savepoint: a failed row leaves the rest of the import usable
	if err != nil {
		return row, uuid.Nil, fmt.Errorf("opening savepoint: %w", err)
	}
	leaseID, err := s.write(ctx, sp, p, in, &row)
	if err != nil {
		if rbErr := sp.Rollback(ctx); rbErr != nil {
			return row, uuid.Nil, fmt.Errorf("rolling back savepoint: %w", rbErr)
		}
		if rowErr, ok := asRowError(err); ok {
			row.Errors = append(row.Errors, rowErr)
			row.Property, row.Unit, row.Tenant, row.Lease = "", "", "", ""
			return row, uuid.Nil, nil
		}
		return row, uuid.Nil, err
	}
	if leaseID != uuid.Nil {
		if err := s.previewInvoices(ctx, sp, p.OrganizationID, leaseID, &row); err != nil {
			if rbErr := sp.Rollback(ctx); rbErr != nil {
				return row, uuid.Nil, fmt.Errorf("rolling back savepoint: %w", rbErr)
			}
			return row, uuid.Nil, err
		}
	}
	if err := sp.Commit(ctx); err != nil {
		return row, uuid.Nil, fmt.Errorf("releasing savepoint: %w", err)
	}
	return row, leaseID, nil
}

// previewInvoices sets the row's invoice count and total from the invoices its new lease gets,
// through the same planning invoice generation uses, so the dry run shows what the import will
// bill. A term with no price in force today gets no invoice, and the row says so in a note.
func (s *Service) previewInvoices(ctx context.Context, tx pgx.Tx, org, leaseID uuid.UUID, row *Row) error {
	n, total, err := billing.PreviewLease(ctx, tx, org, leaseID, tenant.Today(s.clock))
	if errors.Is(err, tenant.ErrNoPrice) {
		row.Notes = append(row.Notes, "Belum ada harga yang berlaku hari ini untuk jangka sewa ini, jadi sewa ini belum ditagih. Isi harganya di unit.")
		return nil
	}
	if err != nil {
		return fmt.Errorf("previewing the lease's invoices: %w", err)
	}
	row.InvoiceCount, row.InvoiceTotal = n, total
	return nil
}

// input is a record turned into the services' inputs, already validated by them.
type input struct {
	property property.PropertyInput
	unit     property.UnitInput // the label only; the terms are resolved against the unit
	tenant   tenant.Input
	terms    tenant.Terms
	// term is the jangka_bulan cell, 0 when it is empty.
	term int
	// rent is the sewa cell, 0 when it is empty.
	rent int64
}

// check parses a record and validates it with the property and tenant rules, adding every
// problem it finds to row.
func check(rec record, row *Row) input {
	fail := func(col, msg string) { row.Errors = append(row.Errors, RowError{Column: col, Message: msg}) }

	rent, ok := parseRupiah(rec.get(colRent))
	switch {
	case !ok:
		fail(colRent, "Harga sewa harus angka rupiah, contoh 850000 atau 850.000.")
	case rec.get(colRent) != "" && rent <= 0:
		fail(colRent, "Harga sewa harus lebih dari Rp0.")
	}
	term, ok := parseTerm(rec.get(colTerm))
	if !ok {
		fail(colTerm, termMessage)
	}
	deposit, ok := parseRupiah(rec.get(colDeposit))
	if !ok {
		fail(colDeposit, "Jaminan harus angka rupiah, contoh 850000 atau 850.000.")
	}
	start, ok := parseDate(rec.get(colStartDate))
	if !ok {
		fail(colStartDate, "Tanggal mulai harus seperti 2026-10-01 atau 01/10/2026.")
	}
	billFrom, ok := parseDate(rec.get(colBillFrom))
	if !ok {
		fail(colBillFrom, "Mulai tagih harus seperti 2026-10-01 atau 01/10/2026.")
	}
	parsed := len(row.Errors) == 0
	if parsed {
		row.Values = &Values{StartDate: start, BilledFrom: billFrom, TermMonths: term, Deposit: deposit}
		if billFrom.IsZero() {
			row.Values.BilledFrom = start
		}
		if rent > 0 {
			row.Values.Rent = &rent
		}
	}
	if note, ok := oldDueDayNote(rec.get(colOldDueDay), start); ok {
		row.Notes = append(row.Notes, note)
	}

	in := input{term: term, rent: rent}
	var err error
	if in.property, err = property.ValidProperty(property.PropertyInput{
		Name: rec.get(colProperty), Kind: parseKind(rec.get(colPropertyKind)),
	}); err != nil {
		addInputError(row, err)
	}
	if in.unit, err = property.ValidUnit(property.UnitInput{Label: rec.get(colUnit)}); err != nil {
		addInputError(row, err)
	}
	if in.tenant, err = tenant.ValidTenant(tenant.Input{Name: rec.get(colTenantName), Phone: rec.get(colPhone)}); err != nil {
		addInputError(row, err)
	} else if isExamplePhone(in.tenant.Phone) {
		fail(colPhone, "Ini baris contoh. Hapus baris ini sebelum mengimpor.")
	}
	if parsed {
		// An empty term cell leaves the period start check to the lease, once the unit gives
		// the term.
		if in.terms, err = tenant.ValidTerms(tenant.Terms{
			StartDate: start, DepositAmount: deposit, BilledFrom: billFrom,
		}, term); err != nil {
			addInputError(row, err)
		}
	}
	return in
}

// write finds or creates the row's records through the services, inside the savepoint tx. It
// returns the id of the lease it created, uuid.Nil when the row matched an existing one.
func (s *Service) write(ctx context.Context, tx pgx.Tx, p authz.Principal, in input, row *Row) (uuid.UUID, error) {
	org := p.OrganizationID

	prop, err := property.FindByName(ctx, tx, org, in.property.Name)
	row.Property = Matched
	if errors.Is(err, property.ErrNotFound) {
		prop, err = s.properties.CreateTx(ctx, tx, p, in.property)
		row.Property = Created
	}
	if err != nil {
		return uuid.Nil, err
	}

	day := tenant.Today(s.clock)
	unit, err := property.FindUnit(ctx, tx, org, prop.ID, in.unit.Label)
	row.Unit = Matched
	if errors.Is(err, property.ErrNotFound) {
		unit, err = s.createUnit(ctx, tx, p, prop, in, row)
		row.Unit = Created
	} else if err == nil {
		err = s.checkUnitPrice(ctx, tx, p, unit, in, row, day)
	}
	if err != nil {
		return uuid.Nil, err
	}
	term := row.Values.TermMonths

	tn, err := tenant.FindByPhone(ctx, tx, org, in.tenant.Phone)
	row.Tenant = Matched
	if errors.Is(err, tenant.ErrNotFound) {
		tn, err = s.tenants.CreateTx(ctx, tx, p, in.tenant)
		row.Tenant = Created
	}
	if err != nil {
		return uuid.Nil, err
	}
	// One phone is one tenant. A different name usually means two people share a number (a
	// parent's phone for two children), which the app cannot hold, so the admin must decide.
	if row.Tenant == Matched && !strings.EqualFold(tn.Name, in.tenant.Name) {
		msg := fmt.Sprintf("Nomor ini sudah dipakai %s. Samakan nama atau pakai nomor lain.", tn.Name)
		return uuid.Nil, &rowError{RowError{Column: colPhone, Message: msg}}
	}

	current, err := tenant.ActiveLeaseForUnit(ctx, tx, org, unit.ID, day)
	switch {
	case err == nil && current.TenantID == tn.ID:
		row.Lease = Matched
		billedFrom := in.terms.BilledFrom // an empty cell means the start date, as for a new lease
		if billedFrom.IsZero() {
			billedFrom = in.terms.StartDate
		}
		if current.TermMonths != term || current.DepositAmount != in.terms.DepositAmount ||
			!current.StartDate.Equal(in.terms.StartDate) || !current.BilledFrom.Equal(billedFrom) {
			row.Notes = append(row.Notes, "Sewa ini sudah ada dengan isian lain; isian baris ini tidak mengubahnya.")
		}
		return uuid.Nil, nil
	case err == nil:
		return uuid.Nil, &rowError{RowError{Column: colUnit, Message: "Unit ini masih disewa penyewa lain."}}
	case !errors.Is(err, tenant.ErrNotFound):
		return uuid.Nil, err
	}
	l, err := s.tenants.CreateLeaseTx(ctx, tx, p, tenant.NewLease{TenantID: tn.ID, UnitID: unit.ID, TermMonths: term, Terms: in.terms})
	row.Lease = Created
	return l.ID, err
}

// createUnit creates the row's unit offering exactly the row's term (its property kind's
// default when the cell is empty), priced from the sewa cell, which a new unit needs. It sets
// the row's resolved term.
func (s *Service) createUnit(ctx context.Context, tx pgx.Tx, p authz.Principal, prop property.Property, in input, row *Row) (property.Unit, error) {
	term := in.term
	if term == 0 {
		term = property.DefaultTerms(prop.Kind)[0]
	}
	row.Values.TermMonths = term
	if in.rent == 0 {
		return property.Unit{}, &rowError{RowError{Column: colRent, Message: noPriceMessage(term)}}
	}
	rent := in.rent
	return s.properties.CreateUnitTx(ctx, tx, p, prop.ID, property.UnitInput{
		Label: in.unit.Label, Terms: []property.TermInput{{Months: term, Price: &rent}},
	})
}

// checkUnitPrice resolves the row's term against an existing unit and checks the sewa cell
// against the unit's price for it on day: equal or empty when there is one, and otherwise
// required and recorded as the unit's price from day. It sets the row's resolved term.
func (s *Service) checkUnitPrice(ctx context.Context, tx pgx.Tx, p authz.Principal, unit property.Unit, in input, row *Row, day time.Time) error {
	offered := make([]int, len(unit.Terms))
	for i, t := range unit.Terms {
		offered[i] = t.Months
	}
	term := in.term
	switch {
	case term != 0 && !slices.Contains(offered, term):
		return &rowError{RowError{Column: colTerm, Message: notOfferedMessage(term)}}
	case term != 0:
	case slices.Contains(offered, 1):
		term = 1
	case len(offered) == 1:
		term = offered[0]
	default:
		return &rowError{RowError{Column: colTerm, Message: "Isi jangka_bulan: unit ini menawarkan beberapa jangka sewa."}}
	}
	row.Values.TermMonths = term

	price, err := tenant.PriceInForce(ctx, dbgen.New(tx), p.OrganizationID, unit.ID, term, day)
	switch {
	case errors.Is(err, tenant.ErrNoPrice) && in.rent == 0:
		return &rowError{RowError{Column: colRent, Message: noPriceMessage(term)}}
	case errors.Is(err, tenant.ErrNoPrice):
		_, err = s.properties.CreateUnitPriceTx(ctx, tx, p, unit.ID, property.PriceInput{Months: term, Amount: in.rent, EffectiveFrom: day})
		return err
	case err != nil:
		return err
	case in.rent != 0 && in.rent != price:
		return &rowError{RowError{Column: colRent, Message: fmt.Sprintf(
			"Harga sewa berbeda dengan harga unit ini untuk %d bulan (Rp%s).", term, formatRupiah(price))}}
	}
	return nil
}

const termMessage = "Jangka sewa harus 1 sampai 60 bulan."

func invoicedPriceMessage(invoicedOn time.Time) string {
	return fmt.Sprintf("Unit ini sudah ditagih pada %s, jadi harga barunya paling cepat berlaku %s. Impor baris ini lagi mulai tanggal itu.",
		invoicedOn.Format("02/01/2006"), invoicedOn.AddDate(0, 0, 1).Format("02/01/2006"))
}

func noPriceMessage(term int) string {
	return fmt.Sprintf("Harga sewa wajib diisi: unit ini belum punya harga untuk %d bulan.", term)
}

func notOfferedMessage(term int) string {
	return fmt.Sprintf("Unit ini tidak menawarkan jangka sewa %d bulan.", term)
}

// rowError carries a row problem found while writing.
type rowError struct{ RowError }

func (e *rowError) Error() string { return "tenantimport: " + e.Column + ": " + e.Message }

// asRowError turns the errors a row can cause into a RowError. Anything else stops the import.
func asRowError(err error) (RowError, bool) {
	var re *rowError
	var propertyInput *property.InputError
	var tenantInput *tenant.InputError
	var termErr *tenant.TermError
	var invoicedPrice *property.InvoicedPriceError
	switch {
	case errors.As(err, &re):
		return re.RowError, true
	case errors.As(err, &invoicedPrice):
		return RowError{Column: colRent, Message: invoicedPriceMessage(invoicedPrice.InvoicedOn)}, true
	case errors.As(err, &propertyInput):
		return RowError{Column: propertyColumns[propertyInput.Field], Message: propertyInput.Message}, true
	case errors.As(err, &tenantInput):
		return RowError{Column: tenantColumns[tenantInput.Field], Message: tenantInput.Message}, true
	case errors.Is(err, tenant.ErrUnitOccupied):
		return RowError{Column: colUnit, Message: "Unit ini masih disewa penyewa lain."}, true
	case errors.As(err, &termErr) && errors.Is(err, tenant.ErrNoPrice):
		return RowError{Column: colRent, Message: noPriceMessage(termErr.Months)}, true
	case errors.As(err, &termErr):
		return RowError{Column: colTerm, Message: notOfferedMessage(termErr.Months)}, true
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
	propertyColumns = map[string]string{
		"name": colProperty, "kind": colPropertyKind, "label": colUnit, "terms": colTerm,
		"months": colTerm, "amount": colRent,
	}
	tenantColumns = map[string]string{
		"name": colTenantName, "phone": colPhone, "startDate": colStartDate, "termMonths": colTerm,
		"depositAmount": colDeposit, "billedFrom": colBillFrom,
	}
)

func addInputError(row *Row, err error) {
	if re, ok := asRowError(err); ok {
		// One column can fail more than one rule; report it once.
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
	s.InvoiceCount += row.InvoiceCount
	s.InvoiceTotal += row.InvoiceTotal
}

// oldDueDayNote warns when an older template's due day differs from the start date's day, since
// rent is now due on the start date's day and the old column is not used.
func oldDueDayNote(cell string, start time.Time) (string, bool) {
	if cell == "" || start.IsZero() {
		return "", false
	}
	if day, err := strconv.Atoi(cell); err == nil && day == start.Day() {
		return "", false
	}
	return fmt.Sprintf("Jatuh tempo kini mengikuti tanggal mulai (tanggal %d); kolom jatuh_tempo tidak dipakai.", start.Day()), true
}
