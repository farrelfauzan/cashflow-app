package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// Limits on transfer proofs, on top of the payment page's: each stored proof costs storage and a
// scan, so one link sends at most proofsPerLink per proofWindow.
const (
	proofWindow   = time.Hour
	proofsPerLink = 10
	// proofBodyBytes caps the request body: the 10 MB photo plus the multipart framing.
	proofBodyBytes = upload.MaxSize + 64<<10
)

// proofFormMessage answers a body that is not multipart/form-data with a file part first.
const proofFormMessage = "Kirim foto bukti transfer sebagai file."

// sendPaymentProof is "Kirim bukti transfer" on the payment page (S6-5). Public like
// getPaymentPage, with the same limits counted before the token is looked up, and its own per-link
// limit; the token grants it (payment:proof), so there is no principal to authorize. It is the one
// route with a body above maxBodyBytes and more time than requestTimeout (limitBody,
// serverHandler). The token is never logged.
func (s *Server) sendPaymentProof(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	if s.Proofs == nil {
		s.writeError(w, http.StatusServiceUnavailable, "uploads_unavailable", "Unggah foto belum tersedia di server ini.")
		return
	}
	ok, err := s.PublicLimiter.Allow(r.Context(), "proof:"+paylink.LimitKey(token), proofsPerLink, proofWindow)
	if err != nil {
		s.fail(w, r, fmt.Errorf("checking the transfer proof rate limit: %w", err))
		return
	}
	if !ok {
		s.writeError(w, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak bukti dikirim. Coba lagi nanti.")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, proofBodyBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", proofFormMessage, "file")
		return
	}
	part, err := mr.NextPart()
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.failProof(w, r, upload.ErrTooLarge)
			return
		}
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", proofFormMessage, "file")
		return
	}
	defer func() { _ = part.Close() }()
	if part.FormName() != "file" {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", proofFormMessage, "file")
		return
	}
	proof, err := s.Proofs.Send(r.Context(), token, part)
	if err != nil {
		s.failProof(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, apigen.PaymentProof{Id: proof.ID, CreatedAt: proof.CreatedAt})
}

// failProof answers a transfer proof's errors; every one names the file field.
func (s *Server) failProof(w http.ResponseWriter, r *http.Request, err error) {
	var input *upload.InputError
	switch {
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, "file")
	case errors.Is(err, upload.ErrTypeNotAllowed):
		s.writeErrorField(w, http.StatusBadRequest, "upload_type_not_allowed",
			"Hanya foto JPEG, PNG atau WebP yang bisa dikirim.", "file")
	case errors.Is(err, upload.ErrTooLarge):
		s.writeErrorField(w, http.StatusBadRequest, "upload_too_large", "Ukuran foto paling besar 10 MB.", "file")
	default:
		s.fail(w, r, err)
	}
}
