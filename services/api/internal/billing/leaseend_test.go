package billing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

func TestPlanPlannedEnd(t *testing.T) {
	t.Parallel()
	oct, nov, dec := uuid.New(), uuid.New(), uuid.New()
	open := func(id uuid.UUID, period time.Time) billing.LeaseInvoice {
		return billing.LeaseInvoice{ID: id, Period: period, Billed: 700_000, Outstanding: 700_000}
	}
	cancelled := func(id uuid.UUID, period time.Time, reason string) billing.LeaseInvoice {
		return billing.LeaseInvoice{ID: id, Period: period, LastReason: reason, LastDelta: -700_000}
	}
	tests := []struct {
		name       string
		invoices   []billing.LeaseInvoice
		plannedEnd *time.Time
		want       []billing.Correction
	}{
		{
			name:       "a period that starts after the date is cancelled for what it bills",
			invoices:   []billing.LeaseInvoice{open(oct, d(2026, 10, 15)), {ID: nov, Period: d(2026, 11, 15), Billed: 750_000, Outstanding: 550_000}},
			plannedEnd: ptr(d(2026, 11, 14)),
			want:       []billing.Correction{{InvoiceID: nov, Delta: -750_000, Reason: billing.ReasonNotRenewed}},
		},
		{
			name:       "a period that starts on the date stays",
			invoices:   []billing.LeaseInvoice{open(oct, d(2026, 10, 15)), open(nov, d(2026, 11, 15))},
			plannedEnd: ptr(d(2026, 11, 15)),
		},
		{
			name:       "an invoice already cancelled is not cancelled twice",
			invoices:   []billing.LeaseInvoice{open(oct, d(2026, 10, 15)), cancelled(nov, d(2026, 11, 15), billing.ReasonNotRenewed)},
			plannedEnd: ptr(d(2026, 11, 14)),
		},
		{
			name:       "clearing the date restores what it cancelled",
			invoices:   []billing.LeaseInvoice{open(oct, d(2026, 10, 15)), cancelled(nov, d(2026, 11, 15), billing.ReasonNotRenewed)},
			plannedEnd: nil,
			want:       []billing.Correction{{InvoiceID: nov, Delta: 700_000, Reason: billing.ReasonPlannedEndChanged}},
		},
		{
			name: "moving the date later restores only the periods back in range",
			invoices: []billing.LeaseInvoice{
				open(oct, d(2026, 10, 15)),
				cancelled(nov, d(2026, 11, 15), billing.ReasonNotRenewed),
				cancelled(dec, d(2026, 12, 15), billing.ReasonNotRenewed),
			},
			plannedEnd: ptr(d(2026, 12, 14)),
			want:       []billing.Correction{{InvoiceID: nov, Delta: 700_000, Reason: billing.ReasonPlannedEndChanged}},
		},
		{
			name:       "an invoice cancelled for another reason is never restored",
			invoices:   []billing.LeaseInvoice{cancelled(nov, d(2026, 11, 15), "salah tagih")},
			plannedEnd: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, billing.PlanPlannedEnd(tc.invoices, tc.plannedEnd))
		})
	}
}

func TestPlanMoveOut(t *testing.T) {
	t.Parallel()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	invoices := []billing.LeaseInvoice{
		{ID: a, Period: d(2026, 10, 15), Billed: 700_000},
		{ID: b, Period: d(2026, 11, 15), Billed: 700_000},
		{ID: c, Period: d(2026, 12, 15), Billed: 0},
	}
	t.Run("periods after the end date are cancelled", func(t *testing.T) {
		t.Parallel()
		require.Equal(t,
			[]billing.Correction{{InvoiceID: b, Delta: -700_000, Reason: billing.ReasonNotRenewed}},
			billing.PlanMoveOut(invoices, d(2026, 10, 15), d(2026, 11, 10)))
	})
	t.Run("a period that started stays billed in full", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, billing.PlanMoveOut(invoices, d(2026, 10, 15), d(2026, 11, 15)))
	})
	t.Run("a started period a planned end had cancelled is restored", func(t *testing.T) {
		t.Parallel()
		cancelled := []billing.LeaseInvoice{
			{ID: a, Period: d(2026, 10, 15), Billed: 700_000},
			{ID: b, Period: d(2026, 11, 15), Billed: 0, LastReason: billing.ReasonNotRenewed, LastDelta: -700_000},
		}
		require.Equal(t,
			[]billing.Correction{{InvoiceID: b, Delta: 700_000, Reason: billing.ReasonPlannedEndChanged}},
			billing.PlanMoveOut(cancelled, d(2026, 10, 15), d(2026, 11, 20)))
	})
	t.Run("a cancelled booking cancels everything it billed", func(t *testing.T) {
		t.Parallel()
		require.Equal(t,
			[]billing.Correction{
				{InvoiceID: a, Delta: -700_000, Reason: billing.ReasonBookingCancelled},
				{InvoiceID: b, Delta: -700_000, Reason: billing.ReasonBookingCancelled},
			},
			billing.PlanMoveOut(invoices, d(2026, 10, 15), d(2026, 10, 15)))
	})
}

func TestPlanDeposit(t *testing.T) {
	t.Parallel()
	sep, oct, nov := uuid.New(), uuid.New(), uuid.New()
	invoices := []billing.LeaseInvoice{
		{ID: sep, Period: d(2026, 9, 15), Billed: 700_000, Outstanding: 0},
		{ID: oct, Period: d(2026, 10, 15), Billed: 700_000, Outstanding: 350_000},
		{ID: nov, Period: d(2026, 11, 15), Billed: 700_000, Outstanding: 700_000},
	}
	tests := []struct {
		name    string
		deposit int64
		want    []billing.Correction
	}{
		{name: "no deposit pays nothing", deposit: 0},
		{
			name: "a deposit smaller than the debt pays the oldest first", deposit: 500_000,
			want: []billing.Correction{
				{InvoiceID: oct, Delta: -350_000, Reason: billing.ReasonDeposit},
				{InvoiceID: nov, Delta: -150_000, Reason: billing.ReasonDeposit},
			},
		},
		{
			name: "a deposit larger than the debt pays all of it and no more", deposit: 2_000_000,
			want: []billing.Correction{
				{InvoiceID: oct, Delta: -350_000, Reason: billing.ReasonDeposit},
				{InvoiceID: nov, Delta: -700_000, Reason: billing.ReasonDeposit},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, billing.PlanDeposit(invoices, tc.deposit))
		})
	}
	require.Empty(t, billing.PlanDeposit([]billing.LeaseInvoice{{ID: sep, Billed: 700_000}}, 500_000), "nothing owed, nothing taken")
}
