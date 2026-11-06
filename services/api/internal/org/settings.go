package org

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// The broken-promise threshold's bounds and default (decision 41.4): how many broken promises
// put a tenant on the karyawan visit list.
const (
	MinBrokenPromiseThreshold     = 1
	MaxBrokenPromiseThreshold     = 10
	DefaultBrokenPromiseThreshold = 2
)

var (
	// ErrThresholdOutOfRange means a broken-promise threshold outside 1 to 10.
	ErrThresholdOutOfRange = fmt.Errorf("org: the broken-promise threshold must be %d to %d",
		MinBrokenPromiseThreshold, MaxBrokenPromiseThreshold)
	// ErrOrganizationNotFound means no organization has that id.
	ErrOrganizationNotFound = errors.New("org: organization not found")
)

// settingsAuditAction is how changing an organization setting appears in the audit log.
const settingsAuditAction = "settings:update"

// SetBrokenPromiseThreshold sets how many broken promises put one of the organization's tenants on
// the karyawan visit list. It applies from the next promise that closes broken; open visit tasks
// stay as they are. It is the platform operator's action (cmd/admin) until the settings screen
// (W28), recorded in the organization's audit log with no acting member.
func (s *Service) SetBrokenPromiseThreshold(ctx context.Context, org uuid.UUID, n int) error {
	if n < MinBrokenPromiseThreshold || n > MaxBrokenPromiseThreshold {
		return ErrThresholdOutOfRange
	}
	if org == uuid.Nil {
		return ErrOrganizationNotFound
	}
	return s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		rows, err := dbgen.New(tx).SetBrokenPromiseThreshold(ctx, dbgen.SetBrokenPromiseThresholdParams{
			ID: org, Threshold: int16(n), //nolint:gosec // checked above to be 1 to 10
		})
		if err != nil {
			return fmt.Errorf("org: setting the broken-promise threshold: %w", err)
		}
		if rows == 0 {
			return ErrOrganizationNotFound
		}
		return s.authz.RecordSystemAction(ctx, tx, org, settingsAuditAction,
			authz.Resource{Type: "organization", ID: org.String()})
	})
}

// BrokenPromiseThreshold returns the organization's broken-promise threshold.
func (s *Service) BrokenPromiseThreshold(ctx context.Context, org uuid.UUID) (int, error) {
	var n int16
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).GetBrokenPromiseThreshold(ctx, org)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrOrganizationNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("org: reading the broken-promise threshold: %w", err)
	}
	return int(n), nil
}
