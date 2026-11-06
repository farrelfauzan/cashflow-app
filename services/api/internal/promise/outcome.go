package promise

// Progress is where a promise's newest version stands: its terms and what happened since it was
// made.
type Progress struct {
	// PayingNow and PromisedAmount are the version's terms; PromisedAmount nil means everything
	// the version covers.
	PayingNow      int64
	PromisedAmount *int64
	// Paid is what the tenant paid since the version was made, after corrections to those
	// payments.
	Paid int64
	// CoveredSettled is true when none of the invoices the version covers owes anything now (a
	// cancelled invoice owes nothing).
	CoveredSettled bool
}

// Outcome is how a promise ends given its progress (decision 41.3): StatusKept,
// StatusPartiallyKept or StatusBroken.
//
// A promise is kept once every invoice it covers owes nothing (paid, or cancelled by a
// correction): nothing promised is left to pay. With a promised amount it is also kept once the
// tenant paid at least the target since the version, the target being what they said they pay now
// plus that amount. Otherwise some money since the version makes it partially kept and none makes
// it broken.
//
// It is evaluated at the end of the promised day and after every payment; a payment closes the
// promise only when it is kept.
func (p Progress) Outcome() string {
	if p.kept() {
		return StatusKept
	}
	if p.Paid > 0 {
		return StatusPartiallyKept
	}
	return StatusBroken
}

func (p Progress) kept() bool {
	if p.CoveredSettled {
		return true
	}
	return p.PromisedAmount != nil && p.Paid >= p.PayingNow+*p.PromisedAmount
}
