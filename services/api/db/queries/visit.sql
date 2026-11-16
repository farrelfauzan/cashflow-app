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

-- The karyawan's visit list and outcomes (slice 6, S6-4). Who may see a lease is decided in Go
-- for every lease of the organization; the queries get the visible leases in lease_ids.

-- name: ListFieldVisits :many
-- One page of "Kunjungan hari ini" for @collector_id: the tenants they collect from on an active
-- lease, all of whose active leases are visible (in lease_ids; decision 47), that have an open
-- visit task, or on whom @collector_id recorded an outcome in [@day_start, @day_end). Open first,
-- the oldest open task first; then those visited, the latest visit first; the tenant id breaks
-- ties. CountFieldVisits has the same CTE: change both together.
WITH mine AS (
    SELECT t.id AS tenant_id, t.name AS tenant_name, coalesce(b.outstanding, 0)::bigint AS outstanding,
           coalesce(b.oldest_unpaid_due_date, DATE '0001-01-01')::date AS oldest_unpaid_due_date,
           (SELECT coalesce(sum(ib.outstanding), 0) FROM invoice_balance ib
            WHERE ib.organization_id = t.organization_id AND ib.tenant_id = t.id
              AND ib.outstanding > 0 AND ib.due_date < @today::date)::bigint AS overdue,
           (SELECT min(v.created_at) FROM visit_task v
            WHERE v.organization_id = t.organization_id AND v.tenant_id = t.id AND v.resolved_at IS NULL) AS oldest_open_at,
           (SELECT max(o.created_at) FROM visit_outcome o
            WHERE o.organization_id = t.organization_id AND o.tenant_id = t.id AND o.staff_user_id = @collector_id::uuid
              AND o.created_at >= @day_start AND o.created_at < @day_end) AS visited_at
    FROM tenant t
    LEFT JOIN tenant_balance b ON b.organization_id = t.organization_id AND b.tenant_id = t.id
    WHERE t.organization_id = @organization_id
      AND EXISTS (SELECT 1 FROM lease l
                  WHERE l.organization_id = t.organization_id AND l.tenant_id = t.id
                    AND l.status = 'active' AND l.collector_staff_user_id = @collector_id::uuid)
      AND NOT EXISTS (SELECT 1 FROM lease l
                      WHERE l.organization_id = t.organization_id AND l.tenant_id = t.id
                        AND l.status = 'active' AND NOT (l.id = ANY(@lease_ids::uuid[])))
)
SELECT tenant_id, tenant_name, outstanding, oldest_unpaid_due_date, overdue,
       (oldest_open_at IS NOT NULL)::boolean AS open
FROM mine
WHERE oldest_open_at IS NOT NULL OR visited_at IS NOT NULL
ORDER BY (oldest_open_at IS NOT NULL) DESC, oldest_open_at, visited_at DESC, tenant_id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountFieldVisits :one
WITH mine AS (
    SELECT t.id AS tenant_id,
           (SELECT min(v.created_at) FROM visit_task v
            WHERE v.organization_id = t.organization_id AND v.tenant_id = t.id AND v.resolved_at IS NULL) AS oldest_open_at,
           (SELECT max(o.created_at) FROM visit_outcome o
            WHERE o.organization_id = t.organization_id AND o.tenant_id = t.id AND o.staff_user_id = @collector_id::uuid
              AND o.created_at >= @day_start AND o.created_at < @day_end) AS visited_at
    FROM tenant t
    WHERE t.organization_id = @organization_id
      AND EXISTS (SELECT 1 FROM lease l
                  WHERE l.organization_id = t.organization_id AND l.tenant_id = t.id
                    AND l.status = 'active' AND l.collector_staff_user_id = @collector_id::uuid)
      AND NOT EXISTS (SELECT 1 FROM lease l
                      WHERE l.organization_id = t.organization_id AND l.tenant_id = t.id
                        AND l.status = 'active' AND NOT (l.id = ANY(@lease_ids::uuid[])))
)
SELECT count(*) AS total, count(*) FILTER (WHERE oldest_open_at IS NOT NULL) AS open_total
FROM mine WHERE oldest_open_at IS NOT NULL OR visited_at IS NOT NULL;

-- name: ListOpenVisitTasksOfTenants :many
-- The open visit tasks of the given tenants, oldest first.
SELECT id, tenant_id, reason, created_at
FROM visit_task
WHERE organization_id = @organization_id AND tenant_id = ANY(@tenant_ids::uuid[]) AND resolved_at IS NULL
ORDER BY tenant_id, created_at, id;

-- name: ListLatestVisitOutcomes :many
-- The latest outcome @staff_user_id recorded on each of the given tenants in [@day_start, @day_end).
SELECT DISTINCT ON (o.tenant_id) o.*
FROM visit_outcome o
WHERE o.organization_id = @organization_id AND o.tenant_id = ANY(@tenant_ids::uuid[])
  AND o.staff_user_id = @staff_user_id AND o.created_at >= @day_start AND o.created_at < @day_end
ORDER BY o.tenant_id, o.created_at DESC, o.id DESC;

-- name: ListUnlinkedCashToday :many
-- The field cash @staff_user_id took from the given tenants in [@day_start, @day_end) that no visit
-- outcome names yet, each net of its corrections, oldest first.
SELECT p.id, p.tenant_id, p.created_at,
       (p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                    WHERE c.organization_id = p.organization_id AND c.payment_id = p.id))::bigint AS net
FROM payment p
JOIN payment_receipt r ON r.organization_id = p.organization_id AND r.payment_id = p.id
WHERE p.organization_id = @organization_id AND p.tenant_id = ANY(@tenant_ids::uuid[]) AND p.method = 'cash'
  AND p.recorded_by_staff_user_id = @staff_user_id::uuid
  AND p.received_at >= @day_start AND p.received_at < @day_end
  AND NOT EXISTS (SELECT 1 FROM visit_outcome o WHERE o.payment_id = p.id)
ORDER BY p.tenant_id, p.created_at, p.id;

-- name: LockOpenVisitTasksOfTenant :many
-- The tenant's open visit tasks, locked until the transaction ends, so two outcomes for one tenant
-- take turns and the second sees what the first resolved.
SELECT id, reason, created_at
FROM visit_task
WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND resolved_at IS NULL
ORDER BY created_at, id
FOR UPDATE;

-- name: GetVisitOutcomeByRequestID :one
SELECT * FROM visit_outcome
WHERE organization_id = @organization_id AND request_id = @request_id;

-- name: ListVisitOutcomeTaskReasons :many
-- The reasons of the tasks an outcome resolved.
SELECT v.reason
FROM visit_outcome_task ot
JOIN visit_task v ON v.organization_id = ot.organization_id AND v.id = ot.visit_task_id
WHERE ot.organization_id = @organization_id AND ot.outcome_id = @outcome_id
ORDER BY v.created_at, v.id;

-- name: GetVisitPayment :one
-- A payment with what an outcome needs to know about it: its tenant, recorder, method, time and
-- whether it is field cash (has a receipt).
SELECT p.tenant_id, p.recorded_by_staff_user_id, p.method, p.received_at,
       EXISTS (SELECT 1 FROM payment_receipt r WHERE r.organization_id = p.organization_id AND r.payment_id = p.id)::boolean AS field_cash
FROM payment p
WHERE p.organization_id = @organization_id AND p.id = @id;

-- name: GetVisitPromiseVersion :one
-- A promise version with what an outcome needs to know about it.
SELECT tenant_id, source, created_by_staff_user_id, created_at
FROM promise_version
WHERE organization_id = @organization_id AND id = @id;

-- name: InsertVisitOutcome :one
-- Records an outcome under its requestId. One already recorded under it is left as it is (no row).
INSERT INTO visit_outcome (id, organization_id, tenant_id, staff_user_id, request_id, outcome, payment_id,
                           promise_version_id, new_phone, note, created_at)
VALUES (@id, @organization_id, @tenant_id, @staff_user_id, @request_id, @outcome, sqlc.narg(payment_id),
        sqlc.narg(promise_version_id), sqlc.narg(new_phone), sqlc.narg(note), @created_at)
ON CONFLICT (organization_id, request_id) DO NOTHING
RETURNING *;

-- name: InsertVisitOutcomeTask :exec
INSERT INTO visit_outcome_task (visit_task_id, organization_id, tenant_id, outcome_id)
VALUES (@visit_task_id, @organization_id, @tenant_id, @outcome_id);

-- name: ResolveVisitTask :execrows
-- Resolves an open visit task. One already resolved is left as it is (0 rows).
UPDATE visit_task SET resolved_at = @resolved_at
WHERE organization_id = @organization_id AND id = @id AND resolved_at IS NULL;
