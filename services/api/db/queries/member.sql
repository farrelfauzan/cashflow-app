-- Runs in an organization scope (store.WithOrgTx).

-- name: GetActiveMembershipForUser :one
SELECT * FROM membership
WHERE organization_id = $1 AND staff_user_id = $2 AND status = 'active';

-- name: GetMembership :one
SELECT m.id, m.staff_user_id, m.status, m.created_at, u.phone, u.name
FROM membership m
JOIN staff_user u ON u.id = m.staff_user_id
WHERE m.organization_id = $1 AND m.id = $2;

-- name: ListMembers :many
SELECT m.id, m.staff_user_id, m.status, m.created_at, u.phone, u.name
FROM membership m
JOIN staff_user u ON u.id = m.staff_user_id
WHERE m.organization_id = $1
ORDER BY u.name, m.id;

-- name: CreateMembership :one
-- Returns no row when the staff user is already a member.
INSERT INTO membership (id, organization_id, staff_user_id)
VALUES ($1, $2, $3)
ON CONFLICT (organization_id, staff_user_id) DO NOTHING
RETURNING *;

-- name: ListRoleAssignmentsForMembership :many
SELECT ra.id, ra.builtin_role, ra.role_id, ra.property_id, r.name AS role_name, r.policy AS role_policy
FROM role_assignment ra
LEFT JOIN role r ON r.id = ra.role_id
WHERE ra.organization_id = $1 AND ra.membership_id = $2
ORDER BY ra.created_at, ra.id;

-- name: ListOrgRoleAssignments :many
SELECT ra.membership_id, ra.builtin_role, ra.role_id, ra.property_id, r.name AS role_name
FROM role_assignment ra
LEFT JOIN role r ON r.id = ra.role_id
WHERE ra.organization_id = $1
ORDER BY ra.created_at, ra.id;

-- name: DeleteBuiltinRoleAssignments :exec
DELETE FROM role_assignment
WHERE organization_id = $1 AND membership_id = $2 AND builtin_role IS NOT NULL;

-- name: CreateBuiltinRoleAssignment :exec
-- property_id NULL means the role applies to the whole organization.
INSERT INTO role_assignment (id, organization_id, membership_id, builtin_role, property_id)
VALUES ($1, $2, $3, $4, $5);

-- name: CreateRole :one
INSERT INTO role (id, organization_id, name, policy)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: CreateCustomRoleAssignment :exec
INSERT INTO role_assignment (id, organization_id, membership_id, role_id)
VALUES ($1, $2, $3, $4);
