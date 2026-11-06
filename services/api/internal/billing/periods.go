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
	last, bills := lastBilledStart(l)
	if !bills {
		return nil
	}
	horizon := today.AddDate(0, 0, LeadDays)

	var out []tenant.Period
	for n := firstBilledIndex(l); ; n++ {
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

// NextDueAfter returns the earliest due date strictly after today among the lease's billed
// periods (from BilledFrom on, none starting after the planned end date), whether or not that
// period has an invoice yet, and false when there is none: the lease has ended (its End is set),
// or every period after today starts after the planned end date.
//
// Due dates are not in period order for a lease entered late: its current period on the entry
// day is due 7 days after it, which can be after the next period's first day. So it scans every
// period up to the first that starts after today; from there on a period is due on its first day.
func NextDueAfter(l LeaseDates, today time.Time) (time.Time, bool) {
	if l.End != nil {
		return time.Time{}, false
	}
	last, bills := lastBilledStart(l)
	if !bills {
		return time.Time{}, false
	}
	var best time.Time
	found := false
	// A guess one period before the one running today, so a lease that started years ago does
	// not scan from its start; periods before it are due on or before today.
	from := max(firstBilledIndex(l), tenant.PeriodIndex(l.Start, l.TermMonths, l.EnteredOn)-1, 0)
	for n := from; ; n++ {
		p := tenant.PeriodAt(l.Start, l.EnteredOn, l.TermMonths, n)
		if last != nil && p.From.After(*last) {
			break
		}
		if p.Due.After(today) && (!found || p.Due.Before(best)) {
			best, found = p.Due, true
		}
		if p.From.After(today) {
			break // due on its first day, as is every later period
		}
	}
	return best, found
}

// lastBilledStart returns the last day a period of the lease may start on and still be billed:
// the planned end date or the end date, whichever is earlier, or nil for no limit. bills is false
// for a lease ended on its own start date, a cancelled booking, which bills nothing (decisions 17
// and 25).
func lastBilledStart(l LeaseDates) (last *time.Time, bills bool) {
	if l.End != nil && l.End.Equal(l.Start) {
		return nil, false
	}
	last = l.End
	if l.PlannedEnd != nil && (last == nil || l.PlannedEnd.Before(*last)) {
		last = l.PlannedEnd
	}
	return last, true
}

// firstBilledIndex is the index of the lease's first invoiced period ("Mulai tagih dari").
func firstBilledIndex(l LeaseDates) int {
	return max(tenant.PeriodIndex(l.Start, l.TermMonths, l.BilledFrom), 0)
}

// NextInvoice is the next invoice generation will make for a lease: the period it bills (its
// first day) and the Asia/Jakarta day it is made. A metered amount recorded before that day is
// billed on it.
type NextInvoice struct {
	Period   time.Time
	IssuedOn time.Time
}

// NextInvoiceOf returns the lease's next invoice on today, given the period starts that already
// have an invoice, and false when no further invoice will be generated: the lease has ended, or
// every period up to the planned end date is invoiced already.
//
// It follows DuePeriods: the first period from BilledFrom with no invoice is made LeadDays
// before its due date, or today when that day is past (the daily run, or the one after it, makes
// it). The run on that day makes every missing period due within the lead time, and generation
// puts the metered amounts on the newest of them, so that is the period returned. Usually it is
// the same one; it differs only for a lease whose invoices have fallen behind.
func NextInvoiceOf(l LeaseDates, today time.Time, invoiced []time.Time) (NextInvoice, bool) {
	if l.End != nil {
		return NextInvoice{}, false // generation makes nothing more for an ended lease
	}
	last, bills := lastBilledStart(l)
	if !bills {
		return NextInvoice{}, false
	}
	have := make(map[string]bool, len(invoiced))
	for _, d := range invoiced {
		have[d.Format(time.DateOnly)] = true
	}
	// Every iteration either returns or passes a period that has an invoice, so the loop ends
	// after at most len(invoiced)+1 periods.
	for n := firstBilledIndex(l); ; n++ {
		p := tenant.PeriodAt(l.Start, l.EnteredOn, l.TermMonths, n)
		if last != nil && p.From.After(*last) {
			return NextInvoice{}, false
		}
		if have[p.From.Format(time.DateOnly)] {
			continue
		}
		issued := p.Due.AddDate(0, 0, -LeadDays)
		if issued.Before(today) {
			issued = today
		}
		newest := p
		for _, d := range DuePeriods(l, issued) {
			if !have[d.From.Format(time.DateOnly)] {
				newest = d
			}
		}
		return NextInvoice{Period: newest.From, IssuedOn: issued}, true
	}
}
