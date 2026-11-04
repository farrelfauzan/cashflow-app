package secret

import (
	"fmt"
	"log/slog"
)

// redacted is what every printed form of a Value shows.
const redacted = "[redacted]"

// Value is a decrypted secret. Printing it in any way (fmt with any verb, slog, JSON, text
// encoding) shows "[redacted]"; only Reveal returns the text. Pass the Value around and call
// Reveal at the last moment, where the secret is put on the wire.
type Value struct {
	s string
}

// NewValue wraps a plain secret.
func NewValue(s string) Value { return Value{s: s} }

// Reveal returns the secret itself. Never log or print what it returns.
func (v Value) Reveal() string { return v.s }

// IsZero reports whether the secret is empty.
func (v Value) IsZero() bool { return v.s == "" }

// String redacts.
func (Value) String() string { return redacted }

// GoString redacts %#v.
func (Value) GoString() string { return redacted }

// Format redacts every fmt verb, including %v, %+v, %#v, %s, %q and %x.
func (Value) Format(f fmt.State, _ rune) { _, _ = fmt.Fprint(f, redacted) }

// LogValue redacts the secret in slog output.
func (Value) LogValue() slog.Value { return slog.StringValue(redacted) }

// MarshalJSON redacts the secret in JSON.
func (Value) MarshalJSON() ([]byte, error) { return []byte(`"` + redacted + `"`), nil }

// MarshalText redacts the secret in text encodings (and JSON map keys).
func (Value) MarshalText() ([]byte, error) { return []byte(redacted), nil }
