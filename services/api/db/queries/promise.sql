-- name: GetActivePromise :one
-- The tenant's active promise with its newest version. Making a promise reads it after taking the
-- tenant's lock (LockTenantForAllocation), so two makes for one tenant never both miss it.
SELECT p.id AS promise_id, p.created_at AS promise_created_at,
       v.id, v.version, v.kind, v.promised_date, v.promised_amount, v.paying_now, v.outstanding_at,
       v.source, v.created_at, v.payment_seq_mark
FROM payment_promise p
JOIN promise_version v ON v.organization_id = p.organization_id AND v.promise_id = p.id
WHERE p.organization_id = @organization_id AND p.tenant_id = @tenant_id AND p.status = 'active'
ORDER BY v.version DESC
LIMIT 1;

-- name: InsertPaymentPromise :exec
INSERT INTO payment_promise (id, organization_id, tenant_id, status, created_at)
VALUES (@id, @organization_id, @tenant_id, 'active', @created_at);

-- name: InsertPromiseVersion :exec
INSERT INTO promise_version (
    id, organization_id, promise_id, tenant_id, version, kind, promised_date, promised_amount,
    paying_now, outstanding_at, source, note, created_by_staff_user_id, payment_link_id, created_at,
    payment_seq_mark
) VALUES (
    @id, @organization_id, @promise_id, @tenant_id, @version, @kind, @promised_date,
    sqlc.narg(promised_amount), @paying_now, @outstanding_at, @source, sqlc.narg(note),
    sqlc.narg(created_by_staff_user_id), sqlc.narg(payment_link_id), @created_at,
    sqlc.arg(payment_seq_mark)::bigint
);

-- name: TenantPaymentSeqMark :one
-- The tenant's highest payment seq, 0 when they have none: a version's payment_seq_mark. Read
-- under the tenant's allocation lock, which every payment insert holds too, so a payment the
-- mark does not cover gets a higher seq.
SELECT coalesce(max(seq), 0)::bigint AS mark
FROM payment
WHERE organization_id = @organization_id AND tenant_id = @tenant_id;

-- name: InsertPromiseCoveredInvoices :exec
-- The invoices a version covers, in one statement.
INSERT INTO promise_covered_invoice (organization_id, promise_version_id, invoice_id, tenant_id)
SELECT @organization_id, @promise_version_id, unnest(@invoice_ids::uuid[]), @tenant_id;

-- name: ListTenantActiveLeaseDates :many
-- The dates of a tenant's active leases that decide when each is billed next.
SELECT id, start_date, entered_on, billed_from, term_months, planned_end_date, end_date
FROM lease
WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND status = 'active'
ORDER BY id;

-- name: CountTenantPromises :one
SELECT count(*) FROM payment_promise
WHERE organization_id = @organization_id AND tenant_id = @tenant_id;

-- name: ListTenantPromisePage :many
-- One page of a tenant's promises, newest first, the id breaking ties.
SELECT id, status, created_at, closed_at
FROM payment_promise
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListPromiseVersions :many
-- Every version of the given promises, oldest first, with the member who made it (none from the
-- tenant page).
SELECT v.id, v.promise_id, v.version, v.kind, v.promised_date, v.promised_amount, v.paying_now,
       v.outstanding_at, v.source, v.note, v.created_at,
       m.id AS created_by_member_id, su.name AS created_by_name
FROM promise_version v
LEFT JOIN membership m ON m.organization_id = v.organization_id AND m.staff_user_id = v.created_by_staff_user_id
LEFT JOIN staff_user su ON su.id = v.created_by_staff_user_id
WHERE v.organization_id = @organization_id AND v.promise_id = ANY(@promise_ids::uuid[])
ORDER BY v.promise_id, v.version;

-- name: ListPromiseCoveredInvoices :many
-- The invoices the given versions cover, in FIFO order (oldest period first).
SELECT c.promise_version_id, c.invoice_id
FROM promise_covered_invoice c
JOIN invoice i ON i.organization_id = c.organization_id AND i.id = c.invoice_id
WHERE c.organization_id = @organization_id AND c.promise_version_id = ANY(@version_ids::uuid[])
ORDER BY c.promise_version_id, i.period, i.seq;


-- name: SumTenantPaymentsSince :one
-- What the tenant paid since a version was made, after corrections to those payments: their
-- payments with a seq above the version's mark, or, for a version made before marks existed
-- (seq_mark NULL), those created at or after made_at.
SELECT coalesce(sum(p.amount + coalesce((
           SELECT sum(c.amount_delta) FROM correction c
           WHERE c.organization_id = p.organization_id AND c.payment_id = p.id), 0)), 0)::bigint AS paid
FROM payment p
WHERE p.organization_id = @organization_id AND p.tenant_id = @tenant_id
  AND CASE WHEN sqlc.narg(seq_mark)::bigint IS NULL THEN p.created_at >= @made_at
           ELSE p.seq > sqlc.narg(seq_mark)::bigint END;

-- name: CountCoveredInvoicesOwing :one
-- How many of a version's covered invoices still owe something now. A cancelled invoice owes
-- nothing.
SELECT count(*)
FROM promise_covered_invoice c
JOIN invoice_balance b ON b.organization_id = c.organization_id AND b.invoice_id = c.invoice_id
WHERE c.organization_id = @organization_id AND c.promise_version_id = @promise_version_id
  AND b.outstanding > 0;

-- name: ClosePromise :execrows
-- Closes an active promise as kept, partially kept or broken. A closed one is left alone (and a
-- trigger refuses to change it).
UPDATE payment_promise SET status = @status, closed_at = @closed_at
WHERE organization_id = @organization_id AND id = @id AND status = 'active';

-- name: ListTenantsWithDuePromise :many
-- The tenants whose active promise's newest version promised a day before today (Asia/Jakarta):
-- the day ended, so the promise is closed.
SELECT p.tenant_id
FROM payment_promise p
JOIN LATERAL (
    SELECT v.promised_date FROM promise_version v
    WHERE v.organization_id = p.organization_id AND v.promise_id = p.id
    ORDER BY v.version DESC
    LIMIT 1
) latest ON true
WHERE p.organization_id = @organization_id AND p.status = 'active' AND latest.promised_date < @today::date
ORDER BY p.tenant_id;

-- name: CountTenantBrokenPromises :one
SELECT count(*) FROM payment_promise
WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND status = 'broken';
