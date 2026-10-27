// Package messaging defines how the app sends WhatsApp messages: the MessagingProvider
// interface from CLAUDE.md, the message templates, and the errors every adapter maps to.
//
// Adapters live in subpackages:
//   - gowa: the launch provider, a self-hosted GOWA server linked to the platform number.
//   - devlog: local development, writes messages to the log.
//   - messagingtest: an in-memory fake for tests.
//
// A WhatsApp Cloud API adapter can be added later behind the same interface, once there is a
// registered business to own the account. Callers never change: they send a template name and
// parameters, and each adapter turns that into whatever its provider accepts.
package messaging

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrUndeliverable means the number cannot receive WhatsApp messages. Callers flag the
	// tenant on the dashboard and create a visit task to get a new number.
	ErrUndeliverable = errors.New("messaging: number cannot receive WhatsApp")
	// ErrRateLimited means WhatsApp asked the sender to slow down. Retry later, with backoff.
	ErrRateLimited = errors.New("messaging: WhatsApp asked us to slow down")
	// ErrNotConnected means the WhatsApp session behind the provider is not usable, for example
	// the linked device was logged out. An operator has to re-link it.
	ErrNotConnected = errors.New("messaging: the WhatsApp session is not connected")
)

// Button is an action attached to a message. Payload is an opaque reference that maps back to
// an organization and an invoice, lease or payroll run, for providers with native reply buttons.
// URL is a link to our page that does the same thing. Providers without native buttons, such as
// GOWA, send the link instead, so every button needs one. Neither field holds personal data.
type Button struct {
	Title   string
	Payload string
	URL     string
}

// Message is one template message. OrganizationID is recorded on the message log row; it is
// uuid.Nil for platform messages such as login codes. Template bodies name the organization or
// property, because every organization shares one sender number.
type Message struct {
	OrganizationID uuid.UUID
	To             string // E.164 phone number, for example +628123456789
	Template       string
	Params         map[string]string
	Buttons        []Button
}

// Receipt is what the provider returns for an accepted message.
type Receipt struct {
	ProviderMessageID string
}

// Provider sends template messages.
type Provider interface {
	SendTemplate(ctx context.Context, m Message) (Receipt, error)
}
