package telegram_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
)

// fakeHandler records the updates it gets and fails the ones listed in failOn.
type fakeHandler struct {
	mu     sync.Mutex
	got    []int64
	failOn map[int64]bool
}

func (h *fakeHandler) HandleUpdate(_ context.Context, u telegram.Update) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, u.UpdateID)
	if h.failOn[u.UpdateID] {
		return errors.New("handler failed")
	}
	return nil
}

func (h *fakeHandler) ids() []int64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]int64(nil), h.got...)
}

// syncBuffer is a log sink safe to read while Poll writes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// pollServer answers getUpdates with each step in turn, records the offsets asked for, and
// cancels the poll once the steps run out.
type pollStep struct {
	status int
	body   string
}

func runPoll(t *testing.T, steps []pollStep, h telegram.UpdateHandler) (offsets []int64, logs string, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	t.Cleanup(cancel)
	var mu sync.Mutex
	i := 0
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Offset int64 `json:"offset"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		mu.Lock()
		offsets = append(offsets, req.Offset)
		step := i
		i++
		mu.Unlock()
		if step >= len(steps) {
			cancel()
			<-r.Context().Done()
			return
		}
		w.WriteHeader(steps[step].status)
		_, _ = io.WriteString(w, steps[step].body)
	})
	var buf syncBuffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	err = telegram.Poll(ctx, c, h, log, telegram.PollConfig{Timeout: time.Second, MinBackoff: time.Millisecond, MaxBackoff: 5 * time.Millisecond})
	mu.Lock()
	defer mu.Unlock()
	return offsets, buf.String(), err
}

func updatesBody(ids ...int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf(`{"update_id":%d,"message":{"message_id":%d,"from":{"id":5550001112,"is_bot":false,"first_name":"Budi"},`+
			`"chat":{"id":5550001112,"type":"private"},"date":1762400000,"text":"/start rahasia-%d"}}`, id, id, id)
	}
	return `{"ok":true,"result":[` + strings.Join(parts, ",") + `]}`
}

func TestPoll(t *testing.T) {
	t.Parallel()
	ok := http.StatusOK
	tests := []struct {
		name        string
		steps       []pollStep
		failOn      map[int64]bool
		wantErr     error
		wantOffsets []int64
		wantHandled []int64
		wantLog     []string
	}{
		{
			name:        "offset advances past each batch",
			steps:       []pollStep{{ok, updatesBody(10, 11)}, {ok, `{"ok":true,"result":[]}`}, {ok, updatesBody(12)}},
			wantOffsets: []int64{0, 12, 12, 13},
			wantHandled: []int64{10, 11, 12},
		},
		{
			name:        "a handler error is logged and the loop goes on",
			steps:       []pollStep{{ok, updatesBody(20, 21)}, {ok, updatesBody(22)}},
			failOn:      map[int64]bool{20: true},
			wantOffsets: []int64{0, 22, 23},
			wantHandled: []int64{20, 21, 22},
			wantLog:     []string{"telegram update not handled", `"update_id":20`, `"kind":"message"`},
		},
		{
			name: "transient failures are retried with the same offset",
			steps: []pollStep{
				{ok, updatesBody(30)},
				{http.StatusBadGateway, `<html>502 Bad Gateway</html>`},
				{http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 0","parameters":{"retry_after":0}}`},
				{ok, `{"ok":true,"result":`},
				{ok, updatesBody(31)},
			},
			wantOffsets: []int64{0, 31, 31, 31, 31, 32},
			wantHandled: []int64{30, 31},
			wantLog:     []string{"telegram getUpdates failed; retrying"},
		},
		{
			name:        "a webhook stops polling",
			steps:       []pollStep{{http.StatusConflict, `{"ok":false,"error_code":409,"description":"Conflict: can't use getUpdates method while webhook is active; use deleteWebhook to delete the webhook first"}`}},
			wantErr:     telegram.ErrConflict,
			wantOffsets: []int64{0},
		},
		{
			name:        "a revoked token stops polling",
			steps:       []pollStep{{ok, updatesBody(40)}, {http.StatusUnauthorized, `{"ok":false,"error_code":401,"description":"Unauthorized"}`}},
			wantErr:     messaging.ErrNotConnected,
			wantOffsets: []int64{0, 41},
			wantHandled: []int64{40},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &fakeHandler{failOn: tc.failOn}
			offsets, logs, err := runPoll(t, tc.steps, h)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				requireNoLeak(t, err)
			} else {
				require.NoError(t, err, "a cancelled context ends Poll without an error")
			}
			require.Equal(t, tc.wantOffsets, offsets)
			require.Equal(t, tc.wantHandled, h.ids())
			for _, want := range tc.wantLog {
				require.Contains(t, logs, want)
			}
			require.NotContains(t, logs, "rahasia", "update text stays out of logs")
			require.NotContains(t, logs, "5550001112", "chat ids stay out of logs")
			_, secretHalf, _ := strings.Cut(botToken, ":")
			require.NotContains(t, logs, secretHalf, "the token stays out of logs")
		})
	}
}

func TestLogHandlerLogsOnlyIDAndKind(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	h := telegram.LogHandler{Log: slog.New(slog.NewJSONHandler(&buf, nil))}
	updates := []telegram.Update{
		{UpdateID: 1, Message: &telegram.Message{Chat: telegram.Chat{ID: chatID}, Text: "/start rahasia"}},
		{UpdateID: 2, Message: &telegram.Message{Chat: telegram.Chat{ID: chatID}, Contact: &telegram.Contact{PhoneNumber: "6281234567890", UserID: chatID}}},
		{UpdateID: 3},
	}
	for _, u := range updates {
		require.NoError(t, h.HandleUpdate(t.Context(), u))
	}
	logs := buf.String()
	require.Contains(t, logs, `"update":{"update_id":1,"kind":"message"}`)
	require.Contains(t, logs, `"update":{"update_id":2,"kind":"contact"}`)
	require.Contains(t, logs, `"update":{"update_id":3,"kind":"other"}`)
	for _, secret := range []string{"rahasia", "6281234567890", "5550001112"} {
		require.NotContains(t, logs, secret)
	}
}
