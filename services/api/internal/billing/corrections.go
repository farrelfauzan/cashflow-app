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
// nominal") with a new signed row, then reruns the tenant's allocation. It needs
// correction:create (audited) with the size of the change: on an invoice, in its lease's property
// and assigned to its collector; on a payment, for the whole organization or through every lease
// its tenant has or had. A correction that would take the amount below zero returns
// ErrCorrectionBelowZero. requestId works as in RecordPayment.
func (s *Service) CreateCorrection(ctx context.Context, p authz.Principal, in CorrectionInput) (CorrectionResult, error) {
	in, err := ValidCorrection(in)
	if err != nil {
		return CorrectionResult{}, err
	}
	org := p.OrganizationID
	var out CorrectionResult
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		tenantID, err := s.authorizeCorrection(ctx, tx, q, p, in)
		if err != nil {
			return err
		}
		// The tenant row is the last lock (see the lock order in generate.go); taking it before
		// looking the requestId up makes a concurrent repeat wait here and then find the row,
		// rather than fail the not-negative trigger against the first one's change.
		if _, err := q.LockTenantForAllocation(ctx, dbgen.LockTenantForAllocationParams{OrganizationID: org, TenantID: tenantID}); err != nil {
			return fmt.Errorf("locking tenant %s: %w", tenantID, err)
		}
		row, created, err := insertCorrection(ctx, q, org, p.UserID, in)
		if err != nil {
			return err
		}
		if created {
			if _, err := Allocate(ctx, tx, org, tenantID); err != nil {
				return err
			}
		}
		if out, err = correctionResult(ctx, q, org, tenantID, row, tenant.Today(s.clock)); err != nil {
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

// authorizeCorrection loads the corrected invoice or payment, authorizes correction:create on
// it and returns its tenant.
func (s *Service) authorizeCorrection(ctx context.Context, tx pgx.Tx, q *dbgen.Queries, p authz.Principal, in CorrectionInput) (uuid.UUID, error) {
	org := p.OrganizationID
	size := in.AmountDelta
	if size < 0 {
		size = -size
	}
	if in.InvoiceID != uuid.Nil {
		inv, err := q.GetInvoiceScope(ctx, dbgen.GetInvoiceScopeParams{OrganizationID: org, ID: in.InvoiceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, &NotFoundError{Field: "invoiceId"}
		}
		if err != nil {
			return uuid.Nil, fmt.Errorf("loading invoice: %w", err)
		}
		req := authz.Request{Action: authz.ActionCorrectionCreate, Resource: authz.Resource{
			Type: "invoice", ID: inv.ID.String(), PropertyID: inv.PropertyID, OwnerID: inv.CollectorStaffUserID.UUID, Amount: &size,
		}}
		return inv.TenantID, s.authz.Authorize(ctx, tx, p, req)
	}
	pay, err := q.GetPayment(ctx, dbgen.GetPaymentParams{OrganizationID: org, ID: in.PaymentID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, &NotFoundError{Field: "paymentId"}
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("loading payment: %w", err)
	}
	leases, err := tenantLeases(ctx, q, org, pay.TenantID, "paymentId")
	if err != nil {
		return uuid.Nil, err
	}
	req := authz.Request{Action: authz.ActionCorrectionCreate, Resource: authz.Resource{
		Type: "payment", ID: pay.ID.String(), Amount: &size,
	}}
	return pay.TenantID, s.authorizeThroughLeases(ctx, tx, p, req, leases)
}

// insertCorrection inserts the correction under its requestId, or loads the one already stored
// under it and checks it is the same correction. It reports whether it inserted.
func insertCorrection(ctx context.Context, q *dbgen.Queries, org, createdBy uuid.UUID, in CorrectionInput) (dbgen.Correction, bool, error) {
	requestID := uuid.NullUUID{UUID: in.RequestID, Valid: true}
	// Looked up first: the trigger that refuses a correction below zero runs before the unique
	// key, so a repeat of a correction to zero would otherwise fail it.
	if row, ok, err := correctionByRequestID(ctx, q, org, in); ok || err != nil {
		return row, false, err
	}
	row, err := q.InsertCorrectionIfNew(ctx, dbgen.InsertCorrectionIfNewParams{
		ID: uuid.New(), OrganizationID: org,
		InvoiceID:   uuid.NullUUID{UUID: in.InvoiceID, Valid: in.InvoiceID != uuid.Nil},
		PaymentID:   uuid.NullUUID{UUID: in.PaymentID, Valid: in.PaymentID != uuid.Nil},
		AmountDelta: in.AmountDelta, Reason: in.Reason, CreatedByStaffUserID: createdBy, RequestID: requestID,
	})
	switch {
	case err == nil:
		return row, true, nil
	case isCheckViolation(err, "correction_not_negative"):
		return dbgen.Correction{}, false, ErrCorrectionBelowZero
	case errors.Is(err, pgx.ErrNoRows):
		// A request for another tenant, which the tenant lock does not serialize, took the
		// requestId after the lookup.
		row, ok, err := correctionByRequestID(ctx, q, org, in)
		if err == nil && !ok {
			err = errors.New("a correction's requestId conflicts but is not found")
		}
		return row, false, err
	default:
		return dbgen.Correction{}, false, fmt.Errorf("writing correction: %w", err)
	}
}

// correctionByRequestID loads the correction stored under in's requestId: ok is false when there
// is none, and the error is ErrRequestIDReused when it is not the one in describes.
func correctionByRequestID(ctx context.Context, q *dbgen.Queries, org uuid.UUID, in CorrectionInput) (dbgen.Correction, bool, error) {
	row, err := q.GetCorrectionByRequestID(ctx, dbgen.GetCorrectionByRequestIDParams{
		OrganizationID: org, RequestID: uuid.NullUUID{UUID: in.RequestID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Correction{}, false, nil
	}
	if err != nil {
		return dbgen.Correction{}, false, fmt.Errorf("loading the correction of a request: %w", err)
	}
	same := row.InvoiceID.UUID == in.InvoiceID && row.PaymentID.UUID == in.PaymentID &&
		row.AmountDelta == in.AmountDelta && row.Reason == in.Reason
	if !same {
		return dbgen.Correction{}, false, ErrRequestIDReused
	}
	return row, true, nil
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
