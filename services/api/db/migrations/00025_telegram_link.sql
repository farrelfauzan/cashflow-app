-- +goose Up

-- Telegram demo channel (decision 42, TG-2): a tenant links their Telegram chat from the payment
-- page. The page asks for a one-time code (POST /api/t/{token}/telegram) and opens the bot's deep
-- link, https://t.me/<bot>?start=<code>; Telegram then sends "/start <code>" from the tenant's
-- chat to the bot, and the bot's update handler stores the chat for that tenant. The chat id is
-- never matched from a phone number, and the code is never the payment link token or anything
-- derived from it.

-- A one-time linking code. The code is 32 random bytes from crypto/rand in base64url without
-- padding; only the SHA-256 of those 32 bytes is stored, as for payment links (00017): with 256
-- bits of randomness a keyed HMAC adds nothing, since nobody can guess a code from its hash (the
-- login code needs one because six digits can be tried in a second). A code is valid for 10
-- minutes and used once; several may be live for a tenant. Nothing about a code changes except
-- being used, and the app role may update only used_at and may not delete a code.
CREATE TABLE telegram_link_code (
    id              uuid        PRIMARY KEY,
    organization_id uuid        NOT NULL REFERENCES organization (id),
    tenant_id       uuid        NOT NULL,
    code_hash       bytea       NOT NULL UNIQUE CONSTRAINT telegram_link_code_hash_length
        CHECK (octet_length(code_hash) = 32),
    created_at      timestamptz NOT NULL,
    expires_at      timestamptz NOT NULL,
    used_at         timestamptz,
    CONSTRAINT telegram_link_code_expiry CHECK (expires_at > created_at),
    UNIQUE (id, organization_id),
    UNIQUE (id, tenant_id, organization_id),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id)
);

REVOKE UPDATE, DELETE, TRUNCATE ON telegram_link_code FROM cashflow_app;
GRANT UPDATE (used_at) ON telegram_link_code TO cashflow_app;

ALTER TABLE telegram_link_code ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_link_code FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_link_code_org ON telegram_link_code
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- A tenant's Telegram chat. One live link per tenant (unlinked_at IS NULL); a new link unlinks the
-- older one in the same transaction, and /stop unlinks every live link of its chat. A chat may hold
-- live links to tenants of several organizations, so the organization always comes from the link
-- or the code, never from the chat alone. link_code_id is the code that made the link (and its
-- tenant is the code's tenant), so a /start replayed after it succeeded answers "already linked".
-- Nothing about a link changes except being unlinked, and the app role may update only
-- unlinked_at and may not delete a link.
CREATE TABLE telegram_link (
    id               uuid        PRIMARY KEY,
    organization_id  uuid        NOT NULL REFERENCES organization (id),
    tenant_id        uuid        NOT NULL,
    link_code_id     uuid        NOT NULL UNIQUE,
    -- Telegram ids can need more than 32 bits.
    chat_id          bigint      NOT NULL,
    telegram_user_id bigint      NOT NULL,
    linked_at        timestamptz NOT NULL,
    unlinked_at      timestamptz,
    CONSTRAINT telegram_link_unlinked_after_linked CHECK (unlinked_at IS NULL OR unlinked_at >= linked_at),
    FOREIGN KEY (tenant_id, organization_id) REFERENCES tenant (id, organization_id),
    FOREIGN KEY (link_code_id, tenant_id, organization_id) REFERENCES telegram_link_code (id, tenant_id, organization_id)
);
CREATE UNIQUE INDEX telegram_link_live_tenant ON telegram_link (organization_id, tenant_id) WHERE unlinked_at IS NULL;
CREATE INDEX telegram_link_live_chat ON telegram_link (chat_id) WHERE unlinked_at IS NULL;

REVOKE UPDATE, DELETE, TRUNCATE ON telegram_link FROM cashflow_app;
GRANT UPDATE (unlinked_at) ON telegram_link TO cashflow_app;

ALTER TABLE telegram_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_link FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_link_org ON telegram_link
    USING (organization_id = app_org_id())
    WITH CHECK (organization_id = app_org_id());

-- The two lookups the bot's update handler needs before it knows the organization (decision 39's
-- pattern). An update from Telegram has no session and no organization: "/start <code>" carries
-- only the code, and "/stop" only the chat. Each function, run as the role that ran this
-- migration, maps its argument to ids and nothing else; everything after it runs in that
-- organization's scope (WithOrgTx).
--
-- Both tables force row-level security, which holds the functions' owner too unless it is a
-- superuser, so as payment_link_lookup does (00017_payment_link.sql) each function turns its own
-- setting on for its duration and one policy lets only the role that ran this migration read its
-- one table while that setting is on. The app role cannot use the policies whatever it sets. The
-- other SECURITY DEFINER functions (active_organization_ids, payment_link_lookup,
-- autogopay_webhook_lookup) read neither table, and each of these reads only its own table, so no
-- policy widens what another function can see; TestActiveOrganizationIDsIsTheOnlySecurityDefiner
-- lists all five.

-- telegram_link_code_lookup maps a code hash to its code's organization and id, whether or not the
-- code was used or has expired: those are decided inside the organization scope with the
-- application's clock, so a /start replayed after it succeeded can answer "already linked".
CREATE POLICY telegram_link_code_lookup ON telegram_link_code FOR SELECT TO CURRENT_USER
    USING (current_setting('app.telegram_link_code_lookup', true) = 'on');

-- +goose StatementBegin
CREATE FUNCTION telegram_link_code_lookup(p_code_hash bytea)
    RETURNS TABLE (organization_id uuid, code_id uuid)
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
    SET app.telegram_link_code_lookup = 'on'
AS $$
    SELECT c.organization_id, c.id FROM telegram_link_code c WHERE c.code_hash = p_code_hash
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION telegram_link_code_lookup(bytea) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION telegram_link_code_lookup(bytea) TO cashflow_app;

-- telegram_chat_organizations lists the organizations where a chat has a live link, for /stop,
-- which then unlinks that chat inside each organization's scope.
CREATE POLICY telegram_link_chat_organizations ON telegram_link FOR SELECT TO CURRENT_USER
    USING (current_setting('app.telegram_chat_organizations', true) = 'on');

-- +goose StatementBegin
CREATE FUNCTION telegram_chat_organizations(p_chat_id bigint)
    RETURNS TABLE (organization_id uuid)
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path = pg_catalog, public
    SET app.telegram_chat_organizations = 'on'
AS $$
    SELECT DISTINCT l.organization_id FROM telegram_link l
    WHERE l.chat_id = p_chat_id AND l.unlinked_at IS NULL
    ORDER BY l.organization_id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION telegram_chat_organizations(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION telegram_chat_organizations(bigint) TO cashflow_app;

-- +goose Down
DROP FUNCTION telegram_chat_organizations(bigint);
DROP FUNCTION telegram_link_code_lookup(bytea);
DROP TABLE telegram_link;
DROP TABLE telegram_link_code;
