package telegramlink

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// Chats reads and ends tenants' live links for the messaging router (router.Links). Each call
// runs in the tenant's organization scope.
type Chats struct {
	store *store.Store
	clock clock.Clock
}

// NewChats returns the router's view of the links.
func NewChats(st *store.Store, c clock.Clock) *Chats {
	return &Chats{store: st, clock: c}
}

// TenantChat returns the chat of the tenant's live link, and false when there is none.
func (c *Chats) TenantChat(ctx context.Context, org, tenant uuid.UUID) (int64, bool, error) {
	var chatID int64
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		chatID, err = dbgen.New(tx).GetTenantTelegramChat(ctx, dbgen.GetTenantTelegramChatParams{OrganizationID: org, TenantID: tenant})
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("telegramlink: reading the tenant's link: %w", err)
	}
	return chatID, true, nil
}

// UnlinkUndeliverable ends the tenant's live link in chatID after Telegram refused a send to it.
// A link made since in another chat is left alone, and a link already ended is not an error.
func (c *Chats) UnlinkUndeliverable(ctx context.Context, org, tenant uuid.UUID, chatID int64) error {
	now := c.clock.Now()
	err := c.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		_, err := dbgen.New(tx).UnlinkUndeliverableTelegramChat(ctx, dbgen.UnlinkUndeliverableTelegramChatParams{
			OrganizationID: org, TenantID: tenant, ChatID: chatID, UnlinkedAt: &now,
		})
		return err
	})
	if err != nil {
		return fmt.Errorf("telegramlink: ending the unreachable link: %w", err)
	}
	return nil
}
