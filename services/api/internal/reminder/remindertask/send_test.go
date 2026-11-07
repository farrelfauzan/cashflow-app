package remindertask_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
)

// sendQueue is a fake Queue that records what it is asked to enqueue, or fails with err.
type sendQueue struct {
	mu            sync.Mutex
	sends         []uuid.UUID
	confirmations []uuid.UUID
	err           error
}

func (q *sendQueue) EnqueueSend(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.sends = append(q.sends, id)
	return nil
}

func (q *sendQueue) EnqueueConfirm(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.err != nil {
		return q.err
	}
	q.confirmations = append(q.confirmations, id)
	return nil
}

// sender is a fake Sender: Pending answers from the maps, Send and Confirm record their calls.
type sender struct {
	reminders     map[uuid.UUID][]uuid.UUID
	confirmations map[uuid.UUID][]uuid.UUID
	pendingErr    map[uuid.UUID]error
	sendErr       error
	sent          []uuid.UUID
	confirmed     []uuid.UUID
}

func (s *sender) Send(_ context.Context, _, id uuid.UUID) (reminder.Outcome, error) {
	s.sent = append(s.sent, id)
	return reminder.OutcomeSent, s.sendErr
}

func (s *sender) Confirm(_ context.Context, _, id uuid.UUID) (reminder.Outcome, error) {
	s.confirmed = append(s.confirmed, id)
	return reminder.OutcomeSent, s.sendErr
}

func (s *sender) Pending(_ context.Context, org uuid.UUID) ([]uuid.UUID, []uuid.UUID, error) {
	return s.reminders[org], s.confirmations[org], s.pendingErr[org]
}

func TestSendTaskIDs(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000002")
	require.Equal(t, "reminder:send:0b6c3c1e-1111-4a4a-8b8b-000000000002", remindertask.SendTaskID(id))
	require.Equal(t, "promise:confirm:0b6c3c1e-1111-4a4a-8b8b-000000000002", remindertask.ConfirmTaskID(id))
	require.Equal(t, "*/15 8-21 * * *", remindertask.SweepCron)
}

func TestSweep(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	r1, r2, c1 := uuid.New(), uuid.New(), uuid.New()
	tests := []struct {
		name      string
		sender    *sender
		queueErr  error
		wantN     int
		wantSends []uuid.UUID
		wantConf  []uuid.UUID
		wantErr   bool
	}{
		{
			name:      "every organization's pending rows are enqueued again",
			sender:    &sender{reminders: map[uuid.UUID][]uuid.UUID{a: {r1}, b: {r2}}, confirmations: map[uuid.UUID][]uuid.UUID{b: {c1}}},
			wantN:     3,
			wantSends: []uuid.UUID{r1, r2}, wantConf: []uuid.UUID{c1},
		},
		{
			name:      "one organization failing does not stop the other",
			sender:    &sender{reminders: map[uuid.UUID][]uuid.UUID{b: {r2}}, pendingErr: map[uuid.UUID]error{a: errors.New("db down")}},
			wantN:     1,
			wantSends: []uuid.UUID{r2},
			wantErr:   true,
		},
		{
			name:     "an enqueue failing is returned",
			sender:   &sender{reminders: map[uuid.UUID][]uuid.UUID{a: {r1}}},
			queueErr: errors.New("redis down"),
			wantErr:  true,
		},
		{name: "nothing pending", sender: &sender{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &sendQueue{err: tc.queueErr}
			n, err := remindertask.Sweep(context.Background(), orgs{ids: []uuid.UUID{a, b}}, tc.sender, q)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantN, n)
			require.Equal(t, tc.wantSends, q.sends)
			require.Equal(t, tc.wantConf, q.confirmations)
		})
	}
}

func TestHandleSendAndConfirm(t *testing.T) {
	t.Parallel()
	org, id := uuid.New(), uuid.New()
	body, err := json.Marshal(map[string]uuid.UUID{"organizationId": org, "id": id})
	require.NoError(t, err)
	tests := []struct {
		name      string
		task      *asynq.Task
		sendErr   error
		wantSent  int
		wantErr   bool
		wantRetry bool
	}{
		{name: "a send", task: asynq.NewTask(remindertask.TypeSend, body), wantSent: 1},
		{name: "a confirmation", task: asynq.NewTask(remindertask.TypeConfirm, body), wantSent: 1},
		{name: "no organization is never retried", task: asynq.NewTask(remindertask.TypeSend, []byte(`{"id":"`+id.String()+`"}`)), wantErr: true},
		{name: "a missing reminder is never retried", task: asynq.NewTask(remindertask.TypeSend, body), sendErr: reminder.ErrReminderNotFound, wantSent: 1, wantErr: true},
		{name: "a missing confirmation is never retried", task: asynq.NewTask(remindertask.TypeConfirm, body), sendErr: reminder.ErrConfirmationNotFound, wantSent: 1, wantErr: true},
		{name: "a database error is retried", task: asynq.NewTask(remindertask.TypeSend, body), sendErr: errors.New("db down"), wantSent: 1, wantErr: true, wantRetry: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &sender{sendErr: tc.sendErr}
			w := remindertask.SendWorker{Sender: s, Log: discard()}
			handle := w.HandleSend
			if tc.task.Type() == remindertask.TypeConfirm {
				handle = w.HandleConfirm
			}
			err := handle(context.Background(), tc.task)
			require.Equal(t, tc.wantSent, len(s.sent)+len(s.confirmed))
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, !tc.wantRetry, errors.Is(err, asynq.SkipRetry))
		})
	}
}

// orgSweep is a fake extra sweep: it enqueues n rows per organization, or fails on errOrg.
type orgSweep struct {
	n      map[uuid.UUID]int
	errOrg uuid.UUID
	swept  []uuid.UUID
}

func (s *orgSweep) SweepOrg(_ context.Context, org uuid.UUID) (int, error) {
	s.swept = append(s.swept, org)
	if org == s.errOrg {
		return 0, errors.New("redis down")
	}
	return s.n[org], nil
}

func TestSweepAlsoSweepsOtherRows(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	tests := []struct {
		name    string
		also    *orgSweep
		wantN   int
		wantErr bool
	}{
		{name: "every organization's extra rows count", also: &orgSweep{n: map[uuid.UUID]int{a: 2, b: 1}}, wantN: 4},
		{name: "one organization failing does not stop the other", also: &orgSweep{n: map[uuid.UUID]int{b: 1}, errOrg: a}, wantN: 2, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := &sender{reminders: map[uuid.UUID][]uuid.UUID{a: {uuid.New()}}}
			q := &sendQueue{}
			n, err := remindertask.Sweep(context.Background(), orgs{ids: []uuid.UUID{a, b}}, s, q, tc.also)
			require.Equal(t, tc.wantErr, err != nil, "error: %v", err)
			require.Equal(t, tc.wantN, n)
			require.Equal(t, []uuid.UUID{a, b}, tc.also.swept)
		})
	}
}
