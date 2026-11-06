-- The Telegram demo channel (decision 42): one-time linking codes and the tenant's linked chat.

-- name: LookupTelegramLinkCode :one
-- Maps a code hash to its code's organization and id, with no organization scope, through the
-- SECURITY DEFINER function telegram_link_code_lookup. Used and expired codes are found too;
-- whether a code still works is decided inside the organization scope.
SELECT c.organization_id::uuid AS organization_id, c.code_id::uuid AS code_id
FROM telegram_link_code_lookup(@code_hash::bytea) AS c;

-- name: TelegramChatOrganizations :many
-- The organizations where a chat has a live link, with no organization scope, through the
-- SECURITY DEFINER function telegram_chat_organizations.
SELECT o.organization_id::uuid AS organization_id
FROM telegram_chat_organizations(@chat_id::bigint) AS o;

-- name: InsertTelegramLinkCode :exec
INSERT INTO telegram_link_code (id, organization_id, tenant_id, code_hash, created_at, expires_at)
VALUES (@id, @organization_id, @tenant_id, @code_hash, @created_at, @expires_at);

-- name: LockTelegramLinkCode :one
-- Locks one code, so a /start delivered twice at once links once.
SELECT id, tenant_id, expires_at, used_at
FROM telegram_link_code
WHERE organization_id = @organization_id AND id = @id
FOR UPDATE;

-- name: UseTelegramLinkCode :execrows
UPDATE telegram_link_code SET used_at = @used_at
WHERE organization_id = @organization_id AND id = @id AND used_at IS NULL;

-- name: LockTenantTelegramLink :exec
-- Serializes linking per tenant, so two codes of one tenant used at once cannot both find no live
-- link and then collide on the one-live-link index. An advisory lock, not a row lock on tenant,
-- so it never waits on payment allocation, which locks the tenant row.
SELECT pg_advisory_xact_lock(hashtextextended('telegram_link:' || CAST(@tenant_id AS uuid)::text, 0));

-- name: UnlinkTenantTelegram :execrows
UPDATE telegram_link SET unlinked_at = @unlinked_at
WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND unlinked_at IS NULL;

-- name: InsertTelegramLink :exec
INSERT INTO telegram_link (id, organization_id, tenant_id, link_code_id, chat_id, telegram_user_id, linked_at)
VALUES (@id, @organization_id, @tenant_id, @link_code_id, @chat_id, @telegram_user_id, @linked_at);

-- name: GetTelegramLinkByCode :one
-- The link a code made, if any.
SELECT chat_id, unlinked_at
FROM telegram_link
WHERE organization_id = @organization_id AND link_code_id = @link_code_id;

-- name: UnlinkTelegramChat :execrows
-- Unlinks every live link of a chat in one organization (/stop).
UPDATE telegram_link SET unlinked_at = @unlinked_at
WHERE organization_id = @organization_id AND chat_id = @chat_id AND unlinked_at IS NULL;

-- name: TenantHasTelegramLink :one
SELECT EXISTS (
    SELECT 1 FROM telegram_link
    WHERE organization_id = @organization_id AND tenant_id = @tenant_id AND unlinked_at IS NULL
) AS linked;
