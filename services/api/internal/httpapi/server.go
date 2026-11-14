// Package httpapi is the HTTP surface of the API: routing, middleware, health checks and the
// handlers that translate between JSON and the domain services. Request and response types
// come from api/openapi.yaml through the generated apigen package.
package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenantimport"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

const (
	maxBodyBytes   = 1 << 20 // 1 MiB; routes that need more set their own limit
	requestTimeout = 25 * time.Second
	readyTimeout   = 2 * time.Second
)

// Pinger reports whether a dependency answers. The store satisfies it.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the services the handlers call.
type Deps struct {
	Log        *slog.Logger
	DB         Pinger
	Auth       *auth.Service
	Authz      *authz.Service
	Members    *member.Service
	Properties *property.Service
	Tenants    *tenant.Service
	Imports    *tenantimport.Service
	// Billing records payments and corrections and reads a tenant's money account.
	Billing *billing.Service
	// PayLinks mints and revokes tenant payment links and reads the public payment page.
	PayLinks *paylink.Service
	// QRIS makes, reads and checks the tenant's QRIS attempts behind a payment link.
	QRIS *qris.Service
	// Promises makes and lists tenants' payment promises, from the payment page and for members.
	Promises *promise.Service
	// Uploads starts, completes and reads staff photo uploads (S6-1). Nil when this server has no
	// file storage (development without STORAGE_*): the upload routes then answer 503
	// uploads_unavailable.
	Uploads *upload.Service
	// Webhooks handles AutoGoPay's callbacks.
	Webhooks *qris.Webhook
	// TelegramCodes makes the payment page's one-time Telegram deep links (decision 42, demo
	// only). Nil when this environment has no bot: the page then says telegram.available false and
	// linkPaymentPageTelegram answers telegram_unavailable.
	TelegramCodes *telegramlink.Codes
	// TelegramUpdates handles the bot's updates from /webhooks/telegram. The route exists only when
	// it and TelegramWebhookSecret are both set.
	TelegramUpdates telegram.UpdateHandler
	// TelegramWebhookSecret is TELEGRAM_WEBHOOK_SECRET, which Telegram sends back in the
	// X-Telegram-Bot-Api-Secret-Token header. A secret: never log it.
	TelegramWebhookSecret string
	// TelegramStaff reads, makes and ends the signed-in staff member's Telegram link
	// (/api/me/telegram, decision 42). Nil in tests that do not need it: /api/me/telegram then
	// says Telegram is unavailable.
	TelegramStaff *auth.TelegramStaff
	// TelegramBotUsername is the bot's username (TELEGRAM_BOT_USERNAME) when this environment has
	// a bot, and empty otherwise: then /api/me/telegram says Telegram is unavailable.
	TelegramBotUsername string
	// LoginCodesViaTelegram is true when login codes go to staff members' linked Telegram chats
	// (a bot and TELEGRAM_MESSAGES=linked). It is requestLoginCode's telegramAvailable: an
	// environment setting, the same for every request.
	LoginCodesViaTelegram bool
	// PublicLimiter rate-limits the public payment page per token and per client address.
	PublicLimiter ratelimit.Limiter
	// CookieSecure marks the session cookie Secure. Only local development turns it off.
	CookieSecure bool
	// AllowedOrigins are the browser origins allowed to send state-changing requests.
	AllowedOrigins []string
	// DocsSpecPath, when set, serves Swagger UI at /docs for that OpenAPI file. Development only.
	DocsSpecPath string
}

// Server holds the router and its dependencies.
type Server struct {
	Deps
	log     *slog.Logger
	handler http.Handler
}

// New builds the router.
func New(d Deps) *Server {
	s := &Server{Deps: d, log: d.Log}

	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.requestLogger, s.recoverer, limitBody(maxBodyBytes))
	r.NotFound(s.notFound)
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Metode tidak diizinkan.")
	})

	r.Get("/healthz", s.healthz)
	r.Get("/readyz", s.readyz)
	if d.DocsSpecPath != "" {
		r.Get("/docs", s.docsPage)
		r.Get("/docs/openapi.yaml", s.docsSpec)
	}

	// AutoGoPay's callbacks: no session and no origin check (a server calls them); the webhook id
	// in the path finds the organization and its key checks the signature.
	r.Post("/webhooks/autogopay/{webhookID}", s.autogopayWebhook)
	// The Telegram bot's updates (decision 42, demo only): the secret header proves Telegram sent
	// them. Without a bot or a secret the route does not exist (404).
	if d.TelegramUpdates != nil && d.TelegramWebhookSecret != "" {
		r.Post("/webhooks/telegram", s.telegramWebhook)
	}

	r.Route("/api", func(r chi.Router) {
		r.NotFound(s.notFound)
		r.Use(s.checkOrigin)
		r.Post("/auth/otp/request", s.requestLoginCode)
		r.Post("/auth/otp/verify", s.verifyLoginCode)
		// The tenant payment page: public, no session; the token in the path is the access.
		r.With(publicPageHeaders).Get("/t/{token}", s.getPaymentPage)
		r.With(publicPageHeaders).Get("/t/{token}/preview", s.getPaymentPreview)
		r.With(publicPageHeaders).Get("/t/{token}/status", s.getPaymentStatus)
		r.With(publicPageHeaders).Get("/t/{token}/qris.png", s.getPaymentQRImage)
		r.With(publicPageHeaders).Post("/t/{token}/qris", s.createPaymentQris)
		r.With(publicPageHeaders).Post("/t/{token}/check", s.checkPaymentQris)
		r.With(publicPageHeaders).Post("/t/{token}/promise", s.makePaymentPromise)
		r.With(publicPageHeaders).Post("/t/{token}/telegram", s.linkPaymentPageTelegram)

		r.Group(func(r chi.Router) {
			r.Use(s.requireSession)
			r.Post("/auth/logout", s.logout)
			r.Get("/me", s.getMe)
			r.Get("/me/telegram", s.getMyTelegram)
			r.Post("/me/telegram", s.linkMyTelegram)
			r.Delete("/me/telegram", s.unlinkMyTelegram)

			r.Route("/orgs/{orgID}", func(r chi.Router) {
				r.Use(s.requireMember)
				r.Get("/me", s.getOrgMe)
				r.Get("/collector-candidates", s.listCollectorCandidates)
				r.Get("/members", s.listMembers)
				r.Post("/members", s.inviteMember)
				r.Put("/members/{memberID}/roles", s.setMemberRoles)
				r.Put("/members/{memberID}/status", s.setMemberStatus)
				r.Post("/members/{memberID}/invitation", s.resendMemberInvitation)
				r.Get("/properties", s.listProperties)
				r.Post("/properties", s.createProperty)
				r.Get("/properties/{propertyID}", s.getProperty)
				r.Put("/properties/{propertyID}", s.updateProperty)
				r.Get("/properties/{propertyID}/units", s.listUnits)
				r.Post("/properties/{propertyID}/units", s.createUnit)
				r.Put("/units/{unitID}", s.updateUnit)
				r.Get("/units/{unitID}/prices", s.listUnitPrices)
				r.Post("/units/{unitID}/prices", s.createUnitPrice)
				r.Get("/tenants", s.listTenants)
				r.Post("/tenants", s.createTenant)
				r.Get("/tenants/{tenantID}", s.getTenant)
				r.Put("/tenants/{tenantID}", s.updateTenant)
				r.Get("/tenants/{tenantID}/account", s.getTenantAccount)
				r.Get("/tenants/{tenantID}/open-invoices", s.listTenantOpenInvoices)
				r.Get("/tenants/{tenantID}/promises", s.listTenantPromises)
				r.Post("/tenants/{tenantID}/promises", s.makeTenantPromise)
				r.Get("/dashboard", s.getDashboard)
				r.Get("/invoices", s.listInvoices)
				r.Post("/payments", s.recordPayment)
				r.Post("/corrections", s.createCorrection)
				r.Post("/invoices/{invoiceID}/payment-links", s.createPaymentLink)
				r.Post("/invoices/{invoiceID}/payment-links/revoke", s.revokePaymentLinks)
				r.Post("/leases", s.createLease)
				r.Put("/leases/{leaseID}", s.updateLease)
				r.Put("/leases/{leaseID}/planned-end", s.setLeasePlannedEnd)
				r.Post("/leases/{leaseID}/move-out", s.moveOut)
				r.Put("/leases/{leaseID}/charges", s.setLeaseCharges)
				r.Get("/leases/{leaseID}/metered-charges", s.listLeaseMeteredCharges)
				r.Put("/leases/{leaseID}/metered-charges/{chargeTypeID}/{usageMonth}", s.recordMeteredCharge)
				r.Get("/metered-charges/missing", s.listMissingMeterReadings)
				r.Get("/charge-types", s.listChargeTypes)
				r.Post("/charge-types", s.createChargeType)
				r.Post("/uploads", s.createUpload)
				r.Get("/uploads/{uploadID}", s.getUpload)
				r.Post("/uploads/{uploadID}/complete", s.completeUpload)
				r.Post("/imports/tenants", s.importTenants)
				r.Get("/imports/tenants/template", s.tenantImportTemplate)
			})
		})
	})

	s.handler = r
	return s
}

// Handler returns the router, for tests and for Run.
func (s *Server) Handler() http.Handler { return s.handler }

// Run serves until ctx is cancelled, then drains in-flight requests before returning.
func (s *Server) Run(ctx context.Context, addr string) error {
	timeoutBody := `{"error":{"code":"timeout","message":"Permintaan terlalu lama. Coba lagi."}}`
	srv := &http.Server{
		Addr:              addr,
		Handler:           http.TimeoutHandler(s.handler, requestTimeout, timeoutBody),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelError),
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServe() }()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serving http: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down http server: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serving http: %w", err)
	}
	return nil
}

// healthz answers while the process is alive. It never touches a dependency.
func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz answers only when the database does. The detail goes to the log, never to the caller.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		s.log.ErrorContext(r.Context(), "readiness check failed", "error", err)
		s.writeError(w, http.StatusServiceUnavailable, "not_ready", "Layanan belum siap.")
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) notFound(w http.ResponseWriter, _ *http.Request) {
	s.writeError(w, http.StatusNotFound, "not_found", "Tidak ditemukan.")
}

// requestLogger logs one line per request. It logs the route pattern, not the URL, because
// payment-page URLs carry a secret token.
func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := middleware.GetReqID(r.Context())
		w.Header().Set("X-Request-Id", reqID)

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		route := "unmatched"
		if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" {
			route = rc.RoutePattern()
		}
		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		s.log.LogAttrs(r.Context(), slog.LevelInfo, "request",
			slog.String("request_id", reqID),
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// recoverer turns a panic in a handler into a logged 500 with a generic body.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(rec) // net/http uses this panic on purpose to abort a response
			}
			s.log.ErrorContext(r.Context(), "panic in handler",
				"request_id", middleware.GetReqID(r.Context()),
				"panic", fmt.Sprint(rec),
				"stack", string(debug.Stack()),
			)
			s.writeError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan. Coba lagi.")
		}()
		next.ServeHTTP(w, r)
	})
}

func limitBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
