package messaging_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

func TestRupiah(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "zero", in: 0, want: "Rp0"},
		{name: "below a thousand", in: 950, want: "Rp950"},
		{name: "a thousand", in: 1_000, want: "Rp1.000"},
		{name: "rent", in: 1_250_000, want: "Rp1.250.000"},
		{name: "ten million", in: 10_000_000, want: "Rp10.000.000"},
		{name: "a billion", in: 1_500_000_000, want: "Rp1.500.000.000"},
		{name: "negative", in: -50_000, want: "-Rp50.000"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, messaging.Rupiah(tc.in))
		})
	}
}

func TestDateAndMonth(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		in        time.Time
		wantDate  string
		wantMonth string
	}{
		{name: "a date column", in: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC), wantDate: "20 November 2026", wantMonth: "November 2026"},
		{name: "january", in: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), wantDate: "1 Januari 2027", wantMonth: "Januari 2027"},
		{name: "may", in: time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC), wantDate: "31 Mei 2026", wantMonth: "Mei 2026"},
		{name: "december", in: time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC), wantDate: "15 Desember 2026", wantMonth: "Desember 2026"},
		{name: "fields are read as they are", in: time.Date(2026, 8, 17, 23, 30, 0, 0, time.FixedZone("WIB", 7*3600)),
			wantDate: "17 Agustus 2026", wantMonth: "Agustus 2026"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.wantDate, messaging.Date(tc.in))
			require.Equal(t, tc.wantMonth, messaging.Month(tc.in))
		})
	}
}
