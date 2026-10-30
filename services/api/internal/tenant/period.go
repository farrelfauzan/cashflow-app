package tenant

import (
	"slices"
	"time"
)

// StandardTerms are the billing terms, in months, every unit may offer (decision 32), ascending.
var StandardTerms = []int{1, 3, 6, 12}

// MaxTermMonths is the longest term a lease may be billed by. Besides the standard terms a unit
// may offer one custom term of 2 to MaxTermMonths months (decision 33).
const MaxTermMonths = 60

// ValidTerm reports whether months is a term a lease may be billed by: a standard term, or a
// custom one of 2 to MaxTermMonths months.
func ValidTerm(months int) bool { return months >= 1 && months <= MaxTermMonths }

// IsStandardTerm reports whether months is one of StandardTerms. Any other valid term is the
// unit's custom term, and a unit offers at most one.
func IsStandardTerm(months int) bool { return slices.Contains(StandardTerms, months) }

// lateEntryGrace is how long after the entry day the period running on that day is due, for a
// lease entered after it started.
const lateEntryGrace = 7

// Period is one billing period of a lease: From to To, both included, due on Due. N counts the
// periods from 0, the one that starts on the start date.
type Period struct {
	N             int
	From, To, Due time.Time
}

// PeriodStart returns the day a lease's n-th period (n >= 0) begins: the start date plus n
// times the term in months, with the month-end rule of Anniversary, so it is always counted
// from the start date. A 3-month lease starting 31 Jan has periods from 31 Jan, 30 Apr, 31 Jul.
func PeriodStart(start time.Time, term, n int) time.Time {
	return Anniversary(start, n*term)
}

// PeriodAt returns a lease's n-th period (n >= 0) for its term. The period ends the day before
// the next one starts. It is due on its first day (rent is paid in advance), except for a lease
// entered late, whose start date is before the day it was entered (decision 24, revised): its
// periods that ended before the entry day are due on the entry day, and its current period, the
// one running on the entry day (also when it starts that day), is due 7 days after it. Later
// periods are due on their first day, as for any lease.
func PeriodAt(start, enteredOn time.Time, term, n int) Period {
	start, enteredOn = calendarDay(start), calendarDay(enteredOn)
	p := Period{N: n, From: PeriodStart(start, term, n)}
	next := PeriodStart(start, term, n+1)
	p.To = next.AddDate(0, 0, -1)
	switch {
	case !start.Before(enteredOn) || p.From.After(enteredOn):
		p.Due = p.From
	case next.After(enteredOn):
		p.Due = enteredOn.AddDate(0, 0, lateEntryGrace)
	default:
		p.Due = enteredOn
	}
	return p
}

// PeriodIndex returns the n of the period that contains d, or -1 when d is before the start
// date.
func PeriodIndex(start time.Time, term int, d time.Time) int {
	start, d = calendarDay(start), calendarDay(d)
	if d.Before(start) {
		return -1
	}
	// A guess from the calendar months between them; the loops correct it by a period or so.
	n := monthsBetween(start, d) / term
	for n > 0 && PeriodStart(start, term, n).After(d) {
		n--
	}
	for !PeriodStart(start, term, n+1).After(d) {
		n++
	}
	return n
}

// IsPeriodStart reports whether d is the first day of one of the lease's periods for its term.
func IsPeriodStart(start time.Time, term int, d time.Time) bool {
	n := PeriodIndex(start, term, d)
	return n >= 0 && PeriodStart(start, term, n).Equal(calendarDay(d))
}

// NextPeriodStart returns the first period start strictly after d. For a d before the start
// date, it is the start date.
func NextPeriodStart(start time.Time, term int, d time.Time) time.Time {
	return PeriodStart(start, term, PeriodIndex(start, term, d)+1)
}
