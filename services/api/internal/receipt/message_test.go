package receipt_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/receipt"
)

func TestParams(t *testing.T) {
	t.Parallel()
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	paid := func(kind string, period time.Time, amount, outstanding int64) billing.PaidInvoice {
		return billing.PaidInvoice{InvoiceID: uuid.New(), Kind: kind, Period: period, Amount: amount, Outstanding: outstanding}
	}
	payment := func(net int64) billing.AccountPayment {
		return billing.AccountPayment{ID: uuid.New(), Amount: net, Net: net, Method: billing.MethodCash, ReceivedOn: day(11, 14)}
	}
	tests := []struct {
		name    string
		res     billing.PaymentResult
		paid    string
		balance string
		amount  string
	}{
		{
			name: "a part payment of one invoice",
			res: billing.PaymentResult{Payment: payment(300_000), Paid: []billing.PaidInvoice{paid(billing.InvoiceRegular, day(11, 15), 300_000, 400_000)},
				Totals: billing.Totals{Outstanding: 400_000}},
			paid: "Tagihan November 2026 Rp300.000, sisa Rp400.000", balance: "Sisa tagihan Anda sekarang Rp400.000.", amount: "Rp300.000",
		},
		{
			name: "two invoices, oldest first, the last one partly",
			res: billing.PaymentResult{Payment: payment(1_000_000), Paid: []billing.PaidInvoice{
				paid(billing.InvoiceRegular, day(10, 15), 700_000, 0), paid(billing.InvoiceRegular, day(11, 15), 300_000, 400_000),
			}, Totals: billing.Totals{Outstanding: 400_000}},
			paid:    "Tagihan Oktober 2026 Rp700.000 (lunas); Tagihan November 2026 Rp300.000, sisa Rp400.000",
			balance: "Sisa tagihan Anda sekarang Rp400.000.", amount: "Rp1.000.000",
		},
		{
			name: "an overpayment pays everything and leaves credit",
			res: billing.PaymentResult{Payment: payment(750_000), Paid: []billing.PaidInvoice{paid(billing.InvoiceRegular, day(11, 15), 700_000, 0)},
				Totals: billing.Totals{Credit: 50_000}},
			paid:    "Tagihan November 2026 Rp700.000 (lunas); saldo Rp50.000",
			balance: "Semua tagihan sudah lunas. Saldo Anda Rp50.000 dipakai untuk tagihan berikutnya.", amount: "Rp750.000",
		},
		{
			name:    "all of it credit when nothing was owed",
			res:     billing.PaymentResult{Payment: payment(200_000), Paid: []billing.PaidInvoice{}, Totals: billing.Totals{Credit: 200_000}},
			paid:    "disimpan sebagai saldo Rp200.000 untuk tagihan berikutnya",
			balance: "Semua tagihan sudah lunas. Saldo Anda Rp200.000 dipakai untuk tagihan berikutnya.", amount: "Rp200.000",
		},
		{
			name: "paid exactly, nothing left",
			res: billing.PaymentResult{Payment: payment(700_000), Paid: []billing.PaidInvoice{paid(billing.InvoiceFinal, day(11, 1), 700_000, 0)},
				Totals: billing.Totals{}},
			paid: "Tagihan akhir Rp700.000 (lunas)", balance: "Semua tagihan Anda sudah lunas.", amount: "Rp700.000",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			params := receipt.Params("Griya Sejahtera", "Wati Susanti", tc.res)
			require.Equal(t, map[string]string{
				"organization": "Griya Sejahtera", "name": "Wati Susanti", "amount": tc.amount, "date": "14 November 2026",
				"paid": tc.paid, "balance": tc.balance,
			}, params)
			text, err := messaging.Render(messaging.Message{Template: messaging.TemplatePaymentReceipt, Params: params})
			require.NoError(t, err)
			require.Contains(t, text, "*Griya Sejahtera*", "the receipt names the organization")
			require.Contains(t, text, "*"+tc.amount+"*")
			for _, v := range params {
				require.NotContains(t, v, "\n", "every parameter is one line")
			}
		})
	}
}
