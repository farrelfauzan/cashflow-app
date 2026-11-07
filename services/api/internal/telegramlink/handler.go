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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink/linkcode"
)

// The bot's replies, plain text (Telegram reads no markup in them).
const (
	replyLinkedFormat        = "Terhubung. Pesan dari %s untuk %s akan dikirim ke sini."
	replyAlreadyLinkedFormat = "Sudah terhubung. Pesan dari %s untuk %s akan dikirim ke sini."
	replyInvalidCode         = "Link ini sudah tidak berlaku. Buka lagi halaman pembayaran dari link tagihan Anda, lalu tekan Hubungkan Telegram."
	replyHowToLink           = "Untuk menerima pesan tagihan di sini, buka halaman pembayaran dari link tagihan Anda, lalu tekan Hubungkan Telegram."
	replyStopped             = "Berhenti. Pesan tidak lagi dikirim ke Telegram."
	replyHelp                = "Bot ini mengirim pesan tagihan sewa dan kode masuk staf. Untuk menghubungkan, buka halaman pembayaran dari link tagihan Anda (penyewa) atau akun Anda di aplikasi (staf), lalu tekan Hubungkan Telegram. Ketik /stop untuk berhenti."
	replyFailed              = "Maaf, terjadi kesalahan. Coba lagi sebentar lagi."
	// noticeReleasedFormat goes to a chat whose link to a tenant was replaced by a newer one (the
	// pay link was opened and linked from another Telegram): demo only, so the replacement is
	// allowed, but the old chat is told.
	noticeReleasedFormat = "Telegram ini dilepas dari tagihan %s. Pesan tagihan sekarang dikirim ke Telegram lain yang dihubungkan dari halaman pembayaran."

	// The replies to a staff member's /start (TG-4): login codes, not bills. A staff code that is
	// used or expired gets its own reply pointing to the app; a code nobody has gets replyInvalidCode
	// as before, since it cannot be told apart from a tenant's.
	replyStaffLinkedFormat        = "Terhubung. Kode masuk untuk %s akan dikirim ke sini."
	replyStaffAlreadyLinkedFormat = "Sudah terhubung. Kode masuk untuk %s akan dikirim ke sini."
	replyStaffInvalidCode         = "Link ini sudah tidak berlaku. Buka lagi aplikasi, lalu tekan Hubungkan Telegram di akun Anda."
	noticeStaffReleasedFormat     = "Telegram ini dilepas dari akun %s. Kode masuk sekarang dikirim ke Telegram lain yang dihubungkan dari aplikasi."
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

// StaffLinker links and unlinks staff users' chats (TG-4). auth.TelegramStaff implements it; the
// auth package owns it because only auth, member and org may use the auth scope.
type StaffLinker interface {
	// Link uses the staff code whose SHA-256 is codeHash to link chatID. A hash no staff code has
	// gives linkcode.Unknown.
	Link(ctx context.Context, codeHash []byte, chatID, telegramUserID int64) (linkcode.StaffLink, error)
	// UnlinkChat ends every live staff link of chatID and returns how many it ended.
	UnlinkChat(ctx context.Context, chatID int64) (int64, error)
}

// Handler is the bot's one update handler (telegram.UpdateHandler), used by the webhook and by
// the development polling runner alike.
type Handler struct {
	store *store.Store
	bot   Replier
	staff StaffLinker
	clock clock.Clock
	log   *slog.Logger
}

var _ telegram.UpdateHandler = (*Handler)(nil)

// NewHandler returns the update handler. bot sends its replies; staff links staff users' chats, and
// may be nil, when /start knows tenants' codes only; c decides whether a tenant's code expired.
func NewHandler(st *store.Store, bot Replier, staff StaffLinker, c clock.Clock, log *slog.Logger) *Handler {
	return &Handler{store: st, bot: bot, staff: staff, clock: c, log: log}
}

// HandleUpdate answers one update. Only text messages in a private chat from a person are read;
// anything else (an edit, a group, a channel post, another bot) is ignored. "/start <code>"
// links the chat to the code's tenant, or, when no tenant has the code, to the staff user whose
// code it is; "/stop" unlinks the chat everywhere, tenants and staff alike; anything else gets a
// short help reply.
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
		rel            released
	)
	switch cmd.kind {
	case cmdStart:
		if cmd.arg == "" {
			reply, outcome = replyHowToLink, outcomeNoCode
			break
		}
		reply, outcome, org, err = h.start(ctx, m.Chat.ID, m.From.ID, cmd.arg, &rel)
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
	// Tell a chat that lost its link to this one. A failed notice is logged, not returned: the
	// link is made and the new chat was answered.
	for _, old := range rel.chats {
		if _, err := h.bot.SendText(ctx, old, rel.notice); err != nil {
			h.log.WarnContext(ctx, "telegram: telling the replaced chat failed", "update", u, "error", err)
		}
	}
	return nil
}

// released is what a /start that replaced a tenant's link leaves to tell: the chats that lost it
// (never the new chat itself) and the notice for them.
type released struct {
	chats  []int64
	notice string
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

// start links chatID to the tenant of code, or else to the staff user of code. A malformed or
// unknown code, a used one (unless this chat's link from it is still live) and an expired one all
// get the same reply for tenants; a staff code that is used or expired gets the staff reply.
func (h *Handler) start(ctx context.Context, chatID, userID int64, code string, rel *released) (reply, outcome string, org uuid.UUID, err error) {
	hash, ok := linkcode.Hash(code)
	if !ok {
		return replyInvalidCode, outcomeInvalidCode, uuid.Nil, nil
	}
	org, codeID, err := h.store.LookupTelegramLinkCode(ctx, hash)
	if errors.Is(err, store.ErrTelegramLinkCodeNotFound) {
		reply, outcome, err = h.startStaff(ctx, hash, chatID, userID, rel)
		return reply, outcome, uuid.Nil, err
	}
	if err != nil {
		return "", "", uuid.Nil, err
	}
	err = h.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		reply, outcome, err = h.startInOrg(ctx, tx, org, codeID, chatID, userID, rel)
		return err
	})
	if err != nil {
		return "", "", org, err
	}
	return reply, outcome, org, nil
}

// startStaff tries hash as a staff user's code, when staff linking is wired. A hash no staff code
// has answers like an unknown tenant code.
func (h *Handler) startStaff(ctx context.Context, hash []byte, chatID, userID int64, rel *released) (reply, outcome string, err error) {
	if h.staff == nil {
		return replyInvalidCode, outcomeInvalidCode, nil
	}
	res, err := h.staff.Link(ctx, hash, chatID, userID)
	if err != nil {
		return "", "", fmt.Errorf("telegramlink: linking a staff chat: %w", err)
	}
	outcome = "staff_" + string(res.Outcome)
	switch res.Outcome {
	case linkcode.Linked:
		for _, id := range res.Released {
			if id != chatID {
				rel.chats = append(rel.chats, id)
			}
		}
		if len(rel.chats) > 0 {
			rel.notice = fmt.Sprintf(noticeStaffReleasedFormat, res.Name)
		}
		return fmt.Sprintf(replyStaffLinkedFormat, res.Name), outcome, nil
	case linkcode.AlreadyLinked:
		return fmt.Sprintf(replyStaffAlreadyLinkedFormat, res.Name), outcome, nil
	case linkcode.Used, linkcode.Expired:
		return replyStaffInvalidCode, outcome, nil
	default:
		return replyInvalidCode, outcomeInvalidCode, nil
	}
}

// startInOrg uses the code inside its organization's scope.
func (h *Handler) startInOrg(ctx context.Context, tx pgx.Tx, org, codeID uuid.UUID, chatID, userID int64, rel *released) (reply, outcome string, err error) {
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
		old, err := h.link(ctx, q, org, c.ID, c.TenantID, chatID, userID, now)
		if err != nil {
			return "", "", err
		}
		for _, id := range old {
			if id != chatID {
				rel.chats = append(rel.chats, id)
			}
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
	if len(rel.chats) > 0 {
		rel.notice = fmt.Sprintf(noticeReleasedFormat, o.DisplayName)
	}
	return fmt.Sprintf(format, o.DisplayName, t.Name), outcome, nil
}

// link marks the code used and makes the tenant's live link, replacing an older one, and returns
// the chats of the links it replaced.
func (h *Handler) link(ctx context.Context, q *dbgen.Queries, org, codeID, tenantID uuid.UUID, chatID, userID int64, now time.Time) ([]int64, error) {
	if err := q.LockTenantTelegramLink(ctx, tenantID); err != nil {
		return nil, fmt.Errorf("telegramlink: locking the tenant's link: %w", err)
	}
	n, err := q.UseTelegramLinkCode(ctx, dbgen.UseTelegramLinkCodeParams{UsedAt: &now, OrganizationID: org, ID: codeID})
	if err != nil {
		return nil, fmt.Errorf("telegramlink: using the code: %w", err)
	}
	if n != 1 {
		// The code is locked and was unused when read, so this cannot happen.
		return nil, errors.New("telegramlink: the code was used meanwhile")
	}
	old, err := q.UnlinkTenantTelegram(ctx, dbgen.UnlinkTenantTelegramParams{UnlinkedAt: &now, OrganizationID: org, TenantID: tenantID})
	if err != nil {
		return nil, fmt.Errorf("telegramlink: unlinking the older link: %w", err)
	}
	err = q.InsertTelegramLink(ctx, dbgen.InsertTelegramLinkParams{
		ID: uuid.New(), OrganizationID: org, TenantID: tenantID, LinkCodeID: codeID,
		ChatID: chatID, TelegramUserID: userID, LinkedAt: now,
	})
	if err != nil {
		return nil, fmt.Errorf("telegramlink: storing the link: %w", err)
	}
	return old, nil
}

// stop unlinks every live link of chatID: tenants', in each organization that has one, and staff
// users'.
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
	if h.staff != nil {
		n, err := h.staff.UnlinkChat(ctx, chatID)
		if err != nil {
			return "", "", fmt.Errorf("telegramlink: unlinking a chat's staff links: %w", err)
		}
		unlinked += n
	}
	if unlinked == 0 {
		return replyStopped, outcomeNotLinked, nil
	}
	return replyStopped, outcomeUnlinked, nil
}
