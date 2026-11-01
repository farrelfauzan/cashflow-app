package billing_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"pgregory.net/rapid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

// ledger is a tenant's money as the database would hold it: payments and invoices with their
// corrected amounts, and every allocation row written so far.
type ledger struct {
	payments []billing.Payment
	invoices []billing.Invoice
	allocs   []billing.Allocation
	seq      int64 // shared by every table, like the identity columns: only the order matters
}

func (l *ledger) nextSeq() int64 {
	l.seq++
	return l.seq
}

// apply writes planned rows the way Allocate inserts them.
func (l *ledger) apply(rows []billing.Row) {
	for _, r := range rows {
		l.allocs = append(l.allocs, billing.Allocation{Seq: l.nextSeq(), PaymentID: r.PaymentID, InvoiceID: r.InvoiceID, Amount: r.Amount})
	}
}

// sums returns what each payment has allocated, what each invoice has been paid, and what
// each payment-invoice pair holds.
func (l *ledger) sums() (allocated, paid map[uuid.UUID]int64, pairs map[[2]uuid.UUID]int64) {
	allocated, paid, pairs = map[uuid.UUID]int64{}, map[uuid.UUID]int64{}, map[[2]uuid.UUID]int64{}
	for _, a := range l.allocs {
		allocated[a.PaymentID] += a.Amount
		paid[a.InvoiceID] += a.Amount
		pairs[[2]uuid.UUID{a.PaymentID, a.InvoiceID}] += a.Amount
	}
	return allocated, paid, pairs
}

// checkInvariants asserts what must hold after every allocation run.
func checkInvariants(t *rapid.T, l *ledger, rows []billing.Row) {
	for _, r := range rows {
		if r.Amount == 0 {
			t.Fatalf("a zero-amount row was planned: %+v", r)
		}
	}
	allocated, paid, pairs := l.sums()
	for k, v := range pairs {
		if v < 0 {
			t.Fatalf("payment %s holds %d on invoice %s", k[0], v, k[1])
		}
	}
	var totalNet, totalBilled, totalAllocated int64
	anyOutstanding, anyRemainder := false, false
	for _, inv := range l.invoices {
		totalBilled += inv.Billed
		if paid[inv.ID] > inv.Billed {
			t.Fatalf("invoice %s is paid %d but bills %d", inv.ID, paid[inv.ID], inv.Billed)
		}
		if paid[inv.ID] < inv.Billed {
			anyOutstanding = true
		}
	}
	for _, p := range l.payments {
		totalNet += p.Net
		totalAllocated += allocated[p.ID]
		if allocated[p.ID] > p.Net {
			t.Fatalf("payment %s allocated %d but its net is %d", p.ID, allocated[p.ID], p.Net)
		}
		if allocated[p.ID] < p.Net {
			anyRemainder = true
		}
	}
	// FIFO leaves no invoice open while money is left: so no invoice, older or newer, still
	// has something outstanding while a payment has a remainder.
	if anyOutstanding && anyRemainder {
		t.Fatalf("an invoice is still outstanding while a payment has money left")
	}
	// Oldest first, also after reversals: no invoice this run paid may sort after an invoice
	// that is still outstanding once the run is done.
	paidNow := map[uuid.UUID]bool{}
	for _, r := range rows {
		if r.Amount > 0 {
			paidNow[r.InvoiceID] = true
		}
	}
	var oldestOpen *billing.Invoice
	for _, inv := range sortInvoices(l.invoices) {
		if oldestOpen == nil && paid[inv.ID] < inv.Billed {
			oldestOpen = &inv
			continue
		}
		if oldestOpen != nil && paidNow[inv.ID] {
			t.Fatalf("invoice %s was paid while the older invoice %s is still outstanding", inv.ID, oldestOpen.ID)
		}
	}
	if want := min(totalNet, totalBilled); totalAllocated != want {
		t.Fatalf("allocated %d in total, want min(net %d, billed %d) = %d", totalAllocated, totalNet, totalBilled, want)
	}
	again, err := billing.Plan(l.payments, l.invoices, l.allocs)
	if err != nil {
		t.Fatalf("rerun: %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("a second run planned %d rows: %+v", len(again), again)
	}
}

var base = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

// TestPlanProperties builds a tenant's history one event at a time (a new invoice, a new
// payment, a correction of either, up or down, to zero included) and runs the allocator after
// each event, as the services will, checking the invariants every time.
func TestPlanProperties(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		l := &ledger{}
		steps := rapid.IntRange(1, 25).Draw(t, "steps")
		for range steps {
			kinds := []string{"invoice", "payment"}
			if len(l.invoices) > 0 {
				kinds = append(kinds, "correct invoice")
			}
			if len(l.payments) > 0 {
				kinds = append(kinds, "correct payment")
			}
			switch rapid.SampledFrom(kinds).Draw(t, "event") {
			case "invoice":
				// Periods may repeat and may be older than invoices already paid, as when a
				// late lease's past periods arrive after the current one.
				months := rapid.IntRange(0, 11).Draw(t, "period month")
				l.invoices = append(l.invoices, billing.Invoice{
					ID: uuid.New(), Period: base.AddDate(0, months, 0), Seq: l.nextSeq(),
					Billed: rapid.Int64Range(1, 1000).Draw(t, "invoice amount"),
				})
			case "payment":
				days := rapid.IntRange(0, 365).Draw(t, "received day")
				l.payments = append(l.payments, billing.Payment{
					ID: uuid.New(), ReceivedAt: base.AddDate(0, 0, days), Seq: l.nextSeq(),
					Net: rapid.Int64Range(1, 1500).Draw(t, "payment amount"),
				})
			case "correct invoice":
				i := rapid.IntRange(0, len(l.invoices)-1).Draw(t, "invoice")
				// Mostly down (to zero cancels it), sometimes back up.
				l.invoices[i].Billed = rapid.Int64Range(0, l.invoices[i].Billed+300).Draw(t, "corrected billed")
			case "correct payment":
				i := rapid.IntRange(0, len(l.payments)-1).Draw(t, "payment")
				l.payments[i].Net = rapid.Int64Range(0, l.payments[i].Net+300).Draw(t, "corrected net")
			}
			rows, err := billing.Plan(l.payments, l.invoices, l.allocs)
			if err != nil {
				t.Fatalf("plan: %v", err)
			}
			l.apply(rows)
			checkInvariants(t, l, rows)
		}
	})
}

// TestPlanFromCleanSlateIsStrictFIFO checks the stronger ordering when nothing was allocated
// yet: the paid invoices are a prefix of the invoices in FIFO order (all before the first open
// one are paid in full, none after it is touched), and the payments are used up in order.
func TestPlanFromCleanSlateIsStrictFIFO(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		var payments []billing.Payment
		var invoices []billing.Invoice
		var seq int64
		for range rapid.IntRange(0, 8).Draw(t, "invoices") {
			seq++
			invoices = append(invoices, billing.Invoice{
				ID: uuid.New(), Period: base.AddDate(0, rapid.IntRange(0, 11).Draw(t, "month"), 0), Seq: seq,
				Billed: rapid.Int64Range(0, 1000).Draw(t, "billed"),
			})
		}
		for range rapid.IntRange(0, 8).Draw(t, "payments") {
			seq++
			payments = append(payments, billing.Payment{
				ID: uuid.New(), ReceivedAt: base.AddDate(0, 0, rapid.IntRange(0, 365).Draw(t, "day")), Seq: seq,
				Net: rapid.Int64Range(0, 1500).Draw(t, "net"),
			})
		}
		rows, err := billing.Plan(payments, invoices, nil)
		if err != nil {
			t.Fatalf("plan: %v", err)
		}
		paid, allocated := map[uuid.UUID]int64{}, map[uuid.UUID]int64{}
		for _, r := range rows {
			if r.Amount <= 0 {
				t.Fatalf("a clean slate needs no reversal, got %+v", r)
			}
			paid[r.InvoiceID] += r.Amount
			allocated[r.PaymentID] += r.Amount
		}
		sortedInvoices := sortInvoices(invoices)
		open := false
		for _, inv := range sortedInvoices {
			if open && paid[inv.ID] > 0 {
				t.Fatalf("invoice %s was paid after an older one was left open", inv.ID)
			}
			if paid[inv.ID] < inv.Billed {
				open = true
			}
		}
		left := false
		for _, p := range sortPayments(payments) {
			if left && allocated[p.ID] > 0 {
				t.Fatalf("payment %s was used while an older one still had money", p.ID)
			}
			if allocated[p.ID] < p.Net {
				left = true
			}
		}
	})
}
