// Package telegramlink links a tenant's Telegram chat to the platform bot (decision 42, TG-2), a
// demo channel for development and staging.
//
// The payment page asks for a one-time code (Codes.DeepLink) and opens the bot's deep link,
// https://t.me/<bot>?start=<code>. Telegram then delivers "/start <code>" from the tenant's chat
// to the bot, through the webhook in staging or the polling runner in development, and both hand
// it to the one update handler (Handler), which stores the chat for the code's tenant. "/stop"
// unlinks the chat everywhere.
//
// The code is 32 fresh random bytes, never the payment link token or anything derived from it, so
// a forwarded deep link reveals nothing about the payment link, and it is valid for 10 minutes and
// used once. Only its SHA-256 is stored. The chat id comes only from the update that carried the
// code, never from a phone number. An update finds its organization through the code or the
// chat's stored links (two narrow SECURITY DEFINER lookups), never from the sender alone.
//
// Nothing here logs a code, a token, message text, or a chat id next to a name.
package telegramlink

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// CodeLifetime is how long a linking code works after it is made.
const CodeLifetime = 10 * time.Minute

// codeBytes is the entropy of a code; codeLength is its base64url length, within the 64
// characters of [A-Za-z0-9_-] Telegram allows in a start parameter.
const (
	codeBytes  = 32
	codeLength = 43
)

// codeEncoding refuses a non-canonical last character, so each code has exactly one spelling.
var codeEncoding = base64.RawURLEncoding.Strict()

// Links resolves a payment link token and checks that the link still works. paylink.Service
// implements it.
type Links interface {
	// Resolve finds the organization and link behind a token, with no organization scope.
	Resolve(ctx context.Context, token string) (org, linkID uuid.UUID, err error)
	// OpenLink checks inside tx that the link works and returns its tenant and invoice.
	OpenLink(ctx context.Context, tx pgx.Tx, org, linkID uuid.UUID) (tenantID, invoiceID uuid.UUID, err error)
}

// Codes makes one-time linking codes for the payment page.
type Codes struct {
	store       *store.Store
	links       Links
	clock       clock.Clock
	botUsername string
}

// NewCodes returns Codes for the bot named botUsername (TELEGRAM_BOT_USERNAME, without the @).
func NewCodes(st *store.Store, links Links, c clock.Clock, botUsername string) *Codes {
	return &Codes{store: st, links: links, clock: c, botUsername: botUsername}
}

// DeepLink makes a new linking code for the tenant of the payment link behind token and returns
// the bot's deep link carrying it. The token grants it, as it grants the page, and its errors are
// the page's: paylink.ErrLinkNotFound for a malformed, unknown or revoked token, and
// paylink.ErrLinkExpired. Earlier codes stay valid; each is used once. The link holds the code:
// hand it to the tenant only, and never log it.
func (c *Codes) DeepLink(ctx context.Context, token string) (string, error) {
	org, linkID, err := c.links.Resolve(ctx, token)
	if err != nil {
		return "", err
	}
	code, hash, err := newCode()
	if err != nil {
		return "", err
	}
	err = c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		tenantID, _, err := c.links.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		now := c.clock.Now()
		err = dbgen.New(tx).InsertTelegramLinkCode(ctx, dbgen.InsertTelegramLinkCodeParams{
			ID: uuid.New(), OrganizationID: org, TenantID: tenantID, CodeHash: hash,
			CreatedAt: now, ExpiresAt: now.Add(CodeLifetime),
		})
		if err != nil {
			return fmt.Errorf("telegramlink: storing a code: %w", err)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return "https://t.me/" + url.PathEscape(c.botUsername) + "?start=" + code, nil
}

// newCode returns a fresh code and the hash to store. The bytes come from crypto/rand alone, so
// nothing about the payment link token is in them.
func newCode() (code string, hash []byte, err error) {
	raw := make([]byte, codeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("telegramlink: reading random bytes for a code: %w", err)
	}
	sum := sha256.Sum256(raw)
	return codeEncoding.EncodeToString(raw), sum[:], nil
}

// codeHash checks that code has the shape of a linking code and returns the SHA-256 of the 32
// bytes it encodes, which is what the database stores. ok is false for anything else.
func codeHash(code string) (hash []byte, ok bool) {
	if len(code) != codeLength {
		return nil, false
	}
	raw, err := codeEncoding.DecodeString(code)
	if err != nil || len(raw) != codeBytes {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}
