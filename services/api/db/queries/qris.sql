-- QRIS attempts (slice 3, S3-5).

-- name: InsertQrisAttempt :exec
INSERT INTO qris_attempt (
    id, organization_id, tenant_id, payment_link_id, amount, transaction_id, order_id, qr_string,
    status, expires_at, created_at, closed_at, check_requested_at
) VALUES (
    @id, @organization_id, @tenant_id, @payment_link_id, @amount, @transaction_id, @order_id, @qr_string,
    @status, @expires_at, @created_at, sqlc.narg('closed_at'), sqlc.narg('check_requested_at')
);

-- name: GetLatestQrisAttempt :one
-- A tenant's newest attempt, whichever of the tenant's links it came from.
SELECT id, payment_link_id, amount, qr_string, status, expires_at, created_at, closed_at, check_requested_at
FROM qris_attempt
WHERE organization_id = @organization_id AND tenant_id = @tenant_id
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetInvoicePlace :one
-- Where an invoice's lease is: the unit's label and its property's name, printed under the QR.
SELECT u.label AS unit_label, p.name AS property_name
FROM invoice i
JOIN lease l ON l.organization_id = i.organization_id AND l.id = i.lease_id
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE i.organization_id = @organization_id AND i.id = @id;
