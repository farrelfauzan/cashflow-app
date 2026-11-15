package billing

import (
	"errors"

	"github.com/google/uuid"
)

var (
	// ErrPreviewAmountInvalid means a previewed amount below Rp1.
	ErrPreviewAmountInvalid = errors.New("billing: preview amount must be at least 1")
	// ErrAmountAboveOutstanding means a previewed amount above what the tenant owes, including
	// when they owe nothing.
	ErrAmountAboveOutstanding = errors.New("billing: amount above outstanding")
)

// OpenBalance is what one invoice owes, as a payment preview sees it.
type OpenBalance struct {
	InvoiceID uuid.UUID
	Balance   int64 // rupiah still owed; zero or less owes nothing and is skipped
}

// PreviewItem is what a previewed amount would pay on one invoice.
type PreviewItem struct {
	InvoiceID    uuid.UUID
	Balance      int64 // owed before the amount
	Applied      int64 // paid by the amount; always above zero
	BalanceAfter int64 // Balance minus Applied
	PaidInFull   bool  // BalanceAfter is zero
}

// Preview is what paying an amount now would pay.
type Preview struct {
	Amount            int64
	OutstandingBefore int64 // the sum of every positive balance given
	OutstandingAfter  int64 // OutstandingBefore minus what the amount pays
	// CreditAdded is the part of the amount no invoice takes, which becomes credit; always zero
	// from PreviewPayment.
	CreditAdded int64
	// Items are the invoices the amount reaches, in the order given; the last may be paid in part.
	Items []PreviewItem
}

// PreviewPayment walks open, which must already be in FIFO order (oldest period first, then
// seq, as LoadAccountTx and Plan order invoices), and spends amount on each balance in turn. It
// writes nothing: it tells the tenant what a payment of amount would pay before they make it.
// An amount below 1 is ErrPreviewAmountInvalid; one above the sum of the balances is
// ErrAmountAboveOutstanding.
func PreviewPayment(open []OpenBalance, amount int64) (Preview, error) {
	if amount < 1 {
		return Preview{}, ErrPreviewAmountInvalid
	}
	if amount > owed(open) {
		return Preview{}, ErrAmountAboveOutstanding
	}
	return walkPayment(open, amount), nil
}

// PreviewRecordedPayment is PreviewPayment for a payment a member records (a transfer they
// match, say): an amount above what is owed is allowed, and the rest is CreditAdded, as FIFO
// allocation turns an overpayment into credit. An amount below 1 is ErrPreviewAmountInvalid.
func PreviewRecordedPayment(open []OpenBalance, amount int64) (Preview, error) {
	if amount < 1 {
		return Preview{}, ErrPreviewAmountInvalid
	}
	return walkPayment(open, amount), nil
}

// owed is the sum of the positive balances.
func owed(open []OpenBalance) int64 {
	var before int64
	for _, o := range open {
		if o.Balance > 0 {
			before += o.Balance
		}
	}
	return before
}

// walkPayment spends amount on each positive balance in turn; what is left is CreditAdded.
func walkPayment(open []OpenBalance, amount int64) Preview {
	before := owed(open)
	paid := min(amount, before)
	out := Preview{
		Amount: amount, OutstandingBefore: before, OutstandingAfter: before - paid, CreditAdded: amount - paid,
		Items: []PreviewItem{},
	}
	left := amount
	for _, o := range open {
		if left == 0 {
			break
		}
		if o.Balance <= 0 {
			continue
		}
		applied := min(left, o.Balance)
		left -= applied
		out.Items = append(out.Items, PreviewItem{
			InvoiceID: o.InvoiceID, Balance: o.Balance, Applied: applied,
			BalanceAfter: o.Balance - applied, PaidInFull: applied == o.Balance,
		})
	}
	return out
}
