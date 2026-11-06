-- QRIS attempts (slice 3, S3-5).

-- name: InsertQrisAttempt :exec
INSERT INTO qris_attempt (
    id, organization_id, tenant_id, payment_link_id, amount, transaction_id, order_id, qr_string,
    status, expires_at, created_at, closed_at, check_requested_at, paid_amount
) VALUES (
    @id, @organization_id, @tenant_id, @payment_link_id, @amount, @transaction_id, @order_id, @qr_string,
    @status, @expires_at, @created_at, sqlc.narg('closed_at'), sqlc.narg('check_requested_at'),
    sqlc.narg('paid_amount')
);

-- name: GetLatestQrisAttempt :one
-- A tenant's newest attempt, whichever of the tenant's links it came from. The pending attempt,
-- when there is one (at most one per tenant), comes first: created_at comes from the app's clock,
-- so a closed attempt can share it or, after a clock step, be later, and must never hide the QR
-- that can still be paid.
SELECT id, organization_id, tenant_id, payment_link_id, amount, transaction_id, qr_string, status,
       expires_at, created_at, closed_at, check_requested_at
FROM qris_attempt
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
ORDER BY (status = 'pending') DESC, created_at DESC, id DESC
LIMIT 1;

-- name: GetInvoicePlace :one
-- Where an invoice's lease is: the unit's label and its property's name, printed under the QR.
SELECT u.label AS unit_label, p.name AS property_name
FROM invoice i
JOIN lease l ON l.organization_id = i.organization_id AND l.id = i.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE i.organization_id = @organization_id AND i.id = @id;

-- name: TryLockTenantQris :one
-- Takes the tenant's QRIS lock for the rest of the transaction, or answers false at once when
-- another transaction holds it. Making, checking and reconciling a tenant's QR take it, so two of
-- them never call AutoGoPay about the same tenant at the same time. The key is a hash: two
-- tenants that collide only wait on each other.
SELECT pg_try_advisory_xact_lock(hashtextextended('qris_attempt:' || CAST(@tenant_id AS uuid)::text, 0)) AS locked;

-- name: GetQrisAttempt :one
SELECT id, organization_id, tenant_id, payment_link_id, amount, transaction_id, qr_string, status,
       expires_at, created_at, closed_at, check_requested_at
FROM qris_attempt
WHERE organization_id = @organization_id AND id = @id;

-- name: CloseQrisAttempt :execrows
-- Closes a pending attempt as expired or cancelled. An attempt that is no longer pending is left
-- alone and counts no row.
UPDATE qris_attempt
SET status = @status, closed_at = @closed_at
WHERE organization_id = @organization_id AND id = @id AND status = 'pending';

-- name: MarkQrisAttemptPaid :execrows
-- Marks an attempt paid at paid_amount. A pending attempt closes now; an expired or cancelled one
-- keeps the time it closed. An attempt already paid is left alone and counts no row.
UPDATE qris_attempt
SET status = 'paid', paid_amount = @paid_amount, closed_at = COALESCE(closed_at, @now)
WHERE organization_id = @organization_id AND id = @id AND status <> 'paid';

-- name: RequestQrisCheck :execrows
-- Records that the tenant said they paid a pending attempt. The first time stands.
UPDATE qris_attempt
SET check_requested_at = @now
WHERE organization_id = @organization_id AND id = @id AND status = 'pending' AND check_requested_at IS NULL;

-- name: ListQrisAttemptsToReconcile :many
-- The pending attempts reconcile asks AutoGoPay about: those past their expiry, and those the
-- tenant said they paid. Oldest first, at most @max_rows.
SELECT id, tenant_id
FROM qris_attempt
WHERE organization_id = @organization_id AND status = 'pending'
  AND (expires_at <= @now OR check_requested_at IS NOT NULL)
ORDER BY created_at, id
LIMIT @max_rows;

-- name: GetQrisAttemptByTransactionID :one
-- The attempt AutoGoPay's webhook names, inside the organization its URL belongs to.
SELECT id, organization_id, tenant_id, payment_link_id, amount, transaction_id, qr_string, status,
       expires_at, created_at, closed_at, check_requested_at
FROM qris_attempt
WHERE organization_id = @organization_id AND transaction_id = @transaction_id;
