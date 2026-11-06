package promise

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func ptr[T any](v T) *T { return &v }

func monthly(start time.Time) billing.LeaseDates {
	return billing.LeaseDates{Start: start, EnteredOn: start, BilledFrom: start, TermMonths: 1}
}

func TestWindowOn(t *testing.T) {
	t.Parallel()
	plannedEnd := func(l billing.LeaseDates, end time.Time) billing.LeaseDates {
		l.PlannedEnd = &end
		return l
	}
	movedOut := func(l billing.LeaseDates, end time.Time) billing.LeaseDates {
		l.End = &end
		return l
	}
	tests := []struct {
		name        string
		today       time.Time
		leases      []billing.LeaseDates
		wantNext    *time.Time
		wantInRange bool
	}{
		{
			name: "the next anniversary of a monthly lease", today: day(2026, 11, 2),
			leases: []billing.LeaseDates{monthly(day(2026, 10, 1))}, wantNext: ptr(day(2026, 12, 1)), wantInRange: true,
		},
		{
			name: "a start on the 31st is due on the last day of February", today: day(2027, 2, 1),
			leases: []billing.LeaseDates{monthly(day(2026, 10, 31))}, wantNext: ptr(day(2027, 2, 28)), wantInRange: true,
		},
		{
			name: "a start on the 31st, from the end of February, is due on 31 March", today: day(2027, 2, 28),
			leases: []billing.LeaseDates{monthly(day(2026, 10, 31))}, wantNext: ptr(day(2027, 3, 31)), wantInRange: true,
		},
		{
			name: "across the year end", today: day(2026, 12, 31),
			leases: []billing.LeaseDates{monthly(day(2026, 10, 31))}, wantNext: ptr(day(2027, 1, 31)), wantInRange: true,
		},
		{
			name: "due tomorrow is the earliest allowed day", today: day(2026, 11, 30),
			leases: []billing.LeaseDates{monthly(day(2026, 10, 1))}, wantNext: ptr(day(2026, 12, 1)), wantInRange: true,
		},
		{
			name: "no next invoice once the planned end comes first", today: day(2026, 11, 2),
			leases: []billing.LeaseDates{plannedEnd(monthly(day(2026, 10, 1)), day(2026, 11, 30))},
		},
		{
			name: "a period starting on the planned end is still billed", today: day(2026, 11, 2),
			leases:   []billing.LeaseDates{plannedEnd(monthly(day(2026, 10, 1)), day(2026, 12, 1))},
			wantNext: ptr(day(2026, 12, 1)), wantInRange: true,
		},
		{
			name: "a moved-out lease bills no more", today: day(2026, 11, 2),
			leases: []billing.LeaseDates{movedOut(monthly(day(2026, 10, 1)), day(2026, 11, 2))},
		},
		{
			name: "no lease at all", today: day(2026, 11, 2),
		},
		{
			name: "two leases: the earlier due date wins", today: day(2026, 11, 2),
			leases:   []billing.LeaseDates{monthly(day(2026, 10, 20)), monthly(day(2026, 10, 5))},
			wantNext: ptr(day(2026, 11, 5)), wantInRange: true,
		},
		{
			name: "two leases: one moved out, the other still bills", today: day(2026, 11, 2),
			leases:   []billing.LeaseDates{movedOut(monthly(day(2026, 10, 5)), day(2026, 11, 1)), monthly(day(2026, 10, 20))},
			wantNext: ptr(day(2026, 11, 20)), wantInRange: true,
		},
		{
			name: "a yearly lease due beyond 60 days is not offered", today: day(2026, 11, 2),
			leases:   []billing.LeaseDates{{Start: day(2026, 10, 1), EnteredOn: day(2026, 10, 1), BilledFrom: day(2026, 10, 1), TermMonths: 12}},
			wantNext: ptr(day(2027, 10, 1)),
		},
		{
			name: "due exactly 60 days ahead is offered", today: day(2026, 11, 2),
			leases:   []billing.LeaseDates{{Start: day(2026, 10, 1), EnteredOn: day(2026, 10, 1), BilledFrom: day(2026, 10, 1), TermMonths: 3}},
			wantNext: ptr(day(2027, 1, 1)), wantInRange: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := WindowOn(tc.today, tc.leases)
			require.Equal(t, tc.today.AddDate(0, 0, 1), w.Earliest)
			require.Equal(t, tc.today.AddDate(0, 0, MaxDaysAhead), w.Latest)
			require.Equal(t, tc.wantNext, w.NextDue)
			if tc.wantInRange {
				require.Equal(t, tc.wantNext, w.NextDueInRange())
			} else {
				require.Nil(t, w.NextDueInRange())
			}
		})
	}
}

func TestInputChecked(t *testing.T) {
	t.Parallel()
	date := ptr(day(2026, 11, 20))
	tests := []struct {
		name      string
		in        Input
		wantField string // "" when the input is fine
		wantNote  string
	}{
		{name: "a specific date", in: Input{Kind: KindSpecificDate, PromisedDate: date}},
		{name: "with the next invoice and no date", in: Input{Kind: KindWithNextInvoice}},
		{name: "no kind", in: Input{PromisedDate: date}, wantField: "kind"},
		{name: "an unknown kind", in: Input{Kind: "someday", PromisedDate: date}, wantField: "kind"},
		{name: "a specific date without one", in: Input{Kind: KindSpecificDate}, wantField: "promisedDate"},
		{name: "a date sent with the next invoice", in: Input{Kind: KindWithNextInvoice, PromisedDate: date}, wantField: "promisedDate"},
		{name: "a promised amount of zero", in: Input{Kind: KindWithNextInvoice, PromisedAmount: ptr(int64(0))}, wantField: "promisedAmount"},
		{name: "a negative payment now", in: Input{Kind: KindWithNextInvoice, PayingNow: -1}, wantField: "payingNow"},
		{name: "a note is trimmed", in: Input{Kind: KindWithNextInvoice, Note: "  janji lewat telepon \n"}, wantNote: "janji lewat telepon"},
		{name: "a note of 500 letters", in: Input{Kind: KindWithNextInvoice, Note: strings.Repeat("é", 500)}, wantNote: strings.Repeat("é", 500)},
		{name: "a note of 501 letters", in: Input{Kind: KindWithNextInvoice, Note: strings.Repeat("a", 501)}, wantField: "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.in.checked()
			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, tc.wantNote, got.Note)
				return
			}
			var ie *InputError
			require.ErrorAs(t, err, &ie)
			require.Equal(t, tc.wantField, ie.Field)
			require.NotEmpty(t, ie.Message)
		})
	}
}

func TestDecide(t *testing.T) {
	t.Parallel()
	today := day(2026, 11, 2)
	w := WindowOn(today, []billing.LeaseDates{monthly(day(2026, 10, 1))}) // next due 1 Dec
	noNext := WindowOn(today, nil)
	farNext := WindowOn(today, []billing.LeaseDates{{Start: day(2026, 10, 1), EnteredOn: day(2026, 10, 1), BilledFrom: day(2026, 10, 1), TermMonths: 12}})
	on := func(d time.Time) Input { return Input{Kind: KindSpecificDate, PromisedDate: &d} }
	withAmounts := func(in Input, promised *int64, now int64) Input {
		in.PromisedAmount, in.PayingNow = promised, now
		return in
	}
	tests := []struct {
		name        string
		in          Input
		outstanding int64
		w           Window
		want        time.Time
		wantErr     error
		wantField   string
	}{
		{name: "tomorrow", in: on(day(2026, 11, 3)), outstanding: 700_000, w: w, want: day(2026, 11, 3)},
		{name: "60 days ahead", in: on(day(2027, 1, 1)), outstanding: 700_000, w: w, want: day(2027, 1, 1)},
		{name: "today is too early", in: on(today), outstanding: 700_000, w: w, wantErr: ErrDateOutOfRange, wantField: "promisedDate"},
		{name: "yesterday is too early", in: on(day(2026, 11, 1)), outstanding: 700_000, w: w, wantErr: ErrDateOutOfRange, wantField: "promisedDate"},
		{name: "61 days ahead is too late", in: on(day(2027, 1, 2)), outstanding: 700_000, w: w, wantErr: ErrDateOutOfRange, wantField: "promisedDate"},
		{name: "a date with a time of day counts as its day", in: on(time.Date(2026, 11, 3, 15, 0, 0, 0, time.UTC)), outstanding: 700_000, w: w, want: day(2026, 11, 3)},
		{name: "with the next invoice takes its due date", in: Input{Kind: KindWithNextInvoice}, outstanding: 700_000, w: w, want: day(2026, 12, 1)},
		{name: "with no next invoice", in: Input{Kind: KindWithNextInvoice}, outstanding: 700_000, w: noNext, wantErr: ErrNoNextInvoice, wantField: "kind"},
		{name: "with a next invoice beyond 60 days", in: Input{Kind: KindWithNextInvoice}, outstanding: 700_000, w: farNext, wantErr: ErrDateOutOfRange, wantField: "promisedDate"},
		{name: "nothing owed", in: on(day(2026, 11, 3)), outstanding: 0, w: w, wantErr: ErrNothingOwed},
		{name: "nothing owed beats a bad date", in: on(today), outstanding: 0, w: w, wantErr: ErrNothingOwed},
		{name: "paying everything now leaves nothing to promise", in: withAmounts(on(day(2026, 11, 3)), nil, 700_000), outstanding: 700_000, w: w, wantErr: ErrNothingOwed},
		{name: "paying more than owed now", in: withAmounts(on(day(2026, 11, 3)), nil, 700_001), outstanding: 700_000, w: w, wantErr: ErrAmountAboveOutstanding, wantField: "payingNow"},
		{name: "promising the whole rest", in: withAmounts(on(day(2026, 11, 3)), ptr(int64(400_000)), 300_000), outstanding: 700_000, w: w, want: day(2026, 11, 3)},
		{name: "promising more than the rest", in: withAmounts(on(day(2026, 11, 3)), ptr(int64(400_001)), 300_000), outstanding: 700_000, w: w, wantErr: ErrAmountAboveOutstanding, wantField: "promisedAmount"},
		{name: "an amount above what is owed beats a bad date", in: withAmounts(on(today), ptr(int64(700_001)), 0), outstanding: 700_000, w: w, wantErr: ErrAmountAboveOutstanding, wantField: "promisedAmount"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Decide(tc.in, tc.outstanding, tc.w)
			if tc.wantErr == nil {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			var fe *FieldError
			if tc.wantField == "" {
				require.False(t, errors.As(err, &fe), "no field for %v", err)
				return
			}
			require.ErrorAs(t, err, &fe)
			require.Equal(t, tc.wantField, fe.Field)
		})
	}
}

func TestCappedFrom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		today      time.Time
		firstDay   time.Time
		wantLatest time.Time
		wantEmpty  bool
	}{
		{name: "a new promise made today keeps 60 days", today: day(2026, 11, 2), firstDay: day(2026, 11, 2), wantLatest: day(2027, 1, 1)},
		{name: "an edit 24 days later ends at the first version's bound", today: day(2026, 11, 26), firstDay: day(2026, 11, 2), wantLatest: day(2027, 1, 1)},
		{name: "the day before the bound leaves one day", today: day(2026, 12, 31), firstDay: day(2026, 11, 2), wantLatest: day(2027, 1, 1)},
		{name: "on the bound no day is left", today: day(2027, 1, 1), firstDay: day(2026, 11, 2), wantLatest: day(2027, 1, 1), wantEmpty: true},
		{name: "past the bound no day is left", today: day(2027, 2, 15), firstDay: day(2026, 11, 2), wantLatest: day(2027, 1, 1), wantEmpty: true},
		{name: "across February in a common year", today: day(2027, 1, 20), firstDay: day(2027, 1, 10), wantLatest: day(2027, 3, 11)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := WindowOn(tc.today, []billing.LeaseDates{monthly(day(2026, 10, 1))}).CappedFrom(tc.firstDay)
			require.Equal(t, tc.today.AddDate(0, 0, 1), w.Earliest)
			require.Equal(t, tc.wantLatest, w.Latest)
			require.Equal(t, tc.wantEmpty, w.Empty())
			if tc.wantEmpty {
				require.Nil(t, w.NextDueInRange(), "nothing is in an empty window")
				_, err := Decide(Input{Kind: KindSpecificDate, PromisedDate: ptr(tc.today.AddDate(0, 0, 1))}, 100_000, w)
				require.ErrorIs(t, err, ErrDateOutOfRange)
			}
		})
	}
}
