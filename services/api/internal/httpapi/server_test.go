package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

// newTestServer builds a server without domain services. It covers routing, middleware and
// error handling; the integration tests cover the handlers end to end.
func newTestServer(t *testing.T, db Pinger) (*Server, *bytes.Buffer) {
	t.Helper()
	logs := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(logs, nil))
	return New(Deps{Log: log, DB: db, AllowedOrigins: []string{"https://app.example.id"}}), logs
}

func do(t *testing.T, s *Server, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), method, path, strings.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	return do(t, s, http.MethodGet, path, "", nil)
}

func TestHealthzIgnoresDependencies(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{err: errors.New("db down")})
	rec := get(t, s, "/healthz")
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReadyz(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database answers", nil, http.StatusOK, `{"status":"ready"}`},
		{"database down", errors.New("dial tcp 10.0.0.5:5432: connection refused"), http.StatusServiceUnavailable,
			`{"error":{"code":"not_ready","message":"Layanan belum siap."}}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, logs := newTestServer(t, fakePinger{err: tc.pingErr})
			rec := get(t, s, "/readyz")
			require.Equal(t, tc.wantStatus, rec.Code)
			require.JSONEq(t, tc.wantBody, rec.Body.String())
			if tc.pingErr != nil {
				require.NotContains(t, rec.Body.String(), "10.0.0.5", "internal detail must not reach the caller")
				require.Contains(t, logs.String(), "10.0.0.5", "internal detail must be logged")
			}
		})
	}
}

func TestResponsesCarryARequestID(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{})
	rec := get(t, s, "/healthz")
	require.NotEmpty(t, rec.Header().Get("X-Request-Id"))
}

func TestUnknownRoutesAreAGenericJSON404(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{})
	for _, path := range []string{"/nope", "/api/nope"} {
		rec := get(t, s, path)
		require.Equal(t, http.StatusNotFound, rec.Code, path)
		require.JSONEq(t, `{"error":{"code":"not_found","message":"Tidak ditemukan."}}`, rec.Body.String(), path)
	}
}

func TestRequestLogUsesTheRouteNotTheURL(t *testing.T) {
	s, logs := newTestServer(t, fakePinger{})
	get(t, s, "/healthz?token=secret-value")
	require.Contains(t, logs.String(), `"route":"/healthz"`)
	require.NotContains(t, logs.String(), "secret-value")
}

func TestPanicBecomesAGeneric500(t *testing.T) {
	s, logs := newTestServer(t, fakePinger{})
	h := s.recoverer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom with internal detail")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	require.NotContains(t, rec.Body.String(), "boom")
	require.Contains(t, logs.String(), "boom with internal detail")
}

func TestBodyLimitStopsLargeRequests(t *testing.T) {
	var readErr error
	h := limitBody(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", bytes.NewReader(make([]byte, 100)))
	h.ServeHTTP(httptest.NewRecorder(), req)
	require.Error(t, readErr)
}

func TestSignedInRoutesNeedASession(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{})
	for _, path := range []string{"/api/me", "/api/orgs/00000000-0000-0000-0000-000000000001/members"} {
		rec := get(t, s, path)
		require.Equal(t, http.StatusUnauthorized, rec.Code, path)
		require.Contains(t, rec.Body.String(), `"code":"unauthenticated"`, path)
	}
}

func TestCrossSiteWritesAreRejected(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{})
	rec := do(t, s, http.MethodPost, "/api/auth/otp/request", `{"phone":"08123456789"}`,
		map[string]string{"Origin": "https://evil.example.com"})
	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Contains(t, rec.Body.String(), `"code":"bad_origin"`)
}

func TestRequestBodiesAreStrict(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{})
	tests := map[string]string{
		"unknown field": `{"phone":"0812","extra":true}`,
		"not json":      `phone=0812`,
		"two objects":   `{"phone":"0812"}{"phone":"0813"}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := do(t, s, http.MethodPost, "/api/auth/otp/request", body,
				map[string]string{"Origin": "https://app.example.id"})
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.Contains(t, rec.Body.String(), `"code":"invalid_json"`)
		})
	}
}

func TestDocsAreServedOnlyWhenConfigured(t *testing.T) {
	s, _ := newTestServer(t, fakePinger{})
	require.Equal(t, http.StatusNotFound, get(t, s, "/docs").Code)

	spec := filepath.Join(t.TempDir(), "openapi.yaml")
	require.NoError(t, os.WriteFile(spec, []byte("openapi: 3.0.3\n"), 0o600))
	withDocs := New(Deps{Log: s.log, DB: fakePinger{}, DocsSpecPath: spec})

	page := get(t, withDocs, "/docs")
	require.Equal(t, http.StatusOK, page.Code)
	require.Contains(t, page.Body.String(), "SwaggerUIBundle")

	yaml := get(t, withDocs, "/docs/openapi.yaml")
	require.Equal(t, http.StatusOK, yaml.Code)
	require.Equal(t, "openapi: 3.0.3\n", yaml.Body.String())
}
