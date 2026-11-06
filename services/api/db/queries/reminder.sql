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
SELECT invoice_id, period, due_date, billed, paid, outstanding
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

-- Sending (slice 4, S4-5) ---------------------------------------------------------------------

-- name: LockReminder :one
-- A reminder, locked until the sending transaction ends, so two deliveries of its task take turns
-- and the second sees what the first decided.
SELECT id, tenant_id, run_day, promise_version_id, broken_promise_id, status
FROM reminder
WHERE organization_id = @organization_id AND id = @id
FOR UPDATE;

-- name: ListReminderStages :many
SELECT invoice_id, stage
FROM reminder_invoice
WHERE organization_id = @organization_id AND reminder_id = @reminder_id;

-- name: MarkReminder :execrows
-- Settles a pending reminder as sent (with sent_at) or skipped. A reminder already settled is
-- left as it is (0 rows).
UPDATE reminder
SET status = @status, sent_at = sqlc.narg(sent_at)
WHERE organization_id = @organization_id AND id = @id AND status = 'pending';

-- name: ListPendingReminders :many
-- Reminders still pending that were recorded before @before, oldest first: their send task was
-- lost or has not run yet.
SELECT id
FROM reminder
WHERE organization_id = @organization_id AND status = 'pending' AND created_at < @before
ORDER BY created_at, id
LIMIT @row_limit;

-- name: GetMessageRecipient :one
-- Who a tenant message goes to and the organization it names.
SELECT t.name, t.phone, o.display_name
FROM tenant t
JOIN organization o ON o.id = t.organization_id
WHERE t.organization_id = @organization_id AND t.id = @tenant_id;

-- name: ListInvoiceLinesFor :many
-- The lines of the given invoices, in the order they were written.
SELECT invoice_id, label, amount
FROM invoice_line
WHERE organization_id = @organization_id AND invoice_id = ANY(@invoice_ids::uuid[])
ORDER BY seq;

-- name: GetPromiseVersionState :one
-- A promise version's terms with its promise's status and whether it is the promise's newest
-- version.
SELECT v.id, v.promise_id, v.tenant_id, v.promised_date, v.promised_amount, v.created_at,
       p.status AS promise_status,
       (v.version = (SELECT max(v2.version) FROM promise_version v2
                     WHERE v2.organization_id = v.organization_id AND v2.promise_id = v.promise_id))::boolean AS latest
FROM promise_version v
JOIN payment_promise p ON p.organization_id = v.organization_id AND p.id = v.promise_id
WHERE v.organization_id = @organization_id AND v.id = @id;

-- name: GetBrokenPromiseDate :one
-- The promised date of a promise's newest version: the day it was broken on.
SELECT v.promised_date
FROM promise_version v
WHERE v.organization_id = @organization_id AND v.promise_id = @promise_id
ORDER BY v.version DESC
LIMIT 1;

-- name: TenantHasActivePromise :one
SELECT EXISTS (
    SELECT 1 FROM payment_promise
    WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND status = 'active'
) AS active;

-- name: InsertPromiseConfirmation :exec
INSERT INTO promise_confirmation (promise_version_id, organization_id, tenant_id, created_at)
VALUES (@promise_version_id, @organization_id, @tenant_id, @created_at);

-- name: LockPromiseConfirmation :one
SELECT promise_version_id, tenant_id, status, created_at
FROM promise_confirmation
WHERE organization_id = @organization_id AND promise_version_id = @promise_version_id
FOR UPDATE;

-- name: MarkPromiseConfirmation :execrows
UPDATE promise_confirmation
SET status = @status, sent_at = sqlc.narg(sent_at)
WHERE organization_id = @organization_id AND promise_version_id = @promise_version_id AND status = 'pending';

-- name: ListPendingPromiseConfirmations :many
SELECT promise_version_id
FROM promise_confirmation
WHERE organization_id = @organization_id AND status = 'pending' AND created_at < @before
ORDER BY created_at, promise_version_id
LIMIT @row_limit;
