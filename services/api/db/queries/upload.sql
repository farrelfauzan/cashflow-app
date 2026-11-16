-- Uploads (S6-1): staff photos in object storage, scanned before anyone can read them. Runs in an
-- organization scope (store.WithOrgTx).

-- name: InsertUpload :one
INSERT INTO upload (id, organization_id, purpose, tenant_id, object_key, declared_size, uploaded_by_staff_user_id, created_at)
VALUES (@id, @organization_id, @purpose, sqlc.narg(tenant_id), @object_key, @declared_size, @uploaded_by_staff_user_id::uuid, @created_at)
RETURNING *;

-- name: InsertTransferProofUpload :one
-- A tenant's transfer proof (S6-5): no staff uploader, and completed when it is recorded, since
-- the API stored the file itself before recording it.
INSERT INTO upload (id, organization_id, purpose, tenant_id, object_key, declared_size, created_at, completed_at)
VALUES (@id, @organization_id, 'transfer_proof', @tenant_id::uuid, @object_key, @declared_size, @created_at, @created_at)
RETURNING *;

-- name: InsertTransferProof :one
-- A proof the tenant sent from a payment link (S6-5), with the page's requestId when it sent one.
INSERT INTO transfer_proof (id, organization_id, tenant_id, payment_link_id, invoice_id, upload_id, created_at, request_id)
VALUES (@id, @organization_id, @tenant_id, @payment_link_id::uuid, @invoice_id::uuid, @upload_id, @created_at, sqlc.narg(request_id))
RETURNING *;

-- name: GetTransferProofByRequest :one
-- The proof a tenant's sendPaymentProof already stored under a requestId.
SELECT * FROM transfer_proof
WHERE organization_id = @organization_id AND request_id = @request_id::uuid;

-- name: InsertStaffTransferProof :execrows
-- A proof a member sent for the tenant (decision 49): no link or invoice. An upload already
-- behind a proof is left as it is (0 rows), so completing the upload again changes nothing.
INSERT INTO transfer_proof (id, organization_id, tenant_id, upload_id, sent_by_staff_user_id, created_at)
VALUES (@id, @organization_id, @tenant_id, @upload_id, @sent_by_staff_user_id::uuid, @created_at)
ON CONFLICT (upload_id) DO NOTHING;

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

-- name: ListWaitingTransferProofs :many
-- A tenant's newest transfer proofs still waiting for the admin, for the payment page: those whose
-- photo the scan has not refused (pending or clean) and that Antrean cek has not answered (S6-6).
SELECT tp.id, tp.created_at
FROM transfer_proof tp
JOIN upload u ON u.organization_id = tp.organization_id AND u.id = tp.upload_id
WHERE tp.organization_id = @organization_id AND tp.tenant_id = @tenant_id
  AND u.status IN ('pending', 'clean')
  AND NOT EXISTS (SELECT 1 FROM check_resolution r
                  WHERE r.organization_id = tp.organization_id AND r.transfer_proof_id = tp.id)
ORDER BY tp.created_at DESC, tp.id DESC
LIMIT @max_rows;

-- name: LockUploadShared :one
-- An upload, locked against upload:cleanup until the transaction ends: linking it as a payment's
-- photo takes this lock first, then checks UploadDeleted in a statement of its own, so it sees a
-- deletion the job committed while it waited.
SELECT * FROM upload
WHERE organization_id = @organization_id AND id = @id
FOR SHARE;

-- name: UploadDeleted :one
-- Whether upload:cleanup deleted the upload's file.
SELECT EXISTS (SELECT 1 FROM upload_deletion
               WHERE organization_id = @organization_id AND upload_id = @upload_id) AS deleted;

-- name: ListUnusedCashReceipts :many
-- Clean receipt photos made before @before that back no payment and whose file is still stored,
-- oldest first: the photos of cash that was never recorded (decision 50), for upload:cleanup.
SELECT u.id, u.object_key
FROM upload u
WHERE u.organization_id = @organization_id AND u.purpose = 'cash_receipt' AND u.status = 'clean'
  AND u.created_at < @before::timestamptz
  AND NOT EXISTS (SELECT 1 FROM payment_receipt_photo p
                  WHERE p.organization_id = u.organization_id AND p.upload_id = u.id)
  AND NOT EXISTS (SELECT 1 FROM upload_deletion d
                  WHERE d.organization_id = u.organization_id AND d.upload_id = u.id)
ORDER BY u.created_at, u.id
LIMIT @row_limit;

-- name: InsertUploadDeletion :execrows
-- Records that upload:cleanup deletes the upload's file, unless a payment links it as its photo
-- (0 rows then, or when it was already recorded). Run after LockUpload, as a statement of its
-- own, so it sees a link committed while the job waited for the lock.
INSERT INTO upload_deletion (upload_id, organization_id, deleted_at)
SELECT sqlc.arg(upload_id)::uuid, sqlc.arg(organization_id)::uuid, sqlc.arg(deleted_at)::timestamptz
WHERE NOT EXISTS (SELECT 1 FROM payment_receipt_photo p
                  WHERE p.organization_id = sqlc.arg(organization_id)::uuid AND p.upload_id = sqlc.arg(upload_id)::uuid)
ON CONFLICT (upload_id) DO NOTHING;
