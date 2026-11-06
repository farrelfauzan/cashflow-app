package router_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/messagingtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/router"
)

const chatID int64 = 918273645

type unlinkCall struct {
	org, tenant uuid.UUID
	chat        int64
}

// fakeLinks holds live links in memory.
type fakeLinks struct {
	mu        sync.Mutex
	chats     map[[2]uuid.UUID]int64
	lookupErr error
	unlinkErr error
	unlinked  []unlinkCall
}

func (f *fakeLinks) TenantChat(_ context.Context, org, tenant uuid.UUID) (int64, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lookupErr != nil {
		return 0, false, f.lookupErr
	}
	c, ok := f.chats[[2]uuid.UUID{org, tenant}]
	return c, ok, nil
}

func (f *fakeLinks) UnlinkUndeliverable(_ context.Context, org, tenant uuid.UUID, chat int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unlinked = append(f.unlinked, unlinkCall{org, tenant, chat})
	if f.unlinkErr != nil {
		return f.unlinkErr
	}
	delete(f.chats, [2]uuid.UUID{org, tenant})
	return nil
}

type telegramCall struct {
	chat int64
	msg  messaging.Message
}

// fakeTelegram records sends, or fails them with err.
type fakeTelegram struct {
	mu   sync.Mutex
	sent []telegramCall
	err  error
}

func (f *fakeTelegram) Send(_ context.Context, chat int64, m messaging.Message) (messaging.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return messaging.Receipt{}, f.err
	}
	f.sent = append(f.sent, telegramCall{chat, m})
	return messaging.Receipt{ProviderMessageID: "tg-" + strconv.Itoa(len(f.sent))}, nil
}

type setup struct {
	router   *router.Router
	whatsapp *messagingtest.Fake
	telegram *fakeTelegram
	links    *fakeLinks
	logs     *strings.Builder
	org      uuid.UUID
	linked   uuid.UUID // a tenant of org with a live link in chatID
	unlinked uuid.UUID // a tenant of org with none
}

func newSetup(t *testing.T) setup {
	t.Helper()
	s := setup{
		whatsapp: &messagingtest.Fake{},
		telegram: &fakeTelegram{},
		logs:     &strings.Builder{},
		org:      uuid.New(),
		linked:   uuid.New(),
		unlinked: uuid.New(),
	}
	s.links = &fakeLinks{chats: map[[2]uuid.UUID]int64{{s.org, s.linked}: chatID}}
	log := slog.New(slog.NewJSONHandler(s.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	s.router = router.New(s.whatsapp, s.telegram, s.links, log)
	return s
}

func reminderFor(org, tenant uuid.UUID) messaging.Message {
	m := messagingtest.Samples()[messaging.TemplateInvoiceDueSoon]
	m.OrganizationID, m.TenantID, m.To = org, tenant, "+6281200010002"
	return m
}

func TestRouterChoosesTheChannel(t *testing.T) {
	t.Parallel()
	login := messagingtest.Samples()[messaging.TemplateLoginCode]
	login.To = "+6281200010002"

	tests := []struct {
		name         string
		msg          func(s setup) messaging.Message
		wantTelegram bool
	}{
		{name: "a linked tenant's reminder goes to their chat", msg: func(s setup) messaging.Message { return reminderFor(s.org, s.linked) }, wantTelegram: true},
		{name: "a tenant with no link gets WhatsApp", msg: func(s setup) messaging.Message { return reminderFor(s.org, s.unlinked) }},
		{name: "a login code names no tenant and gets WhatsApp", msg: func(setup) messaging.Message { return login }},
		{name: "a message naming no tenant gets WhatsApp even in a linked tenant's organization", msg: func(s setup) messaging.Message {
			m := reminderFor(s.org, s.linked)
			m.TenantID = uuid.Nil
			return m
		}},
		{name: "the same tenant id in another organization is not linked", msg: func(s setup) messaging.Message { return reminderFor(uuid.New(), s.linked) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := newSetup(t)
			m := tc.msg(s)
			receipt, err := s.router.SendTemplate(context.Background(), m)
			require.NoError(t, err)
			if tc.wantTelegram {
				require.Equal(t, []telegramCall{{chat: chatID, msg: m}}, s.telegram.sent)
				require.Empty(t, s.whatsapp.Sent())
				require.Equal(t, "tg-1", receipt.ProviderMessageID)
				return
			}
			require.Empty(t, s.telegram.sent)
			require.Equal(t, []messaging.Message{m}, s.whatsapp.Sent())
		})
	}
}

func TestRouterUndeliverableChatEndsTheLinkWithoutFallback(t *testing.T) {
	t.Parallel()
	s := newSetup(t)
	s.telegram.err = fmt.Errorf("%w (telegram sendMessage, status 403, error_code 403)", messaging.ErrUndeliverable)

	_, err := s.router.SendTemplate(context.Background(), reminderFor(s.org, s.linked))
	require.ErrorIs(t, err, messaging.ErrUndeliverable)
	require.Empty(t, s.whatsapp.Sent(), "no fallback to WhatsApp")
	require.Equal(t, []unlinkCall{{s.org, s.linked, chatID}}, s.links.unlinked)

	// The next message for the tenant goes to WhatsApp.
	s.telegram.err = nil
	_, err = s.router.SendTemplate(context.Background(), reminderFor(s.org, s.linked))
	require.NoError(t, err)
	require.Len(t, s.whatsapp.Sent(), 1)
	require.Empty(t, s.telegram.sent)
}

func TestRouterOtherTelegramErrorsKeepTheLink(t *testing.T) {
	t.Parallel()
	for name, sendErr := range map[string]error{
		"rate limited":  &rateLimited{},
		"bad token":     fmt.Errorf("%w (telegram sendMessage, status 401)", messaging.ErrNotConnected),
		"other refusal": errors.New("telegram: request refused (telegram sendMessage, status 400, error_code 400)"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newSetup(t)
			s.telegram.err = sendErr
			_, err := s.router.SendTemplate(context.Background(), reminderFor(s.org, s.linked))
			require.ErrorIs(t, err, sendErr)
			require.Empty(t, s.links.unlinked)
			require.Empty(t, s.whatsapp.Sent(), "no fallback to WhatsApp")
		})
	}
}

type rateLimited struct{}

func (*rateLimited) Error() string { return "telegram: rate limited" }
func (*rateLimited) Unwrap() error { return messaging.ErrRateLimited }

func TestRouterUnlinkFailureStillReturnsTheSendError(t *testing.T) {
	t.Parallel()
	s := newSetup(t)
	s.telegram.err = fmt.Errorf("%w (chat not found)", messaging.ErrUndeliverable)
	s.links.unlinkErr = errors.New("database down")

	_, err := s.router.SendTemplate(context.Background(), reminderFor(s.org, s.linked))
	require.ErrorIs(t, err, messaging.ErrUndeliverable)
	require.Contains(t, s.logs.String(), "telegram link not ended")
}

func TestRouterLookupFailureSendsNothing(t *testing.T) {
	t.Parallel()
	s := newSetup(t)
	s.links.lookupErr = errors.New("database down")

	_, err := s.router.SendTemplate(context.Background(), reminderFor(s.org, s.linked))
	require.ErrorContains(t, err, "database down")
	require.Empty(t, s.whatsapp.Sent(), "a failed lookup never guesses WhatsApp")
	require.Empty(t, s.telegram.sent)
}

func TestRouterLogsNeitherTheChatNorTheText(t *testing.T) {
	t.Parallel()
	s := newSetup(t)
	m := reminderFor(s.org, s.linked)
	_, err := s.router.SendTemplate(context.Background(), m)
	require.NoError(t, err)
	s.telegram.err = fmt.Errorf("%w (status 403)", messaging.ErrUndeliverable)
	_, err = s.router.SendTemplate(context.Background(), m)
	require.Error(t, err)

	logs := s.logs.String()
	require.Contains(t, logs, `"channel":"telegram"`)
	require.NotContains(t, logs, strconv.FormatInt(chatID, 10))
	for _, v := range m.Params {
		require.NotContains(t, logs, v)
	}
	require.NotContains(t, logs, m.Buttons[0].URL)
	require.NotContains(t, logs, m.To)
}
