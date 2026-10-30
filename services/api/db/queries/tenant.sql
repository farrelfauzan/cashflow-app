-- Runs in an organization scope (store.WithOrgTx).

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

-- name: ListLeaseDetailsForTenant :many
SELECT l.*, u.label AS unit_label, u.property_id, p.name AS property_name,
       m.id AS collector_member_id, su.name AS collector_name,
       m.status AS collector_status
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
LEFT JOIN staff_user su ON su.id = l.collector_staff_user_id
WHERE l.organization_id = $1 AND l.tenant_id = $2
ORDER BY l.status, l.start_date DESC, l.id;

-- name: GetLeaseDetail :one
SELECT l.*, u.label AS unit_label, u.property_id, p.name AS property_name,
       m.id AS collector_member_id, su.name AS collector_name,
       m.status AS collector_status
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
LEFT JOIN staff_user su ON su.id = l.collector_staff_user_id
WHERE l.organization_id = $1 AND l.id = $2;

-- name: CreateLease :exec
INSERT INTO lease (id, organization_id, unit_id, tenant_id, start_date, term_months, deposit_amount,
                   collector_staff_user_id, entered_on, billed_from)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: LockLease :one
-- Locks the lease row for the rest of the transaction, so a concurrent change or move-out
-- waits and then sees the committed status.
SELECT id, status FROM lease
WHERE organization_id = $1 AND id = $2
FOR UPDATE;

-- name: UpdateLeaseTerms :execrows
-- Only an active lease changes. Unit, tenant and term never change: end the lease and start
-- another.
UPDATE lease SET start_date = $3, deposit_amount = $4,
       collector_staff_user_id = $5, billed_from = $6, updated_at = now()
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

-- name: ListLeaseScopes :many
-- What decides who may see each lease, for every lease in the organization.
SELECT l.id, l.tenant_id, l.status, u.property_id, l.collector_staff_user_id
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
WHERE l.organization_id = $1;

-- name: ListTenantPage :many
-- One page of the tenants matching the filters. With org_wide every tenant and lease is
-- visible; otherwise only the leases in lease_ids and the tenants with one of them. Filters
-- look only at visible leases. CountTenantPage has the same WHERE: change both together.
WITH vl AS (
    SELECT l.tenant_id, l.status, l.start_date, u.property_id, u.label,
           (l.status = 'active' AND m.status = 'disabled') AS collector_disabled
    FROM lease l
    JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
    LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
    WHERE l.organization_id = @organization_id
      AND (@org_wide::boolean OR l.id = ANY(@lease_ids::uuid[]))
), vt AS (
    SELECT t.*,
           CASE
               WHEN EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id AND vl.status = 'active' AND vl.start_date <= @today::date) THEN 'active'
               WHEN EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id AND vl.status = 'active') THEN 'upcoming'
               WHEN EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id) THEN 'ended'
               ELSE 'none'
           END::text AS lease_status,
           CASE WHEN t.phone LIKE '+62%' THEN '0' || substr(t.phone, 4) ELSE t.phone END AS national_phone
    FROM tenant t
    WHERE t.organization_id = @organization_id
      AND (@org_wide::boolean OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id))
)
SELECT vt.id, vt.organization_id, vt.name, vt.phone, vt.note, vt.created_at, vt.updated_at
FROM vt
WHERE (sqlc.narg('q')::text IS NULL
       OR vt.name ILIKE sqlc.narg('q')
       OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = vt.id AND vl.label ILIKE sqlc.narg('q'))
       OR vt.national_phone LIKE sqlc.narg('phone_national')::text
       OR vt.national_phone LIKE sqlc.narg('phone_digits')::text)
  AND (sqlc.narg('property_id')::uuid IS NULL
       OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = vt.id AND vl.property_id = sqlc.narg('property_id')))
  AND (sqlc.narg('lease_status')::text IS NULL OR vt.lease_status = sqlc.narg('lease_status'))
  AND (NOT @collector_disabled::boolean
       OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = vt.id AND vl.collector_disabled))
ORDER BY CASE WHEN @sort::text = 'name' THEN vt.name END,
         CASE WHEN @sort::text = '-name' THEN vt.name END DESC,
         CASE WHEN @sort::text = 'createdAt' THEN vt.created_at END,
         CASE WHEN @sort::text = '-createdAt' THEN vt.created_at END DESC,
         vt.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountTenantPage :one
-- Every tenant ListTenantPage would return on any page: keep the WHERE the same.
WITH vl AS (
    SELECT l.tenant_id, l.status, l.start_date, u.property_id, u.label,
           (l.status = 'active' AND m.status = 'disabled') AS collector_disabled
    FROM lease l
    JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
    LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
    WHERE l.organization_id = @organization_id
      AND (@org_wide::boolean OR l.id = ANY(@lease_ids::uuid[]))
), vt AS (
    SELECT t.*,
           CASE
               WHEN EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id AND vl.status = 'active' AND vl.start_date <= @today::date) THEN 'active'
               WHEN EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id AND vl.status = 'active') THEN 'upcoming'
               WHEN EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id) THEN 'ended'
               ELSE 'none'
           END::text AS lease_status,
           CASE WHEN t.phone LIKE '+62%' THEN '0' || substr(t.phone, 4) ELSE t.phone END AS national_phone
    FROM tenant t
    WHERE t.organization_id = @organization_id
      AND (@org_wide::boolean OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = t.id))
)
SELECT count(*)
FROM vt
WHERE (sqlc.narg('q')::text IS NULL
       OR vt.name ILIKE sqlc.narg('q')
       OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = vt.id AND vl.label ILIKE sqlc.narg('q'))
       OR vt.national_phone LIKE sqlc.narg('phone_national')::text
       OR vt.national_phone LIKE sqlc.narg('phone_digits')::text)
  AND (sqlc.narg('property_id')::uuid IS NULL
       OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = vt.id AND vl.property_id = sqlc.narg('property_id')))
  AND (sqlc.narg('lease_status')::text IS NULL OR vt.lease_status = sqlc.narg('lease_status'))
  AND (NOT @collector_disabled::boolean
       OR EXISTS (SELECT 1 FROM vl WHERE vl.tenant_id = vt.id AND vl.collector_disabled));

-- name: ListLeaseDetailsForTenants :many
-- The leases of some tenants with their unit, property and collector, active first.
SELECT l.*, u.label AS unit_label, u.property_id, p.name AS property_name,
       m.id AS collector_member_id, su.name AS collector_name,
       m.status AS collector_status
FROM lease l
JOIN unit u ON u.organization_id = l.organization_id AND u.id = l.unit_id
JOIN property p ON p.organization_id = u.organization_id AND p.id = u.property_id
LEFT JOIN membership m ON m.organization_id = l.organization_id AND m.staff_user_id = l.collector_staff_user_id
LEFT JOIN staff_user su ON su.id = l.collector_staff_user_id
WHERE l.organization_id = @organization_id AND l.tenant_id = ANY(@tenant_ids::uuid[])
ORDER BY l.status, l.start_date DESC, l.id;
