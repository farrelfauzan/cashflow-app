package noticetask_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice/noticetask"
)

// sender is a fake staffnotice.Sender.
type sender struct {
	sent       []uuid.UUID
	sendErr    error
	pending    map[uuid.UUID][]uuid.UUID
	pendingErr error
}

func (s *sender) Send(_ context.Context, _, id uuid.UUID) (staffnotice.Outcome, error) {
	s.sent = append(s.sent, id)
	return staffnotice.OutcomeSent, s.sendErr
}

func (s *sender) Pending(_ context.Context, org uuid.UUID) ([]uuid.UUID, error) {
	return s.pending[org], s.pendingErr
}

// queue is a fake staffnotice.Queue.
type queue struct {
	enqueued []uuid.UUID
	err      error
}

func (q *queue) EnqueueNotice(_ context.Context, _, id uuid.UUID) error {
	if q.err != nil {
		return q.err
	}
	q.enqueued = append(q.enqueued, id)
	return nil
}

func TestTaskID(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000003")
	require.Equal(t, "staff:notice:0b6c3c1e-1111-4a4a-8b8b-000000000003", noticetask.TaskID(id))
}

func TestHandleNotice(t *testing.T) {
	t.Parallel()
	org, id := uuid.New(), uuid.New()
	body, err := json.Marshal(map[string]uuid.UUID{"organizationId": org, "id": id})
	require.NoError(t, err)
	tests := []struct {
		name      string
		body      []byte
		sendErr   error
		wantSent  int
		wantErr   bool
		wantRetry bool
	}{
		{name: "a notice", body: body, wantSent: 1},
		{name: "no organization is never retried", body: []byte(`{"id":"` + id.String() + `"}`), wantErr: true},
		{name: "a malformed payload is never retried", body: []byte(`{`), wantErr: true},
		{name: "a missing notice is never retried", body: body, sendErr: staffnotice.ErrNoticeNotFound, wantSent: 1, wantErr: true},
		{name: "a database error is retried", body: body, sendErr: errors.New("db down"), wantSent: 1, wantErr: true, wantRetry: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &sender{sendErr: tc.sendErr}
			w := noticetask.Worker{Sender: s, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			err := w.HandleNotice(context.Background(), asynq.NewTask(noticetask.TypeNotice, tc.body))
			require.Len(t, s.sent, tc.wantSent)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, !tc.wantRetry, errors.Is(err, asynq.SkipRetry))
		})
	}
}

func TestSweeper(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	n1, n2 := uuid.New(), uuid.New()
	tests := []struct {
		name     string
		sender   *sender
		queueErr error
		wantN    int
		want     []uuid.UUID
		wantErr  bool
	}{
		{name: "pending notices are enqueued again", sender: &sender{pending: map[uuid.UUID][]uuid.UUID{org: {n1, n2}}}, wantN: 2, want: []uuid.UUID{n1, n2}},
		{name: "nothing pending", sender: &sender{}},
		{name: "a failed listing is returned", sender: &sender{pendingErr: errors.New("db down")}, wantErr: true},
		{name: "a failed enqueue is returned", sender: &sender{pending: map[uuid.UUID][]uuid.UUID{org: {n1}}}, queueErr: errors.New("redis down"), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &queue{err: tc.queueErr}
			n, err := noticetask.Sweeper{Sender: tc.sender, Queue: q}.SweepOrg(context.Background(), org)
			require.Equal(t, tc.wantErr, err != nil, "error: %v", err)
			require.Equal(t, tc.wantN, n)
			require.Equal(t, tc.want, q.enqueued)
		})
	}
}
