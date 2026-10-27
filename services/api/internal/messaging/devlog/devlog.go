// Package devlog is a messaging provider for local development. It writes each message to the
// log instead of sending it, so a developer can read login codes from the API output. It must
// never run outside development: config refuses it there, and refuses DEVLOG_FILE with it.
package devlog

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// Provider logs messages instead of sending them.
type Provider struct {
	log  *slog.Logger
	file string
	mu   sync.Mutex // one line at a time from this process
}

var _ messaging.Provider = (*Provider)(nil)

// New returns a Provider that writes to log. When file is not empty, each message is also
// appended to it as one JSON line, for end-to-end tests that read login codes.
func New(log *slog.Logger, file string) *Provider { return &Provider{log: log, file: file} }

// Line is one message as written to the file. Params are in the template's order, so the login
// code is Params[0].
type Line struct {
	Time     time.Time `json:"time"`
	To       string    `json:"to"`
	Template string    `json:"template"`
	Params   []string  `json:"params"`
	Text     string    `json:"text"`
}

// SendTemplate renders the message exactly as GOWA would send it, logs it with its parameters,
// and pretends it was accepted. A rendering mistake fails here just as it would in production.
func (p *Provider) SendTemplate(ctx context.Context, m messaging.Message) (messaging.Receipt, error) {
	text, err := messaging.Render(m)
	if err != nil {
		return messaging.Receipt{}, err
	}
	p.log.WarnContext(ctx, "development only: WhatsApp message written to the log, not sent",
		"to", m.To, "template", m.Template, "params", m.Params, "text", text)
	if p.file != "" {
		if err := p.appendLine(m, text); err != nil {
			return messaging.Receipt{}, err
		}
	}
	return messaging.Receipt{ProviderMessageID: "devlog-" + uuid.NewString()}, nil
}

// appendLine writes the message as one JSON line with a single write to a file opened for
// appending, so readers and other API processes sharing the file never see half a line.
func (p *Provider) appendLine(m messaging.Message, text string) error {
	t, ok := messaging.Lookup(m.Template)
	if !ok {
		return fmt.Errorf("devlog: unknown template %q", m.Template) // Render already refuses this
	}
	params := make([]string, len(t.Params))
	for i, name := range t.Params {
		params[i] = m.Params[name]
	}
	b, err := json.Marshal(Line{Time: time.Now().UTC(), To: m.To, Template: m.Template, Params: params, Text: text})
	if err != nil {
		return fmt.Errorf("devlog: encoding message: %w", err)
	}
	b = append(b, '\n')

	p.mu.Lock()
	defer p.mu.Unlock()
	f, err := os.OpenFile(p.file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return fmt.Errorf("devlog: opening %s: %w", p.file, err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		return fmt.Errorf("devlog: writing %s: %w", p.file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("devlog: closing %s: %w", p.file, err)
	}
	return nil
}
