package autogopaytest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// Server serves a Fake over HTTP the way AutoGoPay's gateway does, so the real autogopay.Client
// can talk to it. The API and the worker run as separate processes, so in development and in the
// web's end-to-end tests they both point AUTOGOPAY_BASE_URL at one Server (cmd/fakeautogopay)
// and see the same QRs.
//
// It answers POST /qris/generate, /qris/status and /qris/cancel with the shapes AutoGoPay sends
// (internal/autogopay/testdata): every answer is HTTP 200 with {success, message, data}, a paid QR
// reads "settlement", an unknown id reads an empty status, and a missing key is HTTP 401.
//
// Under /_control it lets a test act like a tenant or an outage:
//
//	GET  /_control/transactions                          every QR, newest first
//	POST /_control/pay        {"transactionId"?}         the QR is paid (default: newest pending)
//	POST /_control/expire     {"transactionId"?}         the QR expires now (default: newest pending)
//	POST /_control/fail-next  {"method", "amount"?}      the next call of method answers HTTP 503;
//	                                                     with amount, only a generate for it
//	POST /_control/reset                                 forget every QR and queued failure
//
// The control routes have no authentication: the Server is for local runs only.
type Server struct {
	fake *Fake
	mux  *http.ServeMux

	mu    sync.Mutex
	fails []failRule
}

type failRule struct {
	method Method
	amount int64 // 0: any
}

// NewServer serves f.
func NewServer(f *Fake) *Server {
	s := &Server{fake: f, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /qris/generate", s.generate)
	s.mux.HandleFunc("POST /qris/status", s.status)
	s.mux.HandleFunc("POST /qris/cancel", s.cancel)
	s.mux.HandleFunc("GET /_control/transactions", s.listTransactions)
	s.mux.HandleFunc("POST /_control/pay", s.setStatus(f.Pay))
	s.mux.HandleFunc("POST /_control/expire", s.setStatus(f.Expire))
	s.mux.HandleFunc("POST /_control/fail-next", s.failNext)
	s.mux.HandleFunc("POST /_control/reset", s.reset)
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	s.mux.ServeHTTP(w, r)
}

// timeLayout is how AutoGoPay writes times: no zone, in WIB.
const timeLayout = "2006-01-02 15:04:05"

func wib(t time.Time) string { return t.In(clock.Jakarta).Format(timeLayout) }

type envelope struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) //nolint:errcheck // a local test server; nothing to do on a broken connection
}

// apiKey reads the Bearer key, or answers 401 as AutoGoPay does.
func apiKey(w http.ResponseWriter, r *http.Request) (secret.Value, bool) {
	key, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || strings.TrimSpace(key) == "" {
		writeJSON(w, http.StatusUnauthorized, envelope{Message: "API key is required"})
		return secret.Value{}, false
	}
	return secret.NewValue(key), true
}

// failing reports whether a queued failure matches this call, and consumes it.
func (s *Server) failing(m Method, amount int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, f := range s.fails {
		if f.method == m && (f.amount == 0 || f.amount == amount) {
			s.fails = append(s.fails[:i], s.fails[i+1:]...)
			return true
		}
	}
	return false
}

func unavailable(w http.ResponseWriter) {
	writeJSON(w, http.StatusServiceUnavailable, envelope{Message: "Service temporarily unavailable"})
}

func (s *Server) generate(w http.ResponseWriter, r *http.Request) {
	key, ok := apiKey(w, r)
	if !ok {
		return
	}
	var req struct {
		Amount int64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Message: "Invalid request body"})
		return
	}
	if s.failing(MethodGenerate, req.Amount) {
		unavailable(w)
		return
	}
	q, err := s.fake.Generate(r.Context(), key, req.Amount)
	if err != nil {
		writeJSON(w, http.StatusOK, envelope{Message: "Amount must be between 1 and 10000000"})
		return
	}
	writeJSON(w, http.StatusOK, envelope{Success: true, Message: "QRIS created successfully", Data: map[string]any{
		"transaction_id":     q.TransactionID,
		"order_id":           q.OrderID,
		"amount":             q.Amount,
		"transaction_status": "pending",
		"qr_string":          q.QRString,
		"transaction_time":   wib(q.CreatedAt),
		"expiry_time":        wib(q.ExpiresAt),
	}})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.transaction(w, r, MethodStatus, s.fake.Status)
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	s.transaction(w, r, MethodCancel, s.fake.Cancel)
}

func (s *Server) transaction(w http.ResponseWriter, r *http.Request, m Method,
	call func(ctx context.Context, key secret.Value, id string) (autogopay.Transaction, error)) {
	key, ok := apiKey(w, r)
	if !ok {
		return
	}
	var req struct {
		TransactionID string `json:"transaction_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, envelope{Message: "Invalid request body"})
		return
	}
	if s.failing(m, 0) {
		unavailable(w)
		return
	}
	tx, err := call(r.Context(), key, req.TransactionID)
	if err != nil {
		// AutoGoPay answers an unknown id with HTTP 200 and an empty status.
		writeJSON(w, http.StatusOK, envelope{Message: "Transaction status: ", Data: map[string]string{
			"transaction_id": "", "transaction_status": "", "transaction_time": "",
		}})
		return
	}
	switch tx.Status {
	case autogopay.StatusPaid:
		// The real answer for a paid QR (2026-11-06) holds only these two fields.
		writeJSON(w, http.StatusOK, envelope{Success: true, Message: "Transaction successful", Data: map[string]string{
			"transaction_status": "settlement", "transaction_time": wib(tx.Time),
		}})
	case autogopay.StatusCancelled:
		writeJSON(w, http.StatusOK, envelope{Success: m == MethodCancel, Message: cancelMessage(m), Data: map[string]string{
			"transaction_id": tx.ID, "transaction_status": "cancel", "transaction_time": wib(tx.Time),
		}})
	case autogopay.StatusExpired:
		writeJSON(w, http.StatusOK, envelope{Message: "Transaction expire", Data: map[string]string{
			"transaction_id": tx.ID, "transaction_status": "expire", "transaction_time": wib(tx.Time),
		}})
	default:
		writeJSON(w, http.StatusOK, envelope{Message: "Transaction pending", Data: map[string]string{
			"transaction_id": tx.ID, "transaction_status": "pending", "transaction_time": wib(tx.Time),
		}})
	}
}

func cancelMessage(m Method) string {
	if m == MethodCancel {
		return "Transaction cancelled successfully"
	}
	return "Transaction cancelled"
}

// controlTransaction is a QR as the control API shows it.
type controlTransaction struct {
	TransactionID string    `json:"transactionId"`
	Amount        int64     `json:"amount"`
	Status        string    `json:"status"` // pending, paid, expired or cancelled
	CreatedAt     time.Time `json:"createdAt"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

func toControl(t Transaction) controlTransaction {
	return controlTransaction{
		TransactionID: t.ID, Amount: t.Amount, Status: string(t.Status), CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt,
	}
}

type controlError struct {
	Error string `json:"error"`
}

func (s *Server) listTransactions(w http.ResponseWriter, _ *http.Request) {
	all := s.fake.Transactions()
	out := make([]controlTransaction, len(all))
	for i, t := range all {
		out[i] = toControl(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"transactions": out})
}

// setStatus answers a pay or expire: on the named QR, or the newest pending one.
func (s *Server) setStatus(set func(id string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TransactionID string `json:"transactionId"`
		}
		if err := decodeOptional(r.Body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, controlError{"body must be {\"transactionId\"?: string}"})
			return
		}
		id := req.TransactionID
		if id == "" {
			for _, t := range s.fake.Transactions() {
				if t.Status == autogopay.StatusPending {
					id = t.ID
					break
				}
			}
		}
		if id == "" || set(id) != nil {
			writeJSON(w, http.StatusNotFound, controlError{"no such transaction"})
			return
		}
		for _, t := range s.fake.Transactions() {
			if t.ID == id {
				writeJSON(w, http.StatusOK, toControl(t))
				return
			}
		}
		writeJSON(w, http.StatusNotFound, controlError{"no such transaction"})
	}
}

func (s *Server) failNext(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Method Method `json:"method"`
		Amount int64  `json:"amount"`
	}
	if err := decodeOptional(r.Body, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, controlError{"body must be {\"method\": string, \"amount\"?: number}"})
		return
	}
	switch {
	case req.Method != MethodGenerate && req.Method != MethodStatus && req.Method != MethodCancel:
		writeJSON(w, http.StatusBadRequest, controlError{"method must be generate, status or cancel"})
		return
	case req.Amount < 0, req.Amount != 0 && req.Method != MethodGenerate:
		writeJSON(w, http.StatusBadRequest, controlError{"amount is a positive number, for generate only"})
		return
	}
	s.mu.Lock()
	s.fails = append(s.fails, failRule{method: req.Method, amount: req.Amount})
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reset(w http.ResponseWriter, _ *http.Request) {
	s.fake.Reset()
	s.mu.Lock()
	s.fails = nil
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// decodeOptional reads a JSON object into dst; an empty body leaves dst as it is.
func decodeOptional(body io.Reader, dst any) error {
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
