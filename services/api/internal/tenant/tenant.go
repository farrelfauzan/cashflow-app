// Package tenant manages the people who rent (tenants) and their leases: who rents which unit,
// on what terms, collected by whom, and when they moved out.
//
// A tenant is visible through any of their leases. A karyawan sees the tenants whose lease names
// them as collector (the RBAC "self" condition), and a member limited to a property sees the
// tenants with a lease there, and only those leases.
package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

const (
	maxNameLength = 100
	maxNoteLength = 500
)

var (
	// ErrNotFound means there is no such tenant or lease in this organization.
	ErrNotFound = errors.New("tenant: not found")
	// ErrPhoneTaken means another tenant in the organization already has the number.
	ErrPhoneTaken = errors.New("tenant: phone already used")
)

// InputError names the field that is wrong. Message is safe to show to the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return fmt.Sprintf("tenant: %s: %s", e.Field, e.Message) }

// Tenant is a person who rents, with the leases the caller may see.
type Tenant struct {
	ID        uuid.UUID
	Name      string
	Phone     string // E.164
	Note      string
	Leases    []Lease
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Input is the input for creating or changing a tenant.
type Input struct {
	Name  string
	Phone string
	Note  string
}

// Service manages tenants and leases.
type Service struct {
	store *store.Store
	authz *authz.Service
	clock clock.Clock
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service, c clock.Clock) *Service {
	return &Service{store: st, authz: az, clock: c}
}

// The tenant statuses List filters on: the status of the tenant's first lease among the leases
// the caller may see, taking running ones first, then ones booked ahead, then ended ones.
const (
	ListStatusActive   = "active"   // a lease that has started and still runs
	ListStatusUpcoming = "upcoming" // only leases booked ahead: they start after today
	ListStatusEnded    = "ended"    // every lease ended
	ListStatusNone     = "none"     // no lease at all
)

// ListQuery filters, orders and pages List. Zero values filter nothing.
type ListQuery struct {
	Page   listing.Page // the zero Page is listing.FirstPage
	Search listing.Search
	// PropertyID keeps tenants with a lease in this property.
	PropertyID uuid.UUID
	// Status is one of the ListStatus values.
	Status string
	// CollectorDisabled keeps tenants with an active lease whose collector is disabled.
	CollectorDisabled bool
	// Sort is name (the default), -name, createdAt or -createdAt.
	Sort string
}

var (
	listSorts    = map[string]bool{"name": true, "-name": true, "createdAt": true, "-createdAt": true}
	listStatuses = map[string]bool{ListStatusActive: true, ListStatusUpcoming: true, ListStatusEnded: true, ListStatusNone: true}
)

// List returns one page of the tenants the principal may see, each with the leases they may
// see. Filters, search and the total look only at what the principal may see.
func (s *Service) List(ctx context.Context, p authz.Principal, lq ListQuery) (listing.Result[Tenant], error) {
	if lq.Page == (listing.Page{}) {
		lq.Page = listing.FirstPage
	}
	if lq.Sort == "" {
		lq.Sort = "name"
	}
	if !listSorts[lq.Sort] {
		return listing.Result[Tenant]{}, &InputError{Field: "sort", Message: "Urutan tidak dikenal."}
	}
	if lq.Status != "" && !listStatuses[lq.Status] {
		return listing.Result[Tenant]{}, &InputError{Field: "status", Message: "Status tidak dikenal."}
	}
	out := listing.Result[Tenant]{Page: lq.Page, Items: []Tenant{}}
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		all, err := s.authz.AuthorizeList(ctx, tx, p, readRequest(uuid.Nil))
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		args := dbgen.ListTenantPageParams{
			OrganizationID: p.OrganizationID, OrgWide: all, LeaseIds: []uuid.UUID{}, Today: Today(s.clock),
			Q: lq.Search.Text, PhoneNational: lq.Search.PhoneNational, PhoneDigits: lq.Search.PhoneDigits,
			PropertyID:        uuid.NullUUID{UUID: lq.PropertyID, Valid: lq.PropertyID != uuid.Nil},
			CollectorDisabled: lq.CollectorDisabled,
			Sort:              lq.Sort, PageLimit: lq.Page.Limit(), PageOffset: lq.Page.Offset(),
		}
		if lq.Status != "" {
			args.LeaseStatus = &lq.Status
		}
		if !all {
			// Who may see a lease is the policy's decision, so it is made here and the query
			// gets the result.
			scopes, err := q.ListLeaseScopes(ctx, p.OrganizationID)
			if err != nil {
				return fmt.Errorf("listing lease scopes: %w", err)
			}
			for _, l := range scopes {
				if leaseVisible(p, Lease{
					TenantID: l.TenantID, PropertyID: l.PropertyID, CollectorUserID: l.CollectorStaffUserID.UUID, Status: l.Status,
				}) {
					args.LeaseIds = append(args.LeaseIds, l.ID)
				}
			}
		}
		if out.Total, err = q.CountTenantPage(ctx, countTenantArgs(args)); err != nil {
			return fmt.Errorf("counting tenants: %w", err)
		}
		rows, err := q.ListTenantPage(ctx, args)
		if err != nil {
			return fmt.Errorf("listing tenants: %w", err)
		}
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(rows))
		for i, r := range rows {
			ids[i] = r.ID
		}
		leaseRows, err := q.ListLeaseDetailsForTenants(ctx, dbgen.ListLeaseDetailsForTenantsParams{
			OrganizationID: p.OrganizationID, TenantIds: ids,
		})
		if err != nil {
			return fmt.Errorf("listing leases: %w", err)
		}
		leases := make([]Lease, len(leaseRows))
		for i, r := range leaseRows {
			leases[i] = toLease(dbgen.GetLeaseDetailRow(r))
		}
		if err := attachDetails(ctx, q, p.OrganizationID, args.Today, leases); err != nil {
			return err
		}
		byTenant := map[uuid.UUID][]Lease{}
		for _, l := range leases {
			byTenant[l.TenantID] = append(byTenant[l.TenantID], l)
		}
		for _, r := range rows {
			visible, _ := visibleLeases(p, all, byTenant[r.ID])
			out.Items = append(out.Items, toTenant(dbgen.Tenant(r), visible))
		}
		return nil
	})
	if err != nil {
		return listing.Result[Tenant]{}, err
	}
	return out, nil
}

// countTenantArgs is the WHERE of a page query, for counting every page.
func countTenantArgs(a dbgen.ListTenantPageParams) dbgen.CountTenantPageParams {
	return dbgen.CountTenantPageParams{
		OrganizationID: a.OrganizationID, OrgWide: a.OrgWide, LeaseIds: a.LeaseIds, Today: a.Today,
		Q: a.Q, PhoneNational: a.PhoneNational, PhoneDigits: a.PhoneDigits,
		PropertyID: a.PropertyID, LeaseStatus: a.LeaseStatus, CollectorDisabled: a.CollectorDisabled,
	}
}

// Get returns one tenant with the leases the principal may see.
func (s *Service) Get(ctx context.Context, p authz.Principal, id uuid.UUID) (Tenant, error) {
	var out Tenant
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, leases, err := load(ctx, q, p.OrganizationID, id, Today(s.clock))
		if err != nil {
			return err
		}
		all, err := s.authz.AuthorizeList(ctx, tx, p, readRequest(id))
		if err != nil {
			return err
		}
		visible, ok := visibleLeases(p, all, leases)
		if !ok {
			// Records the denial and returns ErrForbidden.
			return s.authz.Authorize(ctx, tx, p, readRequest(id))
		}
		out = toTenant(row, visible)
		return nil
	})
	return out, err
}

// Create adds a tenant. It needs tenant:manage for the whole organization: a new tenant has no
// lease yet, so no property limit can cover them.
func (s *Service) Create(ctx context.Context, p authz.Principal, in Input) (Tenant, error) {
	var out Tenant
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		out, err = s.CreateTx(ctx, tx, p, in)
		return err
	})
	return out, err
}

// CreateTx is Create inside the caller's organization transaction, for changes that span
// several records, such as an import.
func (s *Service) CreateTx(ctx context.Context, tx pgx.Tx, p authz.Principal, in Input) (Tenant, error) {
	in, err := ValidTenant(in)
	if err != nil {
		return Tenant{}, err
	}
	id := uuid.New()
	req := authz.Request{Action: authz.ActionTenantManage, Resource: authz.Resource{Type: "tenant", ID: id.String()}}
	if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
		return Tenant{}, err
	}
	r, err := dbgen.New(tx).CreateTenant(ctx, dbgen.CreateTenantParams{
		ID: id, OrganizationID: p.OrganizationID, Name: in.Name, Phone: in.Phone, Note: in.Note,
	})
	if err != nil {
		return Tenant{}, tenantWriteError(err)
	}
	return toTenant(r, []Lease{}), nil
}

// FindByPhone returns the tenant with this E.164 number, without leases, or ErrNotFound. It does
// no permission check: the caller has authorized the change it is part of.
func FindByPhone(ctx context.Context, tx pgx.Tx, org uuid.UUID, e164 string) (Tenant, error) {
	r, err := dbgen.New(tx).FindTenantByPhone(ctx, dbgen.FindTenantByPhoneParams{OrganizationID: org, Phone: e164})
	if errors.Is(err, pgx.ErrNoRows) {
		return Tenant{}, ErrNotFound
	}
	if err != nil {
		return Tenant{}, fmt.Errorf("finding tenant: %w", err)
	}
	return toTenant(r, nil), nil
}

// Update changes a tenant's name, number and note. A member limited to some properties may do
// it only when every active lease of the tenant is in their properties.
func (s *Service) Update(ctx context.Context, p authz.Principal, id uuid.UUID, in Input) (Tenant, error) {
	in, err := ValidTenant(in)
	if err != nil {
		return Tenant{}, err
	}
	var out Tenant
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		_, leases, err := load(ctx, q, p.OrganizationID, id, Today(s.clock))
		if err != nil {
			return err
		}
		if err := s.authorizeTenantChange(ctx, tx, p, id, leases); err != nil {
			return err
		}
		r, err := q.UpdateTenant(ctx, dbgen.UpdateTenantParams{
			OrganizationID: p.OrganizationID, ID: id, Name: in.Name, Phone: in.Phone, Note: in.Note,
		})
		if err != nil {
			return tenantWriteError(err)
		}
		all, err := s.authz.AuthorizeList(ctx, tx, p, readRequest(id))
		if err != nil {
			return err
		}
		visible, _ := visibleLeases(p, all, leases)
		out = toTenant(r, visible)
		return nil
	})
	return out, err
}

// authorizeTenantChange allows tenant:manage for the whole organization, or through every
// active lease of the tenant. Each allowed check is audited.
func (s *Service) authorizeTenantChange(ctx context.Context, tx pgx.Tx, p authz.Principal, id uuid.UUID, leases []Lease) error {
	whole := authz.Request{Action: authz.ActionTenantManage, Resource: authz.Resource{Type: "tenant", ID: id.String()}}
	var active []Lease
	for _, l := range leases {
		if l.Status == StatusActive {
			active = append(active, l)
		}
	}
	if authz.Allowed(p, whole) || len(active) == 0 {
		return s.authz.Authorize(ctx, tx, p, whole)
	}
	for _, l := range active {
		req := authz.Request{Action: authz.ActionTenantManage, Resource: tenantVia(l)}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
	}
	return nil
}

// visibleLeases returns the leases through which p may read their tenant, and whether p may
// read the tenant at all. all means p may read every tenant.
func visibleLeases(p authz.Principal, all bool, leases []Lease) ([]Lease, bool) {
	if all {
		if leases == nil {
			leases = []Lease{}
		}
		return leases, true
	}
	// Only active leases make a tenant visible to a member who is not organization-wide: after
	// move-out the former collector and a property-limited admin lose the tenant (decided
	// 2026-10-27; revisit in slice 2, when arrears after move-out exist).
	out := []Lease{}
	for _, l := range leases {
		if leaseVisible(p, l) {
			out = append(out, l)
		}
	}
	return out, len(out) > 0
}

// leaseVisible reports whether p, who may not read every tenant, may see the tenant through l.
func leaseVisible(p authz.Principal, l Lease) bool {
	return l.Status == StatusActive && authz.Allowed(p, authz.Request{Action: authz.ActionTenantRead, Resource: tenantVia(l)})
}

// readRequest is tenant:read on the whole organization's tenants, or on one tenant without a
// property or collector, which only an organization-wide grant allows.
func readRequest(id uuid.UUID) authz.Request {
	res := authz.Resource{Type: "tenant"}
	if id != uuid.Nil {
		res.ID = id.String()
	}
	return authz.Request{Action: authz.ActionTenantRead, Resource: res}
}

// tenantVia is the tenant seen through one lease: in that lease's property, assigned to its
// collector.
func tenantVia(l Lease) authz.Resource {
	return viaResource(l.TenantID, l.PropertyID, l.CollectorUserID)
}

func viaResource(tenantID, propertyID, collectorUserID uuid.UUID) authz.Resource {
	return authz.Resource{Type: "tenant", ID: tenantID.String(), PropertyID: propertyID, OwnerID: collectorUserID}
}

// ReadViaLease is tenant:read on a tenant seen through one of their leases, in the lease's
// property and assigned to its collector (uuid.Nil for none). Use it with authz.Allowed to
// filter a record that shows the tenant, such as a unit's current tenant, the way List does.
func ReadViaLease(tenantID, propertyID, collectorUserID uuid.UUID) authz.Request {
	return authz.Request{Action: authz.ActionTenantRead, Resource: viaResource(tenantID, propertyID, collectorUserID)}
}

func load(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID, day time.Time) (dbgen.Tenant, []Lease, error) {
	r, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return r, nil, ErrNotFound
	}
	if err != nil {
		return r, nil, fmt.Errorf("loading tenant: %w", err)
	}
	rows, err := q.ListLeaseDetailsForTenant(ctx, dbgen.ListLeaseDetailsForTenantParams{OrganizationID: org, TenantID: id})
	if err != nil {
		return r, nil, fmt.Errorf("loading leases: %w", err)
	}
	leases := make([]Lease, len(rows))
	for i, row := range rows {
		leases[i] = toLease(dbgen.GetLeaseDetailRow(row))
	}
	if err := attachDetails(ctx, q, org, day, leases); err != nil {
		return r, nil, err
	}
	return r, leases, nil
}

func toTenant(r dbgen.Tenant, leases []Lease) Tenant {
	return Tenant{
		ID: r.ID, Name: r.Name, Phone: r.Phone, Note: r.Note, Leases: leases,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func tenantWriteError(err error) error {
	if isUniqueViolation(err, "tenant_organization_id_phone_key") {
		return ErrPhoneTaken
	}
	return fmt.Errorf("saving tenant: %w", err)
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

// ValidTenant checks a tenant input and returns it trimmed, with the phone in E.164. Errors are
// *InputError.
func ValidTenant(in Input) (Input, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Note = strings.TrimSpace(in.Note)
	number, err := phone.Normalize(in.Phone)
	switch {
	case in.Name == "":
		return in, &InputError{Field: "name", Message: "Nama penyewa wajib diisi."}
	case len([]rune(in.Name)) > maxNameLength:
		return in, &InputError{Field: "name", Message: fmt.Sprintf("Nama penyewa paling panjang %d huruf.", maxNameLength)}
	case err != nil:
		return in, &InputError{Field: "phone", Message: "Nomor WhatsApp tidak valid."}
	case len([]rune(in.Note)) > maxNoteLength:
		return in, &InputError{Field: "note", Message: fmt.Sprintf("Catatan paling panjang %d huruf.", maxNoteLength)}
	}
	in.Phone = number
	return in, nil
}
