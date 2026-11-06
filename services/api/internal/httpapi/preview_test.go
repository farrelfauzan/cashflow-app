package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

// allowAll is a limiter that never limits, for tests that stop before the link is looked up.
type allowAll struct{}

func (allowAll) Allow(context.Context, string, int, time.Duration) (bool, error) { return true, nil }

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Field   string `json:"field"`
	} `json:"error"`
}

// The preview's amount errors name field amount, carry their own code and never the error's
// internal text.
func TestPaymentPreviewErrorsMapWithFieldAmount(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t, fakePinger{})
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
	}{
		{
			name: "above outstanding", err: fmt.Errorf("paylink: boom: %w", billing.ErrAmountAboveOutstanding),
			wantCode: "amount_above_outstanding", wantMsg: "Nominal melebihi sisa tagihan.",
		},
		{
			name: "below Rp1", err: fmt.Errorf("paylink: boom: %w", billing.ErrPreviewAmountInvalid),
			wantCode: "invalid_input", wantMsg: previewAmountMessage,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			s.fail(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/t/x/preview", nil), tc.err)
			require.Equal(t, http.StatusBadRequest, w.Code)
			require.NotContains(t, w.Body.String(), "boom", "no internal text")
			var body errorBody
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tc.wantCode, body.Error.Code)
			require.Equal(t, tc.wantMsg, body.Error.Message)
			require.Equal(t, "amount", body.Error.Field)
		})
	}
}

// A bad amount is refused before the token is looked up, with the public page's headers.
func TestPaymentPreviewRefusesBadAmount(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t, fakePinger{})
	s.PublicLimiter = allowAll{}
	token := strings.Repeat("A", 43)
	tests := []struct {
		name  string
		query string
	}{
		{name: "missing", query: ""},
		{name: "empty", query: "?amount="},
		{name: "not a number", query: "?amount=abc"},
		{name: "a fraction", query: "?amount=1000.5"},
		{name: "zero", query: "?amount=0"},
		{name: "negative", query: "?amount=-1"},
		{name: "beyond int64", query: "?amount=99999999999999999999"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := get(t, s, "/api/t/"+token+"/preview"+tc.query)
			require.Equal(t, http.StatusBadRequest, rec.Code)
			var body errorBody
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "invalid_input", body.Error.Code)
			require.Equal(t, "amount", body.Error.Field)
			require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			require.Equal(t, "no-referrer", rec.Header().Get("Referrer-Policy"))
		})
	}
}
