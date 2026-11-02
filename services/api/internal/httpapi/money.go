package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
)

func (s *Server) getTenantAccount(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	acct, err := s.Billing.Account(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	totals := toAPITotals(acct.Totals)
	out := apigen.TenantAccount{
		TenantId: acct.TenantID, Outstanding: totals.Outstanding, Overdue: totals.Overdue, OldestUnpaidDueDate: totals.OldestUnpaidDueDate,
		Credit:   totals.Credit,
		Invoices: make([]apigen.AccountInvoice, len(acct.Invoices)),
		Payments: make([]apigen.AccountPayment, len(acct.Payments)),
	}
	for i, inv := range acct.Invoices {
		out.Invoices[i] = toAPIAccountInvoice(inv)
	}
	for i, pay := range acct.Payments {
		out.Payments[i] = toAPIAccountPayment(pay)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) recordPayment(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.PaymentInput
	if !s.decode(w, r, &body) {
		return
	}
	in := billing.PaymentInput{
		RequestID: body.RequestId, TenantID: body.TenantId, Amount: body.Amount, Method: string(body.Method),
		ReceivedOn: body.ReceivedOn.Time,
	}
	if body.Note != nil {
		in.Note = *body.Note
	}
	res, err := s.Billing.RecordPayment(r.Context(), p, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	totals := toAPITotals(res.Totals)
	out := apigen.PaymentResult{
		Payment: toAPIAccountPayment(res.Payment), Paid: make([]apigen.PaidInvoice, len(res.Paid)),
		Outstanding: totals.Outstanding, Overdue: totals.Overdue, OldestUnpaidDueDate: totals.OldestUnpaidDueDate, Credit: totals.Credit,
	}
	for i, inv := range res.Paid {
		out.Paid[i] = apigen.PaidInvoice{
			InvoiceId: inv.InvoiceID, Period: date(inv.Period), PeriodEnd: date(inv.PeriodEnd), DueDate: date(inv.DueDate),
			Amount: inv.Amount, Outstanding: inv.Outstanding,
		}
	}
	s.writeJSON(w, createdStatus(res.Created), out)
}

func (s *Server) createCorrection(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.CorrectionInput
	if !s.decode(w, r, &body) {
		return
	}
	in := billing.CorrectionInput{RequestID: body.RequestId, AmountDelta: body.AmountDelta, NewAmount: body.NewAmount, Reason: body.Reason}
	if body.InvoiceId != nil {
		in.InvoiceID = *body.InvoiceId
	}
	if body.PaymentId != nil {
		in.PaymentID = *body.PaymentId
	}
	res, err := s.Billing.CreateCorrection(r.Context(), p, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	totals := toAPITotals(res.Totals)
	s.writeJSON(w, createdStatus(res.Created), apigen.CorrectionResult{
		Correction: toAPICorrection(res.Correction), TenantId: res.TenantID,
		Outstanding: totals.Outstanding, Overdue: totals.Overdue, OldestUnpaidDueDate: totals.OldestUnpaidDueDate, Credit: totals.Credit,
	})
}

// createdStatus is 201 for a new record and 200 for a request repeated under its requestId.
func createdStatus(created bool) int {
	if created {
		return http.StatusCreated
	}
	return http.StatusOK
}

func toAPITotals(t billing.Totals) apigen.AccountTotals {
	out := apigen.AccountTotals{Outstanding: t.Outstanding, Overdue: t.Overdue, Credit: t.Credit}
	if t.OldestUnpaidDueDate != nil {
		d := date(*t.OldestUnpaidDueDate)
		out.OldestUnpaidDueDate = &d
	}
	return out
}

func toAPIAccountInvoice(inv billing.AccountInvoice) apigen.AccountInvoice {
	out := apigen.AccountInvoice{
		Id: inv.ID, LeaseId: inv.LeaseID, UnitLabel: inv.UnitLabel, PropertyName: inv.PropertyName,
		Period: date(inv.Period), PeriodEnd: date(inv.PeriodEnd), DueDate: date(inv.DueDate), IssuedOn: date(inv.IssuedOn),
		Amount: inv.Amount, Billed: inv.Billed, Paid: inv.Paid, Outstanding: inv.Outstanding,
		Status:      apigen.InvoiceStatus(inv.Status),
		Lines:       make([]apigen.InvoiceLine, len(inv.Lines)),
		Corrections: toAPICorrections(inv.Corrections),
	}
	for i, l := range inv.Lines {
		out.Lines[i] = apigen.InvoiceLine{Id: l.ID, Kind: apigen.InvoiceLineKind(l.Kind), Label: l.Label, Amount: l.Amount}
	}
	return out
}

func toAPIAccountPayment(pay billing.AccountPayment) apigen.AccountPayment {
	out := apigen.AccountPayment{
		Id: pay.ID, Amount: pay.Amount, Net: pay.Net, Method: apigen.PaymentMethod(pay.Method),
		ReceivedOn: date(pay.ReceivedOn), Note: pay.Note, CreatedAt: pay.CreatedAt,
		Allocations: make([]apigen.PaymentAllocation, len(pay.Allocations)),
		Corrections: toAPICorrections(pay.Corrections),
	}
	for i, a := range pay.Allocations {
		out.Allocations[i] = apigen.PaymentAllocation{InvoiceId: a.InvoiceID, Amount: a.Amount}
	}
	if pay.RecordedBy != nil {
		out.RecordedBy = &apigen.StaffRef{MemberId: pay.RecordedBy.MemberID, Name: pay.RecordedBy.Name}
	}
	return out
}

func toAPICorrections(cs []billing.CorrectionRecord) []apigen.Correction {
	out := make([]apigen.Correction, len(cs))
	for i, c := range cs {
		out[i] = toAPICorrection(c)
	}
	return out
}

func toAPICorrection(c billing.CorrectionRecord) apigen.Correction {
	out := apigen.Correction{
		Id: c.ID, AmountDelta: c.AmountDelta, Reason: c.Reason, CreatedAt: c.CreatedAt,
		CreatedBy: apigen.StaffRef{MemberId: c.CreatedBy.MemberID, Name: c.CreatedBy.Name},
	}
	if c.InvoiceID != uuid.Nil {
		id := c.InvoiceID
		out.InvoiceId = &id
	}
	if c.PaymentID != uuid.Nil {
		id := c.PaymentID
		out.PaymentId = &id
	}
	return out
}

// date turns a calendar day into the API's date type.
func date(t time.Time) openapi_types.Date { return openapi_types.Date{Time: t} }
