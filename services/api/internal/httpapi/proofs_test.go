package httpapi

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsPaymentProof(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		method string
		target string
		want   bool
	}{
		{name: "a tenant sending a proof", method: http.MethodPost, target: "/api/t/AbC_d-123/proof", want: true},
		{name: "with a query string", method: http.MethodPost, target: "/api/t/AbC_d-123/proof?x=1", want: true},
		{name: "GET of the same path", method: http.MethodGet, target: "/api/t/AbC_d-123/proof"},
		{name: "PUT of the same path", method: http.MethodPut, target: "/api/t/AbC_d-123/proof"},
		{name: "another payment page route", method: http.MethodPost, target: "/api/t/AbC_d-123/promise"},
		{name: "the payment page itself", method: http.MethodPost, target: "/api/t/AbC_d-123"},
		{name: "a path below proof", method: http.MethodPost, target: "/api/t/x/proof/extra"},
		{name: "a trailing slash", method: http.MethodPost, target: "/api/t/x/proof/"},
		{name: "no token", method: http.MethodPost, target: "/api/t//proof"},
		{name: "a token with a slash", method: http.MethodPost, target: "/api/t/a/b/proof"},
		{name: "a staff route ending in proof", method: http.MethodPost, target: "/api/orgs/0d6e1f4a/t/x/proof"},
		{name: "outside /api", method: http.MethodPost, target: "/t/x/proof"},
		{name: "a staff upload", method: http.MethodPost, target: "/api/orgs/0d6e1f4a/uploads"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(t.Context(), tc.method, tc.target, nil)
			require.Equal(t, tc.want, isPaymentProof(r))
		})
	}
}

// TestLimitBodySkipsOnlyPaymentProofs: every body is capped at the limit except a transfer
// proof's, which sendPaymentProof caps itself.
func TestLimitBodySkipsOnlyPaymentProofs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		target     string
		wantCapped bool
	}{
		{name: "a transfer proof", target: "/api/t/x/proof"},
		{name: "a promise", target: "/api/t/x/promise", wantCapped: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var capped bool
			h := limitBody(8)(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				buf := make([]byte, 64)
				n := 0
				for {
					m, err := r.Body.Read(buf[n:])
					n += m
					if err != nil {
						var tooLarge *http.MaxBytesError
						capped = errors.As(err, &tooLarge)
						return
					}
				}
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, tc.target, bytes.NewReader(make([]byte, 32)))
			h.ServeHTTP(httptest.NewRecorder(), req)
			require.Equal(t, tc.wantCapped, capped)
		})
	}
}

// TestServerHandlerGivesProofsMoreTime: a transfer proof reaches the router directly with a
// context of proofTimeout; every other request goes through the timeout handler.
func TestServerHandlerGivesProofsMoreTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		target    string
		wantRoute string
	}{
		{name: "a transfer proof", target: "/api/t/x/proof", wantRoute: "router"},
		{name: "a promise", target: "/api/t/x/promise", wantRoute: "timeout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var route string
			var deadline time.Time
			router := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				route = "router"
				deadline, _ = r.Context().Deadline()
				w.WriteHeader(http.StatusNoContent)
			})
			timeout := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				route = "timeout"
				w.WriteHeader(http.StatusNoContent)
			})
			s := &Server{log: slog.New(slog.DiscardHandler), handler: router}
			srv := httptest.NewServer(s.serverHandler(timeout))
			t.Cleanup(srv.Close)

			start := time.Now()
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL+tc.target, bytes.NewReader(nil))
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
			require.Equal(t, tc.wantRoute, route)
			if tc.wantRoute == "router" {
				require.WithinDuration(t, start.Add(proofTimeout), deadline, 5*time.Second)
			}
		})
	}
}
