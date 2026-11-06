package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
)

// Limits on the public payment page, per fixed one-minute window. The per-token limit stops one
// link from being hammered; the per-address limit stops one client from guessing tokens.
const (
	paymentPageWindow   = time.Minute
	paymentPagePerToken = 30
	paymentPagePerIP    = 120
)

// publicPageHeaders marks every answer under /api/t/ as never to be cached and never to send
// its URL, which holds the token, as a referrer. Caddy sets the same Referrer-Policy on the web
// app's /t/ pages.
func publicPageHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// getPaymentPage is the public tenant payment page. It has no session: the token in the path is
// the access. The token is never logged: the request logger records the route pattern, and no
// error here carries it.
func (s *Server) getPaymentPage(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	page, err := s.PayLinks.PublicPage(r.Context(), token)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIPaymentPage(page))
}

// allowPaymentPage counts the request against the client's address and the token, before
// anything looks the token up, and answers 429 when either is over its limit. When the limiter
// cannot be reached the request is refused with a 500 rather than let through unlimited.
func (s *Server) allowPaymentPage(w http.ResponseWriter, r *http.Request, token string) bool {
	limits := []struct {
		key   string
		limit int
	}{
		{"paylink:ip:" + clientIP(r), paymentPagePerIP},
		{paylink.LimitKey(token), paymentPagePerToken},
	}
	for _, l := range limits {
		ok, err := s.PublicLimiter.Allow(r.Context(), l.key, l.limit, paymentPageWindow)
		if err != nil {
			s.fail(w, r, fmt.Errorf("checking the payment page rate limit: %w", err))
			return false
		}
		if !ok {
			s.writeError(w, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak permintaan. Coba lagi sebentar lagi.")
			return false
		}
	}
	return true
}

func (s *Server) createPaymentLink(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "invoiceID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	link, err := s.PayLinks.Mint(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, apigen.PaymentLink{Url: link.URL})
}

func (s *Server) revokePaymentLinks(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "invoiceID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	n, err := s.PayLinks.RevokeAll(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, apigen.PaymentLinksRevoked{Revoked: n})
}

func toAPIPaymentPage(p paylink.Page) apigen.PaymentPage {
	out := apigen.PaymentPage{
		OrganizationName: p.OrganizationName,
		TenantName:       p.TenantName,
		Invoice:          toAPIPaymentPageInvoice(p.Invoice),
		Outstanding:      p.Outstanding,
		Credit:           p.Credit,
		Paid:             p.Paid,
		OpenInvoices:     make([]apigen.PaymentPageInvoice, len(p.OpenInvoices)),
		QrisAvailable:    p.QRISAvailable,
	}
	for i, inv := range p.OpenInvoices {
		out.OpenInvoices[i] = toAPIPaymentPageInvoice(inv)
	}
	if b := p.BankAccount; b != nil {
		out.BankAccount = &apigen.BankAccount{BankName: b.BankName, AccountNumber: b.AccountNumber, AccountHolder: b.AccountHolder}
	}
	promisePageFields(&out, p.Promise)
	return out
}

func toAPIPaymentPageInvoice(inv paylink.PageInvoice) apigen.PaymentPageInvoice {
	lines := make([]apigen.PaymentPageLine, len(inv.Lines))
	for i, l := range inv.Lines {
		lines[i] = apigen.PaymentPageLine{Label: l.Label, Amount: l.Amount}
	}
	return apigen.PaymentPageInvoice{
		Id: inv.ID, Kind: apigen.InvoiceKind(inv.Kind), UnitLabel: inv.UnitLabel, PropertyName: inv.PropertyName,
		Period: date(inv.Period), PeriodEnd: date(inv.PeriodEnd), DueDate: date(inv.DueDate),
		Amount: inv.Amount, Balance: inv.Balance, Lines: lines,
	}
}
