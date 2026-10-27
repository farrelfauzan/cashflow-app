package gowa

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

var loginCode = messaging.Message{
	To:       "+6281234567890",
	Template: messaging.TemplateLoginCode,
	Params:   map[string]string{"code": "482913"},
}

func newClient(t *testing.T, h http.HandlerFunc, deviceID string) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, Username: "cashflow", Password: "secret", DeviceID: deviceID})
	require.NoError(t, err)
	return c
}

func TestSendTemplate(t *testing.T) {
	var got struct {
		method, path, user, pass, device, contentType string
		body                                          sendMessageRequest
	}
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.user, got.pass, _ = r.BasicAuth()
		got.device = r.Header.Get("X-Device-Id")
		got.contentType = r.Header.Get("Content-Type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got.body))
		_, _ = io.WriteString(w, `{"code":"SUCCESS","message":"Success","results":{"message_id":"3EB0ABC","status":"sent"}}`)
	}, "platform")

	receipt, err := c.SendTemplate(t.Context(), loginCode)
	require.NoError(t, err)
	require.Equal(t, "3EB0ABC", receipt.ProviderMessageID)

	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/send/message", got.path)
	require.Equal(t, "cashflow", got.user)
	require.Equal(t, "secret", got.pass)
	require.Equal(t, "platform", got.device)
	require.Equal(t, "application/json", got.contentType)
	require.Equal(t, "6281234567890", got.body.Phone, "GOWA wants digits without the +")
	require.Contains(t, got.body.Message, "*482913*")
}

func TestNoDeviceHeaderWhenNotConfigured(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, present := r.Header["X-Device-Id"]
		require.False(t, present)
		_, _ = io.WriteString(w, `{"code":"SUCCESS","results":{"message_id":"1"}}`)
	}, "")
	_, err := c.SendTemplate(t.Context(), loginCode)
	require.NoError(t, err)
}

func TestBasePathIsKept(t *testing.T) {
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"code":"SUCCESS","results":{"message_id":"1"}}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL + "/gowa", Username: "u", Password: "p"})
	require.NoError(t, err)
	_, err = c.SendTemplate(t.Context(), loginCode)
	require.NoError(t, err)
	require.Equal(t, "/gowa/send/message", path)
}

func TestErrorsMapToMessagingErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantIs  error
		wantMsg string
	}{
		{"WhatsApp time lock", 429, `{"code":"WA_REACHOUT_TIMELOCK","message":"error 463"}`, messaging.ErrRateLimited, ""},
		{"not a WhatsApp number", 400, `{"code":"INVALID_JID","message":"invalid"}`, messaging.ErrUndeliverable, ""},
		{"device logged out", 401, `{"code":"AUTHENTICATION_ERROR","message":"not logged in"}`, messaging.ErrNotConnected, ""},
		{"no device linked", 404, `{"code":"DEVICE_NOT_FOUND","message":"device not found"}`, messaging.ErrNotConnected, ""},
		{"wrong credentials", 401, `Unauthorized`, nil, "credentials"},
		{"server error", 500, `{"code":"INTERNAL_SERVER_ERROR","message":"boom"}`, nil, "send failed"},
		{"success without an id", 200, `{"code":"SUCCESS","results":{}}`, nil, "without a message id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}, "")
			_, err := c.SendTemplate(t.Context(), loginCode)
			require.Error(t, err)
			if tc.wantIs != nil {
				require.ErrorIs(t, err, tc.wantIs)
			}
			if tc.wantMsg != "" {
				require.ErrorContains(t, err, tc.wantMsg)
			}
			require.NotContains(t, err.Error(), "482913", "errors must never carry the login code")
		})
	}
}

func TestBadMessagesNeverReachTheServer(t *testing.T) {
	var calls atomic.Int32
	c := newClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) }, "")

	for name, m := range map[string]messaging.Message{
		"unknown template":   {To: "+6281234567890", Template: "nope"},
		"phone without plus": {To: "6281234567890", Template: messaging.TemplateLoginCode, Params: map[string]string{"code": "1"}},
		"button without url": {To: "+6281234567890", Template: messaging.TemplateLoginCode, Params: map[string]string{"code": "1"}, Buttons: []messaging.Button{{Title: "Bayar"}}},
	} {
		_, err := c.SendTemplate(t.Context(), m)
		require.Error(t, err, name)
	}
	require.Zero(t, calls.Load())
}

func TestNewValidatesConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"relative url":  {BaseURL: "gowa:3000", Username: "u", Password: "p"},
		"other scheme":  {BaseURL: "ftp://gowa", Username: "u", Password: "p"},
		"no password":   {BaseURL: "http://gowa:3000", Username: "u"},
		"no credential": {BaseURL: "http://gowa:3000"},
	} {
		_, err := New(cfg)
		require.Error(t, err, name)
	}
}
