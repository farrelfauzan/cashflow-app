-- Runs in an organization scope (store.WithOrgTx).

-- name: GetActiveMembershipForUser :one
SELECT * FROM membership
WHERE organization_id = $1 AND staff_user_id = $2 AND status = 'active';

-- name: GetMembership :one
SELECT m.id, m.staff_user_id, m.status, m.created_at, m.invitation_state, m.invitation_last_sent_at,
       m.first_sign_in_at, u.phone, u.name,
       -- Active leases naming the member as collector, organization-wide: a number, no names.
       (SELECT count(*) FROM lease l
        WHERE l.organization_id = m.organization_id AND l.collector_staff_user_id = m.staff_user_id
          AND l.status = 'active') AS collector_lease_count
FROM membership m
JOIN staff_user u ON u.id = m.staff_user_id
WHERE m.organization_id = $1 AND m.id = $2;

-- name: ListMemberPage :many
-- One page of the members matching the filters. CountMemberPage has the same WHERE: change
-- both together.
WITH mm AS (
    SELECT m.id, m.staff_user_id, m.status, m.created_at, m.invitation_state, m.invitation_last_sent_at,
           m.first_sign_in_at, u.phone, u.name,
           -- Active leases naming the member as collector, organization-wide: a number, no names.
           (SELECT count(*) FROM lease l
            WHERE l.organization_id = m.organization_id AND l.collector_staff_user_id = m.staff_user_id
              AND l.status = 'active') AS collector_lease_count,
           CASE WHEN u.phone LIKE '+62%' THEN '0' || substr(u.phone, 4) ELSE u.phone END AS national_phone
    FROM membership m
    JOIN staff_user u ON u.id = m.staff_user_id
    WHERE m.organization_id = @organization_id
)
SELECT mm.id, mm.staff_user_id, mm.status, mm.created_at, mm.invitation_state, mm.invitation_last_sent_at,
       mm.first_sign_in_at, mm.phone, mm.name, mm.collector_lease_count
FROM mm
WHERE (sqlc.narg('q')::text IS NULL
       OR mm.name ILIKE sqlc.narg('q')
       OR mm.national_phone LIKE sqlc.narg('phone_national')::text
       OR mm.national_phone LIKE sqlc.narg('phone_digits')::text)
  AND (sqlc.narg('status')::text IS NULL OR mm.status = sqlc.narg('status'))
  AND (NOT @collector_disabled::boolean OR (mm.status = 'disabled' AND mm.collector_lease_count > 0))
ORDER BY CASE WHEN @sort::text = 'name' THEN mm.name END,
         CASE WHEN @sort::text = '-name' THEN mm.name END DESC,
         CASE WHEN @sort::text = 'createdAt' THEN mm.created_at END,
         CASE WHEN @sort::text = '-createdAt' THEN mm.created_at END DESC,
         mm.id
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMemberPage :one
-- Every member ListMemberPage would return on any page: keep the WHERE the same.
WITH mm AS (
    SELECT m.id, m.staff_user_id, m.status, m.created_at, m.invitation_state, m.invitation_last_sent_at,
           m.first_sign_in_at, u.phone, u.name,
           -- Active leases naming the member as collector, organization-wide: a number, no names.
           (SELECT count(*) FROM lease l
            WHERE l.organization_id = m.organization_id AND l.collector_staff_user_id = m.staff_user_id
              AND l.status = 'active') AS collector_lease_count,
           CASE WHEN u.phone LIKE '+62%' THEN '0' || substr(u.phone, 4) ELSE u.phone END AS national_phone
    FROM membership m
    JOIN staff_user u ON u.id = m.staff_user_id
    WHERE m.organization_id = @organization_id
)
SELECT count(*)
FROM mm
WHERE (sqlc.narg('q')::text IS NULL
       OR mm.name ILIKE sqlc.narg('q')
       OR mm.national_phone LIKE sqlc.narg('phone_national')::text
       OR mm.national_phone LIKE sqlc.narg('phone_digits')::text)
  AND (sqlc.narg('status')::text IS NULL OR mm.status = sqlc.narg('status'))
  AND (NOT @collector_disabled::boolean OR (mm.status = 'disabled' AND mm.collector_lease_count > 0));

-- name: CreateMembership :one
-- Returns no row when the staff user is already a member.
INSERT INTO membership (id, organization_id, staff_user_id)
VALUES ($1, $2, $3)
ON CONFLICT (organization_id, staff_user_id) DO NOTHING
RETURNING *;

-- name: ListRoleAssignmentsForMembership :many
SELECT ra.id, ra.builtin_role, ra.role_id, ra.property_id, r.name AS role_name, r.policy AS role_policy,
       p.name AS property_name
FROM role_assignment ra
LEFT JOIN role r ON r.id = ra.role_id
LEFT JOIN property p ON p.organization_id = ra.organization_id AND p.id = ra.property_id
WHERE ra.organization_id = $1 AND ra.membership_id = $2
ORDER BY ra.created_at, ra.id;

-- name: ListOrgRoleAssignments :many
SELECT ra.membership_id, ra.builtin_role, ra.role_id, ra.property_id, r.name AS role_name,
       p.name AS property_name
FROM role_assignment ra
LEFT JOIN role r ON r.id = ra.role_id
LEFT JOIN property p ON p.organization_id = ra.organization_id AND p.id = ra.property_id
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

-- name: LockOrganizationMembers :exec
-- Serializes changes to who is an active Admin of the organization: a second caller waits
-- here until the first commits, and its next statement sees the committed memberships.
-- FOR NO KEY UPDATE, not FOR UPDATE: it does not block the FOR KEY SHARE locks that foreign
-- keys take, so inserts that reference the organization (such as the audit row of a denial,
-- written in its own transaction while this lock is held) never wait on it.
SELECT id FROM organization WHERE id = $1 FOR NO KEY UPDATE;

-- name: CountOtherActiveOrgAdmins :one
-- Active members other than the given one that hold the Admin role for the whole organization.
SELECT count(*) FROM membership m
WHERE m.organization_id = $1 AND m.status = 'active' AND m.id <> sqlc.arg(excluded_membership_id)
  AND EXISTS (
      SELECT 1 FROM role_assignment ra
      WHERE ra.organization_id = m.organization_id AND ra.membership_id = m.id
        AND ra.builtin_role = 'admin' AND ra.property_id IS NULL
  );

-- name: SetMembershipStatus :execrows
-- Roles are kept, so re-enabling restores the same access.
UPDATE membership SET status = sqlc.arg(status)
WHERE organization_id = $1 AND id = $2;

-- name: SetMembershipFirstSignIn :exec
-- Only the first time the member opens the organization is kept.
UPDATE membership SET first_sign_in_at = $3
WHERE organization_id = $1 AND id = $2 AND first_sign_in_at IS NULL;

-- name: SetMembershipInvitation :execrows
-- Records an invitation's state. A NULL last_sent_at keeps the time of the last send that
-- reached WhatsApp, so a failed resend does not erase it.
UPDATE membership
SET invitation_state = sqlc.arg(state)::text,
    invitation_last_sent_at = COALESCE(sqlc.narg(last_sent_at), invitation_last_sent_at)
WHERE organization_id = $1 AND id = $2;

-- name: ListCollectorCandidates :many
-- Active members a lease's collector can be picked from, karyawan first, then by name.
SELECT m.id, u.name,
       EXISTS (SELECT 1 FROM role_assignment ra
               WHERE ra.organization_id = m.organization_id AND ra.membership_id = m.id
                 AND ra.builtin_role = 'karyawan') AS karyawan
FROM membership m
JOIN staff_user u ON u.id = m.staff_user_id
WHERE m.organization_id = $1 AND m.status = 'active'
ORDER BY karyawan DESC, u.name, m.id;
