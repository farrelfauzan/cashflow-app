package billing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

func TestInvoiceLines(t *testing.T) {
	t.Parallel()
	listrik, wifi := uuid.New(), uuid.New()
	charges := []billing.FixedCharge{
		{ChargeTypeID: listrik, Name: "Listrik", Monthly: 150_000},
		{ChargeTypeID: wifi, Name: "Wi-Fi", Monthly: 100_000},
	}
	tests := []struct {
		name    string
		period  tenant.Period
		term    int
		rent    int64
		charges []billing.FixedCharge
		want    []billing.Line
	}{
		{
			name:   "a monthly lease without extras has one rent line naming its period",
			period: tenant.Period{From: d(2026, 10, 15), To: d(2026, 11, 14)},
			term:   1, rent: 700_000,
			want: []billing.Line{{Kind: billing.LineRent, Label: "Sewa 15 Okt 2026 – 14 Nov 2026", Amount: 700_000}},
		},
		{
			name:   "a monthly lease bills each extra once",
			period: tenant.Period{From: d(2026, 10, 15), To: d(2026, 11, 14)},
			term:   1, rent: 700_000, charges: charges,
			want: []billing.Line{
				{Kind: billing.LineRent, Label: "Sewa 15 Okt 2026 – 14 Nov 2026", Amount: 700_000},
				{Kind: billing.LineCharge, ChargeTypeID: listrik, Label: "Listrik (1 bulan)", Amount: 150_000},
				{Kind: billing.LineCharge, ChargeTypeID: wifi, Label: "Wi-Fi (1 bulan)", Amount: 100_000},
			},
		},
		{
			name:   "a 3-month lease takes the term's price and three months of each extra",
			period: tenant.Period{From: d(2026, 10, 15), To: d(2027, 1, 14)},
			term:   3, rent: 2_000_000, charges: charges,
			want: []billing.Line{
				{Kind: billing.LineRent, Label: "Sewa 15 Okt 2026 – 14 Jan 2027", Amount: 2_000_000},
				{Kind: billing.LineCharge, ChargeTypeID: listrik, Label: "Listrik (3 bulan)", Amount: 450_000},
				{Kind: billing.LineCharge, ChargeTypeID: wifi, Label: "Wi-Fi (3 bulan)", Amount: 300_000},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, billing.InvoiceLines(tc.period, tc.term, tc.rent, tc.charges))
		})
	}
}

func TestFormatDayUsesIndonesianMonths(t *testing.T) {
	t.Parallel()
	want := []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	for i, m := range want {
		require.Equal(t, "1 "+m+" 2027", billing.FormatDay(d(2027, time.Month(i+1), 1)))
	}
	require.Equal(t, "31 Des 2026", billing.FormatDay(d(2026, 12, 31)))
}
