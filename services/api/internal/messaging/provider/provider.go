// Package provider builds the messaging.Provider chosen in config (WHATSAPP_PROVIDER), so the API
// (login codes, invitations) and the worker (reminders, promise confirmations) send through the
// same adapter, with the Telegram router in front of it when TELEGRAM_MESSAGES=linked.
package provider

import (
	"errors"
	"log/slog"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/router"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/twilio"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
)

// New builds the provider w names, both development only for now: Twilio (its sandbox), or the
// log provider, which writes messages (login codes included) to the log instead of sending them.
// config has already checked w.
func New(w config.WhatsApp, log *slog.Logger) (messaging.Provider, error) {
	switch w.Provider {
	case "twilio":
		contentSIDs, err := w.Twilio.ContentSIDMap()
		if err != nil {
			return nil, err
		}
		return twilio.New(twilio.Config{
			AccountSID:  w.Twilio.AccountSID,
			AuthToken:   w.Twilio.AuthToken,
			From:        w.Twilio.WhatsAppFrom,
			ContentSIDs: contentSIDs,
		})
	case "devlog":
		return devlog.New(log, w.DevlogFile), nil
	default:
		// config.LoadAPI and config.LoadWorker have already rejected anything else.
		return nil, errors.New("no WhatsApp provider configured")
	}
}

// WithTelegram returns whatsapp unchanged when t does not send to linked chats
// (TELEGRAM_MESSAGES=off, the default), and otherwise the router in front of it: a tenant's
// message goes to their linked Telegram chat, a staff user's (login codes) to theirs when staff is
// not nil, everything else to whatsapp (decision 42). config has already checked t, and refused it
// in production. Pass a nil staff as an untyped nil, never as a nil pointer.
func WithTelegram(whatsapp messaging.Provider, t config.Telegram, links router.Links, staff router.StaffLinks, log *slog.Logger) (messaging.Provider, error) {
	if !t.SendsToLinked() {
		return whatsapp, nil
	}
	bot, err := telegram.New(telegram.Config{Token: t.BotToken, BaseURL: t.APIBaseURL})
	if err != nil {
		return nil, err
	}
	log.Info("linked tenants' and staff users' messages go to their Telegram chats (demo channel, decision 42)",
		"staff", staff != nil)
	return router.New(whatsapp, bot, links, staff, log), nil
}
