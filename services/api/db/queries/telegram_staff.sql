-- The Telegram demo channel for staff (decision 42, TG-4): one-time linking codes and the staff
-- user's linked chat. Runs in the auth scope (store.WithAuthTx), from the auth package only.

-- name: InsertTelegramStaffLinkCode :exec
INSERT INTO telegram_staff_link_code (id, staff_user_id, code_hash, created_at, expires_at)
VALUES (@id, @staff_user_id, @code_hash, @created_at, @expires_at);

-- name: LockTelegramStaffLinkCode :one
-- Finds a code by its hash and locks it, so a /start delivered twice at once links once. Used and
-- expired codes are found too; the caller decides with the application's clock. Returns the staff
-- user's name for the bot's reply.
SELECT c.id, c.staff_user_id, c.expires_at, c.used_at, u.name AS staff_name
FROM telegram_staff_link_code c
JOIN staff_user u ON u.id = c.staff_user_id
WHERE c.code_hash = @code_hash
FOR UPDATE OF c;

-- name: UseTelegramStaffLinkCode :execrows
UPDATE telegram_staff_link_code SET used_at = @used_at
WHERE id = @id AND used_at IS NULL;

-- name: LockStaffTelegramLink :exec
-- Serializes linking and unlinking per staff user, so two codes used at once cannot both find no
-- live link and then collide on the one-live-link index. The key differs from the tenant's
-- (telegram_link:), so the two never wait on each other.
SELECT pg_advisory_xact_lock(hashtextextended('telegram_staff_link:' || CAST(@staff_user_id AS uuid)::text, 0));

-- name: UnlinkStaffTelegram :many
-- Ends the staff user's live link and returns the chat it was in, so that chat can be told.
UPDATE telegram_staff_link SET unlinked_at = @unlinked_at
WHERE staff_user_id = @staff_user_id AND unlinked_at IS NULL
RETURNING chat_id;

-- name: InsertTelegramStaffLink :exec
INSERT INTO telegram_staff_link (id, staff_user_id, link_code_id, chat_id, telegram_user_id, linked_at)
VALUES (@id, @staff_user_id, @link_code_id, @chat_id, @telegram_user_id, @linked_at);

-- name: GetTelegramStaffLinkByCode :one
-- The link a code made, if any.
SELECT chat_id, unlinked_at
FROM telegram_staff_link
WHERE link_code_id = @link_code_id;

-- name: UnlinkTelegramStaffChat :execrows
-- Unlinks every live staff link of a chat (/stop).
UPDATE telegram_staff_link SET unlinked_at = @unlinked_at
WHERE chat_id = @chat_id AND unlinked_at IS NULL;

-- name: StaffHasTelegramLink :one
SELECT EXISTS (
    SELECT 1 FROM telegram_staff_link
    WHERE staff_user_id = @staff_user_id AND unlinked_at IS NULL
) AS linked;

-- name: GetStaffTelegramChat :one
-- The chat of the staff user's live link, for the router.
SELECT chat_id
FROM telegram_staff_link
WHERE staff_user_id = @staff_user_id AND unlinked_at IS NULL;

-- name: UnlinkUndeliverableStaffTelegramChat :execrows
-- Ends the staff user's live link in that one chat after Telegram refused a send to it. A link made
-- in another chat since is left alone.
UPDATE telegram_staff_link SET unlinked_at = @unlinked_at
WHERE staff_user_id = @staff_user_id AND chat_id = @chat_id AND unlinked_at IS NULL;
