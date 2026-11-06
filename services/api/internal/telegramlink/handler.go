package telegramlink

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// The bot's replies, plain text (Telegram reads no markup in them).
const (
	replyLinkedFormat        = "Terhubung. Pesan dari %s untuk %s akan dikirim ke sini."
	replyAlreadyLinkedFormat = "Sudah terhubung. Pesan dari %s untuk %s akan dikirim ke sini."
	replyInvalidCode         = "Link ini sudah tidak berlaku. Buka lagi halaman pembayaran dari link tagihan Anda, lalu tekan Hubungkan Telegram."
	replyHowToLink           = "Untuk menerima pesan tagihan di sini, buka halaman pembayaran dari link tagihan Anda, lalu tekan Hubungkan Telegram."
	replyStopped             = "Berhenti. Pesan tidak lagi dikirim ke Telegram."
	replyHelp                = "Bot ini mengirim pesan tagihan sewa. Untuk menghubungkan, buka halaman pembayaran dari link tagihan Anda, lalu tekan Hubungkan Telegram. Ketik /stop untuk berhenti."
	replyFailed              = "Maaf, terjadi kesalahan. Coba lagi sebentar lagi."
)

// Outcomes, for logs.
const (
	outcomeIgnored       = "ignored"
	outcomeLinked        = "linked"
	outcomeAlreadyLinked = "already_linked"
	outcomeInvalidCode   = "invalid_code"
	outcomeUsedCode      = "used_code"
	outcomeExpiredCode   = "expired_code"
	outcomeNoCode        = "no_code"
	outcomeUnlinked      = "unlinked"
	outcomeNotLinked     = "not_linked"
	outcomeHelp          = "help"
	outcomeFailed        = "failed"
)

// Replier sends the bot's plain-text replies. telegram.Client implements it.
type Replier interface {
	SendText(ctx context.Context, chatID int64, text string) (messaging.Receipt, error)
}

// Handler is the bot's one update handler (telegram.UpdateHandler), used by the webhook and by
// the development polling runner alike.
type Handler struct {
	store *store.Store
	bot   Replier
	clock clock.Clock
	log   *slog.Logger
}

var _ telegram.UpdateHandler = (*Handler)(nil)

// NewHandler returns the update handler. bot sends its replies; c decides whether a code expired.
func NewHandler(st *store.Store, bot Replier, c clock.Clock, log *slog.Logger) *Handler {
	return &Handler{store: st, bot: bot, clock: c, log: log}
}

// HandleUpdate answers one update. Only text messages in a private chat from a person are read;
// anything else (an edit, a group, a channel post, another bot) is ignored. "/start <code>"
// links the chat to the code's tenant, "/stop" unlinks the chat everywhere, and anything else
// gets a short help reply.
//
// It is safe to deliver an update twice: a /start replayed after it succeeded finds its code used
// and answers "already linked" (or, once the link was replaced or stopped, that the link no longer
// works), and /stop unlinks nothing more. The reply is sent after the change commits. An error
// means the database or the reply failed; the caller logs it and moves on.
func (h *Handler) HandleUpdate(ctx context.Context, u telegram.Update) error {
	m := u.Message
	if m == nil || m.Chat.Type != "private" || m.From == nil || m.From.IsBot || m.Chat.ID == 0 {
		h.logOutcome(ctx, u, "none", outcomeIgnored, uuid.Nil)
		return nil
	}
	cmd := parseCommand(m.Text)
	var (
		reply, outcome string
		org            uuid.UUID
		err            error
	)
	switch cmd.kind {
	case cmdStart:
		if cmd.arg == "" {
			reply, outcome = replyHowToLink, outcomeNoCode
			break
		}
		reply, outcome, org, err = h.start(ctx, m.Chat.ID, m.From.ID, cmd.arg)
	case cmdStop:
		reply, outcome, err = h.stop(ctx, m.Chat.ID)
	default:
		reply, outcome = replyHelp, outcomeHelp
	}
	if err != nil {
		h.logOutcome(ctx, u, cmd.kind.String(), outcomeFailed, org)
		if _, sendErr := h.bot.SendText(ctx, m.Chat.ID, replyFailed); sendErr != nil {
			err = errors.Join(err, fmt.Errorf("replying: %w", sendErr))
		}
		return fmt.Errorf("telegramlink: handling %s: %w", cmd.kind, err)
	}
	h.logOutcome(ctx, u, cmd.kind.String(), outcome, org)
	if _, err := h.bot.SendText(ctx, m.Chat.ID, reply); err != nil {
		return fmt.Errorf("telegramlink: replying to %s: %w", cmd.kind, err)
	}
	return nil
}

// logOutcome logs the update's id and kind (never its chat id or text), the command and what
// happened.
func (h *Handler) logOutcome(ctx context.Context, u telegram.Update, cmd, outcome string, org uuid.UUID) {
	attrs := []any{"update", u, "command", cmd, "outcome", outcome}
	if org != uuid.Nil {
		attrs = append(attrs, "organization_id", org)
	}
	h.log.InfoContext(ctx, "telegram update handled", attrs...)
}

// start links chatID to the tenant of code. A malformed or unknown code, a used one (unless this
// chat's link from it is still live) and an expired one all get the same reply.
func (h *Handler) start(ctx context.Context, chatID, userID int64, code string) (reply, outcome string, org uuid.UUID, err error) {
	hash, ok := codeHash(code)
	if !ok {
		return replyInvalidCode, outcomeInvalidCode, uuid.Nil, nil
	}
	org, codeID, err := h.store.LookupTelegramLinkCode(ctx, hash)
	if errors.Is(err, store.ErrTelegramLinkCodeNotFound) {
		return replyInvalidCode, outcomeInvalidCode, uuid.Nil, nil
	}
	if err != nil {
		return "", "", uuid.Nil, err
	}
	err = h.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		reply, outcome, err = h.startInOrg(ctx, tx, org, codeID, chatID, userID)
		return err
	})
	if err != nil {
		return "", "", org, err
	}
	return reply, outcome, org, nil
}

// startInOrg uses the code inside its organization's scope.
func (h *Handler) startInOrg(ctx context.Context, tx pgx.Tx, org, codeID uuid.UUID, chatID, userID int64) (reply, outcome string, err error) {
	q := dbgen.New(tx)
	c, err := q.LockTelegramLinkCode(ctx, dbgen.LockTelegramLinkCodeParams{OrganizationID: org, ID: codeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return replyInvalidCode, outcomeInvalidCode, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("telegramlink: loading the code: %w", err)
	}
	now := h.clock.Now()
	format := replyLinkedFormat
	switch {
	case c.UsedAt != nil:
		link, err := q.GetTelegramLinkByCode(ctx, dbgen.GetTelegramLinkByCodeParams{OrganizationID: org, LinkCodeID: codeID})
		if errors.Is(err, pgx.ErrNoRows) {
			return replyInvalidCode, outcomeUsedCode, nil
		}
		if err != nil {
			return "", "", fmt.Errorf("telegramlink: loading the code's link: %w", err)
		}
		if link.UnlinkedAt != nil || link.ChatID != chatID {
			return replyInvalidCode, outcomeUsedCode, nil
		}
		format, outcome = replyAlreadyLinkedFormat, outcomeAlreadyLinked
	case !now.Before(c.ExpiresAt):
		return replyInvalidCode, outcomeExpiredCode, nil
	default:
		if err := h.link(ctx, q, org, c.ID, c.TenantID, chatID, userID, now); err != nil {
			return "", "", err
		}
		outcome = outcomeLinked
	}
	o, err := q.GetOrganization(ctx, org)
	if err != nil {
		return "", "", fmt.Errorf("telegramlink: loading the organization: %w", err)
	}
	t, err := q.GetTenant(ctx, dbgen.GetTenantParams{OrganizationID: org, ID: c.TenantID})
	if err != nil {
		return "", "", fmt.Errorf("telegramlink: loading the tenant: %w", err)
	}
	return fmt.Sprintf(format, o.DisplayName, t.Name), outcome, nil
}

// link marks the code used and makes the tenant's live link, replacing an older one.
func (h *Handler) link(ctx context.Context, q *dbgen.Queries, org, codeID, tenantID uuid.UUID, chatID, userID int64, now time.Time) error {
	if err := q.LockTenantTelegramLink(ctx, tenantID); err != nil {
		return fmt.Errorf("telegramlink: locking the tenant's link: %w", err)
	}
	n, err := q.UseTelegramLinkCode(ctx, dbgen.UseTelegramLinkCodeParams{UsedAt: &now, OrganizationID: org, ID: codeID})
	if err != nil {
		return fmt.Errorf("telegramlink: using the code: %w", err)
	}
	if n != 1 {
		// The code is locked and was unused when read, so this cannot happen.
		return errors.New("telegramlink: the code was used meanwhile")
	}
	if _, err := q.UnlinkTenantTelegram(ctx, dbgen.UnlinkTenantTelegramParams{UnlinkedAt: &now, OrganizationID: org, TenantID: tenantID}); err != nil {
		return fmt.Errorf("telegramlink: unlinking the older link: %w", err)
	}
	err = q.InsertTelegramLink(ctx, dbgen.InsertTelegramLinkParams{
		ID: uuid.New(), OrganizationID: org, TenantID: tenantID, LinkCodeID: codeID,
		ChatID: chatID, TelegramUserID: userID, LinkedAt: now,
	})
	if err != nil {
		return fmt.Errorf("telegramlink: storing the link: %w", err)
	}
	return nil
}

// stop unlinks every live link of chatID, in each organization that has one.
func (h *Handler) stop(ctx context.Context, chatID int64) (reply, outcome string, err error) {
	orgs, err := h.store.TelegramChatOrganizations(ctx, chatID)
	if err != nil {
		return "", "", err
	}
	now := h.clock.Now()
	var unlinked int64
	for _, org := range orgs {
		err := h.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			n, err := dbgen.New(tx).UnlinkTelegramChat(ctx, dbgen.UnlinkTelegramChatParams{UnlinkedAt: &now, OrganizationID: org, ChatID: chatID})
			unlinked += n
			return err
		})
		if err != nil {
			return "", "", fmt.Errorf("telegramlink: unlinking a chat: %w", err)
		}
	}
	if unlinked == 0 {
		return replyStopped, outcomeNotLinked, nil
	}
	return replyStopped, outcomeUnlinked, nil
}
