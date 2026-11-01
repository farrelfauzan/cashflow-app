package billing_test

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

func sortInvoices(in []billing.Invoice) []billing.Invoice {
	out := slices.Clone(in)
	slices.SortStableFunc(out, func(a, b billing.Invoice) int {
		return cmp.Or(a.Period.Compare(b.Period), cmp.Compare(a.Seq, b.Seq))
	})
	return out
}

func sortPayments(in []billing.Payment) []billing.Payment {
	out := slices.Clone(in)
	slices.SortStableFunc(out, func(a, b billing.Payment) int {
		return cmp.Or(a.ReceivedAt.Compare(b.ReceivedAt), cmp.Compare(a.Seq, b.Seq))
	})
	return out
}

var (
	p1, p2, p3     = uuid.New(), uuid.New(), uuid.New()
	inv1, inv2, i3 = uuid.New(), uuid.New(), uuid.New()
)

func month(m time.Month) time.Time { return time.Date(2026, m, 15, 0, 0, 0, 0, time.UTC) }

func pay(id uuid.UUID, day int, seq, net int64) billing.Payment {
	return billing.Payment{ID: id, ReceivedAt: time.Date(2026, 10, day, 10, 0, 0, 0, time.UTC), Seq: seq, Net: net}
}

func bill(id uuid.UUID, m time.Month, seq, billed int64) billing.Invoice {
	return billing.Invoice{ID: id, Period: month(m), Seq: seq, Billed: billed}
}

func alloc(seq int64, p, i uuid.UUID, amount int64) billing.Allocation {
	return billing.Allocation{Seq: seq, PaymentID: p, InvoiceID: i, Amount: amount}
}

func row(p, i uuid.UUID, amount int64) billing.Row {
	return billing.Row{PaymentID: p, InvoiceID: i, Amount: amount}
}

func TestPlan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		payments []billing.Payment
		invoices []billing.Invoice
		existing []billing.Allocation
		want     []billing.Row
	}{
		{name: "nothing to allocate"},
		{
			name:     "a payment with no invoice stays credit",
			payments: []billing.Payment{pay(p1, 1, 1, 500_000)},
		},
		{
			name:     "an invoice with no payment stays open",
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000)},
		},
		{
			name:     "exact payment pays the invoice",
			payments: []billing.Payment{pay(p1, 28, 2, 700_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000)},
			want:     []billing.Row{row(p1, inv1, 700_000)},
		},
		{
			name:     "a partial payment leaves the rest outstanding",
			payments: []billing.Payment{pay(p1, 28, 2, 350_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000)},
			want:     []billing.Row{row(p1, inv1, 350_000)},
		},
		{
			name:     "an overpayment pays the invoice and leaves credit",
			payments: []billing.Payment{pay(p1, 28, 2, 1_000_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000)},
			want:     []billing.Row{row(p1, inv1, 700_000)},
		},
		{
			name:     "two payments across three invoices, oldest first",
			payments: []billing.Payment{pay(p2, 20, 5, 800_000), pay(p1, 10, 4, 600_000)},
			invoices: []billing.Invoice{
				bill(i3, time.November, 3, 500_000), bill(inv1, time.September, 1, 500_000), bill(inv2, time.October, 2, 500_000),
			},
			want: []billing.Row{
				row(p1, inv1, 500_000), row(p1, inv2, 100_000), row(p2, inv2, 400_000), row(p2, i3, 400_000),
			},
		},
		{
			name:     "the rest of a partly paid invoice is paid before the next one",
			payments: []billing.Payment{pay(p1, 28, 3, 350_000), pay(p2, 30, 4, 1_050_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000), bill(inv2, time.November, 2, 700_000)},
			existing: []billing.Allocation{alloc(5, p1, inv1, 350_000)},
			want:     []billing.Row{row(p2, inv1, 350_000), row(p2, inv2, 700_000)},
		},
		{
			name:     "credit pays an invoice that arrives later",
			payments: []billing.Payment{pay(p1, 28, 2, 1_000_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000), bill(inv2, time.November, 4, 700_000)},
			existing: []billing.Allocation{alloc(3, p1, inv1, 700_000)},
			want:     []billing.Row{row(p1, inv2, 300_000)},
		},
		{
			name:     "invoices in the same period go by seq",
			payments: []billing.Payment{pay(p1, 28, 3, 100)},
			invoices: []billing.Invoice{bill(inv2, time.October, 2, 100), bill(inv1, time.October, 1, 100)},
			want:     []billing.Row{row(p1, inv1, 100)},
		},
		{
			name:     "payments received at the same moment go by seq",
			payments: []billing.Payment{pay(p2, 28, 4, 100), pay(p1, 28, 3, 100)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 150)},
			want:     []billing.Row{row(p1, inv1, 100), row(p2, inv1, 50)},
		},
		{
			name:     "an invoice cancelled after it was partly paid returns its money to the next open invoice, the rest is credit",
			payments: []billing.Payment{pay(p1, 28, 4, 900_000)},
			invoices: []billing.Invoice{
				bill(inv1, time.October, 1, 700_000), bill(inv2, time.November, 2, 0), bill(i3, time.December, 3, 150_000),
			},
			existing: []billing.Allocation{alloc(5, p1, inv1, 700_000), alloc(6, p1, inv2, 200_000)},
			want:     []billing.Row{row(p1, inv2, -200_000), row(p1, i3, 150_000)},
		},
		{
			name:     "an invoice corrected down gives back from the newest allocation first",
			payments: []billing.Payment{pay(p1, 1, 2, 400_000), pay(p2, 2, 3, 600_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 500_000)},
			existing: []billing.Allocation{alloc(4, p1, inv1, 400_000), alloc(5, p2, inv1, 600_000)},
			want:     []billing.Row{row(p2, inv1, -500_000)},
		},
		{
			name:     "a payment corrected down after it paid two invoices gives back from the newest invoice first",
			payments: []billing.Payment{pay(p1, 1, 3, 300_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 500_000), bill(inv2, time.November, 2, 500_000)},
			existing: []billing.Allocation{alloc(4, p1, inv1, 500_000), alloc(5, p1, inv2, 500_000)},
			want:     []billing.Row{row(p1, inv2, -500_000), row(p1, inv1, -200_000)},
		},
		{
			name: "what a corrected payment gives back is paid again from another payment's credit, oldest first",
			payments: []billing.Payment{
				pay(p1, 1, 3, 300_000), pay(p2, 2, 6, 400_000),
			},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 500_000), bill(inv2, time.November, 2, 500_000)},
			existing: []billing.Allocation{alloc(4, p1, inv1, 500_000), alloc(5, p1, inv2, 500_000)},
			want: []billing.Row{
				row(p1, inv2, -500_000), row(p1, inv1, -200_000), row(p2, inv1, 200_000), row(p2, inv2, 200_000),
			},
		},
		{
			name:     "a payment corrected to zero gives everything back",
			payments: []billing.Payment{pay(p1, 1, 2, 0)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 500_000)},
			existing: []billing.Allocation{alloc(3, p1, inv1, 500_000)},
			want:     []billing.Row{row(p1, inv1, -500_000)},
		},
		{
			name:     "earlier reversals count: a pair already reversed once is not reversed again",
			payments: []billing.Payment{pay(p1, 1, 2, 500_000), pay(p3, 3, 9, 100)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 500_000)},
			existing: []billing.Allocation{
				alloc(3, p1, inv1, 500_000), alloc(4, p1, inv1, -500_000), alloc(5, p1, inv1, 500_000),
			},
		},
		{
			name:     "a fully settled tenant needs nothing",
			payments: []billing.Payment{pay(p1, 1, 2, 700_000)},
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 700_000)},
			existing: []billing.Allocation{alloc(3, p1, inv1, 700_000)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := billing.Plan(tc.payments, tc.invoices, tc.existing)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestPlanRejectsInconsistentInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		payments []billing.Payment
		invoices []billing.Invoice
		existing []billing.Allocation
	}{
		{
			name:     "an allocation of an unknown payment",
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 100)},
			existing: []billing.Allocation{alloc(3, p1, inv1, 100)},
		},
		{
			name:     "an allocation to an unknown invoice",
			payments: []billing.Payment{pay(p1, 1, 1, 100)},
			existing: []billing.Allocation{alloc(3, p1, inv1, 100)},
		},
		{
			name:     "a payment listed twice",
			payments: []billing.Payment{pay(p1, 1, 1, 100), pay(p1, 2, 2, 100)},
		},
		{
			name:     "an invoice listed twice",
			invoices: []billing.Invoice{bill(inv1, time.October, 1, 100), bill(inv1, time.November, 2, 100)},
		},
		{
			name:     "a negative net payment",
			payments: []billing.Payment{pay(p1, 1, 1, -1)},
		},
		{
			name:     "a negative billed invoice",
			invoices: []billing.Invoice{bill(inv1, time.October, 1, -1)},
		},
		{
			name:     "a pair whose allocations add up below zero",
			payments: []billing.Payment{pay(p1, 1, 1, 100)},
			invoices: []billing.Invoice{bill(inv1, time.October, 2, 100)},
			existing: []billing.Allocation{alloc(3, p1, inv1, -50)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := billing.Plan(tc.payments, tc.invoices, tc.existing)
			require.ErrorIs(t, err, billing.ErrInconsistent)
		})
	}
}
