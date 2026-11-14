-- The tenant's receipt for a cash payment (slice 6, S6-2, decision 46.5). Runs in an organization
-- scope (store.WithOrgTx).

-- name: InsertPaymentReceipt :exec
-- Records the receipt of a cash payment, in the payment's transaction.
INSERT INTO payment_receipt (payment_id, organization_id, tenant_id, created_at)
VALUES (@payment_id, @organization_id, @tenant_id, @created_at);

-- name: GetPaymentReceiptStatus :one
-- A receipt's status, for the answer to a cash payment repeated under its requestId.
SELECT status
FROM payment_receipt
WHERE organization_id = @organization_id AND payment_id = @payment_id;

-- name: LockPaymentReceipt :one
-- A receipt, locked until the sending transaction ends, so two deliveries of its task take turns
-- and the second sees what the first decided.
SELECT payment_id, tenant_id, status, created_at
FROM payment_receipt
WHERE organization_id = @organization_id AND payment_id = @payment_id
FOR UPDATE;

-- name: MarkPaymentReceipt :execrows
-- Settles a pending receipt as sent (with sent_at) or skipped. One already settled is left as it
-- is (0 rows).
UPDATE payment_receipt
SET status = @status, sent_at = sqlc.narg(sent_at)
WHERE organization_id = @organization_id AND payment_id = @payment_id AND status = 'pending';

-- name: ListPendingPaymentReceipts :many
-- Receipts still pending that were recorded before @before, oldest first: their task was lost or
-- has not run yet.
SELECT payment_id
FROM payment_receipt
WHERE organization_id = @organization_id AND status = 'pending' AND created_at < @before
ORDER BY created_at, payment_id
LIMIT @row_limit;

-- name: LockCashPaymentForPhoto :one
-- A cash payment with a receipt and who recorded it, its receipt row locked until the transaction
-- ends, so two photo attachments for one payment take turns.
SELECT r.payment_id, r.tenant_id, p.amount, p.recorded_by_staff_user_id
FROM payment_receipt r
JOIN payment p ON p.organization_id = r.organization_id AND p.id = r.payment_id
WHERE r.organization_id = @organization_id AND r.payment_id = @payment_id
FOR UPDATE OF r;

-- name: ListPaymentReceiptPhotoLinks :many
-- Every photo linked to a payment, latest first, with its upload's status now.
SELECT l.upload_id, u.status
FROM payment_receipt_photo l
JOIN upload u ON u.organization_id = l.organization_id AND u.id = l.upload_id
WHERE l.organization_id = @organization_id AND l.payment_id = @payment_id
ORDER BY l.seq DESC;

-- name: InsertPaymentReceiptPhoto :execrows
-- Links a receipt photo to a payment. An upload already linked (to any payment) is left as it is
-- (0 rows).
INSERT INTO payment_receipt_photo (upload_id, organization_id, payment_id, tenant_id, attached_by_staff_user_id, attached_at)
VALUES (@upload_id, @organization_id, @payment_id, @tenant_id, @attached_by_staff_user_id, @attached_at)
ON CONFLICT (upload_id) DO NOTHING;

-- name: ListCurrentReceiptPhotos :many
-- The photo of each of the given payments (its latest link) with its upload's status now. A
-- payment with no photo has no row.
SELECT DISTINCT ON (l.payment_id) l.payment_id, l.upload_id, u.status
FROM payment_receipt_photo l
JOIN upload u ON u.organization_id = l.organization_id AND u.id = l.upload_id
WHERE l.organization_id = @organization_id AND l.payment_id = ANY(@payment_ids::uuid[])
ORDER BY l.payment_id, l.seq DESC;
