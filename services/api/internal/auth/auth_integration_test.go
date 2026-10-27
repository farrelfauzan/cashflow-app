//go:build integration

package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

func TestLoginWithAWhatsAppCode(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Sejahtera", "0812111")

	t.Run("an unknown number gets the same answer and no message", func(t *testing.T) {
		require.NoError(t, env.Auth.RequestCode(ctx, "0899-0000-0000"))
		require.Empty(t, env.Messenger.Sent())
	})

	t.Run("a malformed number is rejected", func(t *testing.T) {
		require.ErrorIs(t, env.Auth.RequestCode(ctx, "021-555-1234"), auth.ErrInvalidPhone)
	})

	t.Run("a member gets a 6-digit code and can sign in once", func(t *testing.T) {
		require.NoError(t, env.Auth.RequestCode(ctx, "+62 812 111 002"))
		sent := env.Messenger.Sent()
		require.Len(t, sent, 1)
		require.Equal(t, "+62812111002", sent[0].To)
		code := env.LastCode(t)
		require.Regexp(t, `^[0-9]{6}$`, code)

		sess, id, err := env.Auth.VerifyCode(ctx, o.Admin.Phone, code)
		require.NoError(t, err)
		require.Equal(t, o.Admin.UserID, id.UserID)
		require.NotEmpty(t, sess.Token)
		require.True(t, sess.ExpiresAt.After(env.Clock.Now().Add(29*24*time.Hour)))

		got, err := env.Auth.Authenticate(ctx, sess.Token)
		require.NoError(t, err)
		require.Equal(t, o.Admin.UserID, got.UserID)
		require.Equal(t, o.Admin.Name, got.Name)

		_, _, err = env.Auth.VerifyCode(ctx, o.Admin.Phone, code)
		require.ErrorIs(t, err, auth.ErrInvalidCode, "a code works only once")

		orgs, err := env.Auth.Organizations(ctx, id.UserID)
		require.NoError(t, err)
		require.Len(t, orgs, 1)
		require.Equal(t, o.ID, orgs[0].ID)
	})

	t.Run("logout ends the session", func(t *testing.T) {
		sess, id := signIn(t, env, o.Supervisor.Phone)
		require.NoError(t, env.Auth.Logout(ctx, id.SessionID))
		_, err := env.Auth.Authenticate(ctx, sess.Token)
		require.ErrorIs(t, err, auth.ErrUnauthenticated)
	})

	t.Run("a session expires after 30 days", func(t *testing.T) {
		sess, _ := signIn(t, env, o.Karyawan.Phone)
		env.Clock.Advance(30*24*time.Hour + time.Minute)
		_, err := env.Auth.Authenticate(ctx, sess.Token)
		require.ErrorIs(t, err, auth.ErrUnauthenticated)
	})

	t.Run("garbage tokens are rejected", func(t *testing.T) {
		for _, token := range []string{"", "not-base64!", "c2hvcnQ"} {
			_, err := env.Auth.Authenticate(ctx, token)
			require.ErrorIs(t, err, auth.ErrUnauthenticated, token)
		}
	})
}

func TestLoginCodeLimits(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	o := env.CreateOrg(t, "Kos Melati", "0812222")

	t.Run("a code expires after 5 minutes", func(t *testing.T) {
		require.NoError(t, env.Auth.RequestCode(ctx, o.Admin.Phone))
		code := env.LastCode(t)
		env.Clock.Advance(5 * time.Minute)
		_, _, err := env.Auth.VerifyCode(ctx, o.Admin.Phone, code)
		require.ErrorIs(t, err, auth.ErrInvalidCode)
	})

	t.Run("five wrong guesses burn the code", func(t *testing.T) {
		require.NoError(t, env.Auth.RequestCode(ctx, o.Supervisor.Phone))
		code := env.LastCode(t)
		wrong := "000000"
		if code == wrong {
			wrong = "111111"
		}
		for range 5 {
			_, _, err := env.Auth.VerifyCode(ctx, o.Supervisor.Phone, wrong)
			require.ErrorIs(t, err, auth.ErrInvalidCode)
		}
		_, _, err := env.Auth.VerifyCode(ctx, o.Supervisor.Phone, code)
		require.ErrorIs(t, err, auth.ErrInvalidCode, "the right code no longer works after 5 wrong tries")
	})

	t.Run("a new code cancels the previous one", func(t *testing.T) {
		env.Clock.Advance(16 * time.Minute) // a fresh rate-limit window
		require.NoError(t, env.Auth.RequestCode(ctx, o.Karyawan.Phone))
		first := env.LastCode(t)
		require.NoError(t, env.Auth.RequestCode(ctx, o.Karyawan.Phone))
		second := env.LastCode(t)
		if first != second {
			_, _, err := env.Auth.VerifyCode(ctx, o.Karyawan.Phone, first)
			require.ErrorIs(t, err, auth.ErrInvalidCode)
		}
		_, _, err := env.Auth.VerifyCode(ctx, o.Karyawan.Phone, second)
		require.NoError(t, err)
	})

	t.Run("more than 3 code requests in 15 minutes are refused", func(t *testing.T) {
		env.Clock.Advance(16 * time.Minute)
		for range 3 {
			require.NoError(t, env.Auth.RequestCode(ctx, o.Admin.Phone))
		}
		require.ErrorIs(t, env.Auth.RequestCode(ctx, o.Admin.Phone), auth.ErrRateLimited)
	})

	t.Run("a failed send is reported", func(t *testing.T) {
		env.Clock.Advance(16 * time.Minute)
		env.Messenger.FailWith(messaging.ErrUndeliverable)
		t.Cleanup(func() { env.Messenger.FailWith(nil) })
		require.ErrorIs(t, env.Auth.RequestCode(ctx, o.Admin.Phone), messaging.ErrUndeliverable)
	})
}

func signIn(t *testing.T, env *testenv.Env, phone string) (auth.Session, auth.Identity) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, env.Auth.RequestCode(ctx, phone))
	sess, id, err := env.Auth.VerifyCode(ctx, phone, env.LastCode(t))
	require.NoError(t, err)
	return sess, id
}
