// Package router is the messaging.Provider callers send through when the Telegram demo channel is
// on (decision 42, TELEGRAM_MESSAGES=linked; config refuses it in production). A message for a
// tenant whose chat is linked goes to that chat on Telegram, and so does a message for a staff user
// whose chat is linked (login codes, TG-4); every other message (a recipient with no live link,
// invitations, a login code the user asked to get on WhatsApp) goes to the WhatsApp provider
// unchanged. A TelegramOnly message (staff notices, TG-5) goes to the linked chat or nowhere.
//
// There is no fallback between channels: when Telegram refuses a send, the send fails like any
// other. When the refusal says the chat cannot be reached (the bot was blocked, the chat is gone),
// the link is ended, so the recipient's next message goes to WhatsApp.
//
// Logs name the channel and the template, never the chat id or the message text.
package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// Links finds and ends tenants' Telegram links. telegramlink.Chats implements it.
type Links interface {
	// TenantChat returns the chat of the tenant's live link, and false when there is none.
	TenantChat(ctx context.Context, org, tenant uuid.UUID) (int64, bool, error)
	// UnlinkUndeliverable ends the tenant's live link in chatID, if it is still that chat's.
	UnlinkUndeliverable(ctx context.Context, org, tenant uuid.UUID, chatID int64) error
}

// StaffLinks finds and ends staff users' Telegram links. auth.TelegramStaff implements it.
type StaffLinks interface {
	// StaffChat returns the chat of the staff user's live link, and false when there is none.
	StaffChat(ctx context.Context, userID uuid.UUID) (int64, bool, error)
	// UnlinkUndeliverable ends the staff user's live link in chatID, if it is still that chat's.
	UnlinkUndeliverable(ctx context.Context, userID uuid.UUID, chatID int64) error
}

// Telegram sends a template message to a chat. telegram.Client implements it.
type Telegram interface {
	Send(ctx context.Context, chatID int64, m messaging.Message) (messaging.Receipt, error)
}

// Router chooses the channel per message.
type Router struct {
	whatsapp messaging.Provider
	telegram Telegram
	links    Links
	staff    StaffLinks
	log      *slog.Logger
}

var _ messaging.Provider = (*Router)(nil)

// New returns a router that sends to linked tenants and staff users through tg and everything else
// through whatsapp. staff may be nil: then a message for a staff user goes to whatsapp, or, when
// it is TelegramOnly, nowhere (messaging.ErrNoTelegramChat).
func New(whatsapp messaging.Provider, tg Telegram, links Links, staff StaffLinks, log *slog.Logger) *Router {
	return &Router{whatsapp: whatsapp, telegram: tg, links: links, staff: staff, log: log}
}

// SendTemplate sends m on Telegram when it is for a tenant (m.OrganizationID and m.TenantID) with a
// live link, or else for a staff user (m.StaffUserID) with a live link, and on WhatsApp otherwise.
// A failed link lookup fails the send rather than guessing a channel.
//
// A TelegramOnly message (staff notices, TG-5) never goes to WhatsApp: with no live link, or no
// recipient the router can look up, nothing is sent and the error is messaging.ErrNoTelegramChat.
func (r *Router) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	switch {
	case m.TenantID != uuid.Nil:
		if m.OrganizationID == uuid.Nil {
			return r.notLinked(ctx, m)
		}
		return r.sendToTenant(ctx, m)
	case m.StaffUserID != uuid.Nil && r.staff != nil:
		return r.sendToStaff(ctx, m)
	default:
		return r.notLinked(ctx, m)
	}
}

// notLinked sends m, whose recipient has no live Telegram link, on WhatsApp, unless m is
// TelegramOnly: then nothing is sent.
func (r *Router) notLinked(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	if m.TelegramOnly {
		return messaging.Receipt{}, messaging.ErrNoTelegramChat
	}
	return r.whatsapp.SendTemplate(ctx, m)
}

func (r *Router) sendToTenant(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	chatID, linked, err := r.links.TenantChat(ctx, m.OrganizationID, m.TenantID)
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("router: finding the tenant's Telegram link: %w", err)
	}
	if !linked {
		return r.notLinked(ctx, m)
	}
	receipt, err := r.telegram.Send(ctx, chatID, m)
	if err == nil {
		r.log.InfoContext(ctx, "message sent", "channel", "telegram", "template", m.Template, "organization", m.OrganizationID)
		return receipt, nil
	}
	if errors.Is(err, messaging.ErrUndeliverable) {
		if unlinkErr := r.links.UnlinkUndeliverable(ctx, m.OrganizationID, m.TenantID, chatID); unlinkErr != nil {
			r.log.ErrorContext(ctx, "telegram link not ended after an undeliverable send",
				"organization", m.OrganizationID, "tenant", m.TenantID, "error", unlinkErr)
		} else {
			r.log.WarnContext(ctx, "telegram chat unreachable, link ended; the tenant's next message goes to WhatsApp",
				"organization", m.OrganizationID, "tenant", m.TenantID)
		}
	}
	return messaging.Receipt{}, err
}

func (r *Router) sendToStaff(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	chatID, linked, err := r.staff.StaffChat(ctx, m.StaffUserID)
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("router: finding the staff user's Telegram link: %w", err)
	}
	if !linked {
		return r.notLinked(ctx, m)
	}
	receipt, err := r.telegram.Send(ctx, chatID, m)
	if err == nil {
		r.log.InfoContext(ctx, "message sent", "channel", "telegram", "template", m.Template, "staff_user_id", m.StaffUserID)
		return receipt, nil
	}
	if errors.Is(err, messaging.ErrUndeliverable) {
		if unlinkErr := r.staff.UnlinkUndeliverable(ctx, m.StaffUserID, chatID); unlinkErr != nil {
			r.log.ErrorContext(ctx, "telegram link not ended after an undeliverable send",
				"staff_user_id", m.StaffUserID, "error", unlinkErr)
		} else {
			r.log.WarnContext(ctx, "telegram chat unreachable, link ended; the staff user's next message goes to WhatsApp",
				"staff_user_id", m.StaffUserID)
		}
	}
	return messaging.Receipt{}, err
}
