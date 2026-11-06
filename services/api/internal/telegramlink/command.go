package telegramlink

import "strings"

// commandKind is what a private message asks the bot.
type commandKind int

const (
	// cmdOther is anything that is not /start or /stop, plain text included.
	cmdOther commandKind = iota
	cmdStart
	cmdStop
)

// String names the command for logs.
func (k commandKind) String() string {
	switch k {
	case cmdStart:
		return "start"
	case cmdStop:
		return "stop"
	default:
		return "other"
	}
}

// command is a parsed message: its kind and, for /start, the text after it (the code, or empty).
type command struct {
	kind commandKind
	arg  string
}

// parseCommand reads a message's text. A command is the first word when it starts with "/",
// matched without case and with an "@botname" suffix dropped (Telegram adds one when a command
// is picked from a menu). The deep link https://t.me/<bot>?start=<code> arrives as "/start <code>";
// for /start, arg is the words after the command joined by one space, so a code followed by more
// words is kept whole and later refused as malformed. /stop ignores what
// follows it.
func parseCommand(text string) command {
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return command{kind: cmdOther}
	}
	name, _, _ := strings.Cut(fields[0], "@")
	switch strings.ToLower(name) {
	case "/start":
		return command{kind: cmdStart, arg: strings.Join(fields[1:], " ")}
	case "/stop":
		return command{kind: cmdStop}
	default:
		return command{kind: cmdOther}
	}
}
