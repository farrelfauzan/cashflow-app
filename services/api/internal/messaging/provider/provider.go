// Package provider builds the messaging.Provider chosen in config (WHATSAPP_PROVIDER), so the API
// (login codes, invitations) and the worker (reminders, promise confirmations) send through the
// same adapter.
package provider

import (
	"errors"
	"log/slog"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/gowa"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/twilio"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
)

// New builds the provider w names: GOWA, Twilio, or in development the log provider, which writes
// messages (login codes included) to the log instead of sending them. config has already checked
// w.
func New(w config.WhatsApp, log *slog.Logger) (messaging.Provider, error) {
	switch w.Provider {
	case "gowa":
		return gowa.New(gowa.Config{
			BaseURL:  w.GOWA.URL,
			Username: w.GOWA.Username,
			Password: w.GOWA.Password,
			DeviceID: w.GOWA.DeviceID,
		})
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
