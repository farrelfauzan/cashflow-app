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
