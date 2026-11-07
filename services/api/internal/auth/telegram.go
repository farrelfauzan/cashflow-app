package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink/linkcode"
)

// linkCodesPerUser caps how many Telegram linking codes one staff user may ask for per
// limitWindow.
const linkCodesPerUser = 5

// TelegramStaff links a staff user's Telegram chat to the platform bot (decision 42, TG-4: demo
// only, development and staging), so their login codes can arrive there. It lives in auth because
// staff users are platform-level and only auth, member and org may use the auth scope.
//
// The signed-in user asks for a one-time code (NewLinkCode) and opens the bot's deep link; the
// bot's update handler hands the "/start <code>" it receives to Link. Codes and links are this
// user's own account, not organization data, so no RBAC action guards them: the session is the
// access, as for GET /api/me.
//
// Nothing here logs a code, its hash or a chat id.
type TelegramStaff struct {
	store   *store.Store
	limiter ratelimit.Limiter
	clock   clock.Clock
	log     *slog.Logger
}

// NewTelegramStaff returns the staff side of the Telegram demo channel. limiter caps NewLinkCode
// per staff user.
func NewTelegramStaff(st *store.Store, limiter ratelimit.Limiter, c clock.Clock, log *slog.Logger) *TelegramStaff {
	return &TelegramStaff{store: st, limiter: limiter, clock: c, log: log}
}

// NewLinkCode makes a one-time linking code for userID, valid for linkcode.Lifetime, and returns
// it with its expiry. Earlier codes stay valid; each is used once. At most 5 per user per 15
// minutes, then ErrRateLimited. The code is a secret: hand it to the user only, never log it.
func (t *TelegramStaff) NewLinkCode(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	ok, err := t.limiter.Allow(ctx, "telegram:staff-link-code:"+userID.String(), linkCodesPerUser, limitWindow)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: limiting telegram link codes: %w", err)
	}
	if !ok {
		return "", time.Time{}, ErrRateLimited
	}
	code, hash, err := linkcode.New()
	if err != nil {
		return "", time.Time{}, err
	}
	now := t.clock.Now()
	expires := now.Add(linkcode.Lifetime)
	err = t.store.WithAuthTx(ctx, userID, func(tx pgx.Tx) error {
		return dbgen.New(tx).InsertTelegramStaffLinkCode(ctx, dbgen.InsertTelegramStaffLinkCodeParams{
			ID: uuid.New(), StaffUserID: userID, CodeHash: hash, CreatedAt: now, ExpiresAt: expires,
		})
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("auth: storing a telegram link code: %w", err)
	}
	return code, expires, nil
}

// Linked reports whether userID has a live Telegram link.
func (t *TelegramStaff) Linked(ctx context.Context, userID uuid.UUID) (bool, error) {
	var linked bool
	err := t.store.WithAuthTx(ctx, userID, func(tx pgx.Tx) error {
		var err error
		linked, err = dbgen.New(tx).StaffHasTelegramLink(ctx, userID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("auth: reading the telegram link: %w", err)
	}
	return linked, nil
}

// Unlink ends userID's live Telegram link, so their login codes go to WhatsApp again. No live link
// is not an error.
func (t *TelegramStaff) Unlink(ctx context.Context, userID uuid.UUID) error {
	now := t.clock.Now()
	err := t.store.WithAuthTx(ctx, userID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.LockStaffTelegramLink(ctx, userID); err != nil {
			return err
		}
		_, err := q.UnlinkStaffTelegram(ctx, dbgen.UnlinkStaffTelegramParams{UnlinkedAt: &now, StaffUserID: userID})
		return err
	})
	if err != nil {
		return fmt.Errorf("auth: ending the telegram link: %w", err)
	}
	return nil
}

// Link uses the staff code whose SHA-256 is codeHash to link chatID, for the bot's "/start
// <code>". It mirrors the tenant's linking (telegramlink): an unknown hash gives linkcode.Unknown;
// a used code gives linkcode.AlreadyLinked when its link is still live in this same chat (a
// replayed /start) and linkcode.Used otherwise; an expired one gives linkcode.Expired. Otherwise the
// code is marked used, the user's earlier live link ends (its chat is returned in Released unless
// it is this chat) and the new link is stored, all in one transaction serialized per staff user.
func (t *TelegramStaff) Link(ctx context.Context, codeHash []byte, chatID, telegramUserID int64) (linkcode.StaffLink, error) {
	var res linkcode.StaffLink
	err := t.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		var err error
		res, err = t.link(ctx, dbgen.New(tx), codeHash, chatID, telegramUserID)
		return err
	})
	if err != nil {
		return linkcode.StaffLink{}, err
	}
	return res, nil
}

func (t *TelegramStaff) link(ctx context.Context, q *dbgen.Queries, codeHash []byte, chatID, telegramUserID int64) (linkcode.StaffLink, error) {
	c, err := q.LockTelegramStaffLinkCode(ctx, codeHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return linkcode.StaffLink{Outcome: linkcode.Unknown}, nil
	}
	if err != nil {
		return linkcode.StaffLink{}, fmt.Errorf("auth: loading the telegram link code: %w", err)
	}
	now := t.clock.Now()
	switch {
	case c.UsedAt != nil:
		l, err := q.GetTelegramStaffLinkByCode(ctx, c.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return linkcode.StaffLink{Outcome: linkcode.Used}, nil
		}
		if err != nil {
			return linkcode.StaffLink{}, fmt.Errorf("auth: loading the code's telegram link: %w", err)
		}
		if l.UnlinkedAt != nil || l.ChatID != chatID {
			return linkcode.StaffLink{Outcome: linkcode.Used}, nil
		}
		return linkcode.StaffLink{Outcome: linkcode.AlreadyLinked, Name: c.StaffName}, nil
	case !now.Before(c.ExpiresAt):
		return linkcode.StaffLink{Outcome: linkcode.Expired}, nil
	}

	if err := q.LockStaffTelegramLink(ctx, c.StaffUserID); err != nil {
		return linkcode.StaffLink{}, fmt.Errorf("auth: locking the staff user's telegram link: %w", err)
	}
	n, err := q.UseTelegramStaffLinkCode(ctx, dbgen.UseTelegramStaffLinkCodeParams{UsedAt: &now, ID: c.ID})
	if err != nil {
		return linkcode.StaffLink{}, fmt.Errorf("auth: using the telegram link code: %w", err)
	}
	if n != 1 {
		// The code is locked and was unused when read, so this cannot happen.
		return linkcode.StaffLink{}, errors.New("auth: the telegram link code was used meanwhile")
	}
	old, err := q.UnlinkStaffTelegram(ctx, dbgen.UnlinkStaffTelegramParams{UnlinkedAt: &now, StaffUserID: c.StaffUserID})
	if err != nil {
		return linkcode.StaffLink{}, fmt.Errorf("auth: ending the earlier telegram link: %w", err)
	}
	err = q.InsertTelegramStaffLink(ctx, dbgen.InsertTelegramStaffLinkParams{
		ID: uuid.New(), StaffUserID: c.StaffUserID, LinkCodeID: c.ID,
		ChatID: chatID, TelegramUserID: telegramUserID, LinkedAt: now,
	})
	if err != nil {
		return linkcode.StaffLink{}, fmt.Errorf("auth: storing the telegram link: %w", err)
	}
	res := linkcode.StaffLink{Outcome: linkcode.Linked, Name: c.StaffName}
	for _, id := range old {
		if id != chatID {
			res.Released = append(res.Released, id)
		}
	}
	t.log.InfoContext(ctx, "telegram linked for login codes", "staff_user_id", c.StaffUserID)
	return res, nil
}

// UnlinkChat ends every live staff link of chatID (the bot's /stop) and returns how many it ended.
func (t *TelegramStaff) UnlinkChat(ctx context.Context, chatID int64) (int64, error) {
	now := t.clock.Now()
	var n int64
	err := t.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).UnlinkTelegramStaffChat(ctx, dbgen.UnlinkTelegramStaffChatParams{UnlinkedAt: &now, ChatID: chatID})
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("auth: unlinking a telegram chat: %w", err)
	}
	return n, nil
}

// StaffChat returns the chat of userID's live link, and false when there is none. The messaging
// router calls it for a message that names a staff user.
func (t *TelegramStaff) StaffChat(ctx context.Context, userID uuid.UUID) (int64, bool, error) {
	var chatID int64
	err := t.store.WithAuthTx(ctx, userID, func(tx pgx.Tx) error {
		var err error
		chatID, err = dbgen.New(tx).GetStaffTelegramChat(ctx, userID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("auth: reading the staff user's telegram link: %w", err)
	}
	return chatID, true, nil
}

// UnlinkUndeliverable ends userID's live link in chatID after Telegram refused a send to it (the
// bot was blocked, the chat is gone). A link made since in another chat is left alone, and a link
// already ended is not an error.
func (t *TelegramStaff) UnlinkUndeliverable(ctx context.Context, userID uuid.UUID, chatID int64) error {
	now := t.clock.Now()
	err := t.store.WithAuthTx(ctx, userID, func(tx pgx.Tx) error {
		_, err := dbgen.New(tx).UnlinkUndeliverableStaffTelegramChat(ctx, dbgen.UnlinkUndeliverableStaffTelegramChatParams{
			StaffUserID: userID, ChatID: chatID, UnlinkedAt: &now,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("auth: ending the unreachable telegram link: %w", err)
	}
	return nil
}
