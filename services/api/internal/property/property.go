// Package property manages an organization's properties (a kos, a kontrakan, a ruko) and the
// units inside them.
package property

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

const (
	maxNameLength    = 100
	maxAddressLength = 300
	maxLabelLength   = 50
)

// The kinds of property.
const (
	KindKos       = "kos"
	KindKontrakan = "kontrakan"
	KindRuko      = "ruko"
	KindOther     = "other"
)

var kinds = []string{KindKos, KindKontrakan, KindRuko, KindOther}

var (
	// ErrNotFound means there is no such property or unit in this organization.
	ErrNotFound = errors.New("property: not found")
	// ErrNameTaken means another property in the organization already has the name.
	ErrNameTaken = errors.New("property: name already used")
	// ErrLabelTaken means another unit in the same property already has the label.
	ErrLabelTaken = errors.New("property: unit label already used")
)

// InputError names the field that is wrong. Message is safe to show to the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return fmt.Sprintf("property: %s: %s", e.Field, e.Message) }

// Property is a building or site that holds units.
type Property struct {
	ID        uuid.UUID
	Name      string
	Kind      string
	Address   string
	UnitCount int64
	// OccupiedUnitCount is how many of the units have an active lease.
	OccupiedUnitCount int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PropertyInput is the input for creating or changing a property.
type PropertyInput struct {
	Name    string
	Kind    string
	Address string
}

// Unit is a rentable unit inside a property.
type Unit struct {
	ID         uuid.UUID
	PropertyID uuid.UUID
	Label      string
	// DefaultRent is the usual monthly rent in rupiah, or nil when none is set.
	DefaultRent *int64
	// Occupied is true while the unit has an active lease, a booking that has not started yet
	// included.
	Occupied bool
	// CurrentTenant is the tenant of the active lease, or nil when the unit is free or the
	// principal may not read that tenant.
	CurrentTenant *UnitTenant
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UnitTenant is the tenant of a unit's active lease.
type UnitTenant struct {
	ID   uuid.UUID
	Name string
}

// UnitInput is the input for creating or changing a unit.
type UnitInput struct {
	Label       string
	DefaultRent *int64
}

// Service manages properties and units.
type Service struct {
	store *store.Store
	authz *authz.Service
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service) *Service {
	return &Service{store: st, authz: az}
}

// ListQuery filters, orders and pages List. Zero values filter nothing.
type ListQuery struct {
	Page   listing.Page // the zero Page is listing.FirstPage
	Search listing.Search
	// Sort is name (the default) or -name, numbers in natural order.
	Sort string
}

// UnitQuery filters, orders and pages ListUnits. Zero values filter nothing.
type UnitQuery struct {
	Page   listing.Page // the zero Page is listing.FirstPage
	Search listing.Search
	// Sort is label (the default) or -label, numbers in natural order.
	Sort string
}

// List returns one page of the properties the principal may read: all of them, or only those
// their roles are limited to. The total counts only those.
func (s *Service) List(ctx context.Context, p authz.Principal, lq ListQuery) (listing.Result[Property], error) {
	page, sort, err := checkQuery(lq.Page, lq.Sort, "name")
	if err != nil {
		return listing.Result[Property]{}, err
	}
	out := listing.Result[Property]{Page: page, Items: []Property{}}
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		all, err := s.authz.AuthorizeList(ctx, tx, p, authz.Request{
			Action: authz.ActionPropertyRead, Resource: authz.Resource{Type: "property"},
		})
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		args := dbgen.ListPropertyPageParams{
			OrganizationID: p.OrganizationID, OrgWide: all, PropertyIds: []uuid.UUID{}, Q: lq.Search.Text,
			Sort: sort, PageLimit: page.Limit(), PageOffset: page.Offset(),
		}
		if !all {
			ids, err := q.ListPropertyIDs(ctx, p.OrganizationID)
			if err != nil {
				return fmt.Errorf("listing property ids: %w", err)
			}
			for _, id := range ids {
				if authz.Allowed(p, propertyRequest(authz.ActionPropertyRead, id)) {
					args.PropertyIds = append(args.PropertyIds, id)
				}
			}
		}
		out.Total, err = q.CountPropertyPage(ctx, dbgen.CountPropertyPageParams{
			OrganizationID: args.OrganizationID, OrgWide: args.OrgWide, PropertyIds: args.PropertyIds, Q: args.Q,
		})
		if err != nil {
			return fmt.Errorf("counting properties: %w", err)
		}
		rows, err := q.ListPropertyPage(ctx, args)
		if err != nil {
			return fmt.Errorf("listing properties: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, Property{
				ID: r.ID, Name: r.Name, Kind: r.Kind, Address: r.Address,
				UnitCount: r.UnitCount, OccupiedUnitCount: r.OccupiedUnitCount,
				CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			})
		}
		return nil
	})
	if err != nil {
		return listing.Result[Property]{}, err
	}
	return out, nil
}

// checkQuery fills in the default page and sort, and refuses a sort other than by def,
// ascending or descending.
func checkQuery(page listing.Page, sort, def string) (listing.Page, string, error) {
	if page == (listing.Page{}) {
		page = listing.FirstPage
	}
	switch sort {
	case "":
		sort = def
	case def, "-" + def:
	default:
		return page, sort, &InputError{Field: "sort", Message: "Urutan tidak dikenal."}
	}
	return page, sort, nil
}

// Create adds a property. It needs property:manage for the whole organization, so a member
// whose roles are limited to some properties cannot add new ones.
func (s *Service) Create(ctx context.Context, p authz.Principal, in PropertyInput) (Property, error) {
	var out Property
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		out, err = s.CreateTx(ctx, tx, p, in)
		return err
	})
	return out, err
}

// CreateTx is Create inside the caller's organization transaction, for changes that span
// several records, such as an import.
func (s *Service) CreateTx(ctx context.Context, tx pgx.Tx, p authz.Principal, in PropertyInput) (Property, error) {
	in, err := ValidProperty(in)
	if err != nil {
		return Property{}, err
	}
	id := uuid.New()
	req := authz.Request{Action: authz.ActionPropertyManage, Resource: authz.Resource{Type: "property", ID: id.String()}}
	if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
		return Property{}, err
	}
	r, err := dbgen.New(tx).CreateProperty(ctx, dbgen.CreatePropertyParams{
		ID: id, OrganizationID: p.OrganizationID, Name: in.Name, Kind: in.Kind, Address: in.Address,
	})
	if err != nil {
		return Property{}, propertyWriteError(err)
	}
	return Property{ID: r.ID, Name: r.Name, Kind: r.Kind, Address: r.Address, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, nil
}

// Get returns one property with its unit counts. It needs property:read on that property, so a
// member whose roles are limited to other properties is refused.
func (s *Service) Get(ctx context.Context, p authz.Principal, id uuid.UUID) (Property, error) {
	var out Property
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		r, err := getProperty(ctx, dbgen.New(tx), p.OrganizationID, id)
		if err != nil {
			return err
		}
		if err := s.authz.Authorize(ctx, tx, p, propertyRequest(authz.ActionPropertyRead, id)); err != nil {
			return err
		}
		out = Property{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Address: r.Address,
			UnitCount: r.UnitCount, OccupiedUnitCount: r.OccupiedUnitCount,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		return nil
	})
	return out, err
}

// Update changes a property's name, kind and address.
func (s *Service) Update(ctx context.Context, p authz.Principal, id uuid.UUID, in PropertyInput) (Property, error) {
	in, err := ValidProperty(in)
	if err != nil {
		return Property{}, err
	}
	var out Property
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := getProperty(ctx, q, p.OrganizationID, id)
		if err != nil {
			return err
		}
		if err := s.authz.Authorize(ctx, tx, p, propertyRequest(authz.ActionPropertyManage, id)); err != nil {
			return err
		}
		r, err := q.UpdateProperty(ctx, dbgen.UpdatePropertyParams{
			OrganizationID: p.OrganizationID, ID: id, Name: in.Name, Kind: in.Kind, Address: in.Address,
		})
		if err != nil {
			return propertyWriteError(err)
		}
		out = Property{
			ID: r.ID, Name: r.Name, Kind: r.Kind, Address: r.Address,
			UnitCount: current.UnitCount, OccupiedUnitCount: current.OccupiedUnitCount,
			CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
		}
		return nil
	})
	return out, err
}

// ListUnits returns one page of the units of one property.
func (s *Service) ListUnits(ctx context.Context, p authz.Principal, propertyID uuid.UUID, uq UnitQuery) (listing.Result[Unit], error) {
	page, sort, err := checkQuery(uq.Page, uq.Sort, "label")
	if err != nil {
		return listing.Result[Unit]{}, err
	}
	out := listing.Result[Unit]{Page: page, Items: []Unit{}}
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := getProperty(ctx, q, p.OrganizationID, propertyID); err != nil {
			return err
		}
		req := authz.Request{Action: authz.ActionUnitRead, Resource: authz.Resource{Type: "unit", PropertyID: propertyID}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		out.Total, err = q.CountUnits(ctx, dbgen.CountUnitsParams{
			OrganizationID: p.OrganizationID, PropertyID: propertyID, Q: uq.Search.Text,
		})
		if err != nil {
			return fmt.Errorf("counting units: %w", err)
		}
		rows, err := q.ListUnits(ctx, dbgen.ListUnitsParams{
			OrganizationID: p.OrganizationID, PropertyID: propertyID, Q: uq.Search.Text,
			Sort: sort, PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("listing units: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, toOccupiedUnit(p, r.Unit, r.Occupied, r.LeaseTenantID, r.LeaseTenantName, r.CollectorStaffUserID))
		}
		return nil
	})
	if err != nil {
		return listing.Result[Unit]{}, err
	}
	return out, nil
}

// CreateUnit adds a unit to a property.
func (s *Service) CreateUnit(ctx context.Context, p authz.Principal, propertyID uuid.UUID, in UnitInput) (Unit, error) {
	var out Unit
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		out, err = s.CreateUnitTx(ctx, tx, p, propertyID, in)
		return err
	})
	return out, err
}

// CreateUnitTx is CreateUnit inside the caller's organization transaction.
func (s *Service) CreateUnitTx(ctx context.Context, tx pgx.Tx, p authz.Principal, propertyID uuid.UUID, in UnitInput) (Unit, error) {
	in, err := ValidUnit(in)
	if err != nil {
		return Unit{}, err
	}
	q := dbgen.New(tx)
	if _, err := getProperty(ctx, q, p.OrganizationID, propertyID); err != nil {
		return Unit{}, err
	}
	id := uuid.New()
	if err := s.authz.Authorize(ctx, tx, p, unitRequest(authz.ActionUnitManage, propertyID, id)); err != nil {
		return Unit{}, err
	}
	r, err := q.CreateUnit(ctx, dbgen.CreateUnitParams{
		ID: id, OrganizationID: p.OrganizationID, PropertyID: propertyID, Label: in.Label, DefaultRent: in.DefaultRent,
	})
	if err != nil {
		return Unit{}, unitWriteError(err)
	}
	return toUnit(r), nil
}

// FindByName returns the property with this name, ignoring case, or ErrNotFound. It does no
// permission check: the caller has authorized the change it is part of. It does not fill
// OccupiedUnitCount.
func FindByName(ctx context.Context, tx pgx.Tx, org uuid.UUID, name string) (Property, error) {
	r, err := dbgen.New(tx).FindPropertyByName(ctx, dbgen.FindPropertyByNameParams{OrganizationID: org, Name: strings.TrimSpace(name)})
	if errors.Is(err, pgx.ErrNoRows) {
		return Property{}, ErrNotFound
	}
	if err != nil {
		return Property{}, fmt.Errorf("finding property: %w", err)
	}
	return Property{ID: r.ID, Name: r.Name, Kind: r.Kind, Address: r.Address, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, nil
}

// FindUnit returns the unit with this label in a property, ignoring case, or ErrNotFound. It
// does no permission check and does not fill the occupancy fields.
func FindUnit(ctx context.Context, tx pgx.Tx, org, propertyID uuid.UUID, label string) (Unit, error) {
	r, err := dbgen.New(tx).FindUnitByLabel(ctx, dbgen.FindUnitByLabelParams{
		OrganizationID: org, PropertyID: propertyID, Label: strings.TrimSpace(label),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, ErrNotFound
	}
	if err != nil {
		return Unit{}, fmt.Errorf("finding unit: %w", err)
	}
	return toUnit(r), nil
}

// UpdateUnit changes a unit's label and default rent. The unit stays in its property.
func (s *Service) UpdateUnit(ctx context.Context, p authz.Principal, id uuid.UUID, in UnitInput) (Unit, error) {
	in, err := ValidUnit(in)
	if err != nil {
		return Unit{}, err
	}
	var out Unit
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := q.GetUnit(ctx, dbgen.GetUnitParams{OrganizationID: p.OrganizationID, ID: id})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("loading unit: %w", err)
		}
		if err := s.authz.Authorize(ctx, tx, p, unitRequest(authz.ActionUnitManage, current.PropertyID, id)); err != nil {
			return err
		}
		r, err := q.UpdateUnit(ctx, dbgen.UpdateUnitParams{
			OrganizationID: p.OrganizationID, ID: id, Label: in.Label, DefaultRent: in.DefaultRent,
		})
		if err != nil {
			return unitWriteError(err)
		}
		occ, err := q.GetUnitOccupancy(ctx, dbgen.GetUnitOccupancyParams{OrganizationID: p.OrganizationID, ID: id})
		if err != nil {
			return fmt.Errorf("loading unit occupancy: %w", err)
		}
		out = toOccupiedUnit(p, r, occ.Occupied, occ.LeaseTenantID, occ.LeaseTenantName, occ.CollectorStaffUserID)
		return nil
	})
	return out, err
}

// Exists reports whether the property belongs to the organization of the open transaction.
// The member package uses it to check a role's property limit.
func Exists(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) (bool, error) {
	_, err := getProperty(ctx, q, org, id)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func getProperty(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) (dbgen.GetPropertyRow, error) {
	r, err := q.GetProperty(ctx, dbgen.GetPropertyParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, fmt.Errorf("loading property: %w", err)
	}
	return r, nil
}

// propertyRequest targets one property. The property is its own property, so a role limited
// to it covers it.
func propertyRequest(action string, id uuid.UUID) authz.Request {
	return authz.Request{Action: action, Resource: authz.Resource{Type: "property", ID: id.String(), PropertyID: id}}
}

func unitRequest(action string, propertyID, id uuid.UUID) authz.Request {
	return authz.Request{Action: action, Resource: authz.Resource{Type: "unit", ID: id.String(), PropertyID: propertyID}}
}

// toOccupiedUnit is toUnit plus the unit's occupancy. The tenant is shown only when p may read
// it through the active lease, the check the tenant list makes per record; it is a filter, so
// it is not audited. A role with unit:read but no tenant:read sees occupied only.
func toOccupiedUnit(p authz.Principal, r dbgen.Unit, occupied bool, tenantID uuid.NullUUID, tenantName *string, collector uuid.NullUUID) Unit {
	u := toUnit(r)
	u.Occupied = occupied
	if occupied && tenantID.Valid && tenantName != nil &&
		authz.Allowed(p, tenant.ReadViaLease(tenantID.UUID, r.PropertyID, collector.UUID)) {
		u.CurrentTenant = &UnitTenant{ID: tenantID.UUID, Name: *tenantName}
	}
	return u
}

func toUnit(r dbgen.Unit) Unit {
	return Unit{
		ID: r.ID, PropertyID: r.PropertyID, Label: r.Label, DefaultRent: r.DefaultRent,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func propertyWriteError(err error) error {
	if isUniqueViolation(err, "property_name_unique") {
		return ErrNameTaken
	}
	return fmt.Errorf("saving property: %w", err)
}

func unitWriteError(err error) error {
	if isUniqueViolation(err, "unit_label_unique") {
		return ErrLabelTaken
	}
	return fmt.Errorf("saving unit: %w", err)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// ValidProperty checks a property input and returns it trimmed. Errors are *InputError.
func ValidProperty(in PropertyInput) (PropertyInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Address = strings.TrimSpace(in.Address)
	switch {
	case in.Name == "":
		return in, &InputError{Field: "name", Message: "Nama properti wajib diisi."}
	case len([]rune(in.Name)) > maxNameLength:
		return in, &InputError{Field: "name", Message: fmt.Sprintf("Nama properti paling panjang %d huruf.", maxNameLength)}
	case !slices.Contains(kinds, in.Kind):
		return in, &InputError{Field: "kind", Message: "Jenis properti tidak dikenal."}
	case len([]rune(in.Address)) > maxAddressLength:
		return in, &InputError{Field: "address", Message: fmt.Sprintf("Alamat paling panjang %d huruf.", maxAddressLength)}
	}
	return in, nil
}

// ValidUnit checks a unit input and returns it trimmed. Errors are *InputError.
func ValidUnit(in UnitInput) (UnitInput, error) {
	in.Label = strings.TrimSpace(in.Label)
	switch {
	case in.Label == "":
		return in, &InputError{Field: "label", Message: "Nama unit wajib diisi."}
	case len([]rune(in.Label)) > maxLabelLength:
		return in, &InputError{Field: "label", Message: fmt.Sprintf("Nama unit paling panjang %d huruf.", maxLabelLength)}
	case in.DefaultRent != nil && *in.DefaultRent <= 0:
		return in, &InputError{Field: "defaultRent", Message: "Harga sewa harus lebih dari Rp0."}
	}
	return in, nil
}
