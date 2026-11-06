-- +goose Up

-- Telegram demo channel for staff (decision 42, TG-4): a signed-in test staff member links their
-- Telegram chat from the app (POST /api/me/telegram) and their login codes then arrive there when
-- TELEGRAM_MESSAGES=linked. The app answers the bot's deep link, https://t.me/<bot>?start=<code>;
-- Telegram sends "/start <code>" from the staff member's chat to the bot, and the bot's update
-- handler stores the chat for that staff user. As for tenants (00025), the chat id is never matched
-- from a phone number, and the code is fresh randomness.
--
-- Staff users are platform-level (one person across organizations), so these tables have no
-- organization_id and live in the auth scope, like otp_challenge and staff_session (00002): only
-- WithAuthTx (the auth package) reads or writes them. The bot's /start finds a staff code by its
-- hash inside that scope, so no SECURITY DEFINER lookup is needed.

-- A one-time linking code: 32 random bytes in base64url without padding; only the SHA-256 of those
-- bytes is stored (256 bits of randomness, so a keyed HMAC adds nothing, as for 00025). Valid for
-- 10 minutes and used once; several may be live for a staff user. Nothing about a code changes
-- except being used, and the app role may update only used_at and may not delete a code.
CREATE TABLE telegram_staff_link_code (
    id            uuid        PRIMARY KEY,
    staff_user_id uuid        NOT NULL REFERENCES staff_user (id),
    code_hash     bytea       NOT NULL UNIQUE CONSTRAINT telegram_staff_link_code_hash_length
        CHECK (octet_length(code_hash) = 32),
    created_at    timestamptz NOT NULL,
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz,
    CONSTRAINT telegram_staff_link_code_expiry CHECK (expires_at > created_at),
    UNIQUE (id, staff_user_id)
);

REVOKE UPDATE, DELETE, TRUNCATE ON telegram_staff_link_code FROM cashflow_app;
GRANT UPDATE (used_at) ON telegram_staff_link_code TO cashflow_app;

ALTER TABLE telegram_staff_link_code ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_staff_link_code FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_staff_link_code_auth ON telegram_staff_link_code
    USING (app_in_auth_scope());

-- A staff user's Telegram chat. One live link per staff user (unlinked_at IS NULL); a new link
-- unlinks the older one in the same transaction, and /stop unlinks every live link of its chat. A
-- chat may hold live links to several staff users (one person testing several accounts).
-- link_code_id is the code that made the link, and the link's staff user is the code's, so a /start
-- replayed after it succeeded answers "already linked". Nothing about a link changes except being
-- unlinked, and the app role may update only unlinked_at and may not delete a link.
CREATE TABLE telegram_staff_link (
    id               uuid        PRIMARY KEY,
    staff_user_id    uuid        NOT NULL REFERENCES staff_user (id),
    link_code_id     uuid        NOT NULL UNIQUE,
    -- Telegram ids can need more than 32 bits.
    chat_id          bigint      NOT NULL,
    telegram_user_id bigint      NOT NULL,
    linked_at        timestamptz NOT NULL,
    unlinked_at      timestamptz,
    CONSTRAINT telegram_staff_link_unlinked_after_linked CHECK (unlinked_at IS NULL OR unlinked_at >= linked_at),
    FOREIGN KEY (link_code_id, staff_user_id) REFERENCES telegram_staff_link_code (id, staff_user_id)
);
CREATE UNIQUE INDEX telegram_staff_link_live_user ON telegram_staff_link (staff_user_id) WHERE unlinked_at IS NULL;
CREATE INDEX telegram_staff_link_live_chat ON telegram_staff_link (chat_id) WHERE unlinked_at IS NULL;

REVOKE UPDATE, DELETE, TRUNCATE ON telegram_staff_link FROM cashflow_app;
GRANT UPDATE (unlinked_at) ON telegram_staff_link TO cashflow_app;

ALTER TABLE telegram_staff_link ENABLE ROW LEVEL SECURITY;
ALTER TABLE telegram_staff_link FORCE ROW LEVEL SECURITY;
CREATE POLICY telegram_staff_link_auth ON telegram_staff_link
    USING (app_in_auth_scope());

-- +goose Down
DROP TABLE telegram_staff_link;
DROP TABLE telegram_staff_link_code;
