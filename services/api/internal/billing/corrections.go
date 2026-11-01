package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// CorrectionResult is a correction and its tenant's totals after allocation reran.
type CorrectionResult struct {
	Correction CorrectionRecord
	TenantID   uuid.UUID
	Totals
	// Created is false when the requestId had already made this correction.
	Created bool
}

// CreateCorrection corrects what an invoice bills or what a payment received ("Koreksi
// nominal") with a new signed row, then reruns the tenant's allocation. The input names either
// the difference or the new amount; for a new amount the difference is taken from the current
// amount under the tenant lock, so a correction or payment saved meanwhile is never overwritten
// by a stale figure, and a new amount equal to the current one returns ErrCorrectionNoChange.
// It needs correction:create (audited) with the size of the change: on an invoice, in its
// lease's property and assigned to its collector; on a payment, through every lease its tenant
// has or had. A correction that would take the amount below zero returns
// ErrCorrectionBelowZero. requestId works as in RecordPayment; a repeat compares what was sent.
func (s *Service) CreateCorrection(ctx context.Context, p authz.Principal, in CorrectionInput) (CorrectionResult, error) {
	in, err := ValidCorrection(in)
	if err != nil {
		return CorrectionResult{}, err
	}
	org := p.OrganizationID
	var out CorrectionResult
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		target, err := loadCorrectionTarget(ctx, q, org, in)
		if err != nil {
			return err
		}
		// The tenant row is the last lock (see the lock order in generate.go). Taken before the
		// requestId lookup and the current amount, it makes a concurrent repeat wait and then
		// find the row, and keeps the current amount from changing before the insert.
		if _, err := q.LockTenantForAllocation(ctx, dbgen.LockTenantForAllocationParams{OrganizationID: org, TenantID: target.tenantID}); err != nil {
			return fmt.Errorf("locking tenant %s: %w", target.tenantID, err)
		}
		stored, found, same, err := correctionByRequestID(ctx, q, org, in)
		if err != nil {
			return err
		}
		delta := stored.AmountDelta
		if !found {
			if delta, err = correctionDelta(ctx, q, org, in); err != nil {
				return err
			}
		}
		if err := s.authorizeCorrection(ctx, tx, p, target, delta); err != nil {
			return err
		}
		row, created := stored, false
		switch {
		case found && !same:
			return ErrRequestIDReused
		case !found && delta == 0:
			return ErrCorrectionNoChange
		case !found:
			if row, created, err = insertCorrection(ctx, q, org, p.UserID, in, delta); err != nil {
				return err
			}
		}
		if created {
			if _, err := Allocate(ctx, tx, org, target.tenantID); err != nil {
				return err
			}
		}
		if out, err = correctionResult(ctx, q, org, target.tenantID, row, tenant.Today(s.clock)); err != nil {
			return err
		}
		out.Created = created
		if !created {
			return errRepeat
		}
		return nil
	})
	if errors.Is(err, errRepeat) {
		return out, nil
	}
	if err != nil {
		return CorrectionResult{}, err
	}
	return out, nil
}

// correctionTarget is the corrected invoice or payment as authorizing the change needs it.
type correctionTarget struct {
	tenantID uuid.UUID
	resource authz.Resource // without the amount, which is known once the tenant is locked
	leases   []leaseScope   // a payment's tenant's leases; nil for an invoice
}

// loadCorrectionTarget loads the corrected invoice or payment; a missing one is a
// *NotFoundError on its field.
func loadCorrectionTarget(ctx context.Context, q *dbgen.Queries, org uuid.UUID, in CorrectionInput) (correctionTarget, error) {
	if in.InvoiceID != uuid.Nil {
		inv, err := q.GetInvoiceScope(ctx, dbgen.GetInvoiceScopeParams{OrganizationID: org, ID: in.InvoiceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return correctionTarget{}, &NotFoundError{Field: "invoiceId"}
		}
		if err != nil {
			return correctionTarget{}, fmt.Errorf("loading invoice: %w", err)
		}
		return correctionTarget{tenantID: inv.TenantID, resource: authz.Resource{
			Type: "invoice", ID: inv.ID.String(), PropertyID: inv.PropertyID, OwnerID: inv.CollectorStaffUserID.UUID,
		}}, nil
	}
	pay, err := q.GetPayment(ctx, dbgen.GetPaymentParams{OrganizationID: org, ID: in.PaymentID})
	if errors.Is(err, pgx.ErrNoRows) {
		return correctionTarget{}, &NotFoundError{Field: "paymentId"}
	}
	if err != nil {
		return correctionTarget{}, fmt.Errorf("loading payment: %w", err)
	}
	leases, err := tenantLeases(ctx, q, org, pay.TenantID, "paymentId")
	if err != nil {
		return correctionTarget{}, err
	}
	return correctionTarget{tenantID: pay.TenantID, resource: authz.Resource{Type: "payment", ID: pay.ID.String()}, leases: leases}, nil
}

// authorizeCorrection authorizes correction:create on the target with the size of the change:
// an invoice in its lease's property, a payment through every lease of its tenant.
func (s *Service) authorizeCorrection(ctx context.Context, tx pgx.Tx, p authz.Principal, t correctionTarget, delta int64) error {
	size := delta
	if size < 0 {
		size = -size
	}
	res := t.resource
	res.Amount = &size
	req := authz.Request{Action: authz.ActionCorrectionCreate, Resource: res}
	if t.leases == nil {
		return s.authz.Authorize(ctx, tx, p, req)
	}
	return s.authorizeThroughLeases(ctx, tx, p, req, t.leases)
}

// correctionDelta is the signed change the input asks for: its AmountDelta, or its NewAmount
// minus the target's current amount (an invoice's billed amount, a payment's net amount), read
// under the tenant lock.
func correctionDelta(ctx context.Context, q *dbgen.Queries, org uuid.UUID, in CorrectionInput) (int64, error) {
	if in.AmountDelta != nil {
		return *in.AmountDelta, nil
	}
	var current int64
	var err error
	if in.InvoiceID != uuid.Nil {
		current, err = q.GetInvoiceBilled(ctx, dbgen.GetInvoiceBilledParams{OrganizationID: org, InvoiceID: in.InvoiceID})
	} else {
		current, err = q.GetPaymentNet(ctx, dbgen.GetPaymentNetParams{OrganizationID: org, ID: in.PaymentID})
	}
	if err != nil {
		return 0, fmt.Errorf("loading the current amount: %w", err)
	}
	return *in.NewAmount - current, nil
}

// insertCorrection inserts the correction with its delta under its requestId. It reports false
// with the stored row when a request for another tenant, which the tenant lock does not
// serialize, took the requestId after the lookup.
func insertCorrection(ctx context.Context, q *dbgen.Queries, org, createdBy uuid.UUID, in CorrectionInput, delta int64) (dbgen.Correction, bool, error) {
	row, err := q.InsertCorrectionIfNew(ctx, dbgen.InsertCorrectionIfNewParams{
		ID: uuid.New(), OrganizationID: org,
		InvoiceID:   uuid.NullUUID{UUID: in.InvoiceID, Valid: in.InvoiceID != uuid.Nil},
		PaymentID:   uuid.NullUUID{UUID: in.PaymentID, Valid: in.PaymentID != uuid.Nil},
		AmountDelta: delta, Reason: in.Reason, CreatedByStaffUserID: createdBy,
		RequestID: uuid.NullUUID{UUID: in.RequestID, Valid: true}, NewAmount: in.NewAmount,
	})
	switch {
	case err == nil:
		return row, true, nil
	case isCheckViolation(err, "correction_not_negative"):
		return dbgen.Correction{}, false, ErrCorrectionBelowZero
	case errors.Is(err, pgx.ErrNoRows):
		stored, found, same, err := correctionByRequestID(ctx, q, org, in)
		switch {
		case err != nil:
			return dbgen.Correction{}, false, err
		case !found:
			return dbgen.Correction{}, false, errors.New("a correction's requestId conflicts but is not found")
		case !same:
			return dbgen.Correction{}, false, ErrRequestIDReused
		}
		return stored, false, nil
	default:
		return dbgen.Correction{}, false, fmt.Errorf("writing correction: %w", err)
	}
}

// correctionByRequestID loads the correction stored under in's requestId, if any, and reports
// whether it is the one in describes: the same target and reason, and the same new amount or,
// for a request that named a difference, the same difference.
func correctionByRequestID(ctx context.Context, q *dbgen.Queries, org uuid.UUID, in CorrectionInput) (row dbgen.Correction, found, same bool, err error) {
	row, err = q.GetCorrectionByRequestID(ctx, dbgen.GetCorrectionByRequestIDParams{
		OrganizationID: org, RequestID: uuid.NullUUID{UUID: in.RequestID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Correction{}, false, false, nil
	}
	if err != nil {
		return dbgen.Correction{}, false, false, fmt.Errorf("loading the correction of a request: %w", err)
	}
	same = row.InvoiceID.UUID == in.InvoiceID && row.PaymentID.UUID == in.PaymentID && row.Reason == in.Reason
	switch {
	case in.NewAmount != nil:
		same = same && row.NewAmount != nil && *row.NewAmount == *in.NewAmount
	default:
		same = same && row.NewAmount == nil && row.AmountDelta == *in.AmountDelta
	}
	return row, true, same, nil
}

func correctionResult(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, row dbgen.Correction, today time.Time) (CorrectionResult, error) {
	ref, err := q.GetStaffRef(ctx, dbgen.GetStaffRefParams{OrganizationID: org, StaffUserID: row.CreatedByStaffUserID})
	if err != nil {
		return CorrectionResult{}, fmt.Errorf("loading the member who made a correction: %w", err)
	}
	totals, err := tenantTotals(ctx, q, org, tenantID, today)
	if err != nil {
		return CorrectionResult{}, err
	}
	return CorrectionResult{
		Correction: CorrectionRecord{
			ID: row.ID, InvoiceID: row.InvoiceID.UUID, PaymentID: row.PaymentID.UUID, AmountDelta: row.AmountDelta,
			Reason: row.Reason, CreatedBy: StaffRef{MemberID: ref.MemberID, Name: ref.Name}, CreatedAt: row.CreatedAt,
		},
		TenantID: tenantID, Totals: totals,
	}, nil
}

func isCheckViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23514" && pgErr.ConstraintName == constraint
}
