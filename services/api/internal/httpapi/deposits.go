package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
)

// "Setor tunai" (slice 6, S6-3): the karyawan's deposits of the cash they hold, and the admin's
// count of them.

func (s *Server) listMyUndepositedCash(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Billing.UndepositedCash(r.Context(), p, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, apigen.UndepositedCashList{
		Items: toAPIHeldPayments(res.Items), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	})
}

func (s *Server) listMyCashDeposits(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Deposits.Mine(r.Context(), p, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIDepositList(res))
}

func (s *Server) recordCashDeposit(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.CashDepositInput
	if !s.decode(w, r, &body) {
		return
	}
	in := deposit.Input{
		RequestID: body.RequestId, ExpectedAmount: body.ExpectedAmount, ExpectedCount: body.ExpectedCount, Amount: body.Amount,
	}
	if body.Note != nil {
		in.Note = *body.Note
	}
	if body.PhotoUploadId != nil {
		in.PhotoUploadID = *body.PhotoUploadId
	}
	d, created, err := s.Deposits.Record(r.Context(), p, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, createdStatus(created), toAPIDeposit(d))
}

func (s *Server) listCashDeposits(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	q := deposit.Query{Status: lp.status}
	if raw := r.URL.Query().Get("staffUserId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Anggota tidak dikenal.", "staffUserId")
			return
		}
		q.StaffUserID = id
	}
	res, err := s.Deposits.List(r.Context(), p, q, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIDepositList(res))
}

func (s *Server) getCashDeposit(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "depositID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	d, err := s.Deposits.Get(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIDeposit(d))
}

func (s *Server) listCashDepositPayments(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "depositID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Deposits.Payments(r.Context(), p, id, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, apigen.CashDepositPaymentList{
		Items: toAPIHeldPayments(res.Items), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	})
}

func (s *Server) confirmCashDeposit(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "depositID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.CashDepositConfirmationInput
	if !s.decode(w, r, &body) {
		return
	}
	in := deposit.ConfirmInput{ReceivedAmount: body.ReceivedAmount}
	if body.Note != nil {
		in.Note = *body.Note
	}
	d, created, err := s.Deposits.Confirm(r.Context(), p, id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, createdStatus(created), toAPIDeposit(d))
}

// failDeposit answers the deposit package's errors; false when err is not one of them.
func (s *Server) failDeposit(w http.ResponseWriter, err error) bool {
	var input *deposit.InputError
	switch {
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, input.Field)
	case errors.Is(err, deposit.ErrStatusUnknown):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Status tidak dikenal.", "status")
	case errors.Is(err, deposit.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "not_found", "Tidak ditemukan.")
	case errors.Is(err, deposit.ErrPhotoNotFound):
		s.writeErrorField(w, http.StatusNotFound, "not_found", "Tidak ditemukan.", "photoUploadId")
	case errors.Is(err, deposit.ErrNothingToDeposit):
		s.writeError(w, http.StatusConflict, "nothing_to_deposit", "Tidak ada tunai yang perlu disetor.")
	case errors.Is(err, deposit.ErrCashChanged):
		s.writeError(w, http.StatusConflict, "deposit_cash_changed", "Jumlah tunai berubah. Periksa lagi lalu setor.")
	case errors.Is(err, deposit.ErrPhotoInvalid):
		s.writeErrorField(w, http.StatusBadRequest, "deposit_photo_upload_invalid",
			"Foto ini tidak bisa dipakai untuk setoran ini. Ambil foto lagi.", "photoUploadId")
	case errors.Is(err, deposit.ErrPhotoRefused):
		s.writeErrorField(w, http.StatusConflict, "deposit_photo_upload_refused",
			"Foto ini ditolak saat diperiksa. Ambil foto lagi.", "photoUploadId")
	case errors.Is(err, deposit.ErrRequestIDReused):
		s.writeErrorField(w, http.StatusConflict, "request_id_reused",
			"Permintaan ini sudah dipakai untuk data lain. Muat ulang halaman lalu coba lagi.", "requestId")
	case errors.Is(err, deposit.ErrAlreadyConfirmed):
		s.writeError(w, http.StatusConflict, "deposit_already_confirmed", "Setoran ini sudah dihitung dengan jumlah lain.")
	default:
		return false
	}
	return true
}

func toAPIHeldPayments(items []billing.HeldPayment) []apigen.HeldCashPayment {
	out := make([]apigen.HeldCashPayment, len(items))
	for i, h := range items {
		units := make([]apigen.FieldUnit, len(h.Units))
		for j, u := range h.Units {
			units[j] = apigen.FieldUnit{UnitLabel: u.UnitLabel, PropertyName: u.PropertyName, PropertyAddress: u.PropertyAddress}
		}
		out[i] = apigen.HeldCashPayment{
			PaymentId: h.PaymentID, TenantId: h.TenantID, TenantName: h.TenantName, Units: units,
			Amount: h.Amount, RecordedAt: h.RecordedAt, Photo: toAPIReceiptPhoto(h.Photo),
		}
	}
	return out
}

func toAPIDepositList(res listing.Result[deposit.Deposit]) apigen.CashDepositList {
	out := apigen.CashDepositList{
		Items: make([]apigen.CashDeposit, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	}
	for i, d := range res.Items {
		out.Items[i] = toAPIDeposit(d)
	}
	return out
}

func toAPIDeposit(d deposit.Deposit) apigen.CashDeposit {
	out := apigen.CashDeposit{
		Id: d.ID, StaffUserId: d.StaffUserID, StaffName: d.StaffName, Amount: d.Amount, ExpectedAmount: d.ExpectedAmount,
		PaymentCount: d.PaymentCount, Note: optionalString(d.Note), Photo: toAPIReceiptPhoto(d.Photo),
		RecordedAt: d.RecordedAt, Status: apigen.CashDepositStatus(d.Status),
	}
	if c := d.Confirmation; c != nil {
		out.Confirmation = &apigen.CashDepositConfirmation{
			ReceivedAmount: c.ReceivedAmount, Gap: c.Gap, Note: optionalString(c.Note),
			ConfirmedBy: c.ConfirmedBy, ConfirmedByName: c.ConfirmedByName, ConfirmedAt: c.ConfirmedAt,
		}
	}
	return out
}

// optionalString is s, or nil when it is empty.
func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
