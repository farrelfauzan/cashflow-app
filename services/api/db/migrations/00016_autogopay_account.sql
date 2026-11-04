-- +goose Up

-- Each organization's own AutoGoPay account (slice 3): QRIS money goes to the landlord's account,
-- so every organization stores its own API key. The key is encrypted in Go (internal/secret,
-- AES-256-GCM under SECRETS_KEY, bound to this organization by its additional data); this table
-- holds only the ciphertext and the id of the master key that made it. Neither is ever logged or
-- returned by the API.
--
-- AutoGoPay's webhook carries no account id, so each organization gets its own webhook URL,
-- /webhooks/autogopay/<webhook_id>: 32 random bytes from crypto/rand in base64url without padding
-- (43 characters), made in Go once and kept when the key is replaced. The lookup from webhook_id
-- to the organization is not part of this migration.
--
-- Setting the key again updates the row in place: it is configuration, not money. The app role
-- may not delete it.
CREATE TABLE autogopay_account (
    organization_id          uuid        PRIMARY KEY REFERENCES organization (id),
    -- nonce (12 bytes) || sealed key || GCM tag (16 bytes), so at least 29 bytes.
    api_key_ciphertext       bytea       NOT NULL CONSTRAINT autogopay_account_ciphertext_length
        CHECK (octet_length(api_key_ciphertext) >= 29),
    key_id                   text        NOT NULL CONSTRAINT autogopay_account_key_id_shape
        CHECK (key_id ~ '^[a-z0-9-]{1,32}$'),
    webhook_id               text        NOT NULL UNIQUE CONSTRAINT autogopay_account_webhook_id_shape
        CHECK (webhook_id ~ '^[A-Za-z0-9_-]{43}$'),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    -- The member who last set the key; NULL when the platform operator set it with cmd/admin.
    updated_by_staff_user_id uuid,
    FOREIGN KEY (organization_id, updated_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);

REVOKE DELETE, TRUNCATE ON autogopay_account FROM cashflow_app;

ALTER TABLE autogopay_account ENABLE ROW LEVEL SECURITY;
ALTER TABLE autogopay_account FORCE ROW LEVEL SECURITY;
CREATE POLICY autogopay_account_org ON autogopay_account
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- +goose Down
DROP TABLE autogopay_account;
