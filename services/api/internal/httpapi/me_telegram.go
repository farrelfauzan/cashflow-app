package httpapi

import (
	"net/http"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink/linkcode"
)

// The signed-in staff member's Telegram link (decision 42, TG-4: demo only), so their login codes
// can arrive on Telegram. These routes act on the user's own account, which belongs to no
// organization, so no RBAC action guards them: the session is the access, as for getMe. The POST
// and DELETE pass the origin check like every state-changing route.

// telegramForStaff reports whether this environment offers a staff member a Telegram link: it has
// a bot, the staff links are wired, and login codes go to linked chats (TELEGRAM_MESSAGES=linked).
// A link made where codes still go to WhatsApp would promise something that does not happen.
func (s *Server) telegramForStaff() bool {
	return s.LoginCodesViaTelegram && s.TelegramStaff != nil && s.TelegramBotUsername != ""
}

// getMyTelegram is the "Hubungkan Telegram" card's state. Without a bot, or with
// TELEGRAM_MESSAGES off, it answers {available: false, linked: false}, never 404, and reads nothing.
func (s *Server) getMyTelegram(w http.ResponseWriter, r *http.Request) {
	id, ok := identityFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	if !s.telegramForStaff() {
		s.writeJSON(w, http.StatusOK, apigen.MyTelegram{Available: false, Linked: false})
		return
	}
	linked, err := s.TelegramStaff.Linked(r.Context(), id.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, apigen.MyTelegram{Available: true, Linked: linked})
}

// linkMyTelegram answers a deep link to the bot carrying a fresh one-time code for the signed-in
// staff member. Neither the code nor the link is logged.
func (s *Server) linkMyTelegram(w http.ResponseWriter, r *http.Request) {
	id, ok := identityFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	if !s.telegramForStaff() {
		s.writeError(w, http.StatusNotFound, "telegram_unavailable", "Telegram belum tersedia.")
		return
	}
	code, _, err := s.TelegramStaff.NewLinkCode(r.Context(), id.UserID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, apigen.TelegramDeepLink{DeepLink: linkcode.DeepLink(s.TelegramBotUsername, code)})
}

// unlinkMyTelegram ends the signed-in staff member's live link, so their login codes go to
// WhatsApp again. It answers 204 whether or not there was one, and also without a bot, when it
// still ends a link left from when the environment had one.
func (s *Server) unlinkMyTelegram(w http.ResponseWriter, r *http.Request) {
	id, ok := identityFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	if s.TelegramStaff != nil {
		if err := s.TelegramStaff.Unlink(r.Context(), id.UserID); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}
