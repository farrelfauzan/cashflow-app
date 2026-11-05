// Package autogopaytest holds an in-memory AutoGoPay used by tests and local runs. It has the
// same methods as autogopay.Client and returns the same errors, so a consumer's interface is
// satisfied by both.
package autogopaytest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// QRValidity is how long a generated QR can be paid, as at AutoGoPay.
const QRValidity = 15 * time.Minute

// ErrUnknownTransaction is returned by Pay and Expire for an id the fake never generated.
var ErrUnknownTransaction = errors.New("autogopaytest: unknown transaction")

// Method names a Fake method, for FailNext and Call.
type Method string

// The methods a Fake records.
const (
	MethodGenerate Method = "generate"
	MethodStatus   Method = "status"
	MethodCancel   Method = "cancel"
)

// Call is one call that reached the fake (calls refused before a request would be sent, such as
// an amount out of range, are not recorded, as the real client sends nothing).
type Call struct {
	Method        Method
	Key           string // the API key used, so tests can check which organization's key it was
	Amount        int64  // Generate only
	TransactionID string // Status and Cancel only
}

type transaction struct {
	qris   autogopay.QRIS
	status autogopay.TransactionStatus
	// key is the API key that generated it. Each organization has its own AutoGoPay merchant
	// account, so only that key can see the transaction; any other key reads it as not found.
	key string
}

// Fake is an in-memory AutoGoPay. The zero value is not usable; call New.
type Fake struct {
	clock clock.Clock

	mu    sync.Mutex
	next  int
	txs   map[string]*transaction
	calls []Call
	fail  map[Method][]error
}

// New returns a fake that reads the time from clk.
func New(clk clock.Clock) *Fake {
	return &Fake{clock: clk, txs: map[string]*transaction{}, fail: map[Method][]error{}}
}

// Generate creates a pending QR valid for QRValidity, with a sequential UUID-shaped id.
func (f *Fake) Generate(_ context.Context, key secret.Value, amount int64) (autogopay.QRIS, error) {
	if key.IsZero() {
		return autogopay.QRIS{}, fmt.Errorf("autogopaytest: generate: %w", autogopay.ErrInvalidAPIKey)
	}
	if amount < autogopay.MinAmount || amount > autogopay.MaxAmount {
		return autogopay.QRIS{}, fmt.Errorf("autogopaytest: generate: %w (got %d)", autogopay.ErrAmountOutOfRange, amount)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Method: MethodGenerate, Key: key.Reveal(), Amount: amount})
	if err := f.takeFailure(MethodGenerate); err != nil {
		return autogopay.QRIS{}, err
	}
	f.next++
	// AutoGoPay reports times to the second in Asia/Jakarta.
	now := f.clock.Now().In(clock.Jakarta).Truncate(time.Second)
	q := autogopay.QRIS{
		TransactionID: fmt.Sprintf("00000000-0000-4000-8000-%012x", f.next),
		OrderID:       fmt.Sprintf("FAKE-ORDER-%d", f.next),
		Amount:        amount,
		QRString:      fmt.Sprintf("FAKE-QRIS-%d", f.next),
		Status:        autogopay.StatusPending,
		CreatedAt:     now,
		ExpiresAt:     now.Add(QRValidity),
	}
	f.txs[q.TransactionID] = &transaction{qris: q, status: autogopay.StatusPending, key: key.Reveal()}
	return q, nil
}

// Status reports the transaction's state. A pending QR past its expiry reports Expired. Only the
// key that generated the transaction sees it; any other key gets ErrTransactionNotFound.
func (f *Fake) Status(_ context.Context, key secret.Value, transactionID string) (autogopay.Transaction, error) {
	return f.transaction(MethodStatus, key, transactionID, func(tx *transaction) {})
}

// Cancel cancels a pending QR. Like the real client it returns the state afterwards, so a paid
// QR stays Paid and an expired one Expired.
func (f *Fake) Cancel(_ context.Context, key secret.Value, transactionID string) (autogopay.Transaction, error) {
	return f.transaction(MethodCancel, key, transactionID, func(tx *transaction) {
		if tx.status == autogopay.StatusPending {
			tx.status = autogopay.StatusCancelled
		}
	})
}

func (f *Fake) transaction(m Method, key secret.Value, id string, change func(*transaction)) (autogopay.Transaction, error) {
	if key.IsZero() {
		return autogopay.Transaction{}, fmt.Errorf("autogopaytest: %s: %w", m, autogopay.ErrInvalidAPIKey)
	}
	if strings.TrimSpace(id) == "" {
		return autogopay.Transaction{}, fmt.Errorf("autogopaytest: %s: %w", m, autogopay.ErrInvalidTransactionID)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Method: m, Key: key.Reveal(), TransactionID: id})
	if err := f.takeFailure(m); err != nil {
		return autogopay.Transaction{}, err
	}
	tx, ok := f.txs[id]
	if !ok || tx.key != key.Reveal() {
		return autogopay.Transaction{}, fmt.Errorf("autogopaytest: %s: %w", m, autogopay.ErrTransactionNotFound)
	}
	f.expireIfDue(tx)
	change(tx)
	return autogopay.Transaction{ID: id, Status: tx.status, Time: tx.qris.CreatedAt}, nil
}

// expireIfDue turns a pending QR past its expiry into an expired one. Callers hold mu.
func (f *Fake) expireIfDue(tx *transaction) {
	if tx.status == autogopay.StatusPending && !f.clock.Now().Before(tx.qris.ExpiresAt) {
		tx.status = autogopay.StatusExpired
	}
}

// takeFailure pops the next injected error for m, if any. Callers hold mu.
func (f *Fake) takeFailure(m Method) error {
	errs := f.fail[m]
	if len(errs) == 0 {
		return nil
	}
	f.fail[m] = errs[1:]
	return errs[0]
}

// Pay marks a transaction paid, whatever its state, as a tenant paying it would.
func (f *Fake) Pay(id string) error {
	return f.set(id, autogopay.StatusPaid)
}

// Expire marks a transaction expired before its time.
func (f *Fake) Expire(id string) error {
	return f.set(id, autogopay.StatusExpired)
}

func (f *Fake) set(id string, s autogopay.TransactionStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	tx, ok := f.txs[id]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTransaction, id)
	}
	tx.status = s
	return nil
}

// FailNext makes the next call of m return err, once. Several calls queue up in order. The call
// is still recorded, as a request that reached AutoGoPay and failed.
func (f *Fake) FailNext(m Method, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail[m] = append(f.fail[m], err)
}

// Calls returns a copy of the calls so far, oldest first.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}
