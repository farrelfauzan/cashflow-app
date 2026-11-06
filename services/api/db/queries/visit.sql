-- name: OpenVisitTask :execrows
-- Opens a visit task for the tenant and reason, unless one is already open: 1 when it opened one.
INSERT INTO visit_task (id, organization_id, tenant_id, reason, created_at)
VALUES (@id, @organization_id, @tenant_id, @reason, @created_at)
ON CONFLICT (organization_id, tenant_id, reason) WHERE resolved_at IS NULL DO NOTHING;

-- name: ListOpenVisitTasks :many
-- The organization's open visit tasks, oldest first.
SELECT id, tenant_id, reason, created_at
FROM visit_task
WHERE organization_id = @organization_id AND resolved_at IS NULL
ORDER BY created_at, id;
