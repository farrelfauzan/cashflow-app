package qris

import (
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
)

// ReuseMargin is how long a live QR must still be valid to be shown again instead of replaced.
// A QR with less left would die while the tenant opens their banking app.
const ReuseMargin = 2 * time.Minute

// step is what making a QR does with the tenant's latest attempt.
type step int

const (
	// stepGenerate: there is no pending attempt; generate a new QR.
	stepGenerate step = iota
	// stepReuse: the pending attempt is live, for the same amount, with time left; show it again.
	stepReuse
	// stepReplace: a pending attempt is in the way (another amount, nearly or already expired);
	// close it at AutoGoPay first, then generate.
	stepReplace
)

// planMake decides what making a QR for amount at now does with the tenant's latest attempt a
// (nil when there is none). AutoGoPay treats repeated generates as spam, so a live QR for the same
// amount is shown again rather than replaced.
func planMake(a *Attempt, amount int64, now time.Time) step {
	if a == nil || a.Status != AttemptPending {
		return stepGenerate
	}
	if a.Amount == amount && now.Add(ReuseMargin).Before(a.ExpiresAt) {
		return stepReuse
	}
	return stepReplace
}

// outcome is what an AutoGoPay answer about an attempt means for it.
type outcome int

const (
	// outcomeOpen: still pending at AutoGoPay; nothing to record.
	outcomeOpen outcome = iota
	// outcomePaid: record the payment and mark the attempt paid.
	outcomePaid
	// outcomeExpired and outcomeCancelled close the attempt.
	outcomeExpired
	outcomeCancelled
)

// outcomeOf maps a transaction status AutoGoPay reported.
func outcomeOf(s autogopay.TransactionStatus) (outcome, bool) {
	switch s {
	case autogopay.StatusPending:
		return outcomeOpen, true
	case autogopay.StatusPaid:
		return outcomePaid, true
	case autogopay.StatusExpired:
		return outcomeExpired, true
	case autogopay.StatusCancelled:
		return outcomeCancelled, true
	default:
		return outcomeOpen, false
	}
}
