// Package billing holds slice 2's money rules. This file is the FIFO allocation of a tenant's
// payments to their invoices as a pure function; allocate.go runs it against the database.
//
// Money rows are append-only, so allocation never changes a row: it only plans new allocation
// rows, positive to pay and negative to take money back, and the caller inserts them.
package billing

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// ErrInconsistent is returned by Plan when its input cannot come from the database: an
// allocation naming a payment or invoice that is not in the input, a payment or invoice listed
// twice, a negative net or billed amount, or a payment-invoice pair whose allocations add up to
// less than zero.
var ErrInconsistent = errors.New("billing: inconsistent allocation input")

// Payment is one of the tenant's payments as allocation sees it.
type Payment struct {
	ID         uuid.UUID
	ReceivedAt time.Time // with Seq, the FIFO order: oldest payment first
	Seq        int64
	Net        int64 // amount plus its corrections, rupiah; never negative
}

// Invoice is one of the tenant's invoices as allocation sees it.
type Invoice struct {
	ID     uuid.UUID
	Period time.Time // with Seq, the FIFO order: oldest period first
	Seq    int64
	Billed int64 // amount plus its corrections, rupiah; zero when cancelled, never negative
}

// Allocation is an allocation row already written for the tenant.
type Allocation struct {
	Seq       int64 // the order it was written in
	PaymentID uuid.UUID
	InvoiceID uuid.UUID
	Amount    int64 // signed: negative takes money back
}

// Row is an allocation row to write. Amount is never zero.
type Row struct {
	PaymentID uuid.UUID
	InvoiceID uuid.UUID
	Amount    int64
}

// holding is what one payment currently has on one invoice: the sum of their allocation rows.
type holding struct {
	pay, inv int   // indexes into the sorted payments and invoices
	amount   int64 // never negative
	lastSeq  int64 // the newest positive row of the pair
}

// planner holds the state Plan works on.
type planner struct {
	pays      []Payment
	invs      []Invoice
	allocated []int64 // per payment
	paid      []int64 // per invoice
	holdings  []*holding
	out       []Row
}

// Plan returns the allocation rows that bring a tenant's allocations in line with its payments
// and invoices, in the order to write them:
//
//  1. Reversals. An invoice paid more than it bills (it was corrected down, or cancelled) gives
//     the excess back, from the payments that paid it, newest allocation first. Then a payment
//     allocated more than its net amount (it was corrected down) takes the excess back from the
//     invoices it paid, newest invoice first.
//  2. FIFO. Payments with money left, oldest first (received_at, then seq), pay invoices with
//     something outstanding, oldest period first (then seq); each step allocates the smaller of
//     the two. What is left over is the tenant's credit, and nothing is written for it.
//
// On the state that results, Plan returns no rows, so running it again changes nothing.
func Plan(payments []Payment, invoices []Invoice, existing []Allocation) ([]Row, error) {
	p, err := newPlanner(payments, invoices, existing)
	if err != nil {
		return nil, err
	}
	p.reverseOverpaidInvoices()
	p.reverseOverallocatedPayments()
	p.payFIFO()
	return p.out, nil
}

func newPlanner(payments []Payment, invoices []Invoice, existing []Allocation) (*planner, error) {
	p := &planner{pays: slices.Clone(payments), invs: slices.Clone(invoices)}
	slices.SortStableFunc(p.pays, func(a, b Payment) int {
		return cmp.Or(a.ReceivedAt.Compare(b.ReceivedAt), cmp.Compare(a.Seq, b.Seq))
	})
	slices.SortStableFunc(p.invs, func(a, b Invoice) int {
		return cmp.Or(a.Period.Compare(b.Period), cmp.Compare(a.Seq, b.Seq))
	})

	payIndex := make(map[uuid.UUID]int, len(p.pays))
	for i, x := range p.pays {
		if _, dup := payIndex[x.ID]; dup || x.Net < 0 {
			return nil, fmt.Errorf("%w: payment %s", ErrInconsistent, x.ID)
		}
		payIndex[x.ID] = i
	}
	invIndex := make(map[uuid.UUID]int, len(p.invs))
	for i, x := range p.invs {
		if _, dup := invIndex[x.ID]; dup || x.Billed < 0 {
			return nil, fmt.Errorf("%w: invoice %s", ErrInconsistent, x.ID)
		}
		invIndex[x.ID] = i
	}

	type pair struct{ pay, inv int }
	byPair := map[pair]*holding{}
	for _, a := range existing {
		pi, okP := payIndex[a.PaymentID]
		ii, okI := invIndex[a.InvoiceID]
		if !okP || !okI {
			return nil, fmt.Errorf("%w: allocation %d names an unknown payment or invoice", ErrInconsistent, a.Seq)
		}
		h := byPair[pair{pi, ii}]
		if h == nil {
			h = &holding{pay: pi, inv: ii}
			byPair[pair{pi, ii}] = h
			p.holdings = append(p.holdings, h)
		}
		h.amount += a.Amount
		if a.Amount > 0 && a.Seq > h.lastSeq {
			h.lastSeq = a.Seq
		}
	}

	p.allocated = make([]int64, len(p.pays))
	p.paid = make([]int64, len(p.invs))
	for _, h := range p.holdings {
		if h.amount < 0 {
			return nil, fmt.Errorf("%w: payment %s has a negative total on invoice %s",
				ErrInconsistent, p.pays[h.pay].ID, p.invs[h.inv].ID)
		}
		p.allocated[h.pay] += h.amount
		p.paid[h.inv] += h.amount
	}
	return p, nil
}

// takeBack writes a negative row that moves amount off a holding.
func (p *planner) takeBack(h *holding, amount int64) {
	h.amount -= amount
	p.allocated[h.pay] -= amount
	p.paid[h.inv] -= amount
	p.out = append(p.out, Row{PaymentID: p.pays[h.pay].ID, InvoiceID: p.invs[h.inv].ID, Amount: -amount})
}

// reverse takes excess back from the given holdings in their order.
func (p *planner) reverse(hs []*holding, excess int64) {
	for _, h := range hs {
		if excess == 0 {
			return
		}
		amount := min(h.amount, excess)
		p.takeBack(h, amount)
		excess -= amount
	}
}

// heldBy returns the holdings with money on them that match keep.
func (p *planner) heldBy(keep func(*holding) bool) []*holding {
	var out []*holding
	for _, h := range p.holdings {
		if h.amount > 0 && keep(h) {
			out = append(out, h)
		}
	}
	return out
}

func (p *planner) reverseOverpaidInvoices() {
	for i, inv := range p.invs {
		excess := p.paid[i] - inv.Billed
		if excess <= 0 {
			continue
		}
		hs := p.heldBy(func(h *holding) bool { return h.inv == i })
		// Newest allocation first.
		slices.SortFunc(hs, func(a, b *holding) int { return cmp.Compare(b.lastSeq, a.lastSeq) })
		p.reverse(hs, excess)
	}
}

func (p *planner) reverseOverallocatedPayments() {
	for i, pay := range p.pays {
		excess := p.allocated[i] - pay.Net
		if excess <= 0 {
			continue
		}
		hs := p.heldBy(func(h *holding) bool { return h.pay == i })
		// Newest invoice first: invoices are sorted oldest first, so the highest index.
		slices.SortFunc(hs, func(a, b *holding) int { return cmp.Compare(b.inv, a.inv) })
		p.reverse(hs, excess)
	}
}

func (p *planner) payFIFO() {
	pi, ii := 0, 0
	for pi < len(p.pays) && ii < len(p.invs) {
		remainder := p.pays[pi].Net - p.allocated[pi]
		if remainder <= 0 {
			pi++
			continue
		}
		outstanding := p.invs[ii].Billed - p.paid[ii]
		if outstanding <= 0 {
			ii++
			continue
		}
		amount := min(remainder, outstanding)
		p.allocated[pi] += amount
		p.paid[ii] += amount
		p.out = append(p.out, Row{PaymentID: p.pays[pi].ID, InvoiceID: p.invs[ii].ID, Amount: amount})
	}
}
