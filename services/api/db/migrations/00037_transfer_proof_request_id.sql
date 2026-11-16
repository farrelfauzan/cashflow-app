-- +goose Up

-- The tenant's "Kirim bukti transfer" (sendPaymentProof) may carry a requestId: the page makes
-- one per photo it sends and sends it again unchanged on a retry, so a retry after a lost answer
-- stores one proof. A member's proof (decision 49) has none: it is already one per upload.
ALTER TABLE transfer_proof ADD COLUMN request_id uuid;
CREATE UNIQUE INDEX transfer_proof_request_id_unique ON transfer_proof (organization_id, request_id)
    WHERE request_id IS NOT NULL;

-- +goose Down
DROP INDEX transfer_proof_request_id_unique;
ALTER TABLE transfer_proof DROP COLUMN request_id;
