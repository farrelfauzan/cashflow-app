// Package autogopay holds each organization's AutoGoPay account: its API key, encrypted at rest
// with internal/secret, and the random id in its webhook URL. QRIS money goes to the landlord's
// own AutoGoPay account, so every organization has its own key (phase B, slice 3).
//
// The key is never logged, printed or returned by the API. APIKey hands it to callers as a
// secret.Value, which redacts itself everywhere except Reveal.
package autogopay

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

var (
	// ErrNoAccount means the organization has no AutoGoPay API key yet.
	ErrNoAccount = errors.New("autogopay: the organization has no AutoGoPay account")
	// ErrInvalidAPIKey means the key is empty, longer than MaxAPIKeyBytes, not UTF-8, or holds a
	// control character (a pasted newline or tab inside it, for example).
	ErrInvalidAPIKey = errors.New("autogopay: the API key must be 1 to 512 bytes of text without control characters")
	// ErrOrganizationNotFound means no organization has that id.
	ErrOrganizationNotFound = errors.New("autogopay: organization not found")
)

// MaxAPIKeyBytes bounds an API key. AutoGoPay's keys are far shorter; the limit stops a pasted
// file from being stored.
const MaxAPIKeyBytes = 512

// auditAction is how setting the key appears in the audit log. It is a catalog action, so the
// audit log reads the same once members can set the key from a settings screen.
const auditAction = "settings:update"

// webhookIDBytes is the entropy of a webhook id: 32 random bytes, 43 characters of base64url.
const webhookIDBytes = 32

// Service stores and reads AutoGoPay accounts.
type Service struct {
	store *store.Store
	keys  *secret.Keyring
	authz *authz.Service
	clock clock.Clock
}

// NewService returns a Service that encrypts keys with keys.
func NewService(st *store.Store, keys *secret.Keyring, az *authz.Service, clk clock.Clock) *Service {
	return &Service{store: st, keys: keys, authz: az, clock: clk}
}

// WebhookPath is the path AutoGoPay must call for the account with this webhook id.
func WebhookPath(webhookID string) string { return "/webhooks/autogopay/" + webhookID }

// SetAPIKey stores org's AutoGoPay API key, encrypted, and returns the account's webhook id. The
// first call creates the account and its webhook id; later calls replace the key and keep the
// webhook id, so the URL registered at AutoGoPay stays the same. Surrounding whitespace is
// trimmed. It is the platform operator's action (cmd/admin), recorded in the organization's
// audit log with no acting member.
func (s *Service) SetAPIKey(ctx context.Context, org uuid.UUID, key string) (string, error) {
	key, err := NormalizeAPIKey(key)
	if err != nil {
		return "", err
	}
	if org == uuid.Nil {
		return "", ErrOrganizationNotFound
	}
	keyID, ciphertext, err := s.keys.Encrypt([]byte(key), additionalData(org))
	if err != nil {
		return "", fmt.Errorf("autogopay: encrypting the API key: %w", err)
	}
	candidate, err := newWebhookID()
	if err != nil {
		return "", err
	}

	var webhookID string
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetOrganization(ctx, org); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrganizationNotFound
			}
			return fmt.Errorf("autogopay: reading the organization: %w", err)
		}
		webhookID, err = q.UpsertAutogopayAccount(ctx, dbgen.UpsertAutogopayAccountParams{
			OrganizationID:   org,
			ApiKeyCiphertext: ciphertext,
			KeyID:            keyID,
			WebhookID:        candidate,
			Now:              s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("autogopay: storing the account: %w", err)
		}
		return s.authz.RecordSystemAction(ctx, tx, org, auditAction,
			authz.Resource{Type: "autogopay_account", ID: org.String()})
	})
	if err != nil {
		return "", err
	}
	return webhookID, nil
}

// APIKey decrypts org's AutoGoPay API key inside tx, which must be scoped to org (WithOrgTx):
// row-level security hides every other organization's account, so another org reads as
// ErrNoAccount. A ciphertext that does not belong to org fails with secret.ErrDecrypt.
func (s *Service) APIKey(ctx context.Context, tx pgx.Tx, org uuid.UUID) (secret.Value, error) {
	acc, err := dbgen.New(tx).GetAutogopayAccount(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return secret.Value{}, ErrNoAccount
	}
	if err != nil {
		return secret.Value{}, fmt.Errorf("autogopay: reading the account: %w", err)
	}
	plain, err := s.keys.Decrypt(acc.KeyID, acc.ApiKeyCiphertext, additionalData(org))
	if err != nil {
		return secret.Value{}, fmt.Errorf("autogopay: decrypting the API key (key id %s): %w", acc.KeyID, err)
	}
	return secret.NewValue(string(plain)), nil
}

// additionalData binds a ciphertext to its organization's row, so a ciphertext copied onto
// another organization's account does not decrypt there.
func additionalData(org uuid.UUID) []byte {
	return []byte("autogopay_api_key:" + org.String())
}

// NormalizeAPIKey trims surrounding whitespace and checks what is left, as SetAPIKey does, so a
// caller can refuse a bad key before it connects anywhere. Its error never includes the key.
func NormalizeAPIKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > MaxAPIKeyBytes || !utf8.ValidString(key) {
		return "", ErrInvalidAPIKey
	}
	if strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return "", ErrInvalidAPIKey
	}
	return key, nil
}

// newWebhookID returns 32 random bytes as base64url without padding.
func newWebhookID() (string, error) {
	b := make([]byte, webhookIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("autogopay: reading random bytes for the webhook id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
