package telegramlink

import (
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
