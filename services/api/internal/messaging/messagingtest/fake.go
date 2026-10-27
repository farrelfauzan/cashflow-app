// Package messagingtest holds the in-memory messaging provider used by tests and local runs.
package messagingtest

import (
	"context"
	"fmt"
	"sync"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// Fake records every message instead of sending it.
type Fake struct {
	mu   sync.Mutex
	sent []messaging.Message
	next int
	err  error
}

var _ messaging.Provider = (*Fake)(nil)

// SendTemplate records m and returns a sequential fake provider id, or the error set by FailWith.
func (f *Fake) SendTemplate(_ context.Context, m messaging.Message) (messaging.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return messaging.Receipt{}, f.err
	}
	f.next++
	f.sent = append(f.sent, m)
	return messaging.Receipt{ProviderMessageID: fmt.Sprintf("fake-%d", f.next)}, nil
}

// FailWith makes every later send return err. Pass nil to send normally again.
func (f *Fake) FailWith(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

// Sent returns a copy of the messages sent so far, oldest first.
func (f *Fake) Sent() []messaging.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]messaging.Message(nil), f.sent...)
}
