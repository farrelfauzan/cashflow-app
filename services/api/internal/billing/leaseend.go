package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// The reasons of the corrections the end of a lease makes, in Bahasa Indonesia as the
// Supervisor report shows them.
const (
	// ReasonNotRenewed cancels an invoice for a period that starts after the planned end date or
	// the move-out date (decision 25).
	ReasonNotRenewed = "sewa tidak diperpanjang"
	// ReasonPlannedEndChanged restores an invoice ReasonNotRenewed cancelled, when the planned
	// end date moves later or is cleared.
	ReasonPlannedEndChanged = "rencana keluar diubah"
	// ReasonBookingCancelled cancels every invoice of a lease ended on its own start date, a
	// cancelled booking, which is never billed (decision 17).
	ReasonBookingCancelled = "sewa dibatalkan"
	// ReasonDeposit pays what a lease still owes at move-out from its deposit (decision 36).
	ReasonDeposit = "potong deposit"
)

// LeaseInvoice is one invoice of a lease as the end-of-lease rules see it.
type LeaseInvoice struct {
	ID          uuid.UUID
	Period      time.Time // the period's first day
	Billed      int64     // amount plus corrections; 0 when cancelled
	Outstanding int64     // billed minus what is allocated to it
	// LastReason and LastDelta are its newest correction, "" and 0 when it has none.
	LastReason string
	LastDelta  int64
}

// Correction is one correction to write on an invoice.
type Correction struct {
	InvoiceID uuid.UUID
	Delta     int64 // signed, never 0
	Reason    string
}

// PlanPlannedEnd returns the corrections that bring a lease's invoices in line with its planned
// end date (nil when it is cleared), oldest period first:
//
//   - an invoice for a period that starts after the date, still billing something, is cancelled:
//     minus what it bills, ReasonNotRenewed;
//   - an invoice for a period on or before the date (any, when the date is cleared) whose newest
//     correction is such a cancellation, and still billing nothing, is restored: plus what that
//     correction took off, ReasonPlannedEndChanged.
//
// An invoice cancelled for any other reason, or corrected again since, is left alone.
func PlanPlannedEnd(invoices []LeaseInvoice, plannedEnd *time.Time) []Correction {
	var out []Correction
	for _, inv := range invoices {
		inRange := plannedEnd == nil || !inv.Period.After(*plannedEnd)
		switch {
		case !inRange && inv.Billed > 0:
			out = append(out, Correction{InvoiceID: inv.ID, Delta: -inv.Billed, Reason: ReasonNotRenewed})
		case inRange && inv.Billed == 0 && inv.LastReason == ReasonNotRenewed && inv.LastDelta < 0:
			out = append(out, Correction{InvoiceID: inv.ID, Delta: -inv.LastDelta, Reason: ReasonPlannedEndChanged})
		}
	}
	return out
}

// PlanMoveOut returns the corrections that bring an ended lease's invoices in line with its end
// date. For a cancelled booking (ended on its start date) every invoice that still bills
// something is cancelled, ReasonBookingCancelled. Otherwise the end date works as a planned end
// (see PlanPlannedEnd): every invoice for a period that starts after it is cancelled,
// ReasonNotRenewed, and one an earlier planned end had cancelled for a period the tenant started
// after all (planned end 14 Nov, moved out 20 Nov) is restored, ReasonPlannedEndChanged, since a
// started period is billed in full.
func PlanMoveOut(invoices []LeaseInvoice, start, end time.Time) []Correction {
	if !end.Equal(start) {
		return PlanPlannedEnd(invoices, &end)
	}
	var out []Correction
	for _, inv := range invoices {
		if inv.Billed > 0 {
			out = append(out, Correction{InvoiceID: inv.ID, Delta: -inv.Billed, Reason: ReasonBookingCancelled})
		}
	}
	return out
}

// PlanDeposit returns the corrections that pay what a lease's invoices still owe from its
// deposit, oldest period first, up to the deposit: minus the smaller of what the invoice owes
// and what is left of the deposit, ReasonDeposit. What the deposit does not cover stays owed;
// what it does not use is the tenant's to take back, outside the app.
func PlanDeposit(invoices []LeaseInvoice, deposit int64) []Correction {
	var out []Correction
	for _, inv := range invoices {
		if deposit <= 0 {
			break
		}
		if inv.Outstanding <= 0 {
			continue
		}
		amount := min(inv.Outstanding, deposit)
		deposit -= amount
		out = append(out, Correction{InvoiceID: inv.ID, Delta: -amount, Reason: ReasonDeposit})
	}
	return out
}

// PlannedEndChanged implements tenant.LeaseMoney: inside the lease service's transaction, it
// cancels and restores the lease's invoices for its planned end date (see PlanPlannedEnd), each
// correction authorized as correction:create by the acting member and audited, then reruns the
// tenant's allocation, so money paid on a cancelled invoice pays older debt first and the rest
// is credit.
func (s *Service) PlannedEndChanged(ctx context.Context, tx pgx.Tx, p authz.Principal, l tenant.Lease) error {
	q := dbgen.New(tx)
	invoices, err := leaseInvoices(ctx, q, p.OrganizationID, l.ID)
	if err != nil {
		return err
	}
	changes := PlanPlannedEnd(invoices, l.PlannedEndDate)
	if len(changes) == 0 {
		return nil
	}
	if err := s.correct(ctx, tx, q, p, l, changes); err != nil {
		return err
	}
	_, err = Allocate(ctx, tx, p.OrganizationID, l.TenantID)
	return err
}

// MovedOut implements tenant.LeaseMoney for a lease the service has just ended, inside its
// transaction:
//
//  1. a period the tenant started that has no invoice yet (the generate task had not run) gets
//     one, as the task would have made it;
//  2. invoices it must not bill are cancelled, and a started period a planned end had cancelled
//     is restored (see PlanMoveOut); allocation reruns;
//  3. what the lease's invoices still owe is paid from its deposit (see PlanDeposit; decision 36,
//     the only arrears rule until organization settings exist) and allocation reruns.
//
// Every correction is authorized as correction:create by the acting member and audited.
func (s *Service) MovedOut(ctx context.Context, tx pgx.Tx, p authz.Principal, l tenant.Lease) error {
	if l.EndDate == nil {
		return errors.New("billing: MovedOut needs an ended lease")
	}
	org := p.OrganizationID
	q := dbgen.New(tx)
	created, err := s.createMissing(ctx, q, tx, org, billedLease{
		ID: l.ID, TenantID: l.TenantID, UnitID: l.UnitID, PropertyID: l.PropertyID,
		Dates: LeaseDates{
			Start: l.StartDate, EnteredOn: l.EnteredOn, BilledFrom: l.BilledFrom, TermMonths: l.TermMonths,
			PlannedEnd: l.PlannedEndDate, End: l.EndDate,
		},
	}, tenant.Today(s.clock))
	if err != nil && !errors.Is(err, tenant.ErrNoPrice) { // no price: logged, the move-out goes on
		return err
	}

	invoices, err := leaseInvoices(ctx, q, org, l.ID)
	if err != nil {
		return err
	}
	cancel := PlanMoveOut(invoices, l.StartDate, *l.EndDate)
	if err := s.correct(ctx, tx, q, p, l, cancel); err != nil {
		return err
	}
	if created > 0 || len(cancel) > 0 {
		if _, err := Allocate(ctx, tx, org, l.TenantID); err != nil {
			return err
		}
	}

	if l.DepositAmount == 0 {
		return nil
	}
	if invoices, err = leaseInvoices(ctx, q, org, l.ID); err != nil {
		return err
	}
	deduct := PlanDeposit(invoices, l.DepositAmount)
	if len(deduct) == 0 {
		return nil
	}
	if err := s.correct(ctx, tx, q, p, l, deduct); err != nil {
		return err
	}
	_, err = Allocate(ctx, tx, org, l.TenantID)
	return err
}

// correct authorizes and writes each correction, as the acting member. Authorize audits each
// allowed one in tx and records a denial in its own transaction; a denial returns an error
// wrapping authz.ErrForbidden, and the caller's rollback undoes everything.
func (s *Service) correct(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, p authz.Principal, l tenant.Lease, changes []Correction) error {
	for _, c := range changes {
		amount := c.Delta
		if amount < 0 {
			amount = -amount
		}
		req := authz.Request{Action: authz.ActionCorrectionCreate, Resource: authz.Resource{
			Type: "invoice", ID: c.InvoiceID.String(), PropertyID: l.PropertyID, OwnerID: l.CollectorUserID, Amount: &amount,
		}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		_, err := q.CreateCorrection(ctx, dbgen.CreateCorrectionParams{
			ID: uuid.New(), OrganizationID: p.OrganizationID,
			InvoiceID:   uuid.NullUUID{UUID: c.InvoiceID, Valid: true},
			AmountDelta: c.Delta, Reason: c.Reason, CreatedByStaffUserID: p.UserID,
		})
		if err != nil {
			return fmt.Errorf("writing correction on invoice %s: %w", c.InvoiceID, err)
		}
	}
	return nil
}

func leaseInvoices(ctx context.Context, q *dbgen.Queries, org, leaseID uuid.UUID) ([]LeaseInvoice, error) {
	rows, err := q.ListLeaseInvoicesForChange(ctx, dbgen.ListLeaseInvoicesForChangeParams{OrganizationID: org, LeaseID: leaseID})
	if err != nil {
		return nil, fmt.Errorf("loading the lease's invoices: %w", err)
	}
	out := make([]LeaseInvoice, len(rows))
	for i, r := range rows {
		out[i] = LeaseInvoice{
			ID: r.InvoiceID, Period: r.Period, Billed: r.Billed, Outstanding: r.Outstanding,
			LastReason: r.LastReason, LastDelta: r.LastDelta,
		}
	}
	return out, nil
}
