package uploadtask_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtask"
)

// checker is a fake uploadtask.Checker.
type checker struct {
	mu       sync.Mutex
	scanned  []uuid.UUID
	scanErr  error
	stale    map[uuid.UUID][]uuid.UUID
	staleErr map[uuid.UUID]error
}

func (c *checker) Scan(_ context.Context, _, id uuid.UUID) (upload.Result, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.scanned = append(c.scanned, id)
	return upload.Result{Status: upload.StatusClean, Changed: true}, c.scanErr
}

func (c *checker) Stale(_ context.Context, org uuid.UUID) ([]uuid.UUID, error) {
	return c.stale[org], c.staleErr[org]
}

// enqueuer is a fake uploadtask.Enqueuer.
type enqueuer struct {
	mu    sync.Mutex
	tasks []string
	err   error
}

func (e *enqueuer) EnqueueScanAs(_ context.Context, _, _ uuid.UUID, taskID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.err != nil {
		return e.err
	}
	e.tasks = append(e.tasks, taskID)
	return nil
}

type orgs []uuid.UUID

func (o orgs) ActiveOrganizationIDs(context.Context) ([]uuid.UUID, error) { return o, nil }

func TestTaskIDs(t *testing.T) {
	t.Parallel()
	id := uuid.MustParse("0b6c3c1e-1111-4a4a-8b8b-000000000003")
	require.Equal(t, "upload:scan:0b6c3c1e-1111-4a4a-8b8b-000000000003", uploadtask.TaskID(id))
	// 09:07 and 09:03 Jakarta share the 09:00 slot; 09:10 starts the next.
	at := time.Date(2026, 11, 14, 9, 7, 0, 0, clock.Jakarta)
	require.Equal(t, "upload:scan:0b6c3c1e-1111-4a4a-8b8b-000000000003:2026-11-14T09:00", uploadtask.ReconcileTaskID(id, at))
	require.Equal(t, uploadtask.ReconcileTaskID(id, at), uploadtask.ReconcileTaskID(id, at.Add(-4*time.Minute)))
	require.NotEqual(t, uploadtask.ReconcileTaskID(id, at), uploadtask.ReconcileTaskID(id, at.Add(3*time.Minute)))
	// The same moment named in UTC gives the same Jakarta slot.
	require.Equal(t, uploadtask.ReconcileTaskID(id, at), uploadtask.ReconcileTaskID(id, at.UTC()))
}

func TestHandleScan(t *testing.T) {
	t.Parallel()
	org, id := uuid.New(), uuid.New()
	body, err := json.Marshal(map[string]uuid.UUID{"organizationId": org, "uploadId": id})
	require.NoError(t, err)
	tests := []struct {
		name        string
		body        []byte
		scanErr     error
		wantScanned int
		wantErr     bool
		wantRetry   bool
	}{
		{name: "an upload is scanned", body: body, wantScanned: 1},
		{name: "no organization is never retried", body: []byte(`{"uploadId":"` + id.String() + `"}`), wantErr: true},
		{name: "no upload is never retried", body: []byte(`{"organizationId":"` + org.String() + `"}`), wantErr: true},
		{name: "a malformed payload is never retried", body: []byte(`{`), wantErr: true},
		{name: "an upload that does not exist is never retried", body: body, scanErr: upload.ErrNotFound, wantScanned: 1, wantErr: true},
		{name: "a storage or scanner error is retried", body: body, scanErr: errors.New("clamd down"), wantScanned: 1, wantErr: true, wantRetry: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := &checker{scanErr: tc.scanErr}
			w := uploadtask.Worker{Checker: c, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
			err := w.HandleScan(context.Background(), asynq.NewTask(uploadtask.TypeScan, tc.body))
			require.Len(t, c.scanned, tc.wantScanned)
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Equal(t, !tc.wantRetry, errors.Is(err, asynq.SkipRetry))
		})
	}
}

func TestReconcile(t *testing.T) {
	t.Parallel()
	orgA, orgB := uuid.New(), uuid.New()
	u1, u2, u3 := uuid.New(), uuid.New(), uuid.New()
	at := time.Date(2026, 11, 14, 9, 7, 0, 0, clock.Jakarta)
	tests := []struct {
		name    string
		checker *checker
		enqErr  error
		wantN   int
		want    []string
		wantErr bool
	}{
		{
			name:    "every stale upload of every organization is enqueued under this slot's id",
			checker: &checker{stale: map[uuid.UUID][]uuid.UUID{orgA: {u1, u2}, orgB: {u3}}},
			wantN:   3,
			want:    []string{uploadtask.ReconcileTaskID(u1, at), uploadtask.ReconcileTaskID(u2, at), uploadtask.ReconcileTaskID(u3, at)},
		},
		{name: "nothing stale", checker: &checker{}},
		{
			name:    "a failed listing does not stop the other organization",
			checker: &checker{stale: map[uuid.UUID][]uuid.UUID{orgB: {u3}}, staleErr: map[uuid.UUID]error{orgA: errors.New("db down")}},
			wantN:   1, want: []string{uploadtask.ReconcileTaskID(u3, at)}, wantErr: true,
		},
		{
			name:    "a failed enqueue is returned",
			checker: &checker{stale: map[uuid.UUID][]uuid.UUID{orgA: {u1}}},
			enqErr:  errors.New("redis down"), wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enq := &enqueuer{err: tc.enqErr}
			n, err := uploadtask.Reconcile(context.Background(), orgs{orgA, orgB}, tc.checker, enq, at)
			require.Equal(t, tc.wantErr, err != nil, "error: %v", err)
			require.Equal(t, tc.wantN, n)
			require.ElementsMatch(t, tc.want, enq.tasks)
		})
	}
}
