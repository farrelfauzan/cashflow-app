package invoicetask_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask/invoicetasktest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

// orgs is a fake OrgLister.
type orgs struct {
	ids []uuid.UUID
	err error
}

func (o orgs) ActiveOrganizationIDs(context.Context) ([]uuid.UUID, error) { return o.ids, o.err }

// generator is a fake Generator that records the organizations it ran for.
type generator struct {
	ran []uuid.UUID
	err error
}

func (g *generator) GenerateOrg(_ context.Context, org uuid.UUID) (billing.Result, error) {
	g.ran = append(g.ran, org)
	return billing.Result{Leases: 2, Invoices: 1}, g.err
}

func discard() *slog.Logger { return slog.New(slog.DiscardHandler) }

func TestTaskIDs(t *testing.T) {
	t.Parallel()
	org := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000001")
	lease := uuid.MustParse("0b6c3c1e-2222-4a4a-8b8b-000000000002")
	require.Equal(t, "invoices:generate:0b6c3c1e-1111-4a4a-8b8b-000000000001:2026-11-24",
		invoicetask.DailyTaskID(org, time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, "invoices:generate:0b6c3c1e-1111-4a4a-8b8b-000000000001:0b6c3c1e-2222-4a4a-8b8b-000000000002",
		invoicetask.LeaseTaskID(org, lease))
}

func TestEnqueueDaily(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	day := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		orgs      orgs
		failWith  error
		want      []invoicetask.Task
		wantN     int
		wantError bool
	}{
		{
			name:  "one task per organization, named after the organization and the day",
			orgs:  orgs{ids: []uuid.UUID{a, b}},
			want:  []invoicetask.Task{{ID: invoicetask.DailyTaskID(a, day), OrganizationID: a}, {ID: invoicetask.DailyTaskID(b, day), OrganizationID: b}},
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
			fake := &invoicetasktest.Fake{}
			fake.FailWith(tc.failWith)
			n, err := invoicetask.EnqueueDaily(context.Background(), tc.orgs, fake, day)
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.wantN, n)
			require.Equal(t, tc.want, fake.Tasks())
		})
	}
}

func TestLeaseQueue(t *testing.T) {
	t.Parallel()
	org, lease := uuid.New(), uuid.New()

	t.Run("enqueues the organization's task named after the lease", func(t *testing.T) {
		t.Parallel()
		fake := &invoicetasktest.Fake{}
		invoicetask.NewLeaseQueue(fake, discard()).LeaseSaved(context.Background(), org, lease)
		require.Equal(t, []invoicetask.Task{{ID: invoicetask.LeaseTaskID(org, lease), OrganizationID: org}}, fake.Tasks())
	})

	t.Run("a failure is logged, not returned", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		fake := &invoicetasktest.Fake{}
		fake.FailWith(errors.New("redis down"))
		invoicetask.NewLeaseQueue(fake, slog.New(slog.NewJSONHandler(&logs, nil))).LeaseSaved(context.Background(), org, lease)
		require.Empty(t, fake.Tasks())
		require.Contains(t, logs.String(), `"level":"ERROR"`)
		require.Contains(t, logs.String(), lease.String())
	})

	t.Run("a cancelled request still enqueues", func(t *testing.T) {
		t.Parallel()
		fake := &invoicetasktest.Fake{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		invoicetask.NewLeaseQueue(ctxCheckingEnqueuer{fake}, discard()).LeaseSaved(ctx, org, lease)
		require.Len(t, fake.Tasks(), 1)
	})
}

// ctxCheckingEnqueuer fails like a real client when its context is already done.
type ctxCheckingEnqueuer struct{ next invoicetask.Enqueuer }

func (e ctxCheckingEnqueuer) Enqueue(ctx context.Context, t invoicetask.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return e.next.Enqueue(ctx, t)
}

func TestHandleGenerate(t *testing.T) {
	t.Parallel()
	org := uuid.New()
	payload, err := json.Marshal(map[string]string{"organizationId": org.String()})
	require.NoError(t, err)

	t.Run("runs the organization in the payload", func(t *testing.T) {
		t.Parallel()
		gen := &generator{}
		w := invoicetask.Worker{Generator: gen, Log: discard()}
		require.NoError(t, w.HandleGenerate(context.Background(), asynq.NewTask(invoicetask.TypeGenerate, payload)))
		require.Equal(t, []uuid.UUID{org}, gen.ran)
	})

	t.Run("a failed run is returned so the queue retries it", func(t *testing.T) {
		t.Parallel()
		gen := &generator{err: errors.New("one lease failed")}
		w := invoicetask.Worker{Generator: gen, Log: discard()}
		err := w.HandleGenerate(context.Background(), asynq.NewTask(invoicetask.TypeGenerate, payload))
		require.Error(t, err)
		require.NotErrorIs(t, err, asynq.SkipRetry)
	})

	for name, body := range map[string][]byte{
		"no organization": []byte(`{}`),
		"not json":        []byte(`nope`),
	} {
		t.Run("a payload with "+name+" is never retried", func(t *testing.T) {
			t.Parallel()
			gen := &generator{}
			w := invoicetask.Worker{Generator: gen, Log: discard()}
			err := w.HandleGenerate(context.Background(), asynq.NewTask(invoicetask.TypeGenerate, body))
			require.ErrorIs(t, err, asynq.SkipRetry)
			require.Empty(t, gen.ran)
		})
	}
}

func TestHandleDailyUsesTheDayInJakarta(t *testing.T) {
	t.Parallel()
	a := uuid.New()
	fake := &invoicetasktest.Fake{}
	// 23 Nov 22:00 UTC is already 24 Nov 05:00 in Jakarta, when the cron fires.
	w := invoicetask.Worker{
		Orgs: orgs{ids: []uuid.UUID{a}}, Enqueuer: fake, Log: discard(),
		Clock: clock.NewFake(time.Date(2026, 11, 23, 22, 0, 0, 0, time.UTC)),
	}
	require.NoError(t, w.HandleDaily(context.Background(), asynq.NewTask(invoicetask.TypeDaily, nil)))
	require.Equal(t, []invoicetask.Task{{ID: "invoices:generate:" + a.String() + ":2026-11-24", OrganizationID: a}}, fake.Tasks())
}
