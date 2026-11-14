-- Cash deposits, "Setor tunai" (slice 6, S6-3, decision 46.3). Runs in an organization scope
-- (store.WithOrgTx or WithOrgSnapshot). Field cash is a cash payment with a receipt, recorded
-- through recordCashPayment; it is undeposited while no cash_deposit_payment covers it. A payment
-- counts net of its corrections.

-- name: ListUndepositedCash :many
-- One page of the field cash @staff_user_id recorded that no deposit covers yet, oldest first, with
-- each payment's tenant. GetFieldCash has the total and the count.
SELECT p.id, p.tenant_id, t.name AS tenant_name, p.created_at,
       (p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                    WHERE c.organization_id = p.organization_id AND c.payment_id = p.id))::bigint AS net
FROM payment p
JOIN payment_receipt r ON r.organization_id = p.organization_id AND r.payment_id = p.id
JOIN tenant t ON t.organization_id = p.organization_id AND t.id = p.tenant_id
WHERE p.organization_id = @organization_id AND p.recorded_by_staff_user_id = @staff_user_id
  AND NOT EXISTS (SELECT 1 FROM cash_deposit_payment dp WHERE dp.payment_id = p.id)
ORDER BY p.created_at, p.id
LIMIT @page_limit OFFSET @page_offset;

-- name: InsertCashDeposit :one
-- Records a deposit under its requestId. One already recorded under it is left as it is (no row).
INSERT INTO cash_deposit (id, organization_id, staff_user_id, request_id, amount, expected_amount, payment_count,
                          note, photo_upload_id, created_at)
VALUES (@id, @organization_id, @staff_user_id, @request_id, @amount, @expected_amount, @payment_count,
        sqlc.narg(note), sqlc.narg(photo_upload_id), @created_at)
ON CONFLICT (organization_id, request_id) DO NOTHING
RETURNING *;

-- name: GetCashDepositByRequestID :one
SELECT * FROM cash_deposit
WHERE organization_id = @organization_id AND request_id = @request_id;

-- name: CoverUndepositedCash :many
-- Links every field cash payment of @staff_user_id that no deposit covers yet to the deposit, and
-- returns each one's net amount. A payment another deposit covered meanwhile is skipped.
WITH covered AS (
    INSERT INTO cash_deposit_payment (payment_id, organization_id, deposit_id, staff_user_id)
    SELECT p.id, p.organization_id, @deposit_id, p.recorded_by_staff_user_id
    FROM payment p
    JOIN payment_receipt r ON r.organization_id = p.organization_id AND r.payment_id = p.id
    WHERE p.organization_id = @organization_id AND p.recorded_by_staff_user_id = @staff_user_id::uuid
      AND NOT EXISTS (SELECT 1 FROM cash_deposit_payment dp WHERE dp.payment_id = p.id)
    ON CONFLICT (payment_id) DO NOTHING
    RETURNING payment_id
)
SELECT p.id,
       (p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                    WHERE c.organization_id = p.organization_id AND c.payment_id = p.id))::bigint AS net
FROM covered
JOIN payment p ON p.organization_id = @organization_id AND p.id = covered.payment_id;

-- name: ListCashDeposits :many
-- One page of the organization's deposits, newest first, optionally of one member and by status
-- (pending: not confirmed yet; confirmed), each with its member, its photo's status and its
-- confirmation. CountCashDeposits has the same WHERE: change both together.
SELECT d.id, d.staff_user_id, u.name AS staff_name, d.amount, d.expected_amount, d.payment_count, d.note,
       d.photo_upload_id, up.status AS photo_status, d.created_at,
       c.received_amount, c.note AS confirmation_note, c.confirmed_by_staff_user_id, cu.name AS confirmed_by_name,
       c.confirmed_at
FROM cash_deposit d
JOIN staff_user u ON u.id = d.staff_user_id
LEFT JOIN upload up ON up.organization_id = d.organization_id AND up.id = d.photo_upload_id
LEFT JOIN cash_deposit_confirmation c ON c.organization_id = d.organization_id AND c.deposit_id = d.id
LEFT JOIN staff_user cu ON cu.id = c.confirmed_by_staff_user_id
WHERE d.organization_id = @organization_id
  AND (sqlc.narg(staff_user_id)::uuid IS NULL OR d.staff_user_id = sqlc.narg(staff_user_id))
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status) = 'pending' AND c.deposit_id IS NULL)
       OR (sqlc.narg(status) = 'confirmed' AND c.deposit_id IS NOT NULL))
ORDER BY d.created_at DESC, d.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountCashDeposits :one
SELECT count(*)
FROM cash_deposit d
LEFT JOIN cash_deposit_confirmation c ON c.organization_id = d.organization_id AND c.deposit_id = d.id
WHERE d.organization_id = @organization_id
  AND (sqlc.narg(staff_user_id)::uuid IS NULL OR d.staff_user_id = sqlc.narg(staff_user_id))
  AND (sqlc.narg(status)::text IS NULL
       OR (sqlc.narg(status) = 'pending' AND c.deposit_id IS NULL)
       OR (sqlc.narg(status) = 'confirmed' AND c.deposit_id IS NOT NULL));

-- name: GetCashDeposit :one
-- One deposit, as ListCashDeposits lists it.
SELECT d.id, d.staff_user_id, u.name AS staff_name, d.amount, d.expected_amount, d.payment_count, d.note,
       d.photo_upload_id, up.status AS photo_status, d.created_at,
       c.received_amount, c.note AS confirmation_note, c.confirmed_by_staff_user_id, cu.name AS confirmed_by_name,
       c.confirmed_at
FROM cash_deposit d
JOIN staff_user u ON u.id = d.staff_user_id
LEFT JOIN upload up ON up.organization_id = d.organization_id AND up.id = d.photo_upload_id
LEFT JOIN cash_deposit_confirmation c ON c.organization_id = d.organization_id AND c.deposit_id = d.id
LEFT JOIN staff_user cu ON cu.id = c.confirmed_by_staff_user_id
WHERE d.organization_id = @organization_id AND d.id = @id;

-- name: InsertCashDepositConfirmation :execrows
-- Records the admin's count of a deposit. A deposit already confirmed is left as it is (0 rows).
INSERT INTO cash_deposit_confirmation (deposit_id, organization_id, staff_user_id, received_amount, note,
                                       confirmed_by_staff_user_id, confirmed_at)
VALUES (@deposit_id, @organization_id, @staff_user_id, @received_amount, sqlc.narg(note),
        @confirmed_by_staff_user_id, @confirmed_at)
ON CONFLICT (deposit_id) DO NOTHING;

-- name: ListCashDepositPayments :many
-- One page of the payments a deposit covers, oldest recorded first, each with its tenant and its
-- net amount now. The deposit's payment_count is the total.
SELECT p.id, p.tenant_id, t.name AS tenant_name, p.created_at,
       (p.amount + (SELECT coalesce(sum(c.amount_delta), 0) FROM correction c
                    WHERE c.organization_id = p.organization_id AND c.payment_id = p.id))::bigint AS net
FROM cash_deposit_payment dp
JOIN payment p ON p.organization_id = dp.organization_id AND p.id = dp.payment_id
JOIN tenant t ON t.organization_id = p.organization_id AND t.id = p.tenant_id
WHERE dp.organization_id = @organization_id AND dp.deposit_id = @deposit_id
ORDER BY p.created_at, p.id
LIMIT @page_limit OFFSET @page_offset;

-- name: ListCollectedUnits :many
-- The units of the given tenants' leases, current or ended, that @collector_id collects or
-- collected, by property then unit.
SELECT l.tenant_id, u.label AS unit_label, p.name AS property_name, p.address AS property_address
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
WHERE l.organization_id = @organization_id AND l.tenant_id = ANY(@tenant_ids::uuid[])
  AND l.collector_staff_user_id = @collector_id::uuid
ORDER BY l.tenant_id, p.name, u.label, l.id;
