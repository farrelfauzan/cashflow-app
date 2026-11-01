package twilio_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/twilio"
)

// fakeHex makes placeholder SIDs. They are built at run time so no source line looks like a
// real Twilio identifier to secret scanning.
var fakeHex = strings.Repeat("0123456789abcdef", 2)

var (
	accountSID = "AC" + fakeHex
	messageSID = "SM" + fakeHex
	mediaSID   = "MM" + fakeHex
	loginHX    = "HX" + fakeHex
	inviteHX   = "HX" + strings.Repeat("fedcba9876543210", 2)
)

// Placeholder credentials and numbers: none of them is a real account or phone.
const (
	authToken = "test-auth-token" //nolint:gosec // G101: a test placeholder, not a credential
	sandbox   = "+14155238886"
	recipient = "+6281234567890"
	loginCode = "482913"
)

var loginMessage = messaging.Message{
	To:       recipient,
	Template: messaging.TemplateLoginCode,
	Params:   map[string]string{"code": loginCode},
}

// createdBody is a trimmed copy of Twilio's 201 response to a new WhatsApp message.
var createdBody = strings.NewReplacer("{account}", accountSID, "{message}", messageSID).Replace(
	`{"account_sid":"{account}","api_version":"2010-04-01",` +
		`"body":"Kode masuk Anda","date_created":"Sun, 01 Nov 2026 03:00:00 +0000","direction":"outbound-api",` +
		`"error_code":null,"error_message":null,"from":"whatsapp:+14155238886","num_segments":"1",` +
		`"sid":"{message}","status":"queued","to":"whatsapp:+6281234567890",` +
		`"uri":"/2010-04-01/Accounts/{account}/Messages/{message}.json"}`)

func newClient(t *testing.T, h http.HandlerFunc) *twilio.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := twilio.New(twilio.Config{AccountSID: accountSID, AuthToken: authToken, From: sandbox, BaseURL: srv.URL})
	require.NoError(t, err)
	return c
}

func TestSendTemplate(t *testing.T) {
	t.Parallel()
	var got struct {
		method, path, user, pass, contentType string
		form                                  url.Values
	}
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.user, got.pass, _ = r.BasicAuth()
		got.contentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		got.form = r.PostForm
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, createdBody)
	})

	receipt, err := c.SendTemplate(t.Context(), loginMessage)
	require.NoError(t, err)
	require.Equal(t, messageSID, receipt.ProviderMessageID)

	want, err := messaging.Render(loginMessage)
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, got.method)
	require.Equal(t, "/2010-04-01/Accounts/"+accountSID+"/Messages.json", got.path)
	require.Equal(t, accountSID, got.user)
	require.Equal(t, authToken, got.pass)
	require.Equal(t, "application/x-www-form-urlencoded", got.contentType)
	require.Equal(t, "whatsapp:"+recipient, got.form.Get("To"))
	require.Equal(t, "whatsapp:"+sandbox, got.form.Get("From"))
	require.Equal(t, want, got.form.Get("Body"))
	require.Len(t, got.form, 3, "only To, From and Body are sent")
}

func TestSendMemberInviteSendsTheLoginLinkAsText(t *testing.T) {
	t.Parallel()
	var body string
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			body = r.PostForm.Get("Body")
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"sid":"`+mediaSID+`","status":"queued","error_code":null}`)
	})

	receipt, err := c.SendTemplate(t.Context(), messaging.Message{
		To:       "+6281213139999",
		Template: messaging.TemplateMemberInvite,
		Params: map[string]string{
			"name": "Mbak Sari", "role": "Karyawan", "organization": "Griya Sejahtera", "inviter": "Pak Budi",
		},
		Buttons: []messaging.Button{{Title: "Masuk", Payload: "member_invite", URL: "https://app.example.id/login"}},
	})
	require.NoError(t, err)
	require.Equal(t, mediaSID, receipt.ProviderMessageID)
	require.Contains(t, body, "*Griya Sejahtera*")
	require.Contains(t, body, "Masuk:\nhttps://app.example.id/login", "no native buttons, so the link is in the text")
	require.Equal(t, 1, strings.Count(body, "https://"), "the link appears once")
}

// formRecorder is a Twilio stand-in that keeps the last form it was sent.
func formRecorder(t *testing.T, cfg twilio.Config) (*twilio.Client, func() url.Values) {
	t.Helper()
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		got = r.PostForm
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, createdBody)
	}))
	t.Cleanup(srv.Close)
	cfg.AccountSID, cfg.AuthToken, cfg.From, cfg.BaseURL = accountSID, authToken, sandbox, srv.URL
	c, err := twilio.New(cfg)
	require.NoError(t, err)
	return c, func() url.Values { return got }
}

func TestContentTemplateReplacesTheBody(t *testing.T) {
	t.Parallel()
	c, form := formRecorder(t, twilio.Config{ContentSIDs: map[string]string{messaging.TemplateLoginCode: loginHX}})

	receipt, err := c.SendTemplate(t.Context(), loginMessage)
	require.NoError(t, err)
	require.Equal(t, messageSID, receipt.ProviderMessageID)
	got := form()
	require.Equal(t, "whatsapp:"+recipient, got.Get("To"))
	require.Equal(t, "whatsapp:"+sandbox, got.Get("From"))
	require.Equal(t, loginHX, got.Get("ContentSid"))
	require.JSONEq(t, `{"1":"`+loginCode+`"}`, got.Get("ContentVariables"))
	require.False(t, got.Has("Body"), "Twilio refuses Body together with ContentSid")
	require.Len(t, got, 4, "only To, From, ContentSid and ContentVariables are sent")
}

func TestContentVariablesFollowTheParameterOrderThenTheButtons(t *testing.T) {
	t.Parallel()
	c, form := formRecorder(t, twilio.Config{ContentSIDs: map[string]string{messaging.TemplateMemberInvite: inviteHX}})

	_, err := c.SendTemplate(t.Context(), messaging.Message{
		To:       "+6281213139999",
		Template: messaging.TemplateMemberInvite,
		Params: map[string]string{
			"inviter": "Pak Budi", "organization": "Griya Sejahtera", "role": "Karyawan", "name": "Mbak Sari",
		},
		Buttons: []messaging.Button{{Title: "Masuk", Payload: "member_invite", URL: "https://app.example.id/login"}},
	})
	require.NoError(t, err)
	got := form()
	require.Equal(t, inviteHX, got.Get("ContentSid"))
	require.JSONEq(t, `{"1":"Mbak Sari","2":"Karyawan","3":"Griya Sejahtera","4":"Pak Budi","5":"https://app.example.id/login"}`,
		got.Get("ContentVariables"))
	require.False(t, got.Has("Body"))
}

func TestTemplatesWithoutAContentSidStaySentAsText(t *testing.T) {
	t.Parallel()
	c, form := formRecorder(t, twilio.Config{ContentSIDs: map[string]string{messaging.TemplateMemberInvite: inviteHX}})

	_, err := c.SendTemplate(t.Context(), loginMessage)
	require.NoError(t, err)
	want, err := messaging.Render(loginMessage)
	require.NoError(t, err)
	got := form()
	require.Equal(t, want, got.Get("Body"))
	require.False(t, got.Has("ContentSid"))
	require.False(t, got.Has("ContentVariables"))
}

func TestContentTemplateMessagesStillFollowTheTemplateRules(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)
	c, err := twilio.New(twilio.Config{AccountSID: accountSID, AuthToken: authToken, From: sandbox, BaseURL: srv.URL,
		ContentSIDs: map[string]string{messaging.TemplateLoginCode: loginHX}})
	require.NoError(t, err)

	for name, msg := range map[string]messaging.Message{
		"missing parameter": {To: recipient, Template: messaging.TemplateLoginCode},
		"extra parameter":   {To: recipient, Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode, "x": "y"}},
		"button without url": {To: recipient, Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode},
			Buttons: []messaging.Button{{Title: "Bayar"}}},
	} {
		_, err := c.SendTemplate(t.Context(), msg)
		require.Error(t, err, name)
	}
	require.Zero(t, calls.Load(), "a bad message never reaches Twilio")
}

func TestErrorsMapToMessagingErrors(t *testing.T) {
	t.Parallel()
	twilioError := func(code int, message string, status int) string {
		return `{"code":` + strconv.Itoa(code) + `,"message":"` + message + `","more_info":"https://www.twilio.com/docs/errors/` +
			strconv.Itoa(code) + `","status":` + strconv.Itoa(status) + `}`
	}
	tests := []struct {
		name    string
		status  int
		body    string
		wantIs  error
		wantMsg string
	}{
		{"too many requests", 429, twilioError(20429, "Too Many Requests", 429), messaging.ErrRateLimited, "code 20429"},
		{"WhatsApp rate limit", 400, twilioError(63018, "Rate limit exceeded for +6281234567890", 400), messaging.ErrRateLimited, "code 63018"},
		{"429 without a JSON body", 429, `Too Many Requests`, messaging.ErrRateLimited, "status 429"},
		{"invalid To number", 400, twilioError(21211, "The 'To' number +6281234567890 is not a valid phone number.", 400), messaging.ErrUndeliverable, "code 21211"},
		{"not a mobile number", 400, twilioError(21614, "'To' number +6281234567890 is not a valid mobile number", 400), messaging.ErrUndeliverable, "code 21614"},
		{"invalid destination", 400, twilioError(63003, "Channel could not find the To address +6281234567890", 400), messaging.ErrUndeliverable, "code 63003"},
		{"recipient replied STOP", 400, twilioError(21610, "Attempt to send to unsubscribed recipient +6281234567890", 400), messaging.ErrUndeliverable, "code 21610"},
		{"sandbox recipient not joined", 400, twilioError(63015, "Channel Sandbox can only send messages to phone numbers that have joined the Sandbox", 400), messaging.ErrUndeliverable, "code 63015"},
		{"wrong credentials", 401, `{"code":20003,"detail":"Your AccountSid or AuthToken was incorrect.","message":"Authenticate","more_info":"https://www.twilio.com/docs/errors/20003","status":401}`, messaging.ErrNotConnected, "code 20003"},
		{"401 without a JSON body", 401, `Unauthorized`, messaging.ErrNotConnected, "status 401"},
		{"forbidden", 403, twilioError(20403, "Forbidden", 403), messaging.ErrNotConnected, "status 403"},
		{"sender has no WhatsApp channel", 400, twilioError(63007, "Twilio could not find a Channel with the specified From address", 400), messaging.ErrNotConnected, "code 63007"},
		{"sender not valid for the account", 400, twilioError(21606, "The From phone number +14155238886 is not a valid, SMS-capable inbound phone number or short code for your account.", 400), messaging.ErrNotConnected, "code 21606"},
		{"content template required", 400, twilioError(21654, "ContentSid Required", 400), messaging.ErrNotConnected, "code 21654"},
		{"outside the 24-hour window", 400, twilioError(63016, "Failed to send freeform message because you are outside the allowed window. If you want to send messages outside this window, please use a Template.", 400), messaging.ErrNotConnected, "code 63016"},
		{"unrecognised Twilio refusal", 400, twilioError(21602, "Message body is required.", 400), messaging.ErrNotConnected, "status 400, code 21602"},
		{"unrecognised 4xx without a JSON body", 404, `Not Found`, messaging.ErrNotConnected, "status 404, code 0"},
		{"server error", 500, twilioError(20500, "Internal Server Error", 500), messaging.ErrNotConnected, "status 500, code 20500"},
		{"500 without a JSON body", 500, `<html>Internal Server Error</html>`, messaging.ErrNotConnected, "status 500, code 0"},
		{"created without a sid", 201, `{"status":"queued","error_code":null}`, nil, "without a message sid"},
		{"200 without a body", 200, ``, nil, "without a message sid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := c.SendTemplate(t.Context(), loginMessage)
			require.Error(t, err)
			for _, sentinel := range []error{messaging.ErrRateLimited, messaging.ErrUndeliverable, messaging.ErrNotConnected} {
				require.Equal(t, errors.Is(tc.wantIs, sentinel), errors.Is(err, sentinel), "errors.Is(err, %v)", sentinel)
			}
			require.ErrorContains(t, err, tc.wantMsg)
			requireNoLeak(t, err)
		})
	}
}

func TestContextCancelStopsTheSend(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	})
	// Registered after the server, so it runs first and the handler returns before Close waits.
	t.Cleanup(func() { close(release) })
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	t.Cleanup(cancel)

	_, err := c.SendTemplate(ctx, loginMessage)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	requireNoLeak(t, err)
}

func TestClientTimeout(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c, err := twilio.New(twilio.Config{AccountSID: accountSID, AuthToken: authToken, From: sandbox, BaseURL: srv.URL, Timeout: 50 * time.Millisecond})
	require.NoError(t, err)

	_, err = c.SendTemplate(t.Context(), loginMessage)
	require.ErrorContains(t, err, "twilio: calling API")
	requireNoLeak(t, err)
}

func TestBadMessagesNeverReachTwilio(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	c := newClient(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })

	tests := []struct {
		name string
		msg  messaging.Message
	}{
		{"unknown template", messaging.Message{To: recipient, Template: "nope"}},
		{"phone without plus", messaging.Message{To: "6281234567890", Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode}}},
		{"phone with letters", messaging.Message{To: "+62812345abc", Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode}}},
		{"phone too long", messaging.Message{To: "+6281234567890123", Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode}}},
		{"button without url", messaging.Message{To: recipient, Template: messaging.TemplateLoginCode, Params: map[string]string{"code": loginCode}, Buttons: []messaging.Button{{Title: "Bayar"}}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.SendTemplate(t.Context(), tc.msg)
			require.Error(t, err)
			require.NotContains(t, err.Error(), tc.msg.To[1:], "the recipient stays out of errors")
			require.NotContains(t, err.Error(), loginCode)
		})
	}
	require.Zero(t, calls.Load())
}

func TestNewValidatesConfig(t *testing.T) {
	t.Parallel()
	valid := twilio.Config{AccountSID: accountSID, AuthToken: authToken, From: sandbox}
	tests := []struct {
		name    string
		mutate  func(*twilio.Config)
		wantErr string
	}{
		{"valid sandbox config", func(*twilio.Config) {}, ""},
		{"verified sender", func(c *twilio.Config) { c.From = "+6281100001111" }, ""},
		{"missing account sid", func(c *twilio.Config) { c.AccountSID = "" }, "account SID"},
		{"account sid with wrong prefix", func(c *twilio.Config) { c.AccountSID = "SK" + fakeHex }, "account SID"},
		{"account sid too short", func(c *twilio.Config) { c.AccountSID = "AC0123456789abcdef" }, "account SID"},
		{"account sid not hex", func(c *twilio.Config) { c.AccountSID = "AC" + fakeHex[:31] + "z" }, "account SID"},
		{"missing auth token", func(c *twilio.Config) { c.AuthToken = "" }, "auth token"},
		{"missing sender", func(c *twilio.Config) { c.From = "" }, "E.164"},
		{"sender without plus", func(c *twilio.Config) { c.From = "14155238886" }, "E.164"},
		{"sender with whatsapp prefix", func(c *twilio.Config) { c.From = "whatsapp:+14155238886" }, "E.164"},
		{"relative base url", func(c *twilio.Config) { c.BaseURL = "api.twilio.com" }, "base URL"},
		{"other scheme", func(c *twilio.Config) { c.BaseURL = "ftp://api.twilio.com" }, "base URL"},
		{"content template", func(c *twilio.Config) {
			c.ContentSIDs = map[string]string{messaging.TemplateLoginCode: loginHX}
		}, ""},
		{"content template for an unknown message template", func(c *twilio.Config) {
			c.ContentSIDs = map[string]string{"login": loginHX}
		}, `unknown message template "login"`},
		{"content sid with the wrong prefix", func(c *twilio.Config) {
			c.ContentSIDs = map[string]string{messaging.TemplateLoginCode: "MG" + fakeHex}
		}, "content SID for login_code"},
		{"content sid too short", func(c *twilio.Config) {
			c.ContentSIDs = map[string]string{messaging.TemplateLoginCode: "HX0123"}
		}, "content SID for login_code"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := valid
			tc.mutate(&cfg)
			_, err := twilio.New(cfg)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
			require.NotContains(t, err.Error(), authToken, "the auth token never appears in an error")
		})
	}
}

// requireNoLeak checks that an error carries neither the recipient, the message text nor
// Twilio's own message text, since callers log errors as they are.
func requireNoLeak(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	require.NotContains(t, msg, "6281234567890", "errors must never carry the recipient")
	require.NotContains(t, msg, loginCode, "errors must never carry the login code")
	require.NotContains(t, msg, authToken, "errors must never carry the auth token")
	for _, twilioText := range []string{"not a valid", "Authenticate", "AccountSid or AuthToken", "Sandbox", "Too Many", "Channel", "unsubscribed", "Internal Server Error", "Rate limit"} {
		require.NotContains(t, msg, twilioText, "errors must never carry Twilio's message text")
	}
}
