package billing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

func i64p(n int64) *int64 { return &n }

func TestMeteredLine(t *testing.T) {
	t.Parallel()
	id, listrik := uuid.New(), uuid.New()
	tests := []struct {
		name  string
		month time.Time
		label string
	}{
		{"October", d(2026, 10, 1), "Listrik Oktober 2026"},
		{"January", d(2027, 1, 1), "Listrik Januari 2027"},
		{"August", d(2026, 8, 1), "Listrik Agustus 2026"},
		{"December", d(2026, 12, 1), "Listrik Desember 2026"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := billing.MeteredLine(billing.MeteredAmount{ID: id, ChargeTypeID: listrik, Name: "Listrik", UsageMonth: tc.month, Amount: 123_000})
			require.Equal(t, billing.Line{
				Kind: billing.LineCharge, ChargeTypeID: listrik, MeteredChargeID: id, Label: tc.label, Amount: 123_000,
			}, got)
		})
	}
}

func TestFormatMonthCoversEveryMonth(t *testing.T) {
	t.Parallel()
	want := []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	for m := time.January; m <= time.December; m++ {
		require.Equal(t, want[m-1]+" 2026", billing.FormatMonth(d(2026, m, 1)))
	}
}

func TestValidMetered(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		in    billing.MeteredInput
		field string
	}{
		{"an amount alone", billing.MeteredInput{Amount: 123_000}, ""},
		{"with both meter numbers", billing.MeteredInput{Amount: 123_000, MeterStart: i64p(1200), MeterEnd: i64p(1350)}, ""},
		{"the same meter number twice", billing.MeteredInput{Amount: 1, MeterStart: i64p(0), MeterEnd: i64p(0)}, ""},
		{"only the end number", billing.MeteredInput{Amount: 1, MeterEnd: i64p(5)}, ""},
		{"the largest amount", billing.MeteredInput{Amount: billing.MaxAmount}, ""},
		{"zero", billing.MeteredInput{Amount: 0}, "amount"},
		{"negative", billing.MeteredInput{Amount: -1}, "amount"},
		{"above the largest amount", billing.MeteredInput{Amount: billing.MaxAmount + 1}, "amount"},
		{"a negative start", billing.MeteredInput{Amount: 1, MeterStart: i64p(-1)}, "meterStart"},
		{"a negative end", billing.MeteredInput{Amount: 1, MeterEnd: i64p(-1)}, "meterEnd"},
		{"the end before the start", billing.MeteredInput{Amount: 1, MeterStart: i64p(1350), MeterEnd: i64p(1200)}, "meterEnd"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requireInputField(t, billing.ValidMetered(tc.in), tc.field)
		})
	}
}

func TestCheckUsageMonth(t *testing.T) {
	t.Parallel()
	start, today := d(2026, 9, 15), d(2026, 11, 3)
	tests := []struct {
		name  string
		month time.Time
		field string
	}{
		{"the month the lease started", d(2026, 9, 1), ""},
		{"last month", d(2026, 10, 1), ""},
		{"this month", d(2026, 11, 1), ""},
		{"before the lease started", d(2026, 8, 1), "usageMonth"},
		{"next month", d(2026, 12, 1), "usageMonth"},
		{"not a first day", d(2026, 10, 2), "usageMonth"},
		{"no month", time.Time{}, "usageMonth"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requireInputField(t, billing.CheckUsageMonth(tc.month, start, today), tc.field)
		})
	}
}

func TestLastMonth(t *testing.T) {
	t.Parallel()
	require.Equal(t, d(2026, 10, 1), billing.LastMonth(d(2026, 11, 3)))
	require.Equal(t, d(2025, 12, 1), billing.LastMonth(d(2026, 1, 31)))
}

// requireInputField checks that err is nil when field is empty, else a *billing.InputError on
// field.
func requireInputField(t *testing.T, err error, field string) {
	t.Helper()
	if field == "" {
		require.NoError(t, err)
		return
	}
	var in *billing.InputError
	require.ErrorAs(t, err, &in)
	require.Equal(t, field, in.Field)
	require.NotEmpty(t, in.Message)
}
