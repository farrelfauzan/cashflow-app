// Package promise holds tenants' payment promises (phase B, slice 4, decision 41): "Saya janji
// bayar" from the payment page, or recorded by a member for the tenant.
//
// A promise belongs to the tenant, not to one invoice, and a tenant has at most one active
// promise. Making a promise while one is active adds a version to it (an edit); every version is
// kept. Each version covers the tenant's invoices still owing when it was made. A promise closes
// as kept the moment a payment keeps it, or with its outcome (kept, partially kept, broken) once
// its promised day has ended (Closer); the reminders it silences come later in slice 4.
//
// The tenant-page entry point takes the link token, which is a secret: nothing here logs it,
// stores it or puts it in an error.
package promise

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Promise is a tenant's promise with its versions, oldest first.
type Promise struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Status    string
	CreatedAt time.Time
	ClosedAt  *time.Time
	Versions  []Version
}

// Latest is the promise's newest version: its terms now.
func (p Promise) Latest() Version { return p.Versions[len(p.Versions)-1] }

// Version is one version of a promise's terms.
type Version struct {
	ID             uuid.UUID
	Number         int
	Kind           string
	PromisedDate   time.Time // a calendar day, midnight UTC
	PromisedAmount *int64    // nil: everything the version covers
	PayingNow      int64
	OutstandingAt  int64 // what the tenant owed when it was made
	Source         string
	Note           string
	// CreatedBy is the member who made it; nil from the tenant page.
	CreatedBy *billing.StaffRef
	CreatedAt time.Time
	// CoveredInvoiceIDs are the invoices still owing when it was made, oldest period first.
	CoveredInvoiceIDs []uuid.UUID
}

// LinkOpener resolves a payment link token and checks that the link still works. paylink.Service
// is the one implementation.
type LinkOpener interface {
	// Resolve finds the organization and link behind a token, with no organization scope.
	Resolve(ctx context.Context, token string) (org, linkID uuid.UUID, err error)
	// OpenLink checks inside tx that the link works and returns its tenant and invoice.
	OpenLink(ctx context.Context, tx pgx.Tx, org, linkID uuid.UUID) (tenantID, invoiceID uuid.UUID, err error)
}

// TenantAuthorizer checks an action on a tenant through its leases inside tx, as the tenant's
// money account is checked. billing.Service implements it.
type TenantAuthorizer interface {
	AuthorizeTenant(ctx context.Context, tx pgx.Tx, p authz.Principal, tenantID uuid.UUID, action string) error
}

// Deps are what the service needs.
type Deps struct {
	Store   *store.Store
	Links   LinkOpener
	Tenants TenantAuthorizer
	// Clock decides today in Asia/Jakarta.
	Clock clock.Clock
}

// Service makes and lists payment promises.
type Service struct {
	store   *store.Store
	links   LinkOpener
	tenants TenantAuthorizer
	clock   clock.Clock
}

// NewService returns a Service.
func NewService(d Deps) *Service {
	return &Service{store: d.Store, links: d.Links, tenants: d.Tenants, clock: d.Clock}
}

// author is who makes a version.
type author struct {
	source string
	linkID uuid.UUID // the tenant page's link
	userID uuid.UUID // the member's staff user
}

// MakeFromLink makes, or edits, the promise of the tenant behind token, from the payment page.
// The token grants it; there is no principal to authorize (CLAUDE.md "Not RBAC principals"). The
// tenant page has no note. A malformed, unknown or revoked token is paylink.ErrLinkNotFound and an
// expired one paylink.ErrLinkExpired, as for the page itself. See make for the rules.
func (s *Service) MakeFromLink(ctx context.Context, token string, in Input) (Promise, error) {
	in.Note = ""
	in, err := in.checked()
	if err != nil {
		return Promise{}, err
	}
	org, linkID, err := s.links.Resolve(ctx, token)
	if err != nil {
		return Promise{}, err
	}
	var out Promise
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		tenantID, _, err := s.links.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		out, err = s.make(ctx, tx, org, tenantID, in, author{source: SourceTenantPage, linkID: linkID})
		return err
	})
	if err != nil {
		return Promise{}, err
	}
	return out, nil
}

// MakeForTenant makes, or edits, a tenant's promise for a member (source admin). It needs
// promise:create on the tenant through each of its leases, checked and audited inside the
// transaction that saves it. A missing tenant is a *billing.NotFoundError. See make for the rules.
func (s *Service) MakeForTenant(ctx context.Context, p authz.Principal, tenantID uuid.UUID, in Input) (Promise, error) {
	in, err := in.checked()
	if err != nil {
		return Promise{}, err
	}
	var out Promise
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		if err := s.tenants.AuthorizeTenant(ctx, tx, p, tenantID, authz.ActionPromiseCreate); err != nil {
			return err
		}
		out, err = s.make(ctx, tx, p.OrganizationID, tenantID, in, author{source: SourceAdmin, userID: p.UserID})
		return err
	})
	if err != nil {
		return Promise{}, err
	}
	return out, nil
}

// make saves a promise version inside tx, scoped to org, and returns the promise with every
// version. It first takes the tenant's allocation lock (LockTenantForAllocation), the one every
// payment and correction of the tenant takes, so what the tenant owes cannot change while it is
// read and two makes for one tenant run one after the other: the second sees the first's promise
// and adds the next version to it.
//
// The input is decided against what the tenant owes now and their leases (see Decide). With an
// active promise the version is the next one of it (an edit); otherwise a new promise is made with
// version 1. The version covers the tenant's invoices still owing now.
func (s *Service) make(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, in Input, by author) (Promise, error) {
	q := dbgen.New(tx)
	if _, err := q.LockTenantForAllocation(ctx, dbgen.LockTenantForAllocationParams{OrganizationID: org, TenantID: tenantID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Promise{}, &billing.NotFoundError{}
		}
		return Promise{}, fmt.Errorf("promise: locking the tenant: %w", err)
	}
	today, now := tenant.Today(s.clock), s.clock.Now()
	acct, err := billing.LoadAccountTx(ctx, tx, org, tenantID, today)
	if err != nil {
		return Promise{}, fmt.Errorf("promise: loading what the tenant owes: %w", err)
	}
	leases, err := leaseDates(ctx, q, org, tenantID)
	if err != nil {
		return Promise{}, err
	}
	window := WindowOn(today, leases)
	promiseID, number := uuid.New(), int32(1)
	active, err := q.GetActivePromise(ctx, dbgen.GetActivePromiseParams{OrganizationID: org, TenantID: tenantID})
	switch {
	case err == nil:
		// An edit: the date stays within 60 days of the promise's first version.
		promiseID, number = active.PromiseID, active.Version+1
		window = window.CappedFrom(jakartaDay(active.PromiseCreatedAt))
	case !errors.Is(err, pgx.ErrNoRows):
		return Promise{}, fmt.Errorf("promise: loading the active promise: %w", err)
	}
	date, err := Decide(in, acct.Outstanding, window)
	if err != nil {
		return Promise{}, err
	}
	if number == 1 {
		if err := q.InsertPaymentPromise(ctx, dbgen.InsertPaymentPromiseParams{
			ID: promiseID, OrganizationID: org, TenantID: tenantID, CreatedAt: now,
		}); err != nil {
			return Promise{}, fmt.Errorf("promise: saving the promise: %w", err)
		}
	}

	// The payments after this version are the tenant's with a higher seq than any they have now
	// (see Progress). Every payment insert holds the tenant's lock taken above, so none is in
	// flight with a lower seq.
	mark, err := q.TenantPaymentSeqMark(ctx, dbgen.TenantPaymentSeqMarkParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return Promise{}, fmt.Errorf("promise: reading the tenant's last payment: %w", err)
	}
	versionID := uuid.New()
	v := dbgen.InsertPromiseVersionParams{
		ID: versionID, OrganizationID: org, PromiseID: promiseID, TenantID: tenantID, Version: number,
		Kind: in.Kind, PromisedDate: date, PromisedAmount: in.PromisedAmount, PayingNow: in.PayingNow,
		OutstandingAt: acct.Outstanding, Source: by.source, CreatedAt: now, PaymentSeqMark: mark,
	}
	if in.Note != "" {
		v.Note = &in.Note
	}
	if by.linkID != uuid.Nil {
		v.PaymentLinkID = uuid.NullUUID{UUID: by.linkID, Valid: true}
	}
	if by.userID != uuid.Nil {
		v.CreatedByStaffUserID = uuid.NullUUID{UUID: by.userID, Valid: true}
	}
	if err := q.InsertPromiseVersion(ctx, v); err != nil {
		return Promise{}, fmt.Errorf("promise: saving the version: %w", err)
	}
	var covered []uuid.UUID
	for _, inv := range acct.Invoices {
		if inv.Outstanding > 0 {
			covered = append(covered, inv.ID)
		}
	}
	if err := q.InsertPromiseCoveredInvoices(ctx, dbgen.InsertPromiseCoveredInvoicesParams{
		OrganizationID: org, PromiseVersionID: versionID, InvoiceIds: covered, TenantID: tenantID,
	}); err != nil {
		return Promise{}, fmt.Errorf("promise: saving the covered invoices: %w", err)
	}

	head := dbgen.ListTenantPromisePageRow{ID: promiseID, Status: StatusActive, CreatedAt: now}
	if active.PromiseID != uuid.Nil {
		head.CreatedAt = active.PromiseCreatedAt
	}
	got, err := loadPromises(ctx, q, org, tenantID, []dbgen.ListTenantPromisePageRow{head})
	if err != nil {
		return Promise{}, err
	}
	return got[0], nil
}

// List returns one page of a tenant's promises, newest first, each with every version. It needs
// promise:read on the tenant through each of its leases, as the tenant's account does. A missing
// tenant is a *billing.NotFoundError.
func (s *Service) List(ctx context.Context, p authz.Principal, tenantID uuid.UUID, page listing.Page) (listing.Result[Promise], error) {
	org := p.OrganizationID
	out := listing.Result[Promise]{Items: []Promise{}, Page: page}
	// One snapshot, so the total and the page agree.
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		if err := s.tenants.AuthorizeTenant(ctx, tx, p, tenantID, authz.ActionPromiseRead); err != nil {
			return err
		}
		q := dbgen.New(tx)
		total, err := q.CountTenantPromises(ctx, dbgen.CountTenantPromisesParams{OrganizationID: org, TenantID: tenantID})
		if err != nil {
			return fmt.Errorf("promise: counting the tenant's promises: %w", err)
		}
		rows, err := q.ListTenantPromisePage(ctx, dbgen.ListTenantPromisePageParams{
			OrganizationID: org, TenantID: tenantID, PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("promise: listing the tenant's promises: %w", err)
		}
		items, err := loadPromises(ctx, q, org, tenantID, rows)
		if err != nil {
			return err
		}
		out.Items, out.Total = items, total
		return nil
	})
	if err != nil {
		return listing.Result[Promise]{}, err
	}
	return out, nil
}

// loadPromises fills in the versions and covered invoices of the given promises, keeping their
// order.
func loadPromises(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, heads []dbgen.ListTenantPromisePageRow) ([]Promise, error) {
	out := make([]Promise, len(heads))
	if len(heads) == 0 {
		return out, nil
	}
	ids := make([]uuid.UUID, len(heads))
	for i, h := range heads {
		ids[i] = h.ID
	}
	vrows, err := q.ListPromiseVersions(ctx, dbgen.ListPromiseVersionsParams{OrganizationID: org, PromiseIds: ids})
	if err != nil {
		return nil, fmt.Errorf("promise: loading versions: %w", err)
	}
	vids := make([]uuid.UUID, len(vrows))
	for i, r := range vrows {
		vids[i] = r.ID
	}
	crows, err := q.ListPromiseCoveredInvoices(ctx, dbgen.ListPromiseCoveredInvoicesParams{OrganizationID: org, VersionIds: vids})
	if err != nil {
		return nil, fmt.Errorf("promise: loading covered invoices: %w", err)
	}
	covered := map[uuid.UUID][]uuid.UUID{}
	for _, r := range crows {
		covered[r.PromiseVersionID] = append(covered[r.PromiseVersionID], r.InvoiceID)
	}
	versions := map[uuid.UUID][]Version{}
	for _, r := range vrows {
		v := Version{
			ID: r.ID, Number: int(r.Version), Kind: r.Kind, PromisedDate: r.PromisedDate, PromisedAmount: r.PromisedAmount,
			PayingNow: r.PayingNow, OutstandingAt: r.OutstandingAt, Source: r.Source, CreatedAt: r.CreatedAt,
			CoveredInvoiceIDs: covered[r.ID],
		}
		if v.CoveredInvoiceIDs == nil {
			v.CoveredInvoiceIDs = []uuid.UUID{}
		}
		if r.Note != nil {
			v.Note = *r.Note
		}
		if r.CreatedByMemberID.Valid {
			ref := billing.StaffRef{MemberID: r.CreatedByMemberID.UUID}
			if r.CreatedByName != nil {
				ref.Name = *r.CreatedByName
			}
			v.CreatedBy = &ref
		}
		versions[r.PromiseID] = append(versions[r.PromiseID], v)
	}
	for i, h := range heads {
		if len(versions[h.ID]) == 0 {
			// Every promise is inserted with its first version in one transaction.
			return nil, fmt.Errorf("promise: promise %s has no version", h.ID)
		}
		out[i] = Promise{ID: h.ID, TenantID: tenantID, Status: h.Status, CreatedAt: h.CreatedAt, ClosedAt: h.ClosedAt, Versions: versions[h.ID]}
	}
	return out, nil
}

// leaseDates are the billing dates of the tenant's active leases.
func leaseDates(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID) ([]billing.LeaseDates, error) {
	rows, err := q.ListTenantActiveLeaseDates(ctx, dbgen.ListTenantActiveLeaseDatesParams{OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("promise: loading the tenant's leases: %w", err)
	}
	out := make([]billing.LeaseDates, len(rows))
	for i, r := range rows {
		out[i] = billing.LeaseDates{
			Start: r.StartDate, EnteredOn: r.EnteredOn, BilledFrom: r.BilledFrom, TermMonths: int(r.TermMonths),
			PlannedEnd: r.PlannedEndDate, End: r.EndDate,
		}
	}
	return out, nil
}

// PageState is what the tenant payment page shows about promises.
type PageState struct {
	// Active is the active promise's newest version, nil when there is none.
	Active *Version
	// Window is what a promise made now may say; nil when the tenant owes nothing.
	Window *Window
}

// LoadPageTx reads the page's promise state inside tx, which must be scoped to org (the page's
// snapshot); outstanding is what the tenant owes now and today the current day in Asia/Jakarta.
// It checks no permission: the payment page's token grants it. Active carries no note, member or
// covered invoices: the page shows none of them.
func LoadPageTx(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, outstanding int64, today time.Time) (PageState, error) {
	q := dbgen.New(tx)
	var out PageState
	r, err := q.GetActivePromise(ctx, dbgen.GetActivePromiseParams{OrganizationID: org, TenantID: tenantID})
	switch {
	case err == nil:
		out.Active = &Version{
			ID: r.ID, Number: int(r.Version), Kind: r.Kind, PromisedDate: r.PromisedDate, PromisedAmount: r.PromisedAmount,
			PayingNow: r.PayingNow, OutstandingAt: r.OutstandingAt, Source: r.Source, CreatedAt: r.CreatedAt,
		}
	case !errors.Is(err, pgx.ErrNoRows):
		return PageState{}, fmt.Errorf("promise: loading the active promise: %w", err)
	}
	if outstanding <= 0 {
		return out, nil
	}
	leases, err := leaseDates(ctx, q, org, tenantID)
	if err != nil {
		return PageState{}, err
	}
	w := WindowOn(today, leases)
	if out.Active != nil {
		w = w.CappedFrom(jakartaDay(r.PromiseCreatedAt))
	}
	out.Window = &w
	return out, nil
}

// jakartaDay is the calendar day of t in Asia/Jakarta, as midnight UTC like a date column.
func jakartaDay(t time.Time) time.Time { return calendarDay(t.In(clock.Jakarta)) }
