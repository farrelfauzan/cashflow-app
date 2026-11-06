package telegram

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// Update is one incoming Bot API update, trimmed to what linking needs (TG-2). Unknown fields
// are ignored. It holds a chat id, a phone number and message text, so it logs as its id and
// kind only (LogValue).
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

// Message is the part of a Bot API Message that linking reads.
type Message struct {
	MessageID int64    `json:"message_id"`
	From      *User    `json:"from,omitempty"`
	Chat      Chat     `json:"chat"`
	Text      string   `json:"text,omitempty"`
	Contact   *Contact `json:"contact,omitempty"`
}

// User is a Telegram user or bot. Its id can need more than 32 bits.
type User struct {
	ID       int64  `json:"id"`
	IsBot    bool   `json:"is_bot"`
	Username string `json:"username,omitempty"`
}

// Chat is the chat a message belongs to. Its id can need more than 32 bits.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// Contact is a shared contact. UserID is set when the contact is a Telegram user; it equals
// the sender's id when people share their own contact.
type Contact struct {
	PhoneNumber string `json:"phone_number"`
	FirstName   string `json:"first_name"`
	UserID      int64  `json:"user_id,omitempty"`
}

// Update kinds, for logs.
const (
	KindMessage = "message"
	KindContact = "contact"
	KindOther   = "other"
)

// Kind says what the update carries: a shared contact, another message, or something else
// (an edit, a callback, ...).
func (u Update) Kind() string {
	switch {
	case u.Message == nil:
		return KindOther
	case u.Message.Contact != nil:
		return KindContact
	default:
		return KindMessage
	}
}

// LogValue keeps chat ids, phone numbers and text out of logs when an Update is logged.
func (u Update) LogValue() slog.Value {
	return slog.GroupValue(slog.Int64("update_id", u.UpdateID), slog.String("kind", u.Kind()))
}

// UpdateHandler handles one update. The webhook (TG-2) and the development poller feed the same
// handler.
type UpdateHandler interface {
	HandleUpdate(ctx context.Context, u Update) error
}

// PollConfig tunes Poll. Zero values take the defaults.
type PollConfig struct {
	// Timeout is getUpdates' long-polling wait (default 30s).
	Timeout time.Duration
	// MinBackoff and MaxBackoff bound the wait after a failed getUpdates (default 1s to 30s).
	MinBackoff, MaxBackoff time.Duration
}

// Poll long-polls getUpdates until ctx ends, handing each update to h in order and moving the
// offset past it whether or not h succeeds: a handler error is logged, never retried, so one
// bad update cannot stop the loop. A failed getUpdates is retried with backoff (Telegram's
// retry_after on a 429). It returns nil when ctx ends, and an error when polling cannot go on:
// ErrConflict (409: a webhook is set, or another poller runs) or messaging.ErrNotConnected (the
// token is wrong or revoked). It never logs update contents.
func Poll(ctx context.Context, c *Client, h UpdateHandler, log *slog.Logger, cfg PollConfig) error {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MinBackoff <= 0 {
		cfg.MinBackoff = time.Second
	}
	if cfg.MaxBackoff < cfg.MinBackoff {
		cfg.MaxBackoff = max(30*time.Second, cfg.MinBackoff)
	}
	var offset int64
	backoff := cfg.MinBackoff
	for ctx.Err() == nil {
		updates, err := c.GetUpdates(ctx, offset, cfg.Timeout)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if fatal(err) {
				return err
			}
			wait := backoff
			if rl, ok := errors.AsType[*RateLimitError](err); ok && rl.RetryAfter > 0 {
				wait = rl.RetryAfter
			}
			log.Warn("telegram getUpdates failed; retrying", "err", err, "wait", wait)
			if !sleep(ctx, wait) {
				return nil
			}
			backoff = min(backoff*2, cfg.MaxBackoff)
			continue
		}
		backoff = cfg.MinBackoff
		for _, u := range updates {
			if err := h.HandleUpdate(ctx, u); err != nil {
				log.Error("telegram update not handled", "update", u, "err", err)
			}
			offset = max(offset, u.UpdateID+1)
		}
	}
	return nil
}

func fatal(err error) bool {
	return errors.Is(err, ErrConflict) || errors.Is(err, messaging.ErrNotConnected)
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// LogHandler is TG-1's handler for the development poller: it logs each update's id and kind,
// nothing else.
type LogHandler struct {
	Log *slog.Logger
}

// HandleUpdate logs u's id and kind.
func (l LogHandler) HandleUpdate(_ context.Context, u Update) error {
	l.Log.Info("telegram update received", "update", u)
	return nil
}
