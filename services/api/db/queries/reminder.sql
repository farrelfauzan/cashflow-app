-- name: ListTenantsToRemind :many
-- The tenants the reminders:decide run looks at (slice 4, S4-4): those owing on an invoice due
-- by the horizon (today + 3, the first day an H-3 can fire), those with an active promise, and
-- those with a promise closed broken since broken_since. Each one's rules decide the rest.
SELECT t.id
FROM tenant t
WHERE t.organization_id = @organization_id
  AND (
      EXISTS (
          SELECT 1 FROM invoice_balance b
          WHERE b.organization_id = t.organization_id AND b.tenant_id = t.id
            AND b.outstanding > 0 AND b.due_date <= @horizon::date)
      OR EXISTS (
          SELECT 1 FROM payment_promise p
          WHERE p.organization_id = t.organization_id AND p.tenant_id = t.id
            AND (p.status = 'active' OR (p.status = 'broken' AND p.closed_at >= @broken_since::timestamptz)))
  )
ORDER BY t.id;

-- name: ListTenantOwingInvoices :many
-- The tenant's invoices that still owe something, in FIFO order. A cancelled invoice owes
-- nothing.
SELECT invoice_id, due_date, outstanding
FROM invoice_balance
WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND outstanding > 0
ORDER BY period, seq;

-- name: ListRecordedReminderStages :many
-- The stages already recorded for the given invoices.
SELECT invoice_id, stage
FROM reminder_invoice
WHERE organization_id = @organization_id AND invoice_id = ANY(@invoice_ids::uuid[]);

-- name: PromiseVersionReminded :one
-- Whether the H-1 of a promise version is recorded.
SELECT EXISTS (
    SELECT 1 FROM reminder
    WHERE organization_id = @organization_id AND promise_version_id = @promise_version_id::uuid
) AS reminded;

-- name: ListTenantBrokenPromisesSince :many
-- The tenant's promises closed broken at or after closed_since, newest first, with whether their
-- broken notice is recorded.
SELECT p.id, p.closed_at,
       EXISTS (
           SELECT 1 FROM reminder r
           WHERE r.organization_id = p.organization_id AND r.broken_promise_id = p.id
       ) AS notified
FROM payment_promise p
WHERE p.organization_id = @organization_id AND p.tenant_id = @tenant_id
  AND p.status = 'broken' AND p.closed_at >= @closed_since::timestamptz
ORDER BY p.closed_at DESC, p.id DESC;

-- name: InsertReminder :exec
INSERT INTO reminder (id, organization_id, tenant_id, run_day, promise_version_id, broken_promise_id, created_at)
VALUES (@id, @organization_id, @tenant_id, @run_day, sqlc.narg(promise_version_id), sqlc.narg(broken_promise_id), @created_at);

-- name: InsertReminderInvoices :exec
-- A reminder's invoice stages, in one statement: invoice_ids[i] at stages[i]. Two set-returning
-- functions in one select list run in step (PostgreSQL 10 and later); arrays of different
-- lengths would pad with NULL, which stage's NOT NULL refuses.
INSERT INTO reminder_invoice (organization_id, reminder_id, invoice_id, tenant_id, stage)
SELECT @organization_id, @reminder_id, unnest(@invoice_ids::uuid[]), @tenant_id, unnest(@stages::text[]);

-- name: LastResolvedVisitAt :one
-- When the tenant's newest resolved visit task of a reason was resolved; no row when none was.
SELECT resolved_at
FROM visit_task
WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND reason = @reason
  AND resolved_at IS NOT NULL
ORDER BY resolved_at DESC
LIMIT 1;
