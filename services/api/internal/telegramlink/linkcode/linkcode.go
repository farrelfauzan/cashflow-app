// Package linkcode makes and reads the one-time codes that link a Telegram chat to the platform bot
// (decision 42, demo only), for tenants (package telegramlink) and staff (package auth) alike, and
// names what a /start with a staff code did.
//
// A code is 32 bytes from crypto/rand in base64url without padding (43 characters, within the 64
// of [A-Za-z0-9_-] Telegram allows in a start parameter), valid for Lifetime and used once. Only
// its SHA-256 is stored. Nothing here logs a code or a hash.
package linkcode

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"time"
)

// Lifetime is how long a linking code works after it is made.
const Lifetime = 10 * time.Minute

// codeBytes is the entropy of a code; codeLength is its base64url length.
const (
	codeBytes  = 32
	codeLength = 43
)

// encoding refuses a non-canonical last character, so each code has exactly one spelling.
var encoding = base64.RawURLEncoding.Strict()

// New returns a fresh code and the hash to store. The bytes come from crypto/rand alone, so
// nothing about a payment link token or a session is in them.
func New() (code string, hash []byte, err error) {
	raw := make([]byte, codeBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("linkcode: reading random bytes for a code: %w", err)
	}
	sum := sha256.Sum256(raw)
	return encoding.EncodeToString(raw), sum[:], nil
}

// Hash checks that code has the shape of a linking code and returns the SHA-256 of the 32 bytes it
// encodes, which is what the database stores. ok is false for anything else.
func Hash(code string) (hash []byte, ok bool) {
	if len(code) != codeLength {
		return nil, false
	}
	raw, err := encoding.DecodeString(code)
	if err != nil || len(raw) != codeBytes {
		return nil, false
	}
	sum := sha256.Sum256(raw)
	return sum[:], true
}

// DeepLink is the bot's deep link carrying code: https://t.me/<bot>?start=<code>. It holds the
// code: hand it to its owner only, and never log it.
func DeepLink(botUsername, code string) string {
	return "https://t.me/" + url.PathEscape(botUsername) + "?start=" + code
}

// Outcome is what a /start with a staff code did, for the bot's reply and its logs.
type Outcome string

const (
	// Linked: the code was unused and in time, and the chat is now the staff user's.
	Linked Outcome = "linked"
	// AlreadyLinked: the code was used to link this same chat, and that link is still live (a
	// replayed /start).
	AlreadyLinked Outcome = "already_linked"
	// Unknown: no staff code has this hash.
	Unknown Outcome = "invalid_code"
	// Used: the code was used, and its link is no longer live or is another chat's.
	Used Outcome = "used_code"
	// Expired: the code was not used in time.
	Expired Outcome = "expired_code"
)

// StaffLink is the result of a /start with a staff code.
type StaffLink struct {
	Outcome Outcome
	// Name is the staff user's name, for the reply; empty when Outcome is Unknown.
	Name string
	// Released are the chats of the staff user's earlier live link that this one replaced, never
	// the new chat itself. Empty unless Outcome is Linked.
	Released []int64
}
