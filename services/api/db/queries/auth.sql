-- Runs in the auth scope (store.WithAuthTx).

-- name: GetStaffUserByPhone :one
SELECT * FROM staff_user WHERE phone = $1;

-- name: UpsertStaffUser :one
-- Returns the existing user when the phone is already registered. The stored name is kept.
INSERT INTO staff_user (id, phone, name)
VALUES ($1, $2, $3)
ON CONFLICT (phone) DO UPDATE SET phone = EXCLUDED.phone
RETURNING *;

-- name: SupersedeOpenChallenges :exec
UPDATE otp_challenge SET consumed_at = $2
WHERE phone = $1 AND consumed_at IS NULL;

-- name: CreateChallenge :exec
INSERT INTO otp_challenge (id, phone, code_hash, expires_at, created_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetOpenChallengeForUpdate :one
SELECT * FROM otp_challenge
WHERE phone = $1 AND consumed_at IS NULL
ORDER BY created_at DESC
LIMIT 1
FOR UPDATE;

-- name: IncrementChallengeAttempts :exec
UPDATE otp_challenge SET attempts = attempts + 1 WHERE id = $1;

-- name: ConsumeChallenge :exec
UPDATE otp_challenge SET consumed_at = $2 WHERE id = $1;

-- name: CreateSession :exec
INSERT INTO staff_session (id, token_hash, staff_user_id, created_at, last_seen_at, expires_at)
VALUES ($1, $2, $3, $4, $4, $5);

-- name: GetActiveSession :one
SELECT s.id, s.staff_user_id, s.last_seen_at, s.expires_at, u.phone, u.name
FROM staff_session s
JOIN staff_user u ON u.id = s.staff_user_id
WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > $2;

-- name: TouchSession :exec
UPDATE staff_session SET last_seen_at = $2 WHERE id = $1;

-- name: RevokeSession :exec
UPDATE staff_session SET revoked_at = $2 WHERE id = $1 AND revoked_at IS NULL;

-- name: ListMyOrganizations :many
-- Needs app.user_id set: the policies only expose the user's own active memberships.
SELECT o.id, o.name, o.display_name
FROM membership m
JOIN organization o ON o.id = m.organization_id
WHERE m.staff_user_id = $1 AND m.status = 'active'
ORDER BY o.display_name;

-- name: SetFirstSignIn :exec
-- Only the first successful sign-in is kept.
UPDATE staff_user SET first_sign_in_at = $2
WHERE id = $1 AND first_sign_in_at IS NULL;
