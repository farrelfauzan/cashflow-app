-- name: CreateOrganization :one
INSERT INTO organization (id, name, display_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetOrganization :one
SELECT * FROM organization WHERE id = $1;

-- name: GetBrokenPromiseThreshold :one
SELECT broken_promise_threshold FROM organization WHERE id = @id;

-- name: SetBrokenPromiseThreshold :execrows
UPDATE organization SET broken_promise_threshold = @threshold WHERE id = @id;
