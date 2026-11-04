// Package secret encrypts small per-organization secrets, such as an organization's AutoGoPay
// API key, before they are stored, under a master key from the environment.
//
// Encryption is AES-256-GCM with a random 12-byte nonce per call; the stored ciphertext is the
// nonce followed by the sealed text. Each ciphertext is stored next to the id of the key that
// made it, so the master key can be rotated: a Keyring encrypts with its current key and
// decrypts with the current key or any previous one.
//
// # Binding a ciphertext to its row
//
// Every call passes additional authenticated data (AAD) that names what the secret is and which
// row it belongs to, for example
//
//	aad := []byte("autogopay_api_key:" + orgID.String())
//
// GCM authenticates the AAD without storing it, so a ciphertext copied onto another
// organization's row fails to decrypt there. Encrypt refuses an empty AAD so a caller cannot
// forget it.
//
// Errors are sentinels and never carry key material, plaintext or ciphertext.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	// ErrUnknownKey means a ciphertext names a key id the keyring does not hold: the key was
	// rotated out of SECRETS_KEY_PREVIOUS too early, or the row is corrupt.
	ErrUnknownKey = errors.New("secret: unknown key id")
	// ErrDecrypt means a ciphertext failed authentication: the wrong key, the wrong AAD (a row
	// copied from another organization), or a changed or truncated ciphertext.
	ErrDecrypt = errors.New("secret: cannot decrypt")
	// ErrNoAAD means Encrypt was called without additional authenticated data.
	ErrNoAAD = errors.New("secret: additional authenticated data is required")
)

const (
	keySize   = 32 // AES-256
	nonceSize = 12 // the GCM standard nonce
)

// keyID is the shape of a key id: short, lowercase, safe to store and to show in errors.
var keyID = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

// Keyring holds the current master key, which encrypts, and the previous ones, which only
// decrypt. It is safe for concurrent use. Its fmt output shows key ids only.
type Keyring struct {
	current string
	keys    map[string]cipher.AEAD
}

// ParseKeyring builds a keyring from the SECRETS_KEY and SECRETS_KEY_PREVIOUS settings. current
// is "<id>:<base64 of exactly 32 bytes>" (standard base64, padded); previous is empty or a
// comma-separated list of the same form. Ids are 1 to 32 characters of a-z, 0-9 and '-', and
// unique across both settings. Errors name the setting and never echo a key.
func ParseKeyring(current, previous string) (*Keyring, error) {
	const curVar, prevVar = "SECRETS_KEY", "SECRETS_KEY_PREVIOUS"
	current = strings.TrimSpace(current)
	if current == "" {
		return nil, fmt.Errorf("%s is required: %s", curVar, keyFormat)
	}
	id, aead, err := parseKey(current)
	if err != nil {
		return nil, fmt.Errorf("%s %w", curVar, err)
	}
	k := &Keyring{current: id, keys: map[string]cipher.AEAD{id: aead}}

	previous = strings.TrimSpace(previous)
	if previous == "" {
		return k, nil
	}
	i := 0
	for entry := range strings.SplitSeq(previous, ",") {
		i++
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, fmt.Errorf("%s entry %d is empty: %s, separated by commas", prevVar, i, keyFormat)
		}
		pid, paead, err := parseKey(entry)
		if err != nil {
			return nil, fmt.Errorf("%s entry %d %w", prevVar, i, err)
		}
		if pid == id {
			return nil, fmt.Errorf("%s entry %d uses the id %q of SECRETS_KEY; give each key its own id", prevVar, i, pid)
		}
		if _, dup := k.keys[pid]; dup {
			return nil, fmt.Errorf("%s names the key id %q twice", prevVar, pid)
		}
		k.keys[pid] = paead
	}
	return k, nil
}

const keyFormat = "<id>:<standard base64 of 32 random bytes>, such as k1:$(openssl rand -base64 32)"

// parseKey reads one "<id>:<base64>" entry. Its errors start with a verb so the caller can
// prefix the setting's name, and they never include the value.
func parseKey(s string) (string, cipher.AEAD, error) {
	id, encoded, ok := strings.Cut(s, ":")
	if !ok {
		return "", nil, fmt.Errorf("has no ':' between the key id and the key: %s", keyFormat)
	}
	if !keyID.MatchString(id) {
		return "", nil, fmt.Errorf("has a key id that is not 1 to 32 characters of a-z, 0-9 and '-': %s", keyFormat)
	}
	raw, err := base64.StdEncoding.Strict().DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return "", nil, fmt.Errorf("has a key that is not valid standard base64: %s", keyFormat)
	}
	if len(raw) != keySize {
		return "", nil, fmt.Errorf("has a key of %d bytes; it must be exactly %d: %s", len(raw), keySize, keyFormat)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return "", nil, fmt.Errorf("has a key AES refuses: %s", keyFormat)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", nil, fmt.Errorf("has a key GCM refuses: %s", keyFormat)
	}
	return id, aead, nil
}

// CurrentKeyID is the id of the key Encrypt uses. A stored row with another id was made before
// the last rotation.
func (k *Keyring) CurrentKeyID() string { return k.current }

// Encrypt seals plaintext with the current key, bound to aad, and returns that key's id and
// nonce || sealed text. aad must name the secret and its row (see the package comment).
func (k *Keyring) Encrypt(plaintext, aad []byte) (keyID string, ciphertext []byte, err error) {
	if len(aad) == 0 {
		return "", nil, ErrNoAAD
	}
	aead := k.keys[k.current]
	out := make([]byte, nonceSize, nonceSize+len(plaintext)+aead.Overhead())
	if _, err := rand.Read(out); err != nil {
		return "", nil, fmt.Errorf("secret: reading a random nonce: %w", err)
	}
	return k.current, aead.Seal(out, out[:nonceSize], plaintext, aad), nil
}

// Decrypt opens a ciphertext made by Encrypt with the key named keyID and the same aad. It
// returns ErrUnknownKey when the keyring has no such key and ErrDecrypt for anything that does
// not authenticate.
func (k *Keyring) Decrypt(keyID string, ciphertext, aad []byte) ([]byte, error) {
	aead, ok := k.keys[keyID]
	if !ok {
		return nil, ErrUnknownKey
	}
	if len(ciphertext) < nonceSize+aead.Overhead() {
		return nil, ErrDecrypt
	}
	plain, err := aead.Open(nil, ciphertext[:nonceSize], ciphertext[nonceSize:], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plain, nil
}

// Format prints only the key ids, so a keyring passed to a logger or fmt by mistake shows no
// key material whatever the verb.
func (k *Keyring) Format(f fmt.State, _ rune) {
	if k == nil {
		_, _ = fmt.Fprint(f, "secret.Keyring(nil)")
		return
	}
	_, _ = fmt.Fprintf(f, "secret.Keyring{current: %s, previous: %d}", k.current, len(k.keys)-1)
}
