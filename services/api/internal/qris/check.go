package qris

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

const (
	// MaxChecksPerWindow caps the AutoGoPay status reads the tenant's "Saya sudah bayar" taps make
	// in CheckWindow. Past it a tap still marks the QR as being checked, and reconcile reads it.
	MaxChecksPerWindow = 5
	// CheckWindow is the window of MaxChecksPerWindow.
	CheckWindow = 10 * time.Minute
)

// Check is "Saya sudah bayar": the tenant behind token says they paid their latest QR. A pending
// attempt is marked as being checked (the page shows "Sedang dicek" from then on) and AutoGoPay is
// asked about it once: paid records the payment, expired or cancelled closes it, still pending
// leaves it for reconcile. An AutoGoPay failure, or too many checks, only skips the read: the
// tenant sees "Sedang dicek" and reconcile resolves it. An attempt that is no longer pending is
// answered as it is. No attempt is ErrNoAttempt.
func (s *Service) Check(ctx context.Context, token string) (PublicView, error) {
	org, linkID, err := s.links.Resolve(ctx, token)
	if err != nil {
		return PublicView{}, err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), opTimeout)
	defer cancel()

	var out PublicView
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		tenantID, _, err := s.links.OpenLink(ctx, tx, org, linkID)
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		if err := lockTenant(ctx, q, tenantID); err != nil {
			return err
		}
		now := s.clock.Now()
		a, err := latestAttempt(ctx, q, org, tenantID)
		if err != nil {
			return err
		}
		if a == nil {
			return ErrNoAttempt
		}
		if a.Status != AttemptPending {
			out = view(a, now)
			return nil
		}
		if _, err := q.RequestQrisCheck(ctx, dbgen.RequestQrisCheckParams{OrganizationID: org, ID: a.ID, Now: &now}); err != nil {
			return err
		}
		if a.CheckRequestedAt == nil {
			a.CheckRequestedAt = &now
		}
		out = view(a, now)
		if err := s.checkNow(ctx, tx, org, tenantID, a, now); err != nil {
			return err
		}
		out = view(a, now)
		return nil
	})
	if err != nil {
		return PublicView{}, err
	}
	return out, nil
}

// checkNow reads attempt a at AutoGoPay once and records the answer, unless the tenant has checked
// too often. Only a database failure is returned.
func (s *Service) checkNow(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, a *Attempt, now time.Time) error {
	ok, err := s.limiter.Allow(ctx, checkLimitKey(org, tenantID), MaxChecksPerWindow, CheckWindow)
	if err != nil || !ok {
		if err != nil {
			s.log.WarnContext(ctx, "counting QRIS checks; leaving the check to reconcile", "organization", org, "error", err)
		}
		return nil
	}
	key, err := s.apiKey(ctx, tx, org)
	if err != nil {
		return nil // logged; reconcile retries
	}
	o, err := s.ask(ctx, key, org, a, false, now)
	if errors.Is(err, ErrUnavailable) {
		return nil // logged; reconcile retries
	}
	if err != nil {
		return err
	}
	return s.record(ctx, tx, org, a, o)
}

func checkLimitKey(org, tenantID uuid.UUID) string {
	return "qris:check:" + org.String() + ":" + tenantID.String()
}
