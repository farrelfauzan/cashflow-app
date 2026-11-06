package promisetask_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise/promisetask"
)

// orgs is a fake OrgLister.
type orgs struct {
	ids []uuid.UUID
	err error
}

func (o orgs) ActiveOrganizationIDs(context.Context) ([]uuid.UUID, error) { return o.ids, o.err }

// enqueuer is a fake Enqueuer that records the tasks it is asked to enqueue, or fails with err.
type enqueuer struct {
	mu    sync.Mutex
	tasks []promisetask.Task
	err   error
}

func (e *enqueuer) Enqueue(_ context.Context, t promisetask.Task) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	e.tasks = append(e.tasks, t)
	return nil
}

// closer is a fake Closer that records the organizations it ran for.
type closer struct {
	ran []uuid.UUID
	err error
}

func (c *closer) CloseDue(_ context.Context, org uuid.UUID) (promise.CloseResult, error) {
	c.ran = append(c.ran, org)
	return promise.CloseResult{Due: 3, Kept: 1, Broken: 2, VisitTasks: 1}, c.err
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestTaskID(t *testing.T) {
	t.Parallel()
	org := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000001")
	require.Equal(t, "promises:close:0b6c3c1e-1111-4a4a-8b8b-000000000001:2026-11-24",
		promisetask.TaskID(org, time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)))
}

func TestDailyCronIsHalfPastMidnight(t *testing.T) {
	t.Parallel()
	require.Equal(t, "30 0 * * *", promisetask.DailyCron)
}

func TestEnqueueDaily(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	day := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		orgs      orgs
		failWith  error
		want      []promisetask.Task
		wantN     int
		wantError bool
	}{
		{
			name:  "one task per organization, named after the organization and the day",
			orgs:  orgs{ids: []uuid.UUID{a, b}},
			want:  []promisetask.Task{{ID: promisetask.TaskID(a, day), OrganizationID: a}, {ID: promisetask.TaskID(b, day), OrganizationID: b}},
			wantN: 2,
		},
		{
			name: "no organization, no task",
			orgs: orgs{},
		},
		{
			name:      "a failing organization list enqueues nothing",
			orgs:      orgs{err: errors.New("database down")},
			wantError: true,
		},
		{
			name:      "a failing enqueue is returned",
			orgs:      orgs{ids: []uuid.UUID{a, b}},
			failWith:  errors.New("redis down"),
			wantError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enq := &enqueuer{err: tc.failWith}
			n, err := promisetask.EnqueueDaily(context.Background(), tc.orgs, enq, day)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantN, n)
			require.Equal(t, tc.want, enq.tasks)
		})
	}
}

func TestHandleClose(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	payload, err := json.Marshal(map[string]string{"organizationId": org.String()})
	require.NoError(t, err)

	t.Run("closes the organization in the payload", func(t *testing.T) {
		t.Parallel()
		c := &closer{}
		w := promisetask.Worker{Closer: c, Log: discard()}
		require.NoError(t, w.HandleClose(context.Background(), asynq.NewTask(promisetask.TypeClose, payload)))
		require.Equal(t, []uuid.UUID{org}, c.ran)
	})

	t.Run("a failed run is returned so the queue retries it", func(t *testing.T) {
		t.Parallel()
		c := &closer{err: errors.New("one tenant failed")}
		w := promisetask.Worker{Closer: c, Log: discard()}
		err := w.HandleClose(context.Background(), asynq.NewTask(promisetask.TypeClose, payload))
		require.Error(t, err)
		require.NotErrorIs(t, err, asynq.SkipRetry)
	})

	for name, body := range map[string][]byte{
		"no organization": []byte(`{}`),
		"not json":        []byte(`nope`),
	} {
		t.Run("a payload with "+name+" is never retried", func(t *testing.T) {
			t.Parallel()
			c := &closer{}
			w := promisetask.Worker{Closer: c, Log: discard()}
			err := w.HandleClose(context.Background(), asynq.NewTask(promisetask.TypeClose, body))
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, c.ran)
		})
	}
}

func TestHandleDailyUsesTheDayInJakarta(t *testing.T) {
	t.Parallel()
	a := uuid.New()
	enq := &enqueuer{}
	// 23 Nov 17:30 UTC is already 24 Nov 00:30 in Jakarta, when the cron fires.
	w := promisetask.Worker{
		Orgs: orgs{ids: []uuid.UUID{a}}, Enqueuer: enq, Log: discard(),
		Clock: clock.NewFake(time.Date(2026, 11, 23, 17, 30, 0, 0, time.UTC)),
	}
	require.NoError(t, w.HandleDaily(context.Background(), asynq.NewTask(promisetask.TypeDaily, nil)))
	require.Equal(t, []promisetask.Task{{ID: "promises:close:" + a.String() + ":2026-11-24", OrganizationID: a}}, enq.tasks)
}

func TestRegisterSchedulesTheDailyRun(t *testing.T) {
	t.Parallel()
	mux := asynq.NewServeMux()
	// The scheduler only records the entry until it starts; nothing connects to Redis here.
	sched := asynq.NewScheduler(asynq.RedisClientOpt{Addr: "127.0.0.1:0"}, &asynq.SchedulerOpts{Location: clock.Jakarta})
	w := promisetask.Worker{Closer: &closer{}, Log: discard()}
	require.NoError(t, w.Register(mux, sched))
	for _, typ := range []string{promisetask.TypeClose, promisetask.TypeDaily} {
		h, pattern := mux.Handler(asynq.NewTask(typ, nil))
		require.Equal(t, typ, pattern)
		require.NotNil(t, h)
	}
}
