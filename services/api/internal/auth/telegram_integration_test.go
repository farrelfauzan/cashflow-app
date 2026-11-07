//go:build integration

package auth_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/router"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink/linkcode"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// codeHashOf is what the bot hands to Link for a code.
func codeHashOf(t *testing.T, code string) []byte {
	t.Helper()
	hash, ok := linkcode.Hash(code)
	require.True(t, ok, "a NewLinkCode code has the linking code's shape")
	return hash
}

// liveStaffLinks is every live staff link, user to chat, read in the auth scope.
func liveStaffLinks(t *testing.T, env *testenv.Env) map[uuid.UUID]int64 {
	t.Helper()
	ctx := context.Background()
	out := map[uuid.UUID]int64{}
	require.NoError(t, env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT staff_user_id, chat_id FROM telegram_staff_link WHERE unlinked_at IS NULL")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var user uuid.UUID
			var chat int64
			if err := rows.Scan(&user, &chat); err != nil {
				return err
			}
			out[user] = chat
		}
		return rows.Err()
	}))
	return out
}

// TestTelegramStaff covers the staff side of the Telegram demo channel against the database:
// codes, linking with every outcome, unlinking, and the router's view.
func TestTelegramStaff(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812651")
	logs := &bytes.Buffer{}
	tg := auth.NewTelegramStaff(env.Store, env.Limiter, env.Clock, slog.New(slog.NewJSONHandler(logs, nil)))
	const adminChat, otherChat, sharedChat = int64(5550009001), int64(5550009002), int64(5550009003)
	var codes []string

	newCode := func(t *testing.T, user uuid.UUID) string {
		t.Helper()
		code, expires, err := tg.NewLinkCode(ctx, user)
		require.NoError(t, err)
		require.Equal(t, env.Clock.Now().Add(linkcode.Lifetime), expires)
		codes = append(codes, code)
		return code
	}

	var first string
	t.Run("NewLinkCode stores only the code's SHA-256 for that user", func(t *testing.T) {
		first = newCode(t, o.Admin.UserID)
		raw, err := base64.RawURLEncoding.DecodeString(first)
		require.NoError(t, err)
		want := sha256.Sum256(raw)
		var stored []byte
		var user uuid.UUID
		require.NoError(t, env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT code_hash, staff_user_id FROM telegram_staff_link_code").Scan(&stored, &user)
		}))
		require.Equal(t, want[:], stored)
		require.Equal(t, o.Admin.UserID, user)
		linked, err := tg.Linked(ctx, o.Admin.UserID)
		require.NoError(t, err)
		require.False(t, linked, "a code alone links nothing")
	})

	t.Run("an unknown hash is Unknown and changes nothing", func(t *testing.T) {
		res, err := tg.Link(ctx, make([]byte, 32), adminChat, adminChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.StaffLink{Outcome: linkcode.Unknown}, res)
		require.Empty(t, liveStaffLinks(t, env))
	})

	t.Run("Link links the code's user and names them", func(t *testing.T) {
		res, err := tg.Link(ctx, codeHashOf(t, first), adminChat, adminChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.StaffLink{Outcome: linkcode.Linked, Name: o.Admin.Name}, res)
		require.Equal(t, map[uuid.UUID]int64{o.Admin.UserID: adminChat}, liveStaffLinks(t, env))
		linked, err := tg.Linked(ctx, o.Admin.UserID)
		require.NoError(t, err)
		require.True(t, linked)
		chat, ok, err := tg.StaffChat(ctx, o.Admin.UserID)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, adminChat, chat)
	})

	t.Run("a used code is AlreadyLinked from its own chat and Used from another", func(t *testing.T) {
		res, err := tg.Link(ctx, codeHashOf(t, first), adminChat, adminChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.StaffLink{Outcome: linkcode.AlreadyLinked, Name: o.Admin.Name}, res)
		res, err = tg.Link(ctx, codeHashOf(t, first), otherChat, otherChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.StaffLink{Outcome: linkcode.Used}, res)
		require.Equal(t, map[uuid.UUID]int64{o.Admin.UserID: adminChat}, liveStaffLinks(t, env))
	})

	t.Run("an expired code is Expired", func(t *testing.T) {
		code := newCode(t, o.Admin.UserID)
		env.Clock.Advance(linkcode.Lifetime)
		res, err := tg.Link(ctx, codeHashOf(t, code), otherChat, otherChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.StaffLink{Outcome: linkcode.Expired}, res)
		require.Equal(t, map[uuid.UUID]int64{o.Admin.UserID: adminChat}, liveStaffLinks(t, env))
	})

	t.Run("a new link replaces the older one and releases its chat", func(t *testing.T) {
		res, err := tg.Link(ctx, codeHashOf(t, newCode(t, o.Admin.UserID)), sharedChat, sharedChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.StaffLink{Outcome: linkcode.Linked, Name: o.Admin.Name, Released: []int64{adminChat}}, res)
		require.Equal(t, map[uuid.UUID]int64{o.Admin.UserID: sharedChat}, liveStaffLinks(t, env))

		res, err = tg.Link(ctx, codeHashOf(t, newCode(t, o.Admin.UserID)), sharedChat, sharedChat)
		require.NoError(t, err)
		require.Empty(t, res.Released, "relinking the same chat releases nothing to tell")
		res, err = tg.Link(ctx, codeHashOf(t, first), adminChat, adminChat)
		require.NoError(t, err)
		require.Equal(t, linkcode.Used, res.Outcome, "the replaced link's code no longer answers as linked")
	})

	t.Run("one chat may link several staff users, and UnlinkChat ends them all", func(t *testing.T) {
		_, err := tg.Link(ctx, codeHashOf(t, newCode(t, o.Supervisor.UserID)), sharedChat, sharedChat)
		require.NoError(t, err)
		require.Equal(t, map[uuid.UUID]int64{o.Admin.UserID: sharedChat, o.Supervisor.UserID: sharedChat}, liveStaffLinks(t, env))
		n, err := tg.UnlinkChat(ctx, sharedChat)
		require.NoError(t, err)
		require.EqualValues(t, 2, n)
		require.Empty(t, liveStaffLinks(t, env))
		n, err = tg.UnlinkChat(ctx, sharedChat)
		require.NoError(t, err)
		require.Zero(t, n)
	})

	t.Run("Unlink ends the user's link and is fine with none", func(t *testing.T) {
		_, err := tg.Link(ctx, codeHashOf(t, newCode(t, o.Karyawan.UserID)), otherChat, otherChat)
		require.NoError(t, err)
		require.NoError(t, tg.Unlink(ctx, o.Karyawan.UserID))
		linked, err := tg.Linked(ctx, o.Karyawan.UserID)
		require.NoError(t, err)
		require.False(t, linked)
		require.NoError(t, tg.Unlink(ctx, o.Karyawan.UserID))
		_, ok, err := tg.StaffChat(ctx, o.Karyawan.UserID)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("UnlinkUndeliverable ends only the live link in that chat", func(t *testing.T) {
		_, err := tg.Link(ctx, codeHashOf(t, newCode(t, o.Karyawan.UserID)), otherChat, otherChat)
		require.NoError(t, err)
		require.NoError(t, tg.UnlinkUndeliverable(ctx, o.Karyawan.UserID, adminChat), "another chat: nothing to end")
		_, ok, err := tg.StaffChat(ctx, o.Karyawan.UserID)
		require.NoError(t, err)
		require.True(t, ok)
		require.NoError(t, tg.UnlinkUndeliverable(ctx, o.Karyawan.UserID, otherChat))
		_, ok, err = tg.StaffChat(ctx, o.Karyawan.UserID)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("two codes of one user used at once leave one live link", func(t *testing.T) {
		hashes := [][]byte{codeHashOf(t, newCode(t, o.Supervisor.UserID)), codeHashOf(t, newCode(t, o.Supervisor.UserID))}
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, h := range hashes {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = tg.Link(ctx, h, int64(5550009100+i), int64(5550009100+i))
			}()
		}
		wg.Wait()
		require.NoError(t, errors.Join(errs...))
		_, ok, err := tg.StaffChat(ctx, o.Supervisor.UserID)
		require.NoError(t, err)
		require.True(t, ok)
		var live int
		require.NoError(t, env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM telegram_staff_link WHERE staff_user_id = $1 AND unlinked_at IS NULL",
				o.Supervisor.UserID).Scan(&live)
		}))
		require.Equal(t, 1, live)
	})

	t.Run("at most 5 codes per user per 15 minutes", func(t *testing.T) {
		env.Clock.Advance(16 * time.Minute)
		for range 5 {
			newCode(t, o.Karyawan.UserID)
		}
		_, _, err := tg.NewLinkCode(ctx, o.Karyawan.UserID)
		require.ErrorIs(t, err, auth.ErrRateLimited)
		_, _, err = tg.NewLinkCode(ctx, o.Admin.UserID)
		require.NoError(t, err, "the limit is per user")
	})

	t.Run("codes, hashes and chat ids stay out of the logs", func(t *testing.T) {
		out := logs.String()
		require.Contains(t, out, "telegram linked for login codes")
		for _, c := range codes {
			require.NotContains(t, out, c)
		}
		for _, chat := range []int64{adminChat, otherChat, sharedChat} {
			require.NotContains(t, out, strconv.FormatInt(chat, 10))
		}
	})
}

// TestTelegramStaffSchema checks the database side as the app role: the staff tables are seen only
// in the auth scope, codes and links cannot be deleted or rewritten, a user has one live link, and
// a link's user is its code's.
func TestTelegramStaffSchema(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Kos Bulanan Sejahtera", "0812652")
	tg := auth.NewTelegramStaff(env.Store, env.Limiter, env.Clock, env.Log)
	code, _, err := tg.NewLinkCode(ctx, o.Admin.UserID)
	require.NoError(t, err)
	_, err = tg.Link(ctx, codeHashOf(t, code), 5550009201, 5550009201)
	require.NoError(t, err)
	tables := []string{"telegram_staff_link_code", "telegram_staff_link"}

	t.Run("only the auth scope sees the rows", func(t *testing.T) {
		for _, table := range tables {
			var n int
			require.NoError(t, env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
			}))
			require.Equal(t, 1, n, table)
			require.NoError(t, env.Store.WithOrgTx(ctx, o.ID, func(tx pgx.Tx) error {
				return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
			}))
			require.Zero(t, n, "an organization scope sees no staff link: "+table)
		}
		app, err := pgx.Connect(ctx, env.URLs.App)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, app.Close(ctx)) })
		for _, table := range tables {
			var n int
			require.NoError(t, app.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n))
			require.Zero(t, n, "no scope, no rows: "+table)
		}
	})

	t.Run("the app role cannot delete or rewrite a code or a link", func(t *testing.T) {
		for _, stmt := range []string{
			"DELETE FROM telegram_staff_link_code",
			"UPDATE telegram_staff_link_code SET code_hash = code_hash",
			"UPDATE telegram_staff_link_code SET expires_at = expires_at + interval '1 day'",
			"UPDATE telegram_staff_link_code SET staff_user_id = staff_user_id",
			"DELETE FROM telegram_staff_link",
			"UPDATE telegram_staff_link SET chat_id = 1",
			"UPDATE telegram_staff_link SET staff_user_id = staff_user_id",
			"TRUNCATE telegram_staff_link",
		} {
			err := env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, stmt)
				return err
			})
			requirePgCode(t, err, "42501", stmt)
		}
	})

	t.Run("one live link per user, one link per code, a link's user is its code's, a hash is 32 bytes", func(t *testing.T) {
		now := env.Clock.Now()
		insertCode := func(id, user uuid.UUID, hash []byte) error {
			return env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
				return dbgen.New(tx).InsertTelegramStaffLinkCode(ctx, dbgen.InsertTelegramStaffLinkCodeParams{
					ID: id, StaffUserID: user, CodeHash: hash, CreatedAt: now, ExpiresAt: now.Add(linkcode.Lifetime),
				})
			})
		}
		insertLink := func(codeID, user uuid.UUID) error {
			return env.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
				return dbgen.New(tx).InsertTelegramStaffLink(ctx, dbgen.InsertTelegramStaffLinkParams{
					ID: uuid.New(), StaffUserID: user, LinkCodeID: codeID, ChatID: 5550009299, TelegramUserID: 5550009299, LinkedAt: now,
				})
			})
		}
		requirePgCode(t, insertCode(uuid.New(), o.Admin.UserID, make([]byte, 31)), "23514", "short hash")
		second := uuid.New()
		sum := sha256.Sum256([]byte("second staff code"))
		require.NoError(t, insertCode(second, o.Admin.UserID, sum[:]))
		requirePgCode(t, insertCode(uuid.New(), o.Admin.UserID, sum[:]), "23505", "duplicate hash")
		requirePgCode(t, insertLink(second, o.Admin.UserID), "23505", "a live link exists already")
		requirePgCode(t, insertLink(second, o.Supervisor.UserID), "23503", "not the code's user")
		requirePgCode(t, insertCode(uuid.New(), uuid.New(), make([]byte, 32)), "23503", "no such staff user")
	})
}

func requirePgCode(t *testing.T, err error, code, what string) {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "%s: expected a Postgres error, got %v", what, err)
	require.Equal(t, code, pgErr.Code, "%s: %s", what, pgErr.Message)
}

// fakeTelegram records what the router sends to Telegram.
type fakeTelegram struct {
	mu   sync.Mutex
	sent map[int64][]messaging.Message
}

func (f *fakeTelegram) Send(_ context.Context, chat int64, m messaging.Message) (messaging.Receipt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sent == nil {
		f.sent = map[int64][]messaging.Message{}
	}
	f.sent[chat] = append(f.sent[chat], m)
	return messaging.Receipt{ProviderMessageID: "tg"}, nil
}

func (f *fakeTelegram) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, ms := range f.sent {
		n += len(ms)
	}
	return n
}

// TestLoginCodeThroughTheRouter: with the router in front of WhatsApp (TELEGRAM_MESSAGES=linked),
// a linked staff user's code goes to their chat and signs them in; WhatsAppOnly sends it on
// WhatsApp; an unlinked user gets WhatsApp; an unknown number gets the same nil and no message;
// and WhatsAppOnly counts against the same per-phone limit.
func TestLoginCodeThroughTheRouter(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812653")
	tg := auth.NewTelegramStaff(env.Store, env.Limiter, env.Clock, env.Log)
	bot := &fakeTelegram{}
	rtr := router.New(env.Messenger, bot, telegramlink.NewChats(env.Store, env.Clock), tg, env.Log)
	svc, err := auth.NewService(env.Store, rtr, env.Limiter, env.Clock, env.Log, testenv.Secret)
	require.NoError(t, err)
	const adminChat = int64(5550009301)
	code, _, err := tg.NewLinkCode(ctx, o.Admin.UserID)
	require.NoError(t, err)
	_, err = tg.Link(ctx, codeHashOf(t, code), adminChat, adminChat)
	require.NoError(t, err)

	t.Run("a linked staff user's code goes to their chat and signs them in", func(t *testing.T) {
		require.NoError(t, svc.RequestCode(ctx, o.Admin.Phone, auth.CodeOptions{}))
		require.Empty(t, env.Messenger.Sent(), "nothing on WhatsApp")
		require.Len(t, bot.sent[adminChat], 1)
		m := bot.sent[adminChat][0]
		require.Equal(t, messaging.TemplateLoginCode, m.Template)
		require.Equal(t, o.Admin.UserID, m.StaffUserID)
		_, id, err := svc.VerifyCode(ctx, o.Admin.Phone, m.Params["code"])
		require.NoError(t, err)
		require.Equal(t, o.Admin.UserID, id.UserID)
	})

	t.Run("WhatsAppOnly sends it on WhatsApp and names no staff user", func(t *testing.T) {
		require.NoError(t, svc.RequestCode(ctx, o.Admin.Phone, auth.CodeOptions{WhatsAppOnly: true}))
		require.Len(t, bot.sent[adminChat], 1, "nothing more on Telegram")
		sent := env.Messenger.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, uuid.Nil, sent[0].StaffUserID)
		_, _, err := svc.VerifyCode(ctx, o.Admin.Phone, env.LastCode(t))
		require.NoError(t, err)
	})

	t.Run("an unlinked staff user gets WhatsApp", func(t *testing.T) {
		require.NoError(t, svc.RequestCode(ctx, o.Supervisor.Phone, auth.CodeOptions{}))
		sent := env.Messenger.Sent()
		require.Len(t, sent, 2)
		require.Equal(t, o.Supervisor.UserID, sent[1].StaffUserID, "named, so a later link would route it")
		require.Equal(t, 1, bot.count())
	})

	t.Run("an unknown number gets the same nil and no message, either way", func(t *testing.T) {
		for _, opts := range []auth.CodeOptions{{}, {WhatsAppOnly: true}} {
			require.NoError(t, svc.RequestCode(ctx, "0899-0000-0001", opts))
		}
		require.Len(t, env.Messenger.Sent(), 2)
		require.Equal(t, 1, bot.count())
	})

	t.Run("WhatsAppOnly counts against the same per-phone limit", func(t *testing.T) {
		require.NoError(t, svc.RequestCode(ctx, o.Karyawan.Phone, auth.CodeOptions{}))
		require.NoError(t, svc.RequestCode(ctx, o.Karyawan.Phone, auth.CodeOptions{WhatsAppOnly: true}))
		require.NoError(t, svc.RequestCode(ctx, o.Karyawan.Phone, auth.CodeOptions{WhatsAppOnly: true}))
		require.ErrorIs(t, svc.RequestCode(ctx, o.Karyawan.Phone, auth.CodeOptions{WhatsAppOnly: true}), auth.ErrRateLimited)
		require.ErrorIs(t, svc.RequestCode(ctx, o.Karyawan.Phone, auth.CodeOptions{}), auth.ErrRateLimited)
	})
}
