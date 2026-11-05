package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

// getPaymentStatus answers what the payment page shows about the tenant's latest QRIS attempt.
// Public like getPaymentPage, with the same rate limits counted before the token is looked up.
func (s *Server) getPaymentStatus(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	v, err := s.QRIS.PublicStatus(r.Context(), token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIQrisStatus(v))
}

// getPaymentQRImage answers the PNG of the tenant's live QR. Public like getPaymentPage, with
// the same rate limits counted before the token is looked up.
func (s *Server) getPaymentQRImage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	png, err := s.QRIS.QRImage(r.Context(), token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	// G705: the body is a PNG rendered by qrimage and served as image/png with nosniff, not markup;
	// the token only selects which attempt is drawn.
	if _, err := w.Write(png); err != nil { //nolint:gosec // G705: see comment above
		s.log.WarnContext(r.Context(), "writing the QR image", "error", err)
	}
}

func toAPIQrisStatus(v qris.PublicView) apigen.QrisStatus {
	out := apigen.QrisStatus{Status: apigen.QrisStatusStatus(v.Status)}
	if v.Status != qris.StatusNone {
		amount, expires := v.Amount, v.ExpiresAt
		out.Amount, out.ExpiresAt = &amount, &expires
	}
	return out
}
