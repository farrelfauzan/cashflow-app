package demoseed

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// behind is a clock a fixed number of days earlier than another one.
type behind struct {
	c clock.Clock
	d time.Duration
}

func (b behind) Now() time.Time { return b.c.Now().Add(-b.d) }

// Behind returns a clock that reads days earlier than c. The seed gives it to the services that
// create units and leases, so a lease is entered EntryDays ago.
func Behind(c clock.Clock, days int) clock.Clock {
	return behind{c: c, d: time.Duration(days) * 24 * time.Hour}
}

// billedFrom is the "Mulai tagih dari" of a seeded lease: the start of the period running
// BilledFromDays before today, so the lease bills about two months back and no further (a yearly
// lease bills only its current year). It is the zero time, meaning the lease's own start date,
// when that day is before the lease starts (a lease that began or is booked after it). The period
// start is never after the period running on the entry day, which is what the lease service
// allows at most.
func billedFrom(start time.Time, term int, today time.Time) time.Time {
	n := tenant.PeriodIndex(start, term, today.AddDate(0, 0, -BilledFromDays))
	if n < 0 {
		return time.Time{}
	}
	return tenant.PeriodStart(start, term, n)
}

// paymentNamespace derives the request ids of the demo payments, so each payment has the same
// requestId in every run.
var paymentNamespace = uuid.NewSHA1(uuid.NameSpaceOID, []byte("cashflow-app demo payments"))

// The labels that, with the organization and the tenant, name a demo payment.
const (
	labelFull    = "full"
	labelPartial = "partial"
)

// GenerateDeps are the services GenerateInvoices works through.
type GenerateDeps struct {
	Store   *store.Store
	Authz   *authz.Service
	Orgs    *org.Service
	Billing *billing.Service
}

// Summary is what GenerateInvoices did in one demo organization.
type Summary struct {
	Organization string
	Leases       int // active leases looked at
	Invoices     int // invoices created
	Skipped      int // leases with no price in force
	Payments     int // demo payments recorded by this run
}

// GenerateInvoices bills the demo organizations and records their demo payments. It touches only
// the organizations Run creates, found like Run finds them (by name among the Admin's own
// memberships), and fails when one is missing: run Run first. For each one it generates the
// invoices due today (billing.Service.GenerateOrg, which creates nothing the second time), then,
// as the organization's Admin, records the payments of its Payers:
//
//   - Full pays everything the tenant owes after generation, by transfer;
//   - Partial pays about half of the tenant's oldest invoice, in cash;
//   - Overdue pays nothing.
//
// Each payment has a fixed requestId, and a payment whose requestId is already stored is skipped
// before anything is computed. That is the whole of the idempotency: the amount of the full
// payment depends on the invoices that exist, which can grow on a later day, so a repeat would
// not match the stored one and must not be sent. The service then records money once, ever.
func GenerateInvoices(ctx context.Context, d GenerateDeps, c clock.Clock) ([]Summary, error) {
	var out []Summary
	for _, o := range demoOrgs(c) {
		g := &generator{d: d, spec: o, today: tenant.Today(c)}
		sum, err := g.run(ctx)
		if err != nil {
			return nil, fmt.Errorf("invoicing %s: %w", o.Name, err)
		}
		out = append(out, sum)
	}
	return out, nil
}

// generator bills and pays for one demo organization.
type generator struct {
	d     GenerateDeps
	spec  orgSpec
	today time.Time       // in Asia/Jakarta
	p     authz.Principal // the org-wide Admin who records the payments
}

func (g *generator) run(ctx context.Context) (Summary, error) {
	sum := Summary{Organization: g.spec.Name}
	m, err := g.d.Orgs.FindForMember(ctx, g.spec.Actor, g.spec.Name)
	if errors.Is(err, org.ErrNotFound) {
		return sum, fmt.Errorf("the demo organization does not exist; run seed-demo first: %w", err)
	}
	if err != nil {
		return sum, fmt.Errorf("finding organization: %w", err)
	}
	if g.p, err = g.d.Authz.LoadPrincipal(ctx, m.OrganizationID, m.UserID); err != nil {
		return sum, fmt.Errorf("loading the acting admin: %w", err)
	}

	res, err := g.d.Billing.GenerateOrg(ctx, m.OrganizationID)
	sum.Leases, sum.Invoices, sum.Skipped = res.Leases, res.Invoices, res.Skipped
	if err != nil {
		return sum, fmt.Errorf("generating invoices: %w", err)
	}

	pay := g.spec.Payers
	for _, step := range []struct {
		name, label string
		amount      func(billing.Account) int64
		method      string
		note        string
	}{
		{pay.Full, labelFull, fullAmount, billing.MethodTransfer, "Pembayaran lunas (data demo)"},
		{pay.Partial, labelPartial, partialAmount, billing.MethodCash, "Cicilan sebagian, sisanya menyusul (data demo)"},
	} {
		recorded, err := g.pay(ctx, step.name, step.label, step.amount, step.method, step.note)
		if err != nil {
			return sum, fmt.Errorf("payment of %s: %w", step.name, err)
		}
		if recorded {
			sum.Payments++
		}
	}
	return sum, nil
}

// fullAmount is everything the tenant owes.
func fullAmount(a billing.Account) int64 { return a.Outstanding }

// partialAmount is about half of the oldest invoice that still owes something, in whole
// thousands when that leaves anything.
func partialAmount(a billing.Account) int64 {
	for _, inv := range a.Invoices {
		if inv.Outstanding <= 0 {
			continue
		}
		half := inv.Outstanding / 2
		if rounded := half / 1000 * 1000; rounded > 0 {
			return rounded
		}
		return half
	}
	return 0
}

// pay records one demo payment unless its requestId is already stored. It reports whether it
// recorded one. A tenant that owes nothing yet gets none.
func (g *generator) pay(ctx context.Context, name, label string, amount func(billing.Account) int64, method, note string) (bool, error) {
	number, err := g.phoneOf(name)
	if err != nil {
		return false, err
	}
	tenantID, err := g.tenantID(ctx, number)
	if err != nil {
		return false, err
	}
	requestID := uuid.NewSHA1(paymentNamespace, []byte(g.spec.Name+"|"+number+"|"+label))
	stored, err := g.stored(ctx, tenantID, requestID)
	if err != nil || stored {
		return false, err
	}
	acct, err := g.d.Billing.Account(ctx, g.p, tenantID)
	if err != nil {
		return false, err
	}
	due := amount(acct)
	if due <= 0 {
		return false, nil
	}
	res, err := g.d.Billing.RecordPayment(ctx, g.p, billing.PaymentInput{
		RequestID: requestID, TenantID: tenantID, Amount: due, Method: method, ReceivedOn: g.today, Note: note,
	})
	if err != nil {
		return false, err
	}
	return res.Created, nil
}

// phoneOf is the normalized phone of the organization's tenant called name.
func (g *generator) phoneOf(name string) (string, error) {
	for _, t := range g.spec.Tenants {
		if t.Name == name {
			return phone.Normalize(t.Phone)
		}
	}
	return "", fmt.Errorf("the demo has no tenant called %q", name)
}

// tenantID finds the organization's tenant by phone, as a member who may read tenants.
func (g *generator) tenantID(ctx context.Context, number string) (uuid.UUID, error) {
	var id uuid.UUID
	err := g.d.Store.WithOrgTx(ctx, g.p.OrganizationID, func(tx pgx.Tx) error {
		req := authz.Request{Action: authz.ActionTenantRead, Resource: authz.Resource{Type: "tenant"}}
		if err := g.d.Authz.Authorize(ctx, tx, g.p, req); err != nil {
			return err
		}
		tn, err := tenant.FindByPhone(ctx, tx, g.p.OrganizationID, number)
		if err != nil {
			return err
		}
		id = tn.ID
		return nil
	})
	return id, err
}

// stored reports whether a manual payment with the requestId exists in the organization.
func (g *generator) stored(ctx context.Context, tenantID, requestID uuid.UUID) (bool, error) {
	found := false
	err := g.d.Store.WithOrgTx(ctx, g.p.OrganizationID, func(tx pgx.Tx) error {
		req := authz.Request{Action: authz.ActionPaymentRead, Resource: authz.Resource{Type: "tenant", ID: tenantID.String()}}
		if err := g.d.Authz.Authorize(ctx, tx, g.p, req); err != nil {
			return err
		}
		ref := requestID.String()
		_, err := dbgen.New(tx).GetPaymentBySourceRef(ctx, dbgen.GetPaymentBySourceRefParams{
			OrganizationID: g.p.OrganizationID, Source: billing.SourceManual, SourceRef: &ref,
		})
		switch {
		case err == nil:
			found = true
		case !errors.Is(err, pgx.ErrNoRows):
			return fmt.Errorf("looking for the stored payment: %w", err)
		}
		return nil
	})
	return found, err
}
