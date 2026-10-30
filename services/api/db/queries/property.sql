-- Runs in an organization scope (store.WithOrgTx).

-- name: ListPropertyIDs :many
SELECT id FROM property WHERE organization_id = $1;

-- name: ListPropertyPage :many
-- One page of the properties matching the filters: every one with org_wide, otherwise those in
-- property_ids. CountPropertyPage has the same WHERE: change both together.
SELECT p.*, (SELECT count(*) FROM unit u WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS unit_count,
       (SELECT count(*) FROM unit u JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
        WHERE u.organization_id = p.organization_id AND u.property_id = p.id) AS occupied_unit_count
FROM property p
WHERE p.organization_id = @organization_id
  AND (@org_wide::boolean OR p.id = ANY(@property_ids::uuid[]))
  AND (sqlc.narg('q')::text IS NULL OR p.name ILIKE sqlc.narg('q'))
ORDER BY CASE WHEN @sort::text = 'name' THEN p.name END COLLATE natural_sort,
         CASE WHEN @sort::text = '-name' THEN p.name END COLLATE natural_sort DESC,
         p.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountPropertyPage :one
-- Every property ListPropertyPage would return on any page: keep the WHERE the same.
SELECT count(*)
FROM property p
WHERE p.organization_id = @organization_id
  AND (@org_wide::boolean OR p.id = ANY(@property_ids::uuid[]))
  AND (sqlc.narg('q')::text IS NULL OR p.name ILIKE sqlc.narg('q'));

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
-- One page of a property's units, each with its active lease, if any. A partial unique index
-- allows one active lease per unit, so the join adds no rows. CountUnits has the same WHERE:
-- change both together.
SELECT sqlc.embed(u), l.tenant_id AS lease_tenant_id, l.collector_staff_user_id, t.name AS lease_tenant_name,
       (l.id IS NOT NULL)::boolean AS occupied
FROM unit u
LEFT JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
LEFT JOIN tenant t ON t.organization_id = l.organization_id AND t.id = l.tenant_id
WHERE u.organization_id = @organization_id AND u.property_id = @property_id
  AND (sqlc.narg('q')::text IS NULL OR u.label ILIKE sqlc.narg('q'))
ORDER BY CASE WHEN @sort::text = 'label' THEN u.label END COLLATE natural_sort,
         CASE WHEN @sort::text = '-label' THEN u.label END COLLATE natural_sort DESC,
         u.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountUnits :one
-- Every unit ListUnits would return on any page: keep the WHERE the same.
SELECT count(*)
FROM unit u
LEFT JOIN lease l ON l.organization_id = u.organization_id AND l.unit_id = u.id AND l.status = 'active'
LEFT JOIN tenant t ON t.organization_id = l.organization_id AND t.id = l.tenant_id
WHERE u.organization_id = @organization_id AND u.property_id = @property_id
  AND (sqlc.narg('q')::text IS NULL OR u.label ILIKE sqlc.narg('q'));

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
INSERT INTO unit (id, organization_id, property_id, label, terms)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: UpdateUnit :one
UPDATE unit SET label = $3, terms = $4, updated_at = now()
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
