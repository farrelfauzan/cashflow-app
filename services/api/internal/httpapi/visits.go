package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// The karyawan's visits (slice 6, S6-4): "Kunjungan hari ini" and "Catat hasil kunjungan".

func (s *Server) listFieldVisits(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Visits.List(r.Context(), p, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.FieldVisitList{
		Items: make([]apigen.FieldVisit, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	}
	for i, row := range res.Items {
		out.Items[i] = toAPIFieldVisit(row)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) recordVisitOutcome(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	tenantID, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.VisitOutcomeInput
	if !s.decode(w, r, &body) {
		return
	}
	in := visit.Input{RequestID: body.RequestId, Outcome: string(body.Outcome)}
	if body.PaymentId != nil {
		in.PaymentID = *body.PaymentId
	}
	if body.PromiseVersionId != nil {
		in.PromiseVersionID = *body.PromiseVersionId
	}
	if body.NewPhone != nil {
		in.NewPhone = *body.NewPhone
	}
	if body.Note != nil {
		in.Note = *body.Note
	}
	out, created, err := s.Visits.Record(r.Context(), p, tenantID, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, createdStatus(created), toAPIVisitOutcome(out))
}

// failVisit answers the visit package's errors; false when err is not one of them.
func (s *Server) failVisit(w http.ResponseWriter, err error) bool {
	var input *visit.InputError
	switch {
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, input.Field)
	case errors.Is(err, visit.ErrInvalidPhone):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_phone", "Nomor WhatsApp tidak valid.", "newPhone")
	case errors.Is(err, visit.ErrNotOpen):
		s.writeError(w, http.StatusConflict, "visit_not_open", "Penyewa ini tidak ada di daftar kunjungan.")
	case errors.Is(err, visit.ErrPaymentInvalid):
		s.writeErrorField(w, http.StatusBadRequest, "visit_payment_invalid",
			"Pembayaran ini tidak bisa dipakai untuk hasil kunjungan ini.", "paymentId")
	case errors.Is(err, visit.ErrPromiseInvalid):
		s.writeErrorField(w, http.StatusBadRequest, "visit_promise_invalid",
			"Janji ini tidak bisa dipakai untuk hasil kunjungan ini.", "promiseVersionId")
	case errors.Is(err, visit.ErrRequestIDReused):
		s.writeErrorField(w, http.StatusConflict, "request_id_reused",
			"Permintaan ini sudah dipakai untuk data lain. Muat ulang halaman lalu coba lagi.", "requestId")
	default:
		return false
	}
	return true
}

func toAPIFieldVisit(row visit.Row) apigen.FieldVisit {
	out := apigen.FieldVisit{
		TenantId: row.TenantID, TenantName: row.TenantName, Units: make([]apigen.FieldUnit, len(row.Units)),
		Reasons: make([]apigen.VisitReason, len(row.Reasons)), Outstanding: row.Outstanding, Overdue: row.Overdue,
		Status: apigen.FieldVisitStatus(row.Status), UnlinkedCashToday: toAPIFieldCashPayments(row.UnlinkedCashToday),
	}
	for i, u := range row.Units {
		out.Units[i] = apigen.FieldUnit{UnitLabel: u.UnitLabel, PropertyName: u.PropertyName, PropertyAddress: u.PropertyAddress}
	}
	for i, reason := range row.Reasons {
		r := apigen.VisitReason{Code: apigen.VisitReasonCode(reason.Code)}
		switch reason.Code {
		case visit.ReasonOverdue:
			days := reason.DaysOverdue
			r.DaysOverdue = &days
		case visit.ReasonBrokenPromises:
			n := reason.BrokenPromises
			r.BrokenPromises = &n
		}
		out.Reasons[i] = r
	}
	if row.OldestUnpaidDueDate != nil {
		d := date(*row.OldestUnpaidDueDate)
		out.OldestUnpaidDueDate = &d
	}
	if row.LastOutcome != nil {
		o := toAPIVisitOutcome(*row.LastOutcome)
		out.LastOutcome = &o
	}
	if v := row.UnlinkedPromiseToday; v != nil {
		out.UnlinkedPromiseToday = &apigen.FieldPromiseVersion{
			PromiseVersionId: v.VersionID, PromiseId: v.PromiseID, Version: v.Version, Kind: apigen.PromiseKind(v.Kind),
			PromisedDate: date(v.PromisedDate), PromisedAmount: v.PromisedAmount, CreatedAt: v.CreatedAt,
		}
	}
	return out
}

func toAPIVisitOutcome(o visit.Outcome) apigen.VisitOutcome {
	out := apigen.VisitOutcome{
		Id: o.ID, TenantId: o.TenantID, Outcome: apigen.VisitOutcomeKind(o.Outcome), RecordedAt: o.RecordedAt,
		Note: optionalString(o.Note), NewPhone: optionalString(o.NewPhone), Resolved: make([]apigen.VisitReasonCode, len(o.Resolved)),
	}
	if o.PaymentID != uuid.Nil {
		id := o.PaymentID
		out.PaymentId = &id
	}
	if o.PromiseVersionID != uuid.Nil {
		id := o.PromiseVersionID
		out.PromiseVersionId = &id
	}
	for i, r := range o.Resolved {
		out.Resolved[i] = apigen.VisitReasonCode(r)
	}
	return out
}

func toAPIFieldCashPayments(items []billing.FieldCashPayment) []apigen.FieldCashPayment {
	out := make([]apigen.FieldCashPayment, len(items))
	for i, p := range items {
		out[i] = apigen.FieldCashPayment{PaymentId: p.PaymentID, Amount: p.Amount, RecordedAt: p.RecordedAt, Photo: toAPIReceiptPhoto(p.Photo)}
	}
	return out
}
