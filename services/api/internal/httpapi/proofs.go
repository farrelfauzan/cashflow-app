package httpapi

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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

// errProofForm is a multipart body whose first part is missing or not named file.
var errProofForm = errors.New("httpapi: the proof's first part is not file")

// sendPaymentProof is "Kirim bukti transfer" on the payment page (S6-5). Public like
// getPaymentPage, with the same limits counted before the token is looked up, and its own per-link
// limit; the token grants it (payment:proof), so there is no principal to authorize. It is the one
// route with a body above maxBodyBytes and more time than requestTimeout (limitBody,
// serverHandler). The token is never logged.
func (s *Server) sendPaymentProof(w http.ResponseWriter, r *http.Request) {
	// Every answer closes the connection: the server then writes it at once instead of first
	// reading up to 256 KB of a body it was refused with, at whatever pace the sender trickles it.
	w.Header().Set("Connection", "close")
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	if s.Proofs == nil {
		s.writeError(w, http.StatusServiceUnavailable, "uploads_unavailable", "Unggah foto belum tersedia di server ini.")
		return
	}
	var requestID uuid.UUID
	if raw := r.URL.Query().Get("requestId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "requestId tidak valid.", "requestId")
			return
		}
		requestID = id
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
	// Reading the Content-Type header reads nothing of the body.
	mr, err := r.MultipartReader()
	if err != nil {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", proofFormMessage, "file")
		return
	}
	// The first part is read only once Send has opened the link: a request with a link that does
	// not work never has its body read.
	var part *multipart.Part
	defer func() {
		if part != nil {
			_ = part.Close()
		}
	}()
	open := func() (io.Reader, error) {
		p, err := mr.NextPart()
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				return nil, upload.ErrTooLarge
			}
			return nil, errProofForm
		}
		part = p
		if p.FormName() != "file" {
			return nil, errProofForm
		}
		return p, nil
	}
	proof, err := s.Proofs.Send(r.Context(), token, requestID, open)
	if err != nil {
		s.failProof(w, r, err)
		return
	}
	s.writeJSON(w, createdStatus(proof.Created), apigen.PaymentProof{Id: proof.ID, CreatedAt: proof.CreatedAt})
}

// failProof answers a transfer proof's errors; each names the field it is about.
func (s *Server) failProof(w http.ResponseWriter, r *http.Request, err error) {
	var input *upload.InputError
	switch {
	case errors.Is(err, errProofForm):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", proofFormMessage, "file")
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, "file")
	case errors.Is(err, upload.ErrTypeNotAllowed):
		s.writeErrorField(w, http.StatusBadRequest, "upload_type_not_allowed",
			"Hanya foto JPEG, PNG atau WebP yang bisa dikirim.", "file")
	case errors.Is(err, upload.ErrTooLarge):
		s.writeErrorField(w, http.StatusBadRequest, "upload_too_large", "Ukuran foto paling besar 10 MB.", "file")
	case errors.Is(err, upload.ErrRequestIDReused):
		s.writeErrorField(w, http.StatusConflict, "request_id_reused",
			"Permintaan ini sudah dipakai untuk data lain. Muat ulang halaman lalu coba lagi.", "requestId")
	default:
		s.fail(w, r, err)
	}
}
