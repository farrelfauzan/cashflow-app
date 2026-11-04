-- +goose Up

-- Slice 3 (S3-2): tenant payment links and the bank account the payment page shows.

-- The organization's bank account, shown on the tenant payment page for a manual transfer
-- (decision 38): the tenant transfers to it and writes their name in the transfer note, and the
-- admin records the transfer as a manual payment. One per organization, set with cmd/admin until
-- the settings screen exists (decision 39, W28). Setting it again updates the row in place: it is
-- configuration, not money. The app role may not delete it.
CREATE TABLE organization_bank_account (
    organization_id          uuid        PRIMARY KEY REFERENCES organization (id),
    bank_name                text        NOT NULL CONSTRAINT organization_bank_account_bank_name
        CHECK (length(bank_name) BETWEEN 1 AND 60 AND bank_name = btrim(bank_name)),
    -- Digits, spaces and '-', starting and ending with a digit: 5 to 34 characters as typed.
    account_number           text        NOT NULL CONSTRAINT organization_bank_account_number
        CHECK (account_number ~ '^[0-9][0-9 -]{3,32}[0-9]$'),
    account_holder           text        NOT NULL CONSTRAINT organization_bank_account_holder
        CHECK (length(account_holder) BETWEEN 1 AND 100 AND account_holder = btrim(account_holder)),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    -- The member who last set it; NULL when the platform operator set it with cmd/admin.
    updated_by_staff_user_id uuid,
    FOREIGN KEY (organization_id, updated_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);

REVOKE DELETE, TRUNCATE ON organization_bank_account FROM cashflow_app;

ALTER TABLE organization_bank_account ENABLE ROW LEVEL SECURITY;
ALTER TABLE organization_bank_account FORCE ROW LEVEL SECURITY;
CREATE POLICY organization_bank_account_org ON organization_bank_account
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- A link to the tenant payment page, /t/<token> (decision 39). The token is 32 random bytes from
-- crypto/rand in base64url without padding; only the SHA-256 of those 32 bytes is stored, like a
-- staff session, so a link cannot be shown twice and a fresh one is minted for every send. An
-- invoice therefore has many links. A link is valid until 30 days after its invoice is paid
-- (checked in Go against the invoice's derived balance); revoking sets revoked_at on every link
-- of the invoice. Nothing else about a link ever changes, and the app role may update only
-- revoked_at and may not delete a link.
CREATE TABLE payment_link (
    id                       uuid        PRIMARY KEY,
    organization_id          uuid        NOT NULL REFERENCES organization (id),
    invoice_id               uuid        NOT NULL,
    tenant_id                uuid        NOT NULL,
    token_hash               bytea       NOT NULL UNIQUE CONSTRAINT payment_link_token_hash_length
        CHECK (octet_length(token_hash) = 32),
    created_at               timestamptz NOT NULL DEFAULT now(),
    -- The member who minted it ("Salin link bayar"); NULL when the system did (reminders).
    created_by_staff_user_id uuid,
    revoked_at               timestamptz,
    UNIQUE (id, organization_id),
    -- The link's tenant is its invoice's tenant.
    FOREIGN KEY (invoice_id, tenant_id, organization_id) REFERENCES invoice (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (organization_id, created_by_staff_user_id) REFERENCES membership (organization_id, staff_user_id)
);
CREATE INDEX payment_link_invoice ON payment_link (organization_id, invoice_id);

REVOKE UPDATE, DELETE, TRUNCATE ON payment_link FROM cashflow_app;
GRANT UPDATE (revoked_at) ON payment_link TO cashflow_app;

ALTER TABLE payment_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE payment_link FORCE ROW LEVEL SECURITY;
CREATE POLICY payment_link_org ON payment_link
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- The token lookup (decision 39). A tenant opening /t/<token> has no session and no organization
-- yet, so no RLS scope can see the link. This function, run as the role that ran this migration,
-- maps a token hash to the link's organization and id and nothing else; everything after it runs
-- in that organization's scope (WithOrgTx), which also decides whether the link is revoked or
-- expired. It returns revoked links too, so revoked and unknown tokens are told apart only inside
-- the organization scope, and both answer the same 404. There is no third RLS scope.
--
-- payment_link forces row-level security, which holds the function's owner too unless it is a
-- superuser (it is in tests, it should not be in production). So, as active_organization_ids()
-- does for organization (00011_money.sql), the function turns app.payment_link_lookup on for its
-- own duration and one policy lets only the role that ran this migration read links while that
-- setting is on. The app role cannot use the policy whatever it sets. A second SECURITY DEFINER
-- function owned by the migration role that reads payment_link would see every link under the
-- same setting; TestActiveOrganizationIDsIsTheOnlySecurityDefiner fails until such a function is reviewed.
CREATE POLICY payment_link_lookup ON payment_link FOR SELECT TO CURRENT_USER
    USING (current_setting('app.payment_link_lookup', true) = 'on');

-- +goose StatementBegin
CREATE FUNCTION payment_link_lookup(p_token_hash bytea)
    RETURNS TABLE (organization_id uuid, link_id uuid)
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
    SET app.payment_link_lookup = 'on'
AS $$
    SELECT l.organization_id, l.id FROM payment_link l WHERE l.token_hash = p_token_hash
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION payment_link_lookup(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION payment_link_lookup(bytea) TO cashflow_app;

-- +goose Down
DROP FUNCTION payment_link_lookup(bytea);
DROP TABLE payment_link;
DROP TABLE organization_bank_account;
