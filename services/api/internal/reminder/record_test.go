package reminder

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestRecordRefusesAnEmptyReminder: a decision with nothing to tell is never written; record
// returns before touching the database (q is nil here).
func TestRecordRefusesAnEmptyReminder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dec  Decision
	}{
		{name: "nothing at all", dec: Decision{}},
		{name: "only a visit", dec: Decision{OpenH7Visit: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			now := time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC)
			_, err := record(context.Background(), nil, uuid.New(), uuid.New(), now, now, tc.dec)
			require.ErrorIs(t, err, ErrNothingToRecord)
		})
	}
}
