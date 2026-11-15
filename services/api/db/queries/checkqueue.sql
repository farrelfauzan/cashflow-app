-- Antrean cek (S6-6): the admin's check queue, read from the check_queue_item view, and the
-- resolutions that take items off it. Runs in an organization scope (store.WithOrgTx).

-- name: ListCheckQueue :many
-- One page of the waiting items, oldest first, of one kind or all.
SELECT kind, item_id, created_at
FROM check_queue_item
WHERE organization_id = @organization_id
  AND (sqlc.narg(kind)::text IS NULL OR kind = sqlc.narg(kind))
ORDER BY created_at, item_id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCheckQueue :many
-- How many items wait, per kind (kinds with none are absent).
SELECT kind, count(*) AS waiting
FROM check_queue_item
WHERE organization_id = @organization_id
GROUP BY kind;

-- name: GetCheckQueueItem :one
-- One item, if it is still waiting.
SELECT kind, item_id, created_at
FROM check_queue_item
WHERE organization_id = @organization_id AND kind = @kind AND item_id = @item_id;

-- name: ListQueueTransferProofs :many
SELECT tp.id, tp.tenant_id, t.name AS tenant_name, tp.invoice_id, tp.upload_id, u.status AS upload_status, tp.created_at
FROM transfer_proof tp
JOIN tenant t ON t.organization_id = tp.organization_id AND t.id = tp.tenant_id
JOIN upload u ON u.organization_id = tp.organization_id AND u.id = tp.upload_id
WHERE tp.organization_id = @organization_id AND tp.id = ANY(@ids::uuid[]);

-- name: ListQueueDepositGaps :many
SELECT d.id, d.staff_user_id, su.name AS staff_name, d.expected_amount, c.received_amount, d.payment_count,
       c.confirmed_by_staff_user_id, cu.name AS confirmed_by_name, c.confirmed_at
FROM cash_deposit d
JOIN cash_deposit_confirmation c ON c.organization_id = d.organization_id AND c.deposit_id = d.id
JOIN staff_user su ON su.id = d.staff_user_id
JOIN staff_user cu ON cu.id = c.confirmed_by_staff_user_id
WHERE d.organization_id = @organization_id AND d.id = ANY(@ids::uuid[]);

-- name: ListQueueInfectedUploads :many
SELECT u.id, u.purpose, u.tenant_id, t.name AS tenant_name, u.uploaded_by_staff_user_id, su.name AS uploaded_by_name,
       u.created_at, u.scanned_at
FROM upload u
LEFT JOIN tenant t ON t.organization_id = u.organization_id AND t.id = u.tenant_id
LEFT JOIN staff_user su ON su.id = u.uploaded_by_staff_user_id
WHERE u.organization_id = @organization_id AND u.id = ANY(@ids::uuid[]);

-- name: ListQueueQrisMismatches :many
-- An attempt's QR amount, the amount its payment was recorded at, and the amount AutoGoPay
-- reported when it is the recorded one (amount_mismatch); when reconcile recorded the QR's amount
-- first, the reported amount is only in the server log.
SELECT a.id, a.tenant_id, t.name AS tenant_name, a.transaction_id, a.amount, a.paid_amount, a.amount_mismatch
FROM qris_attempt a
JOIN tenant t ON t.organization_id = a.organization_id AND t.id = a.tenant_id
WHERE a.organization_id = @organization_id AND a.id = ANY(@ids::uuid[]);

-- name: ListQueuePhoneChanges :many
SELECT o.id, o.tenant_id, t.name AS tenant_name, t.phone AS current_phone, o.new_phone, o.staff_user_id,
       su.name AS staff_name, o.note, o.created_at
FROM visit_outcome o
JOIN tenant t ON t.organization_id = o.organization_id AND t.id = o.tenant_id
JOIN staff_user su ON su.id = o.staff_user_id
WHERE o.organization_id = @organization_id AND o.id = ANY(@ids::uuid[]);

-- name: ListQueueTenantUnits :many
-- The units of the given tenants' leases, active ones first, then the newest.
SELECT l.tenant_id, u.label AS unit_label, p.name AS property_name, (l.status = 'active')::boolean AS active
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE l.organization_id = @organization_id AND l.tenant_id = ANY(@tenant_ids::uuid[])
ORDER BY l.tenant_id, (l.status = 'active') DESC, l.start_date DESC, l.id;

-- name: InsertCheckResolution :one
-- Takes an item off the queue. Its kind's column is UNIQUE, so a second answer fails (23505).
INSERT INTO check_resolution (id, organization_id, kind, transfer_proof_id, cash_deposit_id, upload_id,
    qris_attempt_id, visit_outcome_id, outcome, payment_id, note, staff_user_id, created_at)
VALUES (@id, @organization_id, @kind, sqlc.narg(transfer_proof_id), sqlc.narg(cash_deposit_id), sqlc.narg(upload_id),
    sqlc.narg(qris_attempt_id), sqlc.narg(visit_outcome_id), @outcome, sqlc.narg(payment_id), sqlc.narg(note),
    @staff_user_id, @created_at)
RETURNING *;

-- name: GetProofResolutionByPayment :one
-- The resolution a payment matched, for a repeated match.
SELECT * FROM check_resolution
WHERE organization_id = @organization_id AND payment_id = @payment_id;

-- name: GetDepositGapClosures :many
-- The closures of the given deposits' gaps, with who closed them.
SELECT r.cash_deposit_id, r.note, r.staff_user_id, su.name AS staff_name, r.created_at
FROM check_resolution r
JOIN staff_user su ON su.id = r.staff_user_id
WHERE r.organization_id = @organization_id AND r.kind = 'deposit_gap' AND r.cash_deposit_id = ANY(@deposit_ids::uuid[]);

-- name: IsCheckItemResolved :one
SELECT EXISTS (
    SELECT 1 FROM check_resolution
    WHERE organization_id = @organization_id AND kind = @kind
      AND @item_id::uuid IN (transfer_proof_id, cash_deposit_id, upload_id, qris_attempt_id, visit_outcome_id)
)::boolean AS resolved;
