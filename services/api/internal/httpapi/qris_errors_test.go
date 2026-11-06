package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
)

// Busy for a moment and down are different answers: the page may retry the first once, and
// offers bank transfer for the second.
func TestQRISOverloadedIsNotAnOutage(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t, fakePinger{})
	tests := []struct {
		name       string
		err        error
		wantCode   string
		retryAfter string
	}{
		{name: "too many waiting on AutoGoPay", err: qris.ErrOverloaded, wantCode: `"code":"qris_overloaded"`, retryAfter: "5"},
		{name: "AutoGoPay down", err: fmt.Errorf("%w: generate: boom", qris.ErrUnavailable), wantCode: `"code":"qris_unavailable"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			s.fail(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/t/x/qris", nil), tc.err)
			require.Equal(t, http.StatusServiceUnavailable, w.Code)
			require.Contains(t, w.Body.String(), tc.wantCode)
			require.NotContains(t, w.Body.String(), "boom", "no internal text")
			require.Equal(t, tc.retryAfter, w.Header().Get("Retry-After"))
		})
	}
}
