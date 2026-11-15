package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/checkqueue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
)

// listCheckQueue is Antrean cek (S6-6).
func (s *Server) listCheckQueue(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.CheckQueue.List(r.Context(), p, r.URL.Query().Get("kind"), lp.page)
	if err != nil {
		s.failCheckQueue(w, r, err)
		return
	}
	out := apigen.CheckQueuePage{
		Items: make([]apigen.CheckQueueItem, len(res.Items)), Page: lp.page.Number, PageSize: lp.page.Size,
		Total: res.Total, Waiting: res.Waiting,
		Counts: apigen.CheckQueueCounts{
			TransferProof: res.Counts[checkqueue.KindTransferProof], DepositGap: res.Counts[checkqueue.KindDepositGap],
			UploadInfected: res.Counts[checkqueue.KindUploadInfected], QrisAmountMismatch: res.Counts[checkqueue.KindQrisAmountMismatch],
			PhoneChanged: res.Counts[checkqueue.KindPhoneChanged],
		},
	}
	for i, it := range res.Items {
		out.Items[i] = toAPICheckItem(it)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// matchTransferProof is "Simpan pembayaran" on a proof in Antrean cek.
func (s *Server) matchTransferProof(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.MatchTransferProofRequest
	if !s.decode(w, r, &body) {
		return
	}
	in := checkqueue.MatchInput{RequestID: body.RequestId, Amount: body.Amount, ReceivedOn: body.ReceivedOn.Time}
	if body.TenantId != nil {
		in.TenantID = *body.TenantId
	}
	if body.Note != nil {
		in.Note = *body.Note
	}
	res, pay, err := s.CheckQueue.Match(r.Context(), p, id, in)
	if err != nil {
		s.failCheckQueue(w, r, err)
		return
	}
	s.writeJSON(w, createdStatus(pay.Created), apigen.MatchTransferProofResult{
		Resolution: toAPICheckResolution(res), Payment: toAPIPaymentResult(pay),
	})
}

// applyPhoneChange is "Pakai nomor baru" on a new number in Antrean cek.
func (s *Server) applyPhoneChange(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	res, err := s.CheckQueue.Apply(r.Context(), p, id)
	if err != nil {
		s.failCheckQueue(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPICheckResolution(res))
}

// dismissCheckItem takes an item off Antrean cek with a note.
func (s *Server) dismissCheckItem(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "itemID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.DismissCheckItemRequest
	if !s.decode(w, r, &body) {
		return
	}
	res, err := s.CheckQueue.Dismiss(r.Context(), p, chi.URLParam(r, "kind"), id, body.Note)
	if err != nil {
		s.failCheckQueue(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPICheckResolution(res))
}

// failCheckQueue answers the check queue's errors; anything else goes to fail.
func (s *Server) failCheckQueue(w http.ResponseWriter, r *http.Request, err error) {
	var input *checkqueue.InputError
	switch {
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, input.Field)
	case errors.Is(err, checkqueue.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "not_found", "Tidak ditemukan.")
	case errors.Is(err, checkqueue.ErrResolved):
		s.writeError(w, http.StatusConflict, "check_item_resolved", "Item ini sudah ditangani. Muat ulang antrean.")
	case errors.Is(err, checkqueue.ErrUploadNotReady):
		s.writeError(w, http.StatusConflict, "upload_not_ready", "Foto bukti masih diperiksa. Tunggu sebentar lalu coba lagi.")
	case errors.Is(err, checkqueue.ErrUploadRefused):
		s.writeError(w, http.StatusConflict, "upload_refused", "Foto bukti ditolak saat diperiksa, jadi tidak bisa dicocokkan.")
	case errors.Is(err, checkqueue.ErrRequestIDReused):
		s.writeError(w, http.StatusConflict, "request_id_reused", "Permintaan ini sudah dipakai untuk pembayaran lain. Muat ulang halaman lalu coba lagi.")
	default:
		s.fail(w, r, err)
	}
}

func toAPICheckResolution(res checkqueue.Resolution) apigen.CheckResolution {
	out := apigen.CheckResolution{
		Id: res.ID, Kind: apigen.CheckQueueKind(res.Kind), ItemId: res.ItemID, Outcome: apigen.CheckResolutionOutcome(res.Outcome),
		ResolvedBy: res.By, ResolvedAt: res.CreatedAt,
	}
	if res.PaymentID != uuid.Nil {
		id := res.PaymentID
		out.PaymentId = &id
	}
	if res.Note != "" {
		note := res.Note
		out.Note = &note
	}
	return out
}

func toAPICheckItem(it checkqueue.Item) apigen.CheckQueueItem {
	out := apigen.CheckQueueItem{Kind: apigen.CheckQueueKind(it.Kind), Id: it.ID, CreatedAt: it.CreatedAt}
	switch {
	case it.TransferProof != nil:
		v := it.TransferProof
		out.TransferProof = &apigen.CheckTransferProof{
			Tenant: toAPICheckTenant(v.Tenant), InvoiceId: v.InvoiceID, UploadId: v.UploadID,
			UploadStatus: apigen.CheckTransferProofUploadStatus(v.UploadStatus),
		}
	case it.DepositGap != nil:
		v := it.DepositGap
		out.DepositGap = &apigen.CheckDepositGap{
			Depositor: toAPICheckStaff(v.Depositor), ExpectedAmount: v.Expected, ReceivedAmount: v.Received, Gap: v.Gap,
			PaymentCount: v.PaymentCount, ConfirmedBy: toAPICheckStaff(v.ConfirmedBy), ConfirmedAt: v.ConfirmedAt,
		}
	case it.UploadInfected != nil:
		v := it.UploadInfected
		u := &apigen.CheckUploadInfected{UploadId: v.UploadID, Purpose: apigen.UploadPurpose(v.Purpose), ScannedAt: v.ScannedAt}
		if v.Tenant != nil {
			t := toAPICheckTenant(*v.Tenant)
			u.Tenant = &t
		}
		if v.UploadedBy != nil {
			by := toAPICheckStaff(*v.UploadedBy)
			u.UploadedBy = &by
		}
		out.UploadInfected = u
	case it.QrisAmountMismatch != nil:
		v := it.QrisAmountMismatch
		out.QrisAmountMismatch = &apigen.CheckQrisAmountMismatch{
			AttemptId: v.AttemptID, Tenant: toAPICheckTenant(v.Tenant), TransactionId: v.TransactionID, QrAmount: v.QRAmount,
			RecordedAmount: v.RecordedAmount, ReportedAmount: v.ReportedAmount,
		}
	case it.PhoneChanged != nil:
		v := it.PhoneChanged
		pc := &apigen.CheckPhoneChanged{
			Tenant: toAPICheckTenant(v.Tenant), CurrentPhone: v.CurrentPhone, NewPhone: v.NewPhone, RecordedBy: toAPICheckStaff(v.RecordedBy),
		}
		if v.Note != "" {
			note := v.Note
			pc.Note = &note
		}
		out.PhoneChanged = pc
	}
	return out
}

func toAPICheckTenant(t checkqueue.TenantRef) apigen.CheckTenant {
	out := apigen.CheckTenant{Id: t.ID, Name: t.Name, Units: make([]apigen.CheckTenantUnit, len(t.Units))}
	for i, u := range t.Units {
		out.Units[i] = apigen.CheckTenantUnit{Active: u.Active, PropertyName: u.PropertyName, UnitLabel: u.Label}
	}
	return out
}

func toAPICheckStaff(s checkqueue.StaffRef) apigen.CheckStaff {
	return apigen.CheckStaff{StaffUserId: s.ID, Name: s.Name}
}
