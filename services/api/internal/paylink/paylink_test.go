package paylink

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

func TestNewTokenRoundTrips(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 100 {
		token, hash, err := newToken()
		require.NoError(t, err)
		require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, token)
		require.False(t, seen[token], "tokens never repeat")
		seen[token] = true

		got, ok := TokenHash(token)
		require.True(t, ok)
		require.Equal(t, hash, got, "the hash checked on the way in is the hash stored")
		raw, err := base64.RawURLEncoding.DecodeString(token)
		require.NoError(t, err)
		sum := sha256.Sum256(raw)
		require.Equal(t, sum[:], got, "the hash is SHA-256 of the 32 bytes, not of the text")
	}
}

func TestTokenHashRefusesAnythingButAToken(t *testing.T) {
	t.Parallel()
	token, _, err := newToken()
	require.NoError(t, err)
	// The last of 43 characters carries 2 unused bits; flipping one gives a second spelling of
	// the same bytes, which a strict decoder refuses.
	last := strings.IndexByte(base64URLAlphabet, token[42])
	nonCanonical := token[:42] + string(base64URLAlphabet[last^1])

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"one character short", token[:42]},
		{"one character long", token + "A"},
		{"standard base64 characters", token[:41] + "+/"},
		{"padding", token[:42] + "="},
		{"a space", token[:42] + " "},
		{"a path separator", token[:20] + "/" + token[21:]},
		{"non-ASCII of the same byte length", "é" + token[2:]},
		{"a non-canonical last character", nonCanonical},
		{"a uuid", "6f1c2a3e-0000-4000-8000-000000000001"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hash, ok := TokenHash(tc.token)
			require.False(t, ok)
			require.Nil(t, hash)
		})
	}
}

const base64URLAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func TestLimitKeyHidesTheToken(t *testing.T) {
	t.Parallel()
	token, _, err := newToken()
	require.NoError(t, err)
	key := LimitKey(token)
	require.NotContains(t, key, token)
	require.Regexp(t, `^paylink:token:[0-9a-f]{64}$`, key)
	require.Equal(t, key, LimitKey(token), "the same token always counts against the same key")
	require.NotEqual(t, key, LimitKey(token[:42]+"x"))
}

// TestExpired is the validity rule with the clock in Asia/Jakarta: a link works while its invoice
// owes anything, and for 30 days after the invoice was paid or cancelled.
func TestExpired(t *testing.T) {
	t.Parallel()
	settled := time.Date(2026, 10, 3, 23, 30, 0, 0, clock.Jakarta) // late evening, Jakarta
	tests := []struct {
		name        string
		now         time.Time
		outstanding int64
		want        bool
	}{
		{"open, long after the last payment", settled.AddDate(1, 0, 0), 250_000, false},
		{"open, nothing paid", settled, 700_000, false},
		{"paid just now", settled, 0, false},
		{"paid 29 days ago", settled.AddDate(0, 0, 29), 0, false},
		{"paid 30 days ago less a minute", settled.AddDate(0, 0, 30).Add(-time.Minute), 0, false},
		{"paid exactly 30 days ago", settled.AddDate(0, 0, 30), 0, true},
		{"paid 30 days ago, across the month end", time.Date(2026, 11, 2, 23, 30, 0, 0, clock.Jakarta), 0, true},
		{"paid 31 days ago", settled.AddDate(0, 0, 31), 0, true},
		{"overpaid 30 days ago (never below zero, but fails closed)", settled.AddDate(0, 0, 30), -1, true},
		{"cancelled by a correction 29 days ago", settled.AddDate(0, 0, 29), 0, false},
		{"cancelled by a correction 30 days ago", settled.AddDate(0, 0, 30), 0, true},
		{"reopened by a correction after it expired", settled.AddDate(0, 0, 45), 50_000, false},
		{"the same moment in UTC", settled.AddDate(0, 0, 30).UTC(), 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := clock.NewFake(tc.now)
			require.Equal(t, tc.want, Expired(c.Now(), tc.outstanding, settled))
		})
	}
}
