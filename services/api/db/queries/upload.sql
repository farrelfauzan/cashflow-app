-- Uploads (S6-1): staff photos in object storage, scanned before anyone can read them. Runs in an
-- organization scope (store.WithOrgTx).

-- name: InsertUpload :one
INSERT INTO upload (id, organization_id, purpose, tenant_id, object_key, declared_size, uploaded_by_staff_user_id, created_at)
VALUES (@id, @organization_id, @purpose, sqlc.narg(tenant_id), @object_key, @declared_size, @uploaded_by_staff_user_id, @created_at)
RETURNING *;

-- name: GetUpload :one
SELECT * FROM upload
WHERE organization_id = @organization_id AND id = @id;

-- name: MarkUploadCompleted :execrows
-- The client says its PUT finished. Only the first call sets the time (0 rows after that, and
-- for an upload already scanned).
UPDATE upload
SET completed_at = @completed_at::timestamptz
WHERE organization_id = @organization_id AND id = @id AND completed_at IS NULL AND status = 'pending';

-- name: FinishUploadScan :execrows
-- Writes the scan's verdict, once: a second run finds the upload no longer pending (0 rows).
UPDATE upload
SET status = @status, content_type = sqlc.narg(content_type), scanned_at = @scanned_at::timestamptz
WHERE organization_id = @organization_id AND id = @id AND status = 'pending' AND completed_at IS NOT NULL;

-- name: ListStaleUploads :many
-- Completed uploads still pending that were completed before @before, oldest first: their scan
-- task was lost, failed every retry, or is waiting for a file that has not arrived.
SELECT id
FROM upload
WHERE organization_id = @organization_id AND status = 'pending' AND completed_at < @before::timestamptz
ORDER BY completed_at, id
LIMIT @row_limit;

-- name: LockUpload :one
-- An upload, locked until the scanning transaction ends, so two scans of it take turns and the
-- second sees what the first decided.
SELECT * FROM upload
WHERE organization_id = @organization_id AND id = @id
FOR UPDATE;
