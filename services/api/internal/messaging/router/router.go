// Package router is the messaging.Provider callers send through when the Telegram demo channel is
// on (decision 42, MESSAGING_TELEGRAM=linked; config refuses it in production). A message for a
// tenant whose chat is linked goes to that chat on Telegram; every other message (a tenant with
// no live link, login codes, invitations) goes to the WhatsApp provider unchanged.
//
// There is no fallback between channels: when Telegram refuses a send, the send fails like any
// other. When the refusal says the chat cannot be reached (the bot was blocked, the chat is gone),
// the link is ended, so the tenant's next message goes to WhatsApp.
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

// Telegram sends a template message to a chat. telegram.Client implements it.
type Telegram interface {
	Send(ctx context.Context, chatID int64, m messaging.Message) (messaging.Receipt, error)
}

// Router chooses the channel per message.
type Router struct {
	whatsapp messaging.Provider
	telegram Telegram
	links    Links
	log      *slog.Logger
}

var _ messaging.Provider = (*Router)(nil)

// New returns a router that sends to linked tenants through tg and everything else through
// whatsapp.
func New(whatsapp messaging.Provider, tg Telegram, links Links, log *slog.Logger) *Router {
	return &Router{whatsapp: whatsapp, telegram: tg, links: links, log: log}
}

// SendTemplate sends m on Telegram when it is for a tenant (m.TenantID) with a live link, and on
// WhatsApp otherwise. A failed link lookup fails the send rather than guessing a channel.
func (r *Router) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	if m.OrganizationID == uuid.Nil || m.TenantID == uuid.Nil {
		return r.whatsapp.SendTemplate(ctx, m)
	}
	chatID, linked, err := r.links.TenantChat(ctx, m.OrganizationID, m.TenantID)
	if err != nil {
		return messaging.Receipt{}, fmt.Errorf("router: finding the tenant's Telegram link: %w", err)
	}
	if !linked {
		return r.whatsapp.SendTemplate(ctx, m)
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
