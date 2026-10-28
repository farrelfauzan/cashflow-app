-- Runs in an organization scope (store.WithOrgTx).

-- name: ListTenants :many
SELECT * FROM tenant
WHERE organization_id = $1
ORDER BY name, id;

-- name: GetTenant :one
SELECT * FROM tenant
WHERE organization_id = $1 AND id = $2;

-- name: CreateTenant :one
INSERT INTO tenant (id, organization_id, name, phone, note)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateTenant :one
UPDATE tenant SET name = $3, phone = $4, note = $5, updated_at = now()
WHERE organization_id = $1 AND id = $2
RETURNING *;

-- name: ListLeaseDetails :many
-- Every lease in the organization with its unit, property and collector, active first.
SELECT l.*, u.label AS unit_label, u.property_id, p.name AS property_name,
       m.id AS collector_member_id, su.name AS collector_name
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
LEFT JOIN staff_user su ON su.id = l.collector_staff_user_id
WHERE l.organization_id = $1
ORDER BY l.status, l.start_date DESC, l.id;

-- name: ListLeaseDetailsForTenant :many
SELECT l.*, u.label AS unit_label, u.property_id, p.name AS property_name,
       m.id AS collector_member_id, su.name AS collector_name
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
LEFT JOIN staff_user su ON su.id = l.collector_staff_user_id
WHERE l.organization_id = $1 AND l.tenant_id = $2
ORDER BY l.status, l.start_date DESC, l.id;

-- name: GetLeaseDetail :one
SELECT l.*, u.label AS unit_label, u.property_id, p.name AS property_name,
       m.id AS collector_member_id, su.name AS collector_name
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
LEFT JOIN staff_user su ON su.id = l.collector_staff_user_id
WHERE l.organization_id = $1 AND l.id = $2;

-- name: CreateLease :exec
INSERT INTO lease (id, organization_id, unit_id, tenant_id, start_date, rent_amount, deposit_amount,
                   collector_staff_user_id, entered_on, billed_from)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: LockLease :one
-- Locks the lease row for the rest of the transaction, so a concurrent change or move-out
-- waits and then sees the committed status.
SELECT id, status FROM lease
WHERE organization_id = $1 AND id = $2
FOR UPDATE;

-- name: UpdateLeaseTerms :execrows
-- Only an active lease changes. Unit and tenant never change: end the lease and start another.
UPDATE lease SET start_date = $3, rent_amount = $4, deposit_amount = $5,
       collector_staff_user_id = $6, billed_from = $7, updated_at = now()
WHERE organization_id = $1 AND id = $2 AND status = 'active';

-- name: SetLeasePlannedEnd :execrows
-- NULL clears it. Only an active lease has one.
UPDATE lease SET planned_end_date = $3, updated_at = now()
WHERE organization_id = $1 AND id = $2 AND status = 'active';

-- name: EndLease :execrows
-- The move-out clears the planned end date: the real end replaces it.
UPDATE lease SET status = 'ended', end_date = $3, planned_end_date = NULL, updated_at = now()
WHERE organization_id = $1 AND id = $2 AND status = 'active';

-- name: FindTenantByPhone :one
SELECT * FROM tenant
WHERE organization_id = $1 AND phone = $2;

-- name: GetActiveLeaseForUnit :one
SELECT * FROM lease
WHERE organization_id = $1 AND unit_id = $2 AND status = 'active';
