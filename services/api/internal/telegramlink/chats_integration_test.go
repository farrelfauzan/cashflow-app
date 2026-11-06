//go:build integration

package telegramlink_test

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/telegramlink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// relink ends the tenant's live link, if any, and links the tenant to chatID with a fresh used
// code, as a /start from another chat does.
func relink(t *testing.T, env *testenv.Env, org, tenantID uuid.UUID, chatID int64) {
	t.Helper()
	ctx := context.Background()
	now := env.Clock.Now()
	codeID := uuid.New()
	sum := sha256.Sum256([]byte(codeID.String()))
	require.NoError(t, env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.UnlinkTenantTelegram(ctx, dbgen.UnlinkTenantTelegramParams{UnlinkedAt: &now, OrganizationID: org, TenantID: tenantID}); err != nil {
			return err
		}
		if err := q.InsertTelegramLinkCode(ctx, dbgen.InsertTelegramLinkCodeParams{
			ID: codeID, OrganizationID: org, TenantID: tenantID, CodeHash: sum[:],
			CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute),
		}); err != nil {
			return err
		}
		if _, err := q.UseTelegramLinkCode(ctx, dbgen.UseTelegramLinkCodeParams{UsedAt: &now, OrganizationID: org, ID: codeID}); err != nil {
			return err
		}
		return q.InsertTelegramLink(ctx, dbgen.InsertTelegramLinkParams{
			ID: uuid.New(), OrganizationID: org, TenantID: tenantID, LinkCodeID: codeID,
			ChatID: chatID, TelegramUserID: chatID, LinkedAt: now,
		})
	}))
}

// TestChats checks the router's view of the links: the chat of a tenant's live link, scoped to
// its organization, and ending a link in one chat only.
func TestChats(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	chats := telegramlink.NewChats(env.Store, env.Clock)

	t.Run("TenantChat: no link, a live link, an ended link, another organization", func(t *testing.T) {
		const chat = int64(5550008001)
		a := newSchemaOrg(t, env, "Griya Asri", "0812641", "Rina Wulandari", chat)
		b := newSchemaOrg(t, env, "Kos Melati", "0812642", "Budi Santoso", int64(5550008002))
		admin := env.Principal(t, a.org.ID, a.org.Admin.UserID)
		unlinked, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: "Dewi Lestari", Phone: "08126415002"})
		require.NoError(t, err)

		tests := []struct {
			name       string
			org        uuid.UUID
			tenant     uuid.UUID
			wantChat   int64
			wantLinked bool
		}{
			{name: "a tenant with no link", org: a.org.ID, tenant: unlinked.ID},
			{name: "a tenant with a live link", org: a.org.ID, tenant: a.tenantID, wantChat: chat, wantLinked: true},
			{name: "organization B asking for A's tenant", org: b.org.ID, tenant: a.tenantID},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				got, linked, err := chats.TenantChat(ctx, tc.org, tc.tenant)
				require.NoError(t, err)
				require.Equal(t, tc.wantLinked, linked)
				require.Equal(t, tc.wantChat, got)
			})
		}

		t.Run("after the link is ended", func(t *testing.T) {
			now := env.Clock.Now()
			require.NoError(t, env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
				_, err := dbgen.New(tx).UnlinkTenantTelegram(ctx, dbgen.UnlinkTenantTelegramParams{UnlinkedAt: &now, OrganizationID: a.org.ID, TenantID: a.tenantID})
				return err
			}))
			got, linked, err := chats.TenantChat(ctx, a.org.ID, a.tenantID)
			require.NoError(t, err)
			require.False(t, linked)
			require.Zero(t, got)
		})
	})

	t.Run("UnlinkUndeliverable ends the live link in that chat", func(t *testing.T) {
		const chat = int64(5550008011)
		a := newSchemaOrg(t, env, "Kontrakan Mawar", "0812643", "Siti Rahayu", chat)
		require.NoError(t, chats.UnlinkUndeliverable(ctx, a.org.ID, a.tenantID, chat))
		_, linked, err := chats.TenantChat(ctx, a.org.ID, a.tenantID)
		require.NoError(t, err)
		require.False(t, linked)

		require.NoError(t, chats.UnlinkUndeliverable(ctx, a.org.ID, a.tenantID, chat), "no live link is not an error")
	})

	t.Run("UnlinkUndeliverable leaves a newer link in another chat", func(t *testing.T) {
		const oldChat, newChat = int64(5550008021), int64(5550008022)
		a := newSchemaOrg(t, env, "Ruko Sentosa", "0812644", "Agus Pratama", oldChat)
		relink(t, env, a.org.ID, a.tenantID, newChat)

		require.NoError(t, chats.UnlinkUndeliverable(ctx, a.org.ID, a.tenantID, oldChat))
		got, linked, err := chats.TenantChat(ctx, a.org.ID, a.tenantID)
		require.NoError(t, err)
		require.True(t, linked, "a send refused in the old chat does not end the link made since")
		require.Equal(t, newChat, got)
	})

	t.Run("UnlinkUndeliverable from another organization ends nothing", func(t *testing.T) {
		const chat = int64(5550008031)
		a := newSchemaOrg(t, env, "Kos Kenanga", "0812645", "Wahyu Hidayat", chat)
		b := newSchemaOrg(t, env, "Kos Anggrek", "0812646", "Fitri Handayani", int64(5550008032))
		require.NoError(t, chats.UnlinkUndeliverable(ctx, b.org.ID, a.tenantID, chat))
		got, linked, err := chats.TenantChat(ctx, a.org.ID, a.tenantID)
		require.NoError(t, err)
		require.True(t, linked)
		require.Equal(t, chat, got)
	})
}
