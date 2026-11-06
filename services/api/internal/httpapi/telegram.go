package httpapi

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
)

const (
	// telegramSecretHeader carries the secret_token given to setWebhook on every webhook call.
	telegramSecretHeader = "X-Telegram-Bot-Api-Secret-Token" //nolint:gosec // G101: a header name, not a credential
	// telegramWebhookMaxBytes bounds an update; a text message is a few hundred bytes.
	telegramWebhookMaxBytes = 64 << 10
)

// linkPaymentPageTelegram is "Hubungkan Telegram" on the payment page: it answers a deep link to
// the bot carrying a fresh one-time code (decision 42). Public like getPaymentPage, with the same
// rate limits counted before the token is looked up. When this environment has no bot it answers
// telegram_unavailable without looking at the token. Neither the token nor the code is logged.
func (s *Server) linkPaymentPageTelegram(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	if s.TelegramCodes == nil {
		s.writeError(w, http.StatusNotFound, "telegram_unavailable", "Telegram belum tersedia.")
		return
	}
	link, err := s.TelegramCodes.DeepLink(r.Context(), token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, apigen.TelegramDeepLink{DeepLink: link})
}

// telegramWebhook receives the bot's updates (staging; development polls instead). It is routed
// only when the bot and TELEGRAM_WEBHOOK_SECRET are both configured. The secret header is
// compared in constant time before the body is read, so a caller without it learns nothing and
// costs nothing: a wrong or missing secret is a bare 401. Once verified, the answer is always an
// empty 200, even for a body that cannot be read or a handler that failed (both logged), because
// Telegram resends an update until it gets a 2xx and resending would not help. An empty body
// matters too: Telegram would run a JSON answer as a Bot API call.
func (s *Server) telegramWebhook(w http.ResponseWriter, r *http.Request) {
	if !s.telegramSecretOK(r.Header.Get(telegramSecretHeader)) {
		s.log.WarnContext(r.Context(), "telegram webhook refused: wrong or missing secret")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, telegramWebhookMaxBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		s.log.WarnContext(r.Context(), "telegram webhook body not read", "too_large", errors.As(err, &tooLarge))
		w.WriteHeader(http.StatusOK)
		return
	}
	var u telegram.Update
	if err := json.Unmarshal(body, &u); err != nil {
		// The error can quote the body, so only that it failed is logged.
		s.log.WarnContext(r.Context(), "telegram webhook body is not an update")
		w.WriteHeader(http.StatusOK)
		return
	}
	if err := s.TelegramUpdates.HandleUpdate(r.Context(), u); err != nil {
		s.log.ErrorContext(r.Context(), "telegram update not handled", "update", u, "error", err)
	}
	w.WriteHeader(http.StatusOK)
}

// telegramSecretOK compares the header with the configured secret in constant time. Both sides
// are hashed first, so the comparison does not end early on a length difference either.
func (s *Server) telegramSecretOK(got string) bool {
	if got == "" || s.TelegramWebhookSecret == "" {
		return false
	}
	a, b := sha256.Sum256([]byte(got)), sha256.Sum256([]byte(s.TelegramWebhookSecret))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
