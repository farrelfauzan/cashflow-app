package remindertask_test

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
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
	tasks []remindertask.Task
	err   error
}

func (e *enqueuer) Enqueue(_ context.Context, t remindertask.Task) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	e.tasks = append(e.tasks, t)
	return nil
}

// decider is a fake Decider that records the organizations it ran for.
type decider struct {
	ran []uuid.UUID
	err error
}

func (c *decider) DecideOrg(_ context.Context, org uuid.UUID) (reminder.Result, error) {
	c.ran = append(c.ran, org)
	return reminder.Result{Tenants: 3, Reminders: 2, Stages: 3, VisitTasks: 1}, c.err
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestTaskID(t *testing.T) {
	t.Parallel()
	org := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000001")
	require.Equal(t, "reminders:decide:0b6c3c1e-1111-4a4a-8b8b-000000000001:2026-11-24",
		remindertask.TaskID(org, time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)))
}

func TestDailyCronIsEightInTheMorning(t *testing.T) {
	t.Parallel()
	require.Equal(t, "0 8 * * *", remindertask.DailyCron)
}

func TestEnqueueDaily(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	day := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		orgs      orgs
		failWith  error
		want      []remindertask.Task
		wantN     int
		wantError bool
	}{
		{
			name:  "one task per organization, named after the organization and the day",
			orgs:  orgs{ids: []uuid.UUID{a, b}},
			want:  []remindertask.Task{{ID: remindertask.TaskID(a, day), OrganizationID: a}, {ID: remindertask.TaskID(b, day), OrganizationID: b}},
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
			n, err := remindertask.EnqueueDaily(context.Background(), tc.orgs, enq, day)
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

func TestHandleDecide(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	payload, err := json.Marshal(map[string]string{"organizationId": org.String()})
	require.NoError(t, err)

	t.Run("decides for the organization in the payload", func(t *testing.T) {
		t.Parallel()
		c := &decider{}
		w := remindertask.Worker{Decider: c, Log: discard()}
		require.NoError(t, w.HandleDecide(context.Background(), asynq.NewTask(remindertask.TypeDecide, payload)))
		require.Equal(t, []uuid.UUID{org}, c.ran)
	})

	t.Run("a failed run is returned so the queue retries it", func(t *testing.T) {
		t.Parallel()
		c := &decider{err: errors.New("one tenant failed")}
		w := remindertask.Worker{Decider: c, Log: discard()}
		err := w.HandleDecide(context.Background(), asynq.NewTask(remindertask.TypeDecide, payload))
		require.Error(t, err)
		require.NotErrorIs(t, err, asynq.SkipRetry)
	})

	for name, body := range map[string][]byte{
		"no organization": []byte(`{}`),
		"not json":        []byte(`nope`),
	} {
		t.Run("a payload with "+name+" is never retried", func(t *testing.T) {
			t.Parallel()
			c := &decider{}
			w := remindertask.Worker{Decider: c, Log: discard()}
			err := w.HandleDecide(context.Background(), asynq.NewTask(remindertask.TypeDecide, body))
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, c.ran)
		})
	}
}

func TestHandleDailyUsesTheDayInJakarta(t *testing.T) {
	t.Parallel()
	a := uuid.New()
	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "08:00 WIB, when the cron fires", now: time.Date(2026, 11, 24, 1, 0, 0, 0, time.UTC), want: "2026-11-24"},
		{name: "00:01 WIB is already the next day in Jakarta", now: time.Date(2026, 11, 23, 17, 1, 0, 0, time.UTC), want: "2026-11-24"},
		{name: "23:59 WIB is still the day before", now: time.Date(2026, 11, 23, 16, 59, 0, 0, time.UTC), want: "2026-11-23"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enq := &enqueuer{}
			w := remindertask.Worker{Orgs: orgs{ids: []uuid.UUID{a}}, Enqueuer: enq, Log: discard(), Clock: clock.NewFake(tc.now)}
			require.NoError(t, w.HandleDaily(context.Background(), asynq.NewTask(remindertask.TypeDaily, nil)))
			require.Equal(t, []remindertask.Task{{ID: "reminders:decide:" + a.String() + ":" + tc.want, OrganizationID: a}}, enq.tasks)
		})
	}
}

func TestRegisterSchedulesTheDailyRun(t *testing.T) {
	t.Parallel()
	mux := asynq.NewServeMux()
	// The scheduler only records the entry until it starts; nothing connects to Redis here.
	sched := asynq.NewScheduler(asynq.RedisClientOpt{Addr: "127.0.0.1:0"}, &asynq.SchedulerOpts{Location: clock.Jakarta})
	w := remindertask.Worker{Decider: &decider{}, Log: discard()}
	require.NoError(t, w.Register(mux, sched))
	for _, typ := range []string{remindertask.TypeDecide, remindertask.TypeDaily} {
		h, pattern := mux.Handler(asynq.NewTask(typ, nil))
		require.Equal(t, typ, pattern)
		require.NotNil(t, h)
	}
}
