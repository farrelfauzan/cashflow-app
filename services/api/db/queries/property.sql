-- Runs in an organization scope (store.WithOrgTx).

-- name: ListProperties :many
SELECT p.*, (SELECT count(*) FROM unit u WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS unit_count,
       (SELECT count(*) FROM unit u JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
        WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS occupied_unit_count
FROM property p
WHERE p.organization_id = $1
ORDER BY p.name COLLATE natural_sort, p.id;

-- name: GetProperty :one
SELECT p.*, (SELECT count(*) FROM unit u WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS unit_count,
       (SELECT count(*) FROM unit u JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
        WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS occupied_unit_count
FROM property p
WHERE p.organization_id = $1 AND p.id = $2;

-- name: CreateProperty :one
INSERT INTO property (id, organization_id, name, kind, address)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateProperty :one
UPDATE property SET name = $3, kind = $4, address = $5, updated_at = now()
WHERE organization_id = $1 AND id = $2
RETURNING *;

-- name: ListUnits :many
-- Each unit with its active lease, if any. A partial unique index allows one active lease per
-- unit, so the join adds no rows.
SELECT sqlc.embed(u), l.tenant_id AS lease_tenant_id, l.collector_staff_user_id, t.name AS lease_tenant_name,
       (l.id IS NOT NULL)::boolean AS occupied
FROM unit u
LEFT JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
LEFT JOIN tenant t ON t.organization_id = l.organization_id AND t.id = l.tenant_id
WHERE u.organization_id = $1 AND u.property_id = $2
ORDER BY u.label COLLATE natural_sort, u.id;

-- name: GetUnit :one
SELECT * FROM unit
WHERE organization_id = $1 AND id = $2;

-- name: GetUnitOccupancy :one
-- One unit with its active lease, if any (see ListUnits).
SELECT sqlc.embed(u), l.tenant_id AS lease_tenant_id, l.collector_staff_user_id, t.name AS lease_tenant_name,
       (l.id IS NOT NULL)::boolean AS occupied
FROM unit u
LEFT JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
LEFT JOIN tenant t ON t.organization_id = l.organization_id AND t.id = l.tenant_id
WHERE u.organization_id = $1 AND u.id = $2;

-- name: CreateUnit :one
INSERT INTO unit (id, organization_id, property_id, label, default_rent)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateUnit :one
UPDATE unit SET label = $3, default_rent = $4, updated_at = now()
WHERE organization_id = $1 AND id = $2
RETURNING *;

-- name: FindPropertyByName :one
-- Names are unique per organization, ignoring case.
SELECT * FROM property
WHERE organization_id = $1 AND lower(name) = lower(sqlc.arg(name));

-- name: FindUnitByLabel :one
-- Labels are unique per property, ignoring case.
SELECT * FROM unit
WHERE organization_id = $1 AND property_id = $2 AND lower(label) = lower(sqlc.arg(label));
