package linkcode_test

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink/linkcode"
)

const sampleCode = "Xk3v9q2LrT7mWb8NcY1pZs4uHf6gJd0eKa5oVi2nQwE"

func TestNew(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 50 {
		code, hash, err := linkcode.New()
		require.NoError(t, err)
		require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, code, "fits Telegram's start parameter: at most 64 of A-Z a-z 0-9 _ -")
		require.False(t, seen[code], "every code is fresh")
		seen[code] = true
		got, ok := linkcode.Hash(code)
		require.True(t, ok)
		require.Equal(t, hash, got, "the stored hash is the one a /start finds")
		require.Len(t, hash, sha256.Size)
	}
}

func TestHash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		code string
		ok   bool
	}{
		{"a code", sampleCode, true},
		{"one character short", sampleCode[:42], false},
		{"one character long", sampleCode + "A", false},
		{"base64 padding", sampleCode[:42] + "=", false},
		{"standard base64 characters", "+" + sampleCode[1:], false},
		{"a non-canonical last character", sampleCode[:42] + "F", false},
		{"a space inside", sampleCode[:20] + " " + sampleCode[21:], false},
		{"empty", "", false},
		{"long enough but not base64", strings.Repeat("!", 43), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			hash, ok := linkcode.Hash(tc.code)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Len(t, hash, sha256.Size)
			} else {
				require.Nil(t, hash)
			}
		})
	}
}

func TestDeepLink(t *testing.T) {
	t.Parallel()
	require.Equal(t, "https://t.me/cashflow_dev_bot?start="+sampleCode, linkcode.DeepLink("cashflow_dev_bot", sampleCode))
}
