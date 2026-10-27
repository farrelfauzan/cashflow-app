-- Runs in an organization scope (store.WithOrgTx).

-- name: ListProperties :many
SELECT p.*, (SELECT count(*) FROM unit u WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS unit_count
FROM property p
WHERE p.organization_id = $1
ORDER BY p.name COLLATE natural_sort, p.id;

-- name: GetProperty :one
SELECT p.*, (SELECT count(*) FROM unit u WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS unit_count
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
SELECT * FROM unit
WHERE organization_id = $1 AND property_id = $2
ORDER BY label COLLATE natural_sort, id;

-- name: GetUnit :one
SELECT * FROM unit
WHERE organization_id = $1 AND id = $2;

-- name: CreateUnit :one
INSERT INTO unit (id, organization_id, property_id, label, default_rent)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateUnit :one
UPDATE unit SET label = $3, default_rent = $4, updated_at = now()
WHERE organization_id = $1 AND id = $2
RETURNING *;
