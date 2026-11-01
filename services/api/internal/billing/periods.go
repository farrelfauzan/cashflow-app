package billing

import (
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// LeadDays is how long before its due date an invoice is generated (decision 36), so it exists
// before the H-3 reminder.
const LeadDays = 7

// LeaseDates are the fields of a lease that decide which periods are invoiced. Dates are
// calendar days, as midnight UTC like a date column.
type LeaseDates struct {
	Start      time.Time
	EnteredOn  time.Time // the Asia/Jakarta day the lease was saved
	BilledFrom time.Time // "Mulai tagih dari": the first period start that is invoiced
	TermMonths int
	PlannedEnd *time.Time // "Rencana keluar", the tenant's planned last day
	End        *time.Time // the last day, once the lease has ended
}

// DuePeriods returns the periods of a lease that should have an invoice on today, oldest first:
// every period from BilledFrom on whose due date is within LeadDays of today, except
//
//   - none for a lease ended on its own start date, a cancelled booking (decision 17);
//   - none for a period that starts after the planned end date or the end date, whichever is
//     earlier (decision 25). A period that starts on or before it is billed in full.
//
// Periods and due dates come from tenant.PeriodAt, so a lease entered late has its periods
// before the one running on the entry day due on that day and that one 7 days after, and any
// term of 1 to 60 months counts its anniversaries from the start date (the 29th to 31st rule).
// It returns the periods whether or not they already have an invoice; the caller skips those.
func DuePeriods(l LeaseDates, today time.Time) []tenant.Period {
	if l.End != nil && l.End.Equal(l.Start) {
		return nil
	}
	last := l.End
	if l.PlannedEnd != nil && (last == nil || l.PlannedEnd.Before(*last)) {
		last = l.PlannedEnd
	}
	horizon := today.AddDate(0, 0, LeadDays)
	first := max(tenant.PeriodIndex(l.Start, l.TermMonths, l.BilledFrom), 0)

	var out []tenant.Period
	for n := first; ; n++ {
		p := tenant.PeriodAt(l.Start, l.EnteredOn, l.TermMonths, n)
		// A period that starts after the horizon is due on its first day (it starts after the
		// entry day, which is never after today), and so is every later one.
		if p.From.After(horizon) || (last != nil && p.From.After(*last)) {
			return out
		}
		if !p.Due.After(horizon) {
			out = append(out, p)
		}
	}
}
