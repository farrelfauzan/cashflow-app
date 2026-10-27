package httpapi

import (
	"context"
	"net/http"
	"slices"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
)

const sessionCookie = "cf_session"

type ctxKey int

const (
	identityKey ctxKey = iota + 1
	principalKey
)

func identityFrom(ctx context.Context) (auth.Identity, bool) {
	id, ok := ctx.Value(identityKey).(auth.Identity)
	return id, ok
}

func principalFrom(ctx context.Context) (authz.Principal, bool) {
	p, ok := ctx.Value(principalKey).(authz.Principal)
	return p, ok
}

// requireSession lets the request through only with a valid session cookie.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(sessionCookie)
		if err != nil {
			s.fail(w, r, auth.ErrUnauthenticated)
			return
		}
		id, err := s.Auth.Authenticate(r.Context(), c.Value)
		if err != nil {
			s.clearSessionCookie(w)
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey, id)))
	})
}

// requireMember loads the signed-in user's principal for the organization in the URL. A user
// who is not a member gets 404, so the response does not confirm the organization exists.
func (s *Server) requireMember(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := identityFrom(r.Context())
		if !ok {
			s.fail(w, r, auth.ErrUnauthenticated)
			return
		}
		orgID, err := uuid.Parse(chi.URLParam(r, "orgID"))
		if err != nil {
			s.notFound(w, r)
			return
		}
		p, err := s.Authz.LoadPrincipal(r.Context(), orgID, id.UserID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

// checkOrigin rejects state-changing requests that a browser sent from another site. Together
// with SameSite=Lax on the session cookie this blocks cross-site request forgery. Requests
// without an Origin header come from non-browser clients and pass.
func (s *Server) checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(s.AllowedOrigins, origin) {
				s.writeError(w, http.StatusForbidden, "bad_origin", "Permintaan ditolak.")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Secure comes from configuration and is false only in local development, which runs over
// plain HTTP; config.API.CookieSecure turns it on everywhere else. HttpOnly and SameSite are
// always set. That is why gosec's G124 is silenced on the two cookies below.

func (s *Server) setSessionCookie(w http.ResponseWriter, sess auth.Session) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: see comment above
		Name:     sessionCookie,
		Value:    sess.Token,
		Path:     "/",
		Expires:  sess.ExpiresAt,
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // G124: see comment above setSessionCookie
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}
