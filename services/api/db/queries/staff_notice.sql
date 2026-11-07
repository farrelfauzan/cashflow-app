-- Staff notices (TG-5, decision 42: demo only). Runs in an organization scope (store.WithOrgTx).

-- name: InsertStaffNotice :execrows
-- Records a notice of an event, in the event's transaction. A source already noticed records
-- nothing (0 rows).
INSERT INTO staff_notice (id, organization_id, tenant_id, kind, promise_version_id, promise_id, visit_task_id, created_at)
VALUES (@id, @organization_id, @tenant_id, @kind, sqlc.narg(promise_version_id), sqlc.narg(promise_id),
        sqlc.narg(visit_task_id), @created_at)
ON CONFLICT DO NOTHING;

-- name: LockStaffNotice :one
-- A notice, locked until the sending transaction ends, so two deliveries of its task take turns
-- and the second sees what the first decided.
SELECT id, tenant_id, kind, promise_version_id, promise_id, visit_task_id, status, created_at
FROM staff_notice
WHERE organization_id = @organization_id AND id = @id
FOR UPDATE;

-- name: MarkStaffNotice :execrows
-- Settles a pending notice as sent (with sent_at) or skipped. One already settled is left as it
-- is (0 rows).
UPDATE staff_notice
SET status = @status, sent_at = sqlc.narg(sent_at)
WHERE organization_id = @organization_id AND id = @id AND status = 'pending';

-- name: ListPendingStaffNotices :many
-- Notices still pending that were recorded before @before, oldest first: their task was lost,
-- held until the morning, or has not run yet.
SELECT id
FROM staff_notice
WHERE organization_id = @organization_id AND status = 'pending' AND created_at < @before
ORDER BY created_at, id
LIMIT @row_limit;

-- name: ListNoticeLeases :many
-- Every lease the tenant has or had, active ones first, with its unit, property and collector:
-- who looks after the tenant, and what the notice calls them by.
SELECT l.id, l.status, u.property_id, u.label AS unit_label, p.name AS property_name, l.collector_staff_user_id
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE l.organization_id = @organization_id AND l.tenant_id = @tenant_id
ORDER BY (l.status = 'active') DESC, l.start_date, l.id;

-- name: GetNoticePromiseVersion :one
-- A promise version's terms and number, with its promise's status and whether it is the
-- promise's newest version.
SELECT v.version, v.promised_date, v.promised_amount, p.status AS promise_status,
       (v.version = (SELECT max(v2.version) FROM promise_version v2
                     WHERE v2.organization_id = v.organization_id AND v2.promise_id = v.promise_id))::boolean AS latest
FROM promise_version v
JOIN payment_promise p ON p.organization_id = v.organization_id AND p.id = v.promise_id
WHERE v.organization_id = @organization_id AND v.id = @id;

-- name: GetNoticeVisitTask :one
SELECT reason, resolved_at
FROM visit_task
WHERE organization_id = @organization_id AND id = @id;
