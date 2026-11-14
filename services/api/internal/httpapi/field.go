package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
)

// The karyawan's field app (slice 6, S6-2): "Terima tunai", "Tagihan hari ini", the cash they
// hold and "Catat janji bayar".

func (s *Server) recordCashPayment(w http.ResponseWriter, r *http.Request) {
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
	var body apigen.CashPaymentInput
	if !s.decode(w, r, &body) {
		return
	}
	in := billing.CashInput{RequestID: body.RequestId, TenantID: id, Amount: body.Amount}
	if body.Note != nil {
		in.Note = *body.Note
	}
	res, err := s.Billing.RecordCashPayment(r.Context(), p, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pr := toAPIPaymentResult(res.PaymentResult)
	s.writeJSON(w, createdStatus(res.Created), apigen.CashPaymentResult{
		Payment: pr.Payment, Paid: pr.Paid,
		Outstanding: pr.Outstanding, Overdue: pr.Overdue, OldestUnpaidDueDate: pr.OldestUnpaidDueDate, Credit: pr.Credit,
		Receipt: apigen.PaymentReceipt{Status: apigen.PaymentReceiptStatus(res.ReceiptStatus)},
	})
}

func (s *Server) listFieldToday(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Billing.FieldToday(r.Context(), p, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.FieldTodayList{
		Items: make([]apigen.FieldTodayItem, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	}
	for i, t := range res.Items {
		out.Items[i] = toAPIFieldTodayItem(t)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) getMyFieldCash(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	cash, err := s.Billing.FieldCash(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPICashTotal(cash))
}

func (s *Server) makeFieldPromise(w http.ResponseWriter, r *http.Request) {
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
	var body apigen.MakeTenantPromiseRequest
	if !s.decode(w, r, &body) {
		return
	}
	in := promiseInput(body.Kind, body.PromisedDate, body.PromisedAmount, body.PayingNow)
	if body.Note != nil {
		in.Note = *body.Note
	}
	made, err := s.Promises.MakeFromField(r.Context(), p, id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPITenantPromise(made))
}

func toAPIFieldTodayItem(t billing.FieldTenant) apigen.FieldTodayItem {
	out := apigen.FieldTodayItem{
		TenantId: t.TenantID, TenantName: t.TenantName, Units: make([]apigen.FieldUnit, len(t.Units)),
		Reason: apigen.FieldDueReason(t.Reason.Code), Outstanding: t.Outstanding, Overdue: t.Overdue,
		CashToday: apigen.FieldCashToday{
			Amount: t.CashToday.Amount, Count: t.CashToday.Count, Payments: make([]apigen.FieldCashPayment, len(t.CashToday.Payments)),
		},
	}
	if t.OldestUnpaidDueDate != nil {
		d := date(*t.OldestUnpaidDueDate)
		out.OldestUnpaidDueDate = &d
	}
	for i, u := range t.Units {
		out.Units[i] = apigen.FieldUnit{UnitLabel: u.UnitLabel, PropertyName: u.PropertyName, PropertyAddress: u.PropertyAddress}
	}
	for i, p := range t.CashToday.Payments {
		out.CashToday.Payments[i] = apigen.FieldCashPayment{PaymentId: p.PaymentID, Amount: p.Amount, RecordedAt: p.RecordedAt}
	}
	days := t.Reason.Days
	switch t.Reason.Code {
	case billing.FieldOverdue:
		out.DaysOverdue = &days
	case billing.FieldDueInDays:
		out.DaysUntilDue = &days
	}
	return out
}

func toAPICashTotal(c billing.CashTotal) apigen.CashTotal {
	return apigen.CashTotal{Amount: c.Amount, Count: c.Count}
}
