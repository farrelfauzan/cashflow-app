-- name: CreateOrganization :one
INSERT INTO organization (id, name, display_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganization :one
SELECT * FROM organization WHERE id = $1;
