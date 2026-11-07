package reminder

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func TestInvoiceMessage(t *testing.T) {
	t.Parallel()
	rina := Recipient{Name: "Rina Wulandari", Phone: "+6281297150001", Organization: "Griya Sejahtera"}
	nov, dec, jan := uuid.New(), uuid.New(), uuid.New()
	decInv := OwingInvoice{ID: dec, Period: date(2026, 12, 10), DueDate: date(2026, 12, 10), Billed: 1_620_000, Outstanding: 1_620_000}
	decLines := map[uuid.UUID][]Line{dec: {{Label: "Sewa Kamar 3", Amount: 1_500_000}, {Label: "Listrik November", Amount: 120_000}}}
	base := map[string]string{"name": "Rina Wulandari", "organization": "Griya Sejahtera"}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	tests := []struct {
		name         string
		stages       []InvoiceStage
		owing        []OwingInvoice
		lines        map[uuid.UUID][]Line
		today        time.Time
		wantTemplate string
		wantParams   map[string]string
	}{
		{
			name: "H-3 of the only invoice", stages: []InvoiceStage{{dec, StageHMinus3}}, owing: []OwingInvoice{decInv},
			lines: decLines, today: date(2026, 12, 7), wantTemplate: messaging.TemplateInvoiceDueSoon,
			wantParams: with("month", "Desember 2026", "amount", "Rp1.620.000",
				"lines", "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000", "due_date", "10 Desember 2026"),
		},
		{
			name: "H0 of the only invoice", stages: []InvoiceStage{{dec, StageH0}}, owing: []OwingInvoice{decInv},
			lines: decLines, today: date(2026, 12, 10), wantTemplate: messaging.TemplateInvoiceDueToday,
			wantParams: with("month", "Desember 2026", "amount", "Rp1.620.000",
				"lines", "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000"),
		},
		{
			name: "H+3 of a partly paid, corrected invoice, sent a day late in its window", stages: []InvoiceStage{{dec, StageHPlus3}},
			owing: []OwingInvoice{{ID: dec, Period: date(2026, 12, 10), DueDate: date(2026, 12, 10), Billed: 1_570_000, Paid: 1_000_000, Outstanding: 570_000}},
			lines: decLines, today: date(2026, 12, 14), wantTemplate: messaging.TemplateInvoiceOverdue,
			wantParams: with("month", "Desember 2026", "amount", "Rp570.000",
				"lines", "Sewa Kamar 3 Rp1.500.000, Listrik November Rp120.000, koreksi -Rp50.000, sudah dibayar Rp1.000.000",
				"days_late", "4"),
		},
		{
			name:   "rollover: this month's H-3 and November's rest",
			stages: []InvoiceStage{{dec, StageHMinus3}},
			owing: []OwingInvoice{
				{ID: nov, Period: date(2026, 11, 10), DueDate: date(2026, 11, 10), Billed: 700_000, Paid: 500_000, Outstanding: 200_000},
				{ID: dec, Period: date(2026, 12, 10), DueDate: date(2026, 12, 10), Billed: 700_000, Outstanding: 700_000},
			},
			today: date(2026, 12, 7), wantTemplate: messaging.TemplateInvoiceRollover,
			wantParams: with("summary", "Tagihan Desember 2026 Rp700.000 + sisa November 2026 Rp200.000", "total", "Rp900.000"),
		},
		{
			name:   "rollover: two rooms due the same month are one term",
			stages: []InvoiceStage{{dec, StageH0}, {jan, StageH0}},
			owing: []OwingInvoice{
				{ID: dec, Period: date(2026, 12, 10), DueDate: date(2026, 12, 10), Billed: 700_000, Outstanding: 700_000},
				{ID: jan, Period: date(2026, 12, 10), DueDate: date(2026, 12, 10), Billed: 900_000, Outstanding: 900_000},
			},
			today: date(2026, 12, 10), wantTemplate: messaging.TemplateInvoiceRollover,
			wantParams: with("summary", "Tagihan Desember 2026 Rp1.600.000", "total", "Rp1.600.000"),
		},
		{
			name:   "rollover: an unpaid older invoice not due yet stays Tagihan, an overdue one is sisa",
			stages: []InvoiceStage{{jan, StageHMinus3}},
			owing: []OwingInvoice{
				{ID: nov, Period: date(2026, 11, 10), DueDate: date(2026, 11, 10), Billed: 700_000, Outstanding: 700_000},
				{ID: dec, Period: date(2026, 12, 10), DueDate: date(2027, 1, 12), Billed: 300_000, Outstanding: 300_000},
				{ID: jan, Period: date(2027, 1, 10), DueDate: date(2027, 1, 10), Billed: 700_000, Outstanding: 700_000},
			},
			today: date(2027, 1, 7), wantTemplate: messaging.TemplateInvoiceRollover,
			wantParams: with("summary", "Tagihan Januari 2027 Rp700.000 + sisa November 2026 Rp700.000 + Tagihan Desember 2026 Rp300.000",
				"total", "Rp1.700.000"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tpl, params := invoiceMessage(rina, tc.stages, tc.owing, tc.lines, tc.today)
			require.Equal(t, tc.wantTemplate, tpl)
			require.Equal(t, tc.wantParams, params)
			_, err := messaging.Render(messaging.Message{Template: tpl, Params: params,
				Buttons: []messaging.Button{{Title: messaging.ButtonPay, URL: "https://app.test/t/x"}}})
			require.NoError(t, err, "the params fit the template")
		})
	}
}

func TestPromisedAmount(t *testing.T) {
	t.Parallel()
	amount := int64(500_000)
	require.Equal(t, "Rp500.000", promisedAmount(&amount))
	require.Equal(t, "seluruh sisa tagihan", promisedAmount(nil))
}
