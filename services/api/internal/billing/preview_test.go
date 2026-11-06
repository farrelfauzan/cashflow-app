package billing_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

func TestPreviewPayment(t *testing.T) {
	t.Parallel()
	oct, nov, dec := uuid.New(), uuid.New(), uuid.New()
	octNov := []billing.OpenBalance{{InvoiceID: oct, Balance: 750_000}, {InvoiceID: nov, Balance: 1_250_000}}

	tests := []struct {
		name    string
		open    []billing.OpenBalance
		amount  int64
		want    billing.Preview
		wantErr error
	}{
		{
			name: "exact balance of the oldest invoice pays it in full and stops", open: octNov, amount: 750_000,
			want: billing.Preview{Amount: 750_000, OutstandingBefore: 2_000_000, OutstandingAfter: 1_250_000, Items: []billing.PreviewItem{
				{InvoiceID: oct, Balance: 750_000, Applied: 750_000, BalanceAfter: 0, PaidInFull: true},
			}},
		},
		{
			name: "amount spanning two invoices pays the first in full and the second in part", open: octNov, amount: 1_000_000,
			want: billing.Preview{Amount: 1_000_000, OutstandingBefore: 2_000_000, OutstandingAfter: 1_000_000, Items: []billing.PreviewItem{
				{InvoiceID: oct, Balance: 750_000, Applied: 750_000, BalanceAfter: 0, PaidInFull: true},
				{InvoiceID: nov, Balance: 1_250_000, Applied: 250_000, BalanceAfter: 1_000_000, PaidInFull: false},
			}},
		},
		{
			name: "partial on the oldest invoice only", open: octNov, amount: 300_000,
			want: billing.Preview{Amount: 300_000, OutstandingBefore: 2_000_000, OutstandingAfter: 1_700_000, Items: []billing.PreviewItem{
				{InvoiceID: oct, Balance: 750_000, Applied: 300_000, BalanceAfter: 450_000, PaidInFull: false},
			}},
		},
		{
			name: "amount equal to everything owed pays every invoice in full", open: octNov, amount: 2_000_000,
			want: billing.Preview{Amount: 2_000_000, OutstandingBefore: 2_000_000, OutstandingAfter: 0, Items: []billing.PreviewItem{
				{InvoiceID: oct, Balance: 750_000, Applied: 750_000, BalanceAfter: 0, PaidInFull: true},
				{InvoiceID: nov, Balance: 1_250_000, Applied: 1_250_000, BalanceAfter: 0, PaidInFull: true},
			}},
		},
		{
			name:   "invoices with nothing owed are skipped and not counted",
			open:   []billing.OpenBalance{{InvoiceID: oct, Balance: 0}, {InvoiceID: nov, Balance: 500_000}, {InvoiceID: uuid.New(), Balance: 0}, {InvoiceID: dec, Balance: 400_000}},
			amount: 600_000,
			want: billing.Preview{Amount: 600_000, OutstandingBefore: 900_000, OutstandingAfter: 300_000, Items: []billing.PreviewItem{
				{InvoiceID: nov, Balance: 500_000, Applied: 500_000, BalanceAfter: 0, PaidInFull: true},
				{InvoiceID: dec, Balance: 400_000, Applied: 100_000, BalanceAfter: 300_000, PaidInFull: false},
			}},
		},
		{
			name: "order given is the order paid", open: []billing.OpenBalance{{InvoiceID: nov, Balance: 1_250_000}, {InvoiceID: oct, Balance: 750_000}}, amount: 100,
			want: billing.Preview{Amount: 100, OutstandingBefore: 2_000_000, OutstandingAfter: 1_999_900, Items: []billing.PreviewItem{
				{InvoiceID: nov, Balance: 1_250_000, Applied: 100, BalanceAfter: 1_249_900, PaidInFull: false},
			}},
		},
		{name: "amount above everything owed is refused", open: octNov, amount: 2_000_001, wantErr: billing.ErrAmountAboveOutstanding},
		{name: "any amount is above outstanding when nothing is owed", open: []billing.OpenBalance{{InvoiceID: oct, Balance: 0}}, amount: 1, wantErr: billing.ErrAmountAboveOutstanding},
		{name: "no invoices at all", open: nil, amount: 1, wantErr: billing.ErrAmountAboveOutstanding},
		{name: "zero amount is refused", open: octNov, amount: 0, wantErr: billing.ErrPreviewAmountInvalid},
		{name: "negative amount is refused", open: octNov, amount: -5, wantErr: billing.ErrPreviewAmountInvalid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := billing.PreviewPayment(tc.open, tc.amount)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			var applied int64
			for _, it := range got.Items {
				applied += it.Applied
			}
			require.Equal(t, tc.amount, applied, "the items spend exactly the amount")
		})
	}
}
