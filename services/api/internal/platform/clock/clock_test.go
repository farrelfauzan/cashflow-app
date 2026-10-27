package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartOfDay(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "afternoon stays on the same Jakarta day",
			in:   time.Date(2026, 11, 1, 15, 30, 0, 0, Jakarta),
			want: time.Date(2026, 11, 1, 0, 0, 0, 0, Jakarta),
		},
		{
			name: "late evening UTC is already the next day in Jakarta",
			in:   time.Date(2026, 10, 31, 17, 30, 0, 0, time.UTC),
			want: time.Date(2026, 11, 1, 0, 0, 0, 0, Jakarta),
		},
		{
			name: "midnight Jakarta is its own start of day",
			in:   time.Date(2026, 11, 1, 0, 0, 0, 0, Jakarta),
			want: time.Date(2026, 11, 1, 0, 0, 0, 0, Jakarta),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, tc.want.Equal(StartOfDay(tc.in)), "got %s", StartOfDay(tc.in))
		})
	}
}

func TestFakeMovesOnlyWhenTold(t *testing.T) {
	start := time.Date(2026, 11, 1, 8, 0, 0, 0, Jakarta)
	f := NewFake(start)
	require.True(t, start.Equal(f.Now()))

	f.Advance(72 * time.Hour)
	require.True(t, start.Add(72*time.Hour).Equal(f.Now()))

	f.Set(start)
	require.True(t, start.Equal(f.Now()))
}

func TestSystemClockIsCurrent(t *testing.T) {
	require.WithinDuration(t, time.Now(), System().Now(), time.Second)
}
