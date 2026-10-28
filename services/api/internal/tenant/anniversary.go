package tenant

import "time"

// Anniversary returns the start date plus n months (n >= 0): the day a lease's n-th period
// begins and, for a regular invoice, is due (decision 24). When that month is shorter than the
// start day it is the month's last day, and it is always counted from the start date, never
// from the previous anniversary: a lease starting 31 Jan has 28 Feb (29 in a leap year), then
// 31 Mar. Dates are calendar days, as midnight UTC.
func Anniversary(start time.Time, n int) time.Time {
	start = calendarDay(start)
	first := time.Date(start.Year(), start.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
	lastDay := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(start.Day(), lastDay), 0, 0, 0, 0, time.UTC)
}

// IsAnniversary reports whether d is the start date or one of its anniversaries.
func IsAnniversary(start, d time.Time) bool {
	start, d = calendarDay(start), calendarDay(d)
	if d.Before(start) {
		return false
	}
	return Anniversary(start, monthsBetween(start, d)).Equal(d)
}

// FirstAnniversaryAfter returns the first anniversary of start that is strictly after d. For a
// d before the start date, it is the start date.
func FirstAnniversaryAfter(start, d time.Time) time.Time {
	start, d = calendarDay(start), calendarDay(d)
	if d.Before(start) {
		return start
	}
	n := monthsBetween(start, d)
	for !Anniversary(start, n).After(d) {
		n++
	}
	return Anniversary(start, n)
}

// monthsBetween is the number of whole calendar months from start's month to d's month.
func monthsBetween(start, d time.Time) int {
	return (d.Year()-start.Year())*12 + int(d.Month()-start.Month())
}
