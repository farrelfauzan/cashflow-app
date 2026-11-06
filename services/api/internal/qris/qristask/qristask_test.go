package qristask_test

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris/qristask"
)

// orgs is a fake OrgLister.
type orgs struct {
	ids []uuid.UUID
	err error
}

func (o orgs) ActiveOrganizationIDs(context.Context) ([]uuid.UUID, error) { return o.ids, o.err }

// enqueuer is a fake Enqueuer that records the tasks it accepted. A task for an organization in
// failFor is refused, as when Redis is unreachable for a moment.
type enqueuer struct {
	mu      sync.Mutex
	tasks   []qristask.Task
	failFor map[uuid.UUID]error
}

func (e *enqueuer) Enqueue(_ context.Context, t qristask.Task) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.failFor[t.OrganizationID]; err != nil {
		return err
	}
	e.tasks = append(e.tasks, t)
	return nil
}

func (e *enqueuer) Tasks() []qristask.Task {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]qristask.Task(nil), e.tasks...)
}

// reconciler is a fake Reconciler that records the organizations it ran for.
type reconciler struct {
	ran []uuid.UUID
	err error
}

func (r *reconciler) ReconcileOrg(_ context.Context, org uuid.UUID) (qris.ReconcileResult, error) {
	r.ran = append(r.ran, org)
	return qris.ReconcileResult{Checked: 3, Paid: 1, Closed: 1, Failed: 1}, r.err
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestTaskID(t *testing.T) {
	t.Parallel()
	org := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000001")
	const prefix = "qris:reconcile:0b6c3c1e-1111-4a4a-8b8b-000000000001:"

	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"10:47 WIB is the 10:30 slot", time.Date(2026, 11, 5, 10, 47, 12, 0, clock.Jakarta), prefix + "2026-11-05T10:30"},
		{"10:30 sharp starts its own slot", time.Date(2026, 11, 5, 10, 30, 0, 0, clock.Jakarta), prefix + "2026-11-05T10:30"},
		{"10:29:59 still belongs to 10:00", time.Date(2026, 11, 5, 10, 29, 59, 0, clock.Jakarta), prefix + "2026-11-05T10:00"},
		{"23:59 is the 23:30 slot", time.Date(2026, 11, 5, 23, 59, 59, 0, clock.Jakarta), prefix + "2026-11-05T23:30"},
		{"midnight is the 00:00 slot", time.Date(2026, 11, 6, 0, 0, 0, 0, clock.Jakarta), prefix + "2026-11-06T00:00"},
		{"a UTC input gives the Jakarta slot", time.Date(2026, 11, 5, 3, 47, 0, 0, time.UTC), prefix + "2026-11-05T10:30"},
		{"a UTC evening is already the next day in Jakarta", time.Date(2026, 11, 5, 17, 10, 0, 0, time.UTC), prefix + "2026-11-06T00:00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, qristask.TaskID(org, tc.at))
		})
	}

	t.Run("two organizations in the same slot have different ids", func(t *testing.T) {
		t.Parallel()
		at := time.Date(2026, 11, 5, 10, 47, 0, 0, clock.Jakarta)
		require.NotEqual(t, qristask.TaskID(org, at), qristask.TaskID(uuid.New(), at))
	})
}

func TestEnqueueAll(t *testing.T) {
	t.Parallel()
	a, b, c := uuid.New(), uuid.New(), uuid.New()
	at := time.Date(2026, 11, 5, 10, 47, 0, 0, clock.Jakarta)
	task := func(org uuid.UUID) qristask.Task {
		return qristask.Task{ID: qristask.TaskID(org, at), OrganizationID: org}
	}
	listerDown := errors.New("database down")
	redisDown := errors.New("redis down")

	tests := []struct {
		name     string
		orgs     orgs
		failFor  map[uuid.UUID]error
		want     []qristask.Task
		wantN    int
		wantErrs []error
	}{
		{
			name:  "one task per organization, named after the organization and the half hour",
			orgs:  orgs{ids: []uuid.UUID{a, b}},
			want:  []qristask.Task{task(a), task(b)},
			wantN: 2,
		},
		{
			name: "no organization, no task",
			orgs: orgs{},
		},
		{
			name:     "one failure does not stop the others, and is returned",
			orgs:     orgs{ids: []uuid.UUID{a, b, c}},
			failFor:  map[uuid.UUID]error{b: redisDown},
			want:     []qristask.Task{task(a), task(c)},
			wantN:    2,
			wantErrs: []error{redisDown},
		},
		{
			name:     "every failure comes back joined",
			orgs:     orgs{ids: []uuid.UUID{a, b}},
			failFor:  map[uuid.UUID]error{a: redisDown, b: errors.New("redis timeout")},
			wantErrs: []error{redisDown},
		},
		{
			name:     "a failing organization list enqueues nothing",
			orgs:     orgs{ids: []uuid.UUID{a}, err: listerDown},
			wantErrs: []error{listerDown},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enq := &enqueuer{failFor: tc.failFor}
			n, err := qristask.EnqueueAll(context.Background(), tc.orgs, enq, at)
			if len(tc.wantErrs) == 0 {
				require.NoError(t, err)
			}
			for _, want := range tc.wantErrs {
				require.ErrorIs(t, err, want)
			}
			require.Equal(t, tc.wantN, n)
			require.Equal(t, tc.want, enq.Tasks())
		})
	}
}

func TestHandleReconcile(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	payload := func(t *testing.T, id uuid.UUID) []byte {
		t.Helper()
		b, err := json.Marshal(map[string]string{"organizationId": id.String()})
		require.NoError(t, err)
		return b
	}

	t.Run("reconciles the organization in the payload", func(t *testing.T) {
		t.Parallel()
		rec := &reconciler{}
		w := qristask.Worker{Reconciler: rec, Log: discard()}
		require.NoError(t, w.HandleReconcile(context.Background(), asynq.NewTask(qristask.TypeReconcile, payload(t, org))))
		require.Equal(t, []uuid.UUID{org}, rec.ran)
	})

	t.Run("a failed run is returned, and may be retried by the queue's own rules", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("database down")
		rec := &reconciler{err: cause}
		w := qristask.Worker{Reconciler: rec, Log: discard()}
		err := w.HandleReconcile(context.Background(), asynq.NewTask(qristask.TypeReconcile, payload(t, org)))
		require.ErrorIs(t, err, cause)
		require.NotErrorIs(t, err, asynq.SkipRetry)
		require.Equal(t, []uuid.UUID{org}, rec.ran)
	})

	tests := []struct {
		name string
		body []byte
	}{
		{"no payload", nil},
		{"an empty object", []byte(`{}`)},
		{"a nil organization", payload(t, uuid.Nil)},
		{"an organization that is not a uuid", []byte(`{"organizationId":"griya-sejahtera"}`)},
		{"not JSON", []byte(`nope`)},
	}
	for _, tc := range tests {
		t.Run("a payload with "+tc.name+" is never retried", func(t *testing.T) {
			t.Parallel()
			rec := &reconciler{}
			w := qristask.Worker{Reconciler: rec, Log: discard()}
			err := w.HandleReconcile(context.Background(), asynq.NewTask(qristask.TypeReconcile, tc.body))
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, rec.ran)
		})
	}
}

func TestHandleReconcileAll(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()

	t.Run("names the tasks after the half hour in Jakarta on the clock", func(t *testing.T) {
		t.Parallel()
		enq := &enqueuer{}
		// 3 Nov 17:47 UTC is already 4 Nov 00:47 in Jakarta, so the slot is 00:30 on the 4th.
		w := qristask.Worker{
			Orgs: orgs{ids: []uuid.UUID{a, b}}, Enqueuer: enq, Log: discard(),
			Clock: clock.NewFake(time.Date(2026, 11, 3, 17, 47, 0, 0, time.UTC)),
		}
		require.NoError(t, w.HandleReconcileAll(context.Background(), asynq.NewTask(qristask.TypeReconcileAll, nil)))
		require.Equal(t, []qristask.Task{
			{ID: "qris:reconcile:" + a.String() + ":2026-11-04T00:30", OrganizationID: a},
			{ID: "qris:reconcile:" + b.String() + ":2026-11-04T00:30", OrganizationID: b},
		}, enq.Tasks())
	})

	t.Run("the next half hour gets new ids", func(t *testing.T) {
		t.Parallel()
		enq := &enqueuer{}
		fake := clock.NewFake(time.Date(2026, 11, 5, 10, 31, 0, 0, clock.Jakarta))
		w := qristask.Worker{Orgs: orgs{ids: []uuid.UUID{a}}, Enqueuer: enq, Log: discard(), Clock: fake}
		task := asynq.NewTask(qristask.TypeReconcileAll, nil)
		require.NoError(t, w.HandleReconcileAll(context.Background(), task))
		fake.Advance(30 * time.Minute)
		require.NoError(t, w.HandleReconcileAll(context.Background(), task))
		got := enq.Tasks()
		require.Len(t, got, 2)
		require.Equal(t, "qris:reconcile:"+a.String()+":2026-11-05T10:30", got[0].ID)
		require.Equal(t, "qris:reconcile:"+a.String()+":2026-11-05T11:00", got[1].ID)
	})

	t.Run("a failure is returned so the run is seen to fail", func(t *testing.T) {
		t.Parallel()
		cause := errors.New("database down")
		w := qristask.Worker{
			Orgs: orgs{err: cause}, Enqueuer: &enqueuer{}, Log: discard(),
			Clock: clock.NewFake(time.Date(2026, 11, 5, 10, 0, 0, 0, clock.Jakarta)),
		}
		err := w.HandleReconcileAll(context.Background(), asynq.NewTask(qristask.TypeReconcileAll, nil))
		require.ErrorIs(t, err, cause)
	})
}
