package secret_test

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// newKey returns "<id>:<base64 of 32 random bytes>", the SECRETS_KEY form.
func newKey(t *testing.T, id string) string {
	t.Helper()
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return id + ":" + base64.StdEncoding.EncodeToString(raw)
}

func mustKeyring(t *testing.T, current, previous string) *secret.Keyring {
	t.Helper()
	k, err := secret.ParseKeyring(current, previous)
	require.NoError(t, err)
	return k
}

const orgA = "autogopay_api_key:7d1c5a3e-0000-4000-8000-00000000000a"

func TestEncryptDecrypt(t *testing.T) {
	t.Parallel()
	k1 := newKey(t, "k1")
	ring := mustKeyring(t, k1, "")
	plain := []byte("agp_live_3f9a1c0b7e")
	aad := []byte(orgA)

	id, ct, err := ring.Encrypt(plain, aad)
	require.NoError(t, err)
	require.Equal(t, "k1", id)
	require.Len(t, ct, 12+len(plain)+16, "nonce || sealed text with a 16-byte tag")
	require.False(t, bytes.Contains(ct, plain), "the ciphertext must not contain the plain text")

	flip := func(i int) []byte {
		c := bytes.Clone(ct)
		c[i] ^= 0x01
		return c
	}
	sameIDOtherBytes := mustKeyring(t, newKey(t, "k1"), "")

	tests := []struct {
		name    string
		ring    *secret.Keyring
		keyID   string
		ct      []byte
		aad     []byte
		want    []byte
		wantErr error
	}{
		{name: "round trip", ring: ring, keyID: id, ct: ct, aad: aad, want: plain},
		{name: "another key under the same id", ring: sameIDOtherBytes, keyID: id, ct: ct, aad: aad, wantErr: secret.ErrDecrypt},
		{name: "a changed ciphertext byte", ring: ring, keyID: id, ct: flip(len(ct) - 20), aad: aad, wantErr: secret.ErrDecrypt},
		{name: "a changed tag byte", ring: ring, keyID: id, ct: flip(len(ct) - 1), aad: aad, wantErr: secret.ErrDecrypt},
		{name: "a changed nonce byte", ring: ring, keyID: id, ct: flip(0), aad: aad, wantErr: secret.ErrDecrypt},
		{name: "truncated by one byte", ring: ring, keyID: id, ct: ct[:len(ct)-1], aad: aad, wantErr: secret.ErrDecrypt},
		{name: "only the nonce", ring: ring, keyID: id, ct: ct[:12], aad: aad, wantErr: secret.ErrDecrypt},
		{name: "empty", ring: ring, keyID: id, ct: nil, aad: aad, wantErr: secret.ErrDecrypt},
		{name: "another organization's aad", ring: ring, keyID: id, ct: ct,
			aad: []byte("autogopay_api_key:7d1c5a3e-0000-4000-8000-00000000000b"), wantErr: secret.ErrDecrypt},
		{name: "no aad", ring: ring, keyID: id, ct: ct, aad: nil, wantErr: secret.ErrDecrypt},
		{name: "an unknown key id", ring: ring, keyID: "k2", ct: ct, aad: aad, wantErr: secret.ErrUnknownKey},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.ring.Decrypt(tc.keyID, tc.ct, tc.aad)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)
				require.NotContains(t, err.Error(), string(plain))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestEncryptIsRandomized(t *testing.T) {
	t.Parallel()
	ring := mustKeyring(t, newKey(t, "k1"), "")
	plain, aad := []byte("same key twice"), []byte(orgA)
	_, a, err := ring.Encrypt(plain, aad)
	require.NoError(t, err)
	_, b, err := ring.Encrypt(plain, aad)
	require.NoError(t, err)
	require.NotEqual(t, a, b)
	require.NotEqual(t, a[:12], b[:12], "each encryption uses a fresh nonce")
}

func TestEncryptRequiresAAD(t *testing.T) {
	t.Parallel()
	ring := mustKeyring(t, newKey(t, "k1"), "")
	for _, aad := range [][]byte{nil, {}} {
		_, _, err := ring.Encrypt([]byte("x"), aad)
		require.ErrorIs(t, err, secret.ErrNoAAD)
	}
}

func TestRotation(t *testing.T) {
	t.Parallel()
	k1, k2 := newKey(t, "k1"), newKey(t, "k2")
	plain, aad := []byte("agp_live_old"), []byte(orgA)

	old := mustKeyring(t, k1, "")
	oldID, oldCT, err := old.Encrypt(plain, aad)
	require.NoError(t, err)
	require.Equal(t, "k1", oldID)

	rotated := mustKeyring(t, k2, k1)
	require.Equal(t, "k2", rotated.CurrentKeyID())
	got, err := rotated.Decrypt(oldID, oldCT, aad)
	require.NoError(t, err, "a previous key still decrypts")
	require.Equal(t, plain, got)

	newID, newCT, err := rotated.Encrypt(plain, aad)
	require.NoError(t, err)
	require.Equal(t, "k2", newID, "new ciphertexts use the current key")
	got, err = rotated.Decrypt(newID, newCT, aad)
	require.NoError(t, err)
	require.Equal(t, plain, got)

	_, err = old.Decrypt(newID, newCT, aad)
	require.ErrorIs(t, err, secret.ErrUnknownKey, "the old keyring does not know k2")

	dropped := mustKeyring(t, k2, "")
	_, err = dropped.Decrypt(oldID, oldCT, aad)
	require.ErrorIs(t, err, secret.ErrUnknownKey, "a key removed from SECRETS_KEY_PREVIOUS no longer decrypts")
}

func TestParseKeyring(t *testing.T) {
	t.Parallel()
	k1, k2, k3 := newKey(t, "k1"), newKey(t, "2026-11-k2"), newKey(t, "k3")
	b64 := func(n int) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, n)) }
	good := b64(32)
	tests := []struct {
		name        string
		current     string
		previous    string
		wantErr     string
		wantCurrent string
	}{
		{name: "current only", current: k1, wantCurrent: "k1"},
		{name: "surrounding spaces are ignored", current: "  " + k1 + "\n", previous: " ", wantCurrent: "k1"},
		{name: "current and two previous", current: k1, previous: k2 + ", " + k3, wantCurrent: "k1"},
		{name: "missing", current: "", wantErr: "SECRETS_KEY is required"},
		{name: "no id", current: good, wantErr: "SECRETS_KEY has no ':'"},
		{name: "empty id", current: ":" + good, wantErr: "SECRETS_KEY has a key id"},
		{name: "upper-case id", current: "K1:" + good, wantErr: "SECRETS_KEY has a key id"},
		{name: "id with an underscore", current: "k_1:" + good, wantErr: "SECRETS_KEY has a key id"},
		{name: "id longer than 32", current: strings.Repeat("a", 33) + ":" + good, wantErr: "SECRETS_KEY has a key id"},
		{name: "id of 32 is fine", current: strings.Repeat("a", 32) + ":" + good, wantCurrent: strings.Repeat("a", 32)},
		{name: "bad base64", current: "k1:not*base64", wantErr: "SECRETS_KEY has a key that is not valid standard base64"},
		{name: "url-safe base64 alphabet", current: "k1:" + strings.NewReplacer("+", "-", "/", "_").Replace(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32))),
			wantErr: "not valid standard base64"},
		{name: "16 bytes", current: "k1:" + b64(16), wantErr: "SECRETS_KEY has a key of 16 bytes; it must be exactly 32"},
		{name: "31 bytes", current: "k1:" + b64(31), wantErr: "of 31 bytes"},
		{name: "33 bytes", current: "k1:" + b64(33), wantErr: "of 33 bytes"},
		{name: "64 bytes", current: "k1:" + b64(64), wantErr: "of 64 bytes"},
		{name: "a bad previous key", current: k1, previous: k2 + ",k9:" + b64(16), wantErr: "SECRETS_KEY_PREVIOUS entry 2 has a key of 16 bytes"},
		{name: "a previous key with a bad id", current: k1, previous: "K2:" + good, wantErr: "SECRETS_KEY_PREVIOUS entry 1 has a key id"},
		{name: "an empty previous entry", current: k1, previous: k2 + ",", wantErr: "SECRETS_KEY_PREVIOUS entry 2 is empty"},
		{name: "a previous key with the current id", current: k1, previous: "k1:" + good, wantErr: `SECRETS_KEY_PREVIOUS entry 1 uses the id "k1" of SECRETS_KEY`},
		{name: "a previous id twice", current: k1, previous: k3 + "," + "k3:" + good, wantErr: `SECRETS_KEY_PREVIOUS names the key id "k3" twice`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ring, err := secret.ParseKeyring(tc.current, tc.previous)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				require.Nil(t, ring)
				// No part of a key reaches an error message.
				for _, s := range []string{good, b64(16), k1, k2, k3, keyPart(k1), keyPart(k2), keyPart(k3)} {
					require.NotContains(t, err.Error(), s)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantCurrent, ring.CurrentKeyID())
		})
	}
}

// keyPart is the base64 half of "<id>:<base64>".
func keyPart(s string) string {
	_, k, _ := strings.Cut(s, ":")
	return k
}

func TestKeyringPrintsNoKeyMaterial(t *testing.T) {
	t.Parallel()
	k1 := newKey(t, "k1")
	ring := mustKeyring(t, k1, newKey(t, "k0"))
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%x"} {
		got := fmt.Sprintf(verb, ring)
		require.Equal(t, "secret.Keyring{current: k1, previous: 1}", got, verb)
	}
	var nilRing *secret.Keyring
	require.Equal(t, "secret.Keyring(nil)", fmt.Sprint(nilRing))
}

func TestValueRedacts(t *testing.T) {
	t.Parallel()
	const plain = "agp_live_do_not_print"
	v := secret.NewValue(plain)
	require.Equal(t, plain, v.Reveal())
	require.False(t, v.IsZero())
	require.True(t, secret.Value{}.IsZero())

	var logs bytes.Buffer
	jsonLog := slog.New(slog.NewJSONHandler(&logs, nil))
	jsonLog.Info("key", "value", v)
	textLog := slog.New(slog.NewTextHandler(&logs, nil))
	textLog.Info("key", "value", v, slog.Any("any", v))

	asJSON, err := json.Marshal(struct {
		Key secret.Value            `json:"key"`
		Map map[secret.Value]string `json:"map"`
	}{Key: v, Map: map[secret.Value]string{v: "x"}})
	require.NoError(t, err)

	tests := []struct {
		name string
		got  string
	}{
		{name: "String", got: v.String()},
		{name: "GoString", got: v.GoString()},
		{name: "%v", got: fmt.Sprintf("%v", v)},
		{name: "%+v", got: fmt.Sprintf("%+v", v)},
		{name: "%#v", got: fmt.Sprintf("%#v", v)},
		{name: "%s", got: fmt.Sprintf("%s", v)},
		{name: "%q", got: fmt.Sprintf("%q", v)},
		{name: "%x", got: fmt.Sprintf("%x", v)},
		{name: "pointer %v", got: fmt.Sprintf("%v", &v)},
		{name: "inside a struct %+v", got: fmt.Sprintf("%+v", struct{ Key secret.Value }{v})},
		{name: "inside a struct %#v", got: fmt.Sprintf("%#v", struct{ Key secret.Value }{v})},
		{name: "Sprint", got: fmt.Sprint(v)},
		{name: "error wrapping", got: fmt.Errorf("calling with %v", v).Error()},
		{name: "slog", got: logs.String()},
		{name: "json", got: string(asJSON)},
		{name: "LogValue", got: v.LogValue().String()},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.NotContains(t, tc.got, plain)
			require.Contains(t, tc.got, "[redacted]")
		})
	}
}
