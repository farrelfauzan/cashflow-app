// Package devlog is a messaging provider for local development. It writes each message to the
// log instead of sending it, so a developer can read login codes from the API output. It must
// never run outside development: config refuses it there.
package devlog

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// Provider logs messages instead of sending them.
type Provider struct {
	log *slog.Logger
}

var _ messaging.Provider = (*Provider)(nil)

// New returns a Provider that writes to log.
func New(log *slog.Logger) *Provider { return &Provider{log: log} }

// SendTemplate renders the message exactly as GOWA would send it, logs it with its parameters,
// and pretends it was accepted. A rendering mistake fails here just as it would in production.
func (p *Provider) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	text, err := messaging.Render(m)
	if err != nil {
		return messaging.Receipt{}, err
	}
	p.log.WarnContext(ctx, "development only: WhatsApp message written to the log, not sent",
		"to", m.To, "template", m.Template, "params", m.Params, "text", text)
	return messaging.Receipt{ProviderMessageID: "devlog-" + uuid.NewString()}, nil
}
