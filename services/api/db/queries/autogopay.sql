-- name: UpsertAutogopayAccount :one
-- Stores an organization's encrypted AutoGoPay API key. The first call creates the account with
-- the webhook id passed in; a later call replaces the key and keeps the existing webhook id, so
-- the URL registered at AutoGoPay stays valid. Returns the webhook id in force.
INSERT INTO autogopay_account (
    organization_id, api_key_ciphertext, key_id, webhook_id, created_at, updated_at, updated_by_staff_user_id
) VALUES (
    @organization_id, @api_key_ciphertext, @key_id, @webhook_id, @now, @now, sqlc.narg(updated_by_staff_user_id)
)
ON CONFLICT (organization_id) DO UPDATE
SET api_key_ciphertext       = EXCLUDED.api_key_ciphertext,
    key_id                   = EXCLUDED.key_id,
    updated_at               = EXCLUDED.updated_at,
    updated_by_staff_user_id = EXCLUDED.updated_by_staff_user_id
RETURNING webhook_id;

-- name: GetAutogopayAccount :one
SELECT * FROM autogopay_account WHERE organization_id = @organization_id;
