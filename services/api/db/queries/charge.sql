-- Runs in an organization scope (store.WithOrgTx).

-- name: ListChargeTypes :many
SELECT * FROM charge_type
WHERE organization_id = $1
ORDER BY name COLLATE natural_sort, id;

-- name: CreateChargeType :one
INSERT INTO charge_type (id, organization_id, name, metered)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetChargeTypes :many
SELECT * FROM charge_type
WHERE organization_id = $1 AND id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListLeaseCharges :many
-- The fixed extras of the given leases, with their type names, by lease then type name.
SELECT lc.lease_id, lc.charge_type_id, ct.name AS charge_type_name, lc.amount
FROM lease_charge lc
JOIN charge_type ct ON ct.organization_id = lc.organization_id AND ct.id = lc.charge_type_id
WHERE lc.organization_id = $1 AND lc.lease_id = ANY(sqlc.arg(lease_ids)::uuid[])
ORDER BY lc.lease_id, ct.name COLLATE natural_sort, ct.id;

-- name: DeleteLeaseChargesExcept :exec
-- Removes the lease's fixed extras whose type is not in keep. A lease term, not a money row.
DELETE FROM lease_charge
WHERE organization_id = $1 AND lease_id = $2
  AND NOT (charge_type_id = ANY(sqlc.arg(keep)::uuid[]));

-- name: UpsertLeaseCharge :exec
-- Sets one fixed extra on a lease; updated_at moves only when the amount changes.
INSERT INTO lease_charge (id, organization_id, lease_id, charge_type_id, amount)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (lease_id, charge_type_id) DO UPDATE
SET amount = EXCLUDED.amount, updated_at = now()
WHERE lease_charge.amount <> EXCLUDED.amount;
