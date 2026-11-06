// Package reminder decides, once a day per organization, what each tenant should be told about
// their invoices and promises (phase B, slice 4, S4-4): the H-3, H0 and H+3 invoice reminders,
// the H-1 of a promised date, the notice of a broken promise, and the H+7 karyawan visit. It
// records the decisions as reminder rows, and Sender sends them (S4-5), along with the
// confirmation of every saved promise version.
//
// The rules are the pure function Decide. Decider runs them for every tenant of an
// organization, each in its own transaction under the tenant's allocation lock, and remindertask
// runs Decider from the worker at 08:00 Asia/Jakarta (decision 41.6).
package reminder

import (
	"time"

	"github.com/google/uuid"
)

// Invoice reminder stages, as stored in reminder_invoice.stage.
const (
	StageHMinus3 = "h_minus_3" // 3 to 1 days before the due date
	StageH0      = "h0"        // the due date to 2 days after it
	StageHPlus3  = "h_plus_3"  // 3 to 6 days after the due date
)

// VisitReasonH7 is the reason of the visit task a tenant gets when an invoice is still unpaid 7
// days after its due date (visit_task.reason).
const VisitReasonH7 = "h7"

// BrokenNoticeDays is how many days back a broken promise is still notified: a promise closed
// broken on day D is notified by the first run from D to D+3. Older ones never are, so a first
// run after a pause sends no flood of stale notices.
const BrokenNoticeDays = 3

// Invoice is one of the tenant's invoices for the rules.
type Invoice struct {
	ID uuid.UUID
	// DueDate is the invoice's due date, a calendar day as midnight UTC like a date column.
	DueDate time.Time
	// Outstanding is what the invoice still owes; a paid or cancelled invoice owes nothing and
	// gets nothing.
	Outstanding int64
	// Covered says whether the newest version of the tenant's active promise covers the invoice.
	Covered bool
}

// InvoiceStage is one stage of one invoice.
type InvoiceStage struct {
	InvoiceID uuid.UUID
	Stage     string
}

// ActivePromise is the newest version of the tenant's active promise.
type ActivePromise struct {
	VersionID uuid.UUID
	// PromisedDate is the promised day, a calendar day as midnight UTC.
	PromisedDate time.Time
	// Reminded says whether this version's H-1 is already recorded.
	Reminded bool
}

// BrokenPromise is a promise of the tenant that closed broken.
type BrokenPromise struct {
	ID uuid.UUID
	// ClosedOn is the Asia/Jakarta calendar day it closed, as midnight UTC.
	ClosedOn time.Time
	// Notified says whether its broken notice is already recorded.
	Notified bool
}

// Input is what the rules know about one tenant on one day.
type Input struct {
	// Today is the Asia/Jakarta calendar day of the run, as midnight UTC.
	Today    time.Time
	Invoices []Invoice
	// Recorded holds the invoice stages already recorded, ever.
	Recorded map[InvoiceStage]bool
	// Promise is the tenant's active promise, nil when there is none.
	Promise *ActivePromise
	// Broken are the tenant's promises closed broken, at least those closed in the last
	// BrokenNoticeDays days.
	Broken []BrokenPromise
	// LastH7VisitResolvedOn is the Asia/Jakarta calendar day (midnight UTC) the tenant's newest
	// resolved h7 visit task was resolved, nil when none was.
	LastH7VisitResolvedOn *time.Time
}

// Decision is what the rules decided for one tenant on one day. Everything but the visit goes on
// one reminder row (one message).
type Decision struct {
	// Stages are the invoice stages to record, in the order of Input.Invoices.
	Stages []InvoiceStage
	// PromiseH1 is the promise version whose H-1 to record, uuid.Nil for none.
	PromiseH1 uuid.UUID
	// BrokenNotice is the broken promise whose notice to record, uuid.Nil for none.
	BrokenNotice uuid.UUID
	// OpenH7Visit says the tenant should have an open h7 visit task (no message).
	OpenH7Visit bool
}

// HasMessage reports whether the decision carries anything to tell the tenant, so whether it
// makes a reminder row.
func (d Decision) HasMessage() bool {
	return len(d.Stages) > 0 || d.PromiseH1 != uuid.Nil || d.BrokenNotice != uuid.Nil
}

// Decide applies the reminder rules to one tenant on one day. Each stage fires on the first run
// inside its window, so a missed day is caught up and nothing recorded is decided again:
//
//   - H-3 when due-3 <= today < due, H0 when due <= today < due+3, and H+3 when
//     due+3 <= today < due+7, for an invoice that still owes something. The windows do not
//     overlap, so an invoice gets at most one stage a run. H+3 is skipped while the newest
//     version of the active promise covers the invoice, and resumes once the promise closes if
//     its window is still open.
//   - H+7 is not a message: an owing invoice with today >= due+7 that the active promise does not
//     cover puts the tenant on the visit list, unless the karyawan already visited for it: like
//     the broken-promise count (decision 41.4), only an invoice whose due+7 is after the day the
//     tenant's last h7 visit task was resolved counts, so a resolved visit does not reopen the
//     next morning for the same debt, while a debt reaching H+7 after the visit opens a new one.
//   - The H-1 of the active promise's newest version when promised-1 <= today <= promised, once
//     per version, so an edit to a new date gets its own; only while an invoice it covers still
//     owes something (a covered invoice cancelled by "Rencana keluar" before the close owes
//     nothing, and there is nothing left to remind of).
//   - The notice of the tenant's newest broken promise when it closed in the last
//     BrokenNoticeDays days, once per promise. Not while the tenant has an active promise: they
//     already made a new one, which is what the notice asks for. An older broken promise is
//     never notified once a newer one is.
func Decide(in Input) Decision {
	var d Decision
	today := in.Today
	for _, inv := range in.Invoices {
		if inv.Outstanding <= 0 {
			continue
		}
		covered := in.Promise != nil && inv.Covered
		due := inv.DueDate
		if h7 := addDays(due, 7); !today.Before(h7) {
			if !covered && (in.LastH7VisitResolvedOn == nil || h7.After(*in.LastH7VisitResolvedOn)) {
				d.OpenH7Visit = true
			}
			continue
		}
		stage := stageOn(due, today)
		if stage == "" || (stage == StageHPlus3 && covered) {
			continue
		}
		key := InvoiceStage{InvoiceID: inv.ID, Stage: stage}
		if in.Recorded[key] {
			continue
		}
		d.Stages = append(d.Stages, key)
	}

	if p := in.Promise; p != nil && !p.Reminded && coveredOwing(in.Invoices) &&
		!today.Before(addDays(p.PromisedDate, -1)) && !today.After(p.PromisedDate) {
		d.PromiseH1 = p.VersionID
	}

	if in.Promise == nil {
		if b, ok := newestBroken(in.Broken); ok && !b.Notified &&
			!b.ClosedOn.Before(addDays(today, -BrokenNoticeDays)) && !b.ClosedOn.After(today) {
			d.BrokenNotice = b.ID
		}
	}
	return d
}

// coveredOwing reports whether an invoice the active promise covers still owes something.
func coveredOwing(invoices []Invoice) bool {
	for _, inv := range invoices {
		if inv.Covered && inv.Outstanding > 0 {
			return true
		}
	}
	return false
}

// stageOn is the message stage whose window holds today for an invoice due on due, "" for none.
func stageOn(due, today time.Time) string {
	switch {
	case today.Before(addDays(due, -3)):
		return ""
	case today.Before(due):
		return StageHMinus3
	case today.Before(addDays(due, 3)):
		return StageH0
	case today.Before(addDays(due, 7)):
		return StageHPlus3
	default:
		return ""
	}
}

// newestBroken is the broken promise that closed last; the first one listed wins a tie.
func newestBroken(bs []BrokenPromise) (BrokenPromise, bool) {
	var newest BrokenPromise
	found := false
	for _, b := range bs {
		if !found || b.ClosedOn.After(newest.ClosedOn) {
			newest, found = b, true
		}
	}
	return newest, found
}

// addDays moves a calendar day (midnight UTC) by n days.
func addDays(day time.Time, n int) time.Time { return day.AddDate(0, 0, n) }
