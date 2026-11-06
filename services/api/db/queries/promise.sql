-- name: GetActivePromise :one
-- The tenant's active promise with its newest version. Making a promise reads it after taking the
-- tenant's lock (LockTenantForAllocation), so two makes for one tenant never both miss it.
SELECT p.id AS promise_id, p.created_at AS promise_created_at,
       v.id, v.version, v.kind, v.promised_date, v.promised_amount, v.paying_now, v.outstanding_at,
       v.source, v.created_at
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
    paying_now, outstanding_at, source, note, created_by_staff_user_id, payment_link_id, created_at
) VALUES (
    @id, @organization_id, @promise_id, @tenant_id, @version, @kind, @promised_date,
    sqlc.narg(promised_amount), @paying_now, @outstanding_at, @source, sqlc.narg(note),
    sqlc.narg(created_by_staff_user_id), sqlc.narg(payment_link_id), @created_at
);

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

