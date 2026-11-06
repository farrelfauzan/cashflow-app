// Command telegrampoll long-polls the development Telegram bot, development only. Telegram
// delivers updates to a webhook on a public HTTPS address in staging and production; locally
// this command asks getUpdates instead, so no tunnel is needed, and hands each update to the
// same handler the webhook uses (telegramlink): "/start <code>" from the payment page's
// "Hubungkan Telegram" links a test tenant's chat, the same from a staff member's account links
// theirs for login codes, and "/stop" unlinks the chat.
//
//	make telegram-poll        # reads TELEGRAM_BOT_TOKEN, TELEGRAM_BOT_USERNAME and DATABASE_URL from services/api/.env
//
// It never logs the token, linking codes, chat ids, phone numbers or message text.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/telegram"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/logging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "telegrampoll:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.LoadTelegramPoll(ctx)
	if err != nil {
		return err
	}
	log, err := logging.New(os.Stdout, cfg.LogLevel)
	if err != nil {
		return err
	}
	client, err := telegram.New(telegram.Config{Token: cfg.Telegram.BotToken, BaseURL: cfg.Telegram.APIBaseURL})
	if err != nil {
		return err
	}

	bot, err := client.GetMe(ctx)
	if err != nil {
		if errors.Is(err, messaging.ErrNotConnected) {
			return fmt.Errorf("telegram refused the bot token; check TELEGRAM_BOT_TOKEN in services/api/.env against BotFather: %w", err)
		}
		return fmt.Errorf("checking the bot with getMe: %w", err)
	}
	if !strings.EqualFold(bot.Username, cfg.Telegram.BotUsername) {
		return fmt.Errorf("the token belongs to @%s, but TELEGRAM_BOT_USERNAME is %s", bot.Username, cfg.Telegram.BotUsername)
	}
	log.Info("telegram poller started; development only", "bot_username", bot.Username, "bot_id", bot.ID)

	st, err := store.Open(ctx, cfg.DatabaseURL, log, store.WithMaxConns(cfg.DatabaseMaxConns))
	if err != nil {
		return err
	}
	defer st.Close()
	clk := clock.System()
	// The poller only links and unlinks chats; NewLinkCode, the one limited call, runs in the API,
	// so an in-process limiter is enough here and the poller needs no Redis.
	staff := auth.NewTelegramStaff(st, ratelimit.NewMemory(clk), clk, log)
	handler := telegramlink.NewHandler(st, client, staff, clk, log)

	err = telegram.Poll(ctx, client, handler, log, telegram.PollConfig{})
	switch {
	case errors.Is(err, telegram.ErrConflict):
		return fmt.Errorf("getUpdates is blocked: the bot has a webhook set, or another poller uses this token. "+
			"Delete the webhook with the Bot API's deleteWebhook method (https://core.telegram.org/bots/api#deletewebhook) "+
			"or stop the other poller, then run make telegram-poll again: %w", err)
	case errors.Is(err, messaging.ErrNotConnected):
		return fmt.Errorf("telegram stopped accepting the bot token (revoked in BotFather?): %w", err)
	case err != nil:
		return err
	}
	log.Info("telegram poller stopped")
	return nil
}
