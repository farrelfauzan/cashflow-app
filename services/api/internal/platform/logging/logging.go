// Package logging builds the process logger: structured JSON on the given writer, which is
// stdout in every binary.
package logging

import (
	"fmt"
	"io"
	"log/slog"
)

// New returns a JSON logger at the given level (debug, info, warn or error).
func New(w io.Writer, level string) (*slog.Logger, error) {
	var lv slog.Level
	if err := lv.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("parsing log level %q: %w", level, err)
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: lv})), nil
}
