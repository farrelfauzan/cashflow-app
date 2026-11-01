//go:build integration

package invoicetask_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/queue"
)

func TestClientEnqueuesOnRedis(t *testing.T) {
	ctx := context.Background()
	ctr, err := tcredis.Run(ctx, "redis:7-alpine")
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)
	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	opts, err := redis.ParseURL(uri)
	require.NoError(t, err)

	client := queue.NewClient(opts.Addr)
	t.Cleanup(func() { _ = client.Close() })
	inspector := asynq.NewInspector(asynq.RedisClientOpt{Addr: opts.Addr})
	t.Cleanup(func() { _ = inspector.Close() })

	org := uuid.New()
	task := invoicetask.Task{ID: invoicetask.LeaseTaskID(org, uuid.New()), OrganizationID: org}
	enq := invoicetask.NewClient(client)
	require.NoError(t, enq.Enqueue(ctx, task))
	require.NoError(t, enq.Enqueue(ctx, task), "the same task id while it is queued counts as enqueued")

	info, err := inspector.GetTaskInfo(queue.Default, task.ID)
	require.NoError(t, err)
	require.Equal(t, invoicetask.TypeGenerate, info.Type)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(info.Payload, &payload))
	require.Equal(t, map[string]any{"organizationId": org.String()}, payload, "the payload is the organization id and nothing else")

	pending, err := inspector.ListPendingTasks(queue.Default)
	require.NoError(t, err)
	require.Len(t, pending, 1)
}
