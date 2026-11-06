package telegramlink

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleCode = "Xk3v9q2LrT7mWb8NcY1pZs4uHf6gJd0eKa5oVi2nQwE"

func TestParseCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want command
	}{
		{"start with a code, as the deep link sends it", "/start " + sampleCode, command{kind: cmdStart, arg: sampleCode}},
		{"start alone, as the Start button sends it without a code", "/start", command{kind: cmdStart}},
		{"start with surrounding space", "  /start \t" + sampleCode + " \n", command{kind: cmdStart, arg: sampleCode}},
		{"start picked from the menu carries the bot's name", "/start@cashflow_dev_bot " + sampleCode, command{kind: cmdStart, arg: sampleCode}},
		{"start in capitals", "/START " + sampleCode, command{kind: cmdStart, arg: sampleCode}},
		{"start with more words keeps them, to be refused", "/start " + sampleCode + " tolong", command{kind: cmdStart, arg: sampleCode + " tolong"}},
		{"start on its own line before the code", "/start\n" + sampleCode, command{kind: cmdStart, arg: sampleCode}},
		{"stop", "/stop", command{kind: cmdStop}},
		{"stop with the bot's name and words after it", "/stop@cashflow_dev_bot sekarang", command{kind: cmdStop}},
		{"stop in mixed case", "/Stop", command{kind: cmdStop}},
		{"plain text", "halo, saya mau bayar", command{kind: cmdOther}},
		{"the word start without a slash", "start " + sampleCode, command{kind: cmdOther}},
		{"another command", "/help", command{kind: cmdOther}},
		{"a command that only begins like start", "/startnow " + sampleCode, command{kind: cmdOther}},
		{"a slash alone", "/", command{kind: cmdOther}},
		{"empty", "", command{kind: cmdOther}},
		{"only space", " \n\t ", command{kind: cmdOther}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, parseCommand(tc.text))
		})
	}
}

func TestCommandKindNames(t *testing.T) {
	t.Parallel()
	require.Equal(t, "start", cmdStart.String())
	require.Equal(t, "stop", cmdStop.String())
	require.Equal(t, "other", cmdOther.String())
}

func TestNewCode(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 50 {
		code, hash, err := newCode()
		require.NoError(t, err)
		require.Regexp(t, `^[A-Za-z0-9_-]{43}$`, code, "fits Telegram's start parameter: at most 64 of A-Z a-z 0-9 _ -")
		require.False(t, seen[code], "every code is fresh")
		seen[code] = true
		got, ok := codeHash(code)
		require.True(t, ok)
		require.Equal(t, hash, got, "the stored hash is the one a /start finds")
		require.Len(t, hash, sha256.Size)
	}
}

func TestCodeHash(t *testing.T) {
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
			hash, ok := codeHash(tc.code)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Len(t, hash, sha256.Size)
			} else {
				require.Nil(t, hash)
			}
		})
	}
}
