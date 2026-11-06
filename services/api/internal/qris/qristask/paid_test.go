package qristask_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris/qristask"
)

// settlement is one SettlePaid call.
type settlement struct {
	org    uuid.UUID
	txID   string
	amount int64
}

// settler is a fake Settler that records its calls and answers err.
type settler struct {
	calls []settlement
	err   error
}

func (s *settler) SettlePaid(_ context.Context, org uuid.UUID, transactionID string, paidAmount int64) error {
	s.calls = append(s.calls, settlement{org: org, txID: transactionID, amount: paidAmount})
	return s.err
}

func TestPaidTaskID(t *testing.T) {
	t.Parallel()
	orgA := uuid.MustParse("00000000-0000-4000-8000-00000000000a")
	orgB := uuid.MustParse("00000000-0000-4000-8000-00000000000b")
	tests := []struct {
		name string
		org  uuid.UUID
		txID string
		want string
	}{
		{"a uuid transaction id", orgA, "00000000-0000-4000-8000-0000000000a1",
			"qris:paid:00000000-0000-4000-8000-00000000000a:00000000-0000-4000-8000-0000000000a1"},
		{"another id gives another task id", orgA, "00000000-0000-4000-8000-0000000000a2",
			"qris:paid:00000000-0000-4000-8000-00000000000a:00000000-0000-4000-8000-0000000000a2"},
		{"another organization naming the same transaction gets its own task id", orgB, "00000000-0000-4000-8000-0000000000a1",
			"qris:paid:00000000-0000-4000-8000-00000000000b:00000000-0000-4000-8000-0000000000a1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, qristask.PaidTaskID(tc.org, tc.txID))
		})
	}
}

func paidPayload(t *testing.T, org uuid.UUID, txID string, amount int64) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"organizationId": org, "transactionId": txID, "amount": amount})
	require.NoError(t, err)
	return b
}

func TestHandlePaid(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	const txID = "00000000-0000-4000-8000-0000000000a1"

	t.Run("settles the transaction in the payload", func(t *testing.T) {
		t.Parallel()
		s := &settler{}
		w := qristask.Worker{Settler: s, Log: discard()}
		require.NoError(t, w.HandlePaid(context.Background(), asynq.NewTask(qristask.TypePaid, paidPayload(t, org, txID, 700_000))))
		require.Equal(t, []settlement{{org: org, txID: txID, amount: 700_000}}, s.calls)
	})

	t.Run("a transaction with no attempt is never retried", func(t *testing.T) {
		t.Parallel()
		s := &settler{err: fmt.Errorf("wrapped: %w", qris.ErrUnknownTransaction)}
		w := qristask.Worker{Settler: s, Log: discard()}
		err := w.HandlePaid(context.Background(), asynq.NewTask(qristask.TypePaid, paidPayload(t, org, txID, 700_000)))
		require.ErrorIs(t, err, asynq.SkipRetry)
		require.ErrorIs(t, err, qris.ErrUnknownTransaction)
		require.Len(t, s.calls, 1)
	})

	t.Run("another failure is returned to be retried", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("database down")
		s := &settler{err: cause}
		w := qristask.Worker{Settler: s, Log: discard()}
		err := w.HandlePaid(context.Background(), asynq.NewTask(qristask.TypePaid, paidPayload(t, org, txID, 700_000)))
		require.ErrorIs(t, err, cause)
		require.NotErrorIs(t, err, asynq.SkipRetry)
		require.Len(t, s.calls, 1)
	})

	tests := []struct {
		name string
		body []byte
	}{
		{"no payload", nil},
		{"not JSON", []byte(`nope`)},
		{"an empty object", []byte(`{}`)},
		{"no organization", paidPayload(t, uuid.Nil, txID, 700_000)},
		{"an organization that is not a uuid", []byte(`{"organizationId":"griya-sejahtera","transactionId":"tx","amount":1}`)},
		{"no transaction id", paidPayload(t, org, "", 700_000)},
		{"amount 0", paidPayload(t, org, txID, 0)},
		{"a negative amount", paidPayload(t, org, txID, -700_000)},
		{"a fractional amount", []byte(`{"organizationId":"` + org.String() + `","transactionId":"tx","amount":1.5}`)},
	}
	for _, tc := range tests {
		t.Run("a payload with "+tc.name+" is never retried", func(t *testing.T) {
			t.Parallel()
			s := &settler{}
			w := qristask.Worker{Settler: s, Log: discard()}
			err := w.HandlePaid(context.Background(), asynq.NewTask(qristask.TypePaid, tc.body))
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, s.calls)
		})
	}
}
