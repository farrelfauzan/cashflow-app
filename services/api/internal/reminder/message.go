package reminder

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// OwingInvoice is one of the tenant's invoices that still owes something at send time.
type OwingInvoice struct {
	ID uuid.UUID
	// Period and DueDate are calendar days, midnight UTC like a date column.
	Period  time.Time
	DueDate time.Time
	// Billed is the amount after corrections, Paid what is allocated to it, Outstanding the rest.
	Billed      int64
	Paid        int64
	Outstanding int64
}

// Line is one line of an invoice, as it was snapshotted when the invoice was made.
type Line struct {
	Label  string
	Amount int64
}

// Recipient is who a tenant message goes to and the organization it names.
type Recipient struct {
	TenantID     uuid.UUID
	Name         string
	Phone        string
	Organization string // the organization's display name
}

// invoiceMessage builds the message of a reminder that carries only invoice stages (after the
// send-time check), at today (a calendar day, midnight UTC).
//
// The rule between one invoice and the rollover: when the tenant owes on exactly one invoice at
// send time (necessarily the one reminded), the stage's own template tells them about it with its
// line breakdown. Whenever they owe on more than one, the rollover sums everything they owe into
// one total with one link ("Tagihan Desember 2026 Rp700.000 + sisa November 2026 Rp200.000 =
// Rp900.000"), whatever the stages: paying through the link pays the oldest first, so one total
// is what they need to know.
func invoiceMessage(rcpt Recipient, stages []InvoiceStage, owing []OwingInvoice, lines map[uuid.UUID][]Line, today time.Time) (string, map[string]string) {
	params := map[string]string{"name": rcpt.Name, "organization": rcpt.Organization}
	if len(owing) == 1 && len(stages) == 1 && stages[0].InvoiceID == owing[0].ID {
		inv := owing[0]
		params["month"] = messaging.Month(inv.Period)
		params["amount"] = messaging.Rupiah(inv.Outstanding)
		params["lines"] = breakdown(inv, lines[inv.ID])
		switch stages[0].Stage {
		case StageHMinus3:
			params["due_date"] = messaging.Date(inv.DueDate)
			return messaging.TemplateInvoiceDueSoon, params
		case StageH0:
			return messaging.TemplateInvoiceDueToday, params
		default:
			params["days_late"] = strconv.Itoa(daysBetween(inv.DueDate, today))
			return messaging.TemplateInvoiceOverdue, params
		}
	}
	summary, total := rolloverSummary(stages, owing, today)
	params["summary"] = summary
	params["total"] = messaging.Rupiah(total)
	return messaging.TemplateInvoiceRollover, params
}

// breakdown is an invoice's line breakdown on one line: each line with its amount, then the
// correction if the lines no longer add up to what is billed, then what was already paid.
func breakdown(inv OwingInvoice, lines []Line) string {
	parts := make([]string, 0, len(lines)+2)
	if len(lines) == 0 {
		// Every invoice is made with its lines; this keeps the parameter filled regardless.
		parts = append(parts, "Tagihan "+messaging.Rupiah(inv.Billed))
	}
	var sum int64
	for _, l := range lines {
		parts = append(parts, l.Label+" "+messaging.Rupiah(l.Amount))
		sum += l.Amount
	}
	if d := inv.Billed - sum; d != 0 && len(lines) > 0 {
		parts = append(parts, "koreksi "+messaging.Rupiah(d))
	}
	if inv.Paid > 0 {
		parts = append(parts, "sudah dibayar "+messaging.Rupiah(inv.Paid))
	}
	return strings.Join(parts, ", ")
}

// rolloverSummary sums everything the tenant owes by month: the reminded invoices as "Tagihan
// <month>" first, then the others, each "sisa <month>" when it is past due or partly paid and
// "Tagihan <month>" otherwise, every group in period order. It returns the terms joined with " + "
// and their total.
func rolloverSummary(stages []InvoiceStage, owing []OwingInvoice, today time.Time) (string, int64) {
	reminded := make(map[uuid.UUID]bool, len(stages))
	for _, s := range stages {
		reminded[s.InvoiceID] = true
	}
	type term struct {
		label  string
		amount int64
	}
	var terms []term
	index := map[string]int{}
	add := func(label string, amount int64) {
		if i, ok := index[label]; ok {
			terms[i].amount += amount
			return
		}
		index[label] = len(terms)
		terms = append(terms, term{label: label, amount: amount})
	}
	var total int64
	for _, inv := range owing {
		if reminded[inv.ID] {
			add("Tagihan "+messaging.Month(inv.Period), inv.Outstanding)
		}
		total += inv.Outstanding
	}
	for _, inv := range owing {
		if reminded[inv.ID] {
			continue
		}
		word := "Tagihan "
		if inv.Paid > 0 || inv.DueDate.Before(today) {
			word = "sisa "
		}
		add(word+messaging.Month(inv.Period), inv.Outstanding)
	}
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = t.label + " " + messaging.Rupiah(t.amount)
	}
	return strings.Join(parts, " + "), total
}

// promisedAmount is how a message names what a promise version promises.
func promisedAmount(amount *int64) string {
	if amount == nil {
		return messaging.AllOutstanding
	}
	return messaging.Rupiah(*amount)
}

// daysBetween is the number of calendar days from a to b (both midnight UTC).
func daysBetween(a, b time.Time) int { return int(b.Sub(a).Hours() / 24) }
