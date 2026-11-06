package httpapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

// webhookMaxBytes bounds an AutoGoPay callback body; a real one is a few hundred bytes.
const webhookMaxBytes = 64 << 10

// webhookAnswer is the only body the webhook sends: AutoGoPay's dashboard refuses to save a
// callback URL unless the answer is JSON with a success field.
type webhookAnswer struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// autogopayWebhook receives AutoGoPay's callbacks. It reads the raw body, since the signature is
// over the exact bytes, and hands it to qris.Webhook. An unknown webhook id, a bad signature and
// an unreadable signed body all get the same 401; a failure on our side gets a 500, so AutoGoPay
// may send the callback again. Nothing internal is ever in the answer, and the request log records
// the route pattern, not the webhook id.
func (s *Server) autogopayWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, webhookMaxBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeJSON(w, http.StatusRequestEntityTooLarge, webhookAnswer{Message: "payload too large"})
			return
		}
		s.writeJSON(w, http.StatusBadRequest, webhookAnswer{Message: "bad request"})
		return
	}
	err = s.Webhooks.Handle(r.Context(), chi.URLParam(r, "webhookID"), r.Header.Get(autogopay.SignatureHeader), body)
	switch {
	case err == nil:
		s.writeJSON(w, http.StatusOK, webhookAnswer{Success: true, Message: "ok"})
	case errors.Is(err, qris.ErrWebhookRefused):
		s.writeJSON(w, http.StatusUnauthorized, webhookAnswer{Message: "unauthorized"})
	default:
		s.log.ErrorContext(r.Context(), "handling an AutoGoPay callback", "error", err)
		s.writeJSON(w, http.StatusInternalServerError, webhookAnswer{Message: "error"})
	}
}
