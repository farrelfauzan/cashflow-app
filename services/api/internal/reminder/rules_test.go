package reminder_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

var (
	invA     = uuid.MustParse("00000000-0000-4000-8000-00000000000a")
	invB     = uuid.MustParse("00000000-0000-4000-8000-00000000000b")
	invC     = uuid.MustParse("00000000-0000-4000-8000-00000000000c")
	version1 = uuid.MustParse("00000000-0000-4000-8000-0000000000f1")
	version2 = uuid.MustParse("00000000-0000-4000-8000-0000000000f2")
	broken1  = uuid.MustParse("00000000-0000-4000-8000-0000000000b1")
	broken2  = uuid.MustParse("00000000-0000-4000-8000-0000000000b2")
)

func owing(id uuid.UUID, due time.Time) reminder.Invoice {
	return reminder.Invoice{ID: id, DueDate: due, Outstanding: 700_000}
}

func covered(inv reminder.Invoice) reminder.Invoice {
	inv.Covered = true
	return inv
}

func stage(id uuid.UUID, s string) reminder.InvoiceStage {
	return reminder.InvoiceStage{InvoiceID: id, Stage: s}
}

func stages(ss ...reminder.InvoiceStage) []reminder.InvoiceStage { return ss }

func recorded(ss ...reminder.InvoiceStage) map[reminder.InvoiceStage]bool {
	m := map[reminder.InvoiceStage]bool{}
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// TestDecideInvoiceWindows walks one invoice due 10 Nov through every window edge.
func TestDecideInvoiceWindows(t *testing.T) {
	t.Parallel()
	due := day(2026, 11, 10)
	tests := []struct {
		name  string
		today time.Time
		want  reminder.Decision
	}{
		{name: "due-4: nothing yet", today: day(2026, 11, 6)},
		{name: "due-3: H-3", today: day(2026, 11, 7), want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHMinus3))}},
		{name: "due-1: still H-3 (a missed day catches up)", today: day(2026, 11, 9), want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHMinus3))}},
		{name: "due: H0", today: due, want: reminder.Decision{Stages: stages(stage(invA, reminder.StageH0))}},
		{name: "due+2: still H0", today: day(2026, 11, 12), want: reminder.Decision{Stages: stages(stage(invA, reminder.StageH0))}},
		{name: "due+3: H+3", today: day(2026, 11, 13), want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHPlus3))}},
		{name: "due+6: still H+3", today: day(2026, 11, 16), want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHPlus3))}},
		{name: "due+7: no message, the visit list", today: day(2026, 11, 17), want: reminder.Decision{OpenH7Visit: true}},
		{name: "due+40: still the visit list", today: day(2026, 12, 20), want: reminder.Decision{OpenH7Visit: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reminder.Decide(reminder.Input{Today: tc.today, Invoices: []reminder.Invoice{owing(invA, due)}})
			require.Equal(t, tc.want, got)
			require.Equal(t, len(tc.want.Stages) > 0, got.HasMessage())
		})
	}
}

// TestDecideMonthEnds: windows count calendar days across month ends and February.
func TestDecideMonthEnds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		due   time.Time
		today time.Time
		want  string // "" for no stage
	}{
		{name: "due 31 Jan, H-3 on 28 Jan", due: day(2027, 1, 31), today: day(2027, 1, 28), want: reminder.StageHMinus3},
		{name: "due 31 Jan, nothing on 27 Jan", due: day(2027, 1, 31), today: day(2027, 1, 27)},
		{name: "due 31 Jan, H+3 on 3 Feb", due: day(2027, 1, 31), today: day(2027, 2, 3), want: reminder.StageHPlus3},
		{name: "due 31 Jan, H0 still on 2 Feb", due: day(2027, 1, 31), today: day(2027, 2, 2), want: reminder.StageH0},
		{name: "due 28 Feb (common year), H-3 on 25 Feb", due: day(2027, 2, 28), today: day(2027, 2, 25), want: reminder.StageHMinus3},
		{name: "due 28 Feb (common year), H+3 on 3 Mar", due: day(2027, 2, 28), today: day(2027, 3, 3), want: reminder.StageHPlus3},
		{name: "due 28 Feb (common year), H0 on 2 Mar", due: day(2027, 2, 28), today: day(2027, 3, 2), want: reminder.StageH0},
		{name: "due 29 Feb (leap year), H-3 on 26 Feb", due: day(2028, 2, 29), today: day(2028, 2, 26), want: reminder.StageHMinus3},
		{name: "due 29 Feb (leap year), H+3 on 3 Mar", due: day(2028, 2, 29), today: day(2028, 3, 3), want: reminder.StageHPlus3},
		{name: "due 29 Feb (leap year), nothing on 25 Feb", due: day(2028, 2, 29), today: day(2028, 2, 25)},
		{name: "a 31st start due 30 Apr, H-3 on 27 Apr", due: day(2027, 4, 30), today: day(2027, 4, 27), want: reminder.StageHMinus3},
		{name: "a 31st start due 30 Apr, H0 on 2 May", due: day(2027, 4, 30), today: day(2027, 5, 2), want: reminder.StageH0},
		{name: "due 2 Jan, H-3 on 30 Dec across the year", due: day(2027, 1, 2), today: day(2026, 12, 30), want: reminder.StageHMinus3},
		{name: "due 29 Dec, H+3 on 1 Jan across the year", due: day(2026, 12, 29), today: day(2027, 1, 1), want: reminder.StageHPlus3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reminder.Decide(reminder.Input{Today: tc.today, Invoices: []reminder.Invoice{owing(invA, tc.due)}})
			if tc.want == "" {
				require.Empty(t, got.Stages)
				return
			}
			require.Equal(t, stages(stage(invA, tc.want)), got.Stages)
		})
	}
}

// TestDecideRecordedAndPaid: a stage recorded once is never decided again, and an invoice that
// owes nothing gets nothing.
func TestDecideRecordedAndPaid(t *testing.T) {
	t.Parallel()
	due := day(2026, 11, 10)
	tests := []struct {
		name string
		in   reminder.Input
		want reminder.Decision
	}{
		{
			name: "H-3 already recorded: nothing the next day",
			in:   reminder.Input{Today: day(2026, 11, 8), Invoices: []reminder.Invoice{owing(invA, due)}, Recorded: recorded(stage(invA, reminder.StageHMinus3))},
		},
		{
			name: "H-3 recorded: H0 still fires in its own window",
			in:   reminder.Input{Today: due, Invoices: []reminder.Invoice{owing(invA, due)}, Recorded: recorded(stage(invA, reminder.StageHMinus3))},
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageH0))},
		},
		{
			name: "H-3 missed entirely: the first run in H0's window sends H0 only",
			in:   reminder.Input{Today: day(2026, 11, 11), Invoices: []reminder.Invoice{owing(invA, due)}},
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageH0))},
		},
		{
			name: "H+3 already recorded: nothing",
			in:   reminder.Input{Today: day(2026, 11, 15), Invoices: []reminder.Invoice{owing(invA, due)}, Recorded: recorded(stage(invA, reminder.StageHPlus3))},
		},
		{
			name: "a paid invoice owes nothing: no H0",
			in:   reminder.Input{Today: due, Invoices: []reminder.Invoice{{ID: invA, DueDate: due, Outstanding: 0}}},
		},
		{
			name: "a paid invoice: no visit at H+7",
			in:   reminder.Input{Today: day(2026, 11, 20), Invoices: []reminder.Invoice{{ID: invA, DueDate: due, Outstanding: 0}}},
		},
		{
			name: "a cancelled invoice (owing below zero is impossible, zero is cancelled): nothing",
			in:   reminder.Input{Today: day(2026, 11, 7), Invoices: []reminder.Invoice{{ID: invA, DueDate: due, Outstanding: 0}}},
		},
		{
			name: "a partly paid invoice still owes: H+3",
			in:   reminder.Input{Today: day(2026, 11, 13), Invoices: []reminder.Invoice{{ID: invA, DueDate: due, Outstanding: 1}}},
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHPlus3))},
		},
		{
			name: "no invoices: nothing",
			in:   reminder.Input{Today: due},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, reminder.Decide(tc.in))
		})
	}
}

// TestDecideSeveralInvoices: everything decided for a tenant in one run comes back together, in
// the order of the invoices.
func TestDecideSeveralInvoices(t *testing.T) {
	t.Parallel()
	today := day(2026, 11, 13)
	got := reminder.Decide(reminder.Input{
		Today: today,
		Invoices: []reminder.Invoice{
			owing(invA, day(2026, 10, 1)),  // due+43: visit list
			owing(invB, day(2026, 11, 10)), // due+3: H+3
			owing(invC, day(2026, 11, 15)), // due-2: H-3
		},
	})
	require.Equal(t, reminder.Decision{
		Stages:      stages(stage(invB, reminder.StageHPlus3), stage(invC, reminder.StageHMinus3)),
		OpenH7Visit: true,
	}, got)
	require.True(t, got.HasMessage())
}

// TestDecideActivePromise: an active promise's newest version silences H+3 and H+7 for the
// invoices it covers, but not H-3 or H0, and not an invoice it does not cover.
func TestDecideActivePromise(t *testing.T) {
	t.Parallel()
	due := day(2026, 11, 10)
	promise := &reminder.ActivePromise{VersionID: version1, PromisedDate: day(2026, 12, 20)}
	tests := []struct {
		name    string
		today   time.Time
		inv     reminder.Invoice
		promise *reminder.ActivePromise
		want    reminder.Decision
	}{
		{name: "covered: no H+3", today: day(2026, 11, 13), inv: covered(owing(invA, due)), promise: promise},
		{name: "covered: no visit at H+7", today: day(2026, 11, 17), inv: covered(owing(invA, due)), promise: promise},
		{name: "covered: H-3 still fires", today: day(2026, 11, 7), inv: covered(owing(invA, due)), promise: promise,
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHMinus3))}},
		{name: "covered: H0 still fires", today: due, inv: covered(owing(invA, due)), promise: promise,
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageH0))}},
		{name: "not covered (made after the promise): H+3 fires", today: day(2026, 11, 13), inv: owing(invA, due), promise: promise,
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHPlus3))}},
		{name: "not covered: the visit list at H+7", today: day(2026, 11, 17), inv: owing(invA, due), promise: promise,
			want: reminder.Decision{OpenH7Visit: true}},
		{name: "the promise closed while H+3's window is open: H+3 resumes", today: day(2026, 11, 15), inv: covered(owing(invA, due)),
			want: reminder.Decision{Stages: stages(stage(invA, reminder.StageHPlus3))}},
		{name: "the promise closed after H+3's window: the visit list", today: day(2026, 11, 21), inv: covered(owing(invA, due)),
			want: reminder.Decision{OpenH7Visit: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reminder.Decide(reminder.Input{Today: tc.today, Invoices: []reminder.Invoice{tc.inv}, Promise: tc.promise})
			require.Equal(t, tc.want, got)
		})
	}
}

// TestDecidePromiseH1: the H-1 fires from the day before the promised date to the promised date,
// once per version.
func TestDecidePromiseH1(t *testing.T) {
	t.Parallel()
	promised := day(2026, 11, 20)
	tests := []struct {
		name     string
		today    time.Time
		promise  reminder.ActivePromise
		invoices []reminder.Invoice // nil: one covered invoice still owing
		want     uuid.UUID
	}{
		{name: "promised-2: not yet", today: day(2026, 11, 18), promise: reminder.ActivePromise{VersionID: version1, PromisedDate: promised}},
		{name: "promised-1: H-1", today: day(2026, 11, 19), promise: reminder.ActivePromise{VersionID: version1, PromisedDate: promised}, want: version1},
		{name: "promised day, H-1 missed: caught up", today: promised, promise: reminder.ActivePromise{VersionID: version1, PromisedDate: promised}, want: version1},
		{name: "promised day, already recorded: nothing", today: promised, promise: reminder.ActivePromise{VersionID: version1, PromisedDate: promised, Reminded: true}},
		{name: "after the promised day (still active before the close): nothing", today: day(2026, 11, 21), promise: reminder.ActivePromise{VersionID: version1, PromisedDate: promised}},
		{name: "an edit to a new date: its own H-1", today: day(2026, 11, 24), promise: reminder.ActivePromise{VersionID: version2, PromisedDate: day(2026, 11, 25)}, want: version2},
		{name: "a promise for tomorrow made today: H-1 today", today: day(2026, 11, 19), promise: reminder.ActivePromise{VersionID: version2, PromisedDate: promised}, want: version2},
		{name: "a 31 Jan promise: H-1 on 30 Jan", today: day(2027, 1, 30), promise: reminder.ActivePromise{VersionID: version1, PromisedDate: day(2027, 1, 31)}, want: version1},
		{name: "a 1 Mar promise: H-1 on 28 Feb", today: day(2027, 2, 28), promise: reminder.ActivePromise{VersionID: version1, PromisedDate: day(2027, 3, 1)}, want: version1},
		{
			name: "the covered invoice was cancelled (owes nothing): no H-1", today: day(2026, 11, 19),
			promise:  reminder.ActivePromise{VersionID: version1, PromisedDate: promised},
			invoices: []reminder.Invoice{{ID: uuid.New(), DueDate: day(2026, 12, 25), Outstanding: 0, Covered: true}},
		},
		{
			name: "only an uncovered invoice owes: no H-1", today: day(2026, 11, 19),
			promise:  reminder.ActivePromise{VersionID: version1, PromisedDate: promised},
			invoices: []reminder.Invoice{{ID: uuid.New(), DueDate: day(2026, 12, 25), Outstanding: 500_000}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := tc.promise
			invoices := tc.invoices
			if invoices == nil {
				// A covered invoice still owing, due well after the window, so it adds no stage.
				invoices = []reminder.Invoice{{ID: uuid.New(), DueDate: day(2027, 6, 1), Outstanding: 500_000, Covered: true}}
			}
			got := reminder.Decide(reminder.Input{Today: tc.today, Promise: &p, Invoices: invoices})
			require.Equal(t, tc.want, got.PromiseH1)
			require.Equal(t, tc.want != uuid.Nil, got.HasMessage())
		})
	}
}

// TestDecideBrokenNotice: a broken promise is notified once, by a run within 3 days of its close.
func TestDecideBrokenNotice(t *testing.T) {
	t.Parallel()
	today := day(2026, 11, 13)
	tests := []struct {
		name    string
		today   time.Time // the run's day, 13 Nov when zero
		broken  []reminder.BrokenPromise
		promise *reminder.ActivePromise
		want    uuid.UUID
	}{
		{name: "closed today", broken: []reminder.BrokenPromise{{ID: broken1, ClosedOn: today}}, want: broken1},
		{name: "closed 3 days ago (a missed run catches up)", broken: []reminder.BrokenPromise{{ID: broken1, ClosedOn: day(2026, 11, 10)}}, want: broken1},
		{name: "closed 4 days ago: never notified", broken: []reminder.BrokenPromise{{ID: broken1, ClosedOn: day(2026, 11, 9)}}},
		{name: "already notified", broken: []reminder.BrokenPromise{{ID: broken1, ClosedOn: today, Notified: true}}},
		{name: "no broken promise", broken: nil},
		{
			name:   "two in the window: only the newest",
			broken: []reminder.BrokenPromise{{ID: broken1, ClosedOn: day(2026, 11, 11)}, {ID: broken2, ClosedOn: day(2026, 11, 13)}},
			want:   broken2,
		},
		{
			name:   "the newest already notified: an older one is not",
			broken: []reminder.BrokenPromise{{ID: broken2, ClosedOn: day(2026, 11, 13), Notified: true}, {ID: broken1, ClosedOn: day(2026, 11, 11)}},
		},
		{
			name:    "the tenant already made a new promise: no notice",
			broken:  []reminder.BrokenPromise{{ID: broken1, ClosedOn: today}},
			promise: &reminder.ActivePromise{VersionID: version2, PromisedDate: day(2026, 11, 30)},
		},
		{name: "across a month end: closed 30 Nov, run 3 Dec", today: day(2026, 12, 3), broken: []reminder.BrokenPromise{{ID: broken1, ClosedOn: day(2026, 11, 30)}}, want: broken1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runDay := today
			if !tc.today.IsZero() {
				runDay = tc.today
			}
			got := reminder.Decide(reminder.Input{Today: runDay, Broken: tc.broken, Promise: tc.promise})
			require.Equal(t, tc.want, got.BrokenNotice)
		})
	}
}

// TestDecideOneMessage: an invoice stage, a broken notice and the resumed H+3 all come in one
// decision, so one reminder row.
func TestDecideOneMessage(t *testing.T) {
	t.Parallel()
	today := day(2026, 11, 13)
	got := reminder.Decide(reminder.Input{
		Today:    today,
		Invoices: []reminder.Invoice{covered(owing(invA, day(2026, 11, 10))), owing(invB, day(2026, 11, 15))},
		Broken:   []reminder.BrokenPromise{{ID: broken1, ClosedOn: today}},
	})
	require.Equal(t, reminder.Decision{
		Stages:       stages(stage(invA, reminder.StageHPlus3), stage(invB, reminder.StageHMinus3)),
		BrokenNotice: broken1,
	}, got)
}

func TestHasMessage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		d    reminder.Decision
		want bool
	}{
		{name: "empty", d: reminder.Decision{}},
		{name: "a visit only is no message", d: reminder.Decision{OpenH7Visit: true}},
		{name: "a stage", d: reminder.Decision{Stages: stages(stage(invA, reminder.StageH0))}, want: true},
		{name: "an H-1", d: reminder.Decision{PromiseH1: version1}, want: true},
		{name: "a broken notice", d: reminder.Decision{BrokenNotice: broken1}, want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.d.HasMessage())
		})
	}
}

// TestDecideH7AfterAResolvedVisit: once a karyawan resolved the tenant's h7 visit, only an invoice
// whose H+7 comes after the day it was resolved opens a new one.
func TestDecideH7AfterAResolvedVisit(t *testing.T) {
	t.Parallel()
	today := day(2026, 12, 20)
	ptr := func(d time.Time) *time.Time { return &d }
	tests := []struct {
		name     string
		invoices []reminder.Invoice
		promise  *reminder.ActivePromise
		resolved *time.Time
		want     bool
	}{
		{name: "no resolved visit: opens", invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10))}, want: true},
		{name: "resolved before the invoice's H+7: opens", invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10))}, resolved: ptr(day(2026, 11, 16)), want: true},
		{name: "resolved on its H+7: does not", invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10))}, resolved: ptr(day(2026, 11, 17))},
		{name: "resolved after its H+7: does not", invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10))}, resolved: ptr(day(2026, 11, 18))},
		{
			name:     "two invoices, one each side: the later one opens",
			invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10)), owing(invB, day(2026, 12, 10))},
			resolved: ptr(day(2026, 11, 18)),
			want:     true,
		},
		{
			name:     "the later one paid: nothing reopens",
			invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10)), {ID: invB, DueDate: day(2026, 12, 10), Outstanding: 0}},
			resolved: ptr(day(2026, 11, 18)),
		},
		{
			name:     "the later one covered by an active promise: nothing reopens",
			invoices: []reminder.Invoice{owing(invA, day(2026, 11, 10)), covered(owing(invB, day(2026, 12, 10)))},
			promise:  &reminder.ActivePromise{VersionID: version1, PromisedDate: day(2027, 1, 5)},
			resolved: ptr(day(2026, 11, 18)),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reminder.Decide(reminder.Input{Today: today, Invoices: tc.invoices, Promise: tc.promise, LastH7VisitResolvedOn: tc.resolved})
			require.Equal(t, tc.want, got.OpenH7Visit)
			require.False(t, got.HasMessage())
		})
	}
}
