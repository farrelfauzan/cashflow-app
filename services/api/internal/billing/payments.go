package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// errRepeat rolls back the transaction of a request repeated under its requestId: nothing was
// written but the audit row of its permission check, which must not commit without a change.
// The service answers the stored row as a success.
var errRepeat = errors.New("billing: repeated request")

// PaidInvoice is one invoice a payment holds money on, and what the invoice still owes now.
type PaidInvoice struct {
	InvoiceID   uuid.UUID
	Kind        string // InvoiceRegular or InvoiceFinal
	Period      time.Time
	PeriodEnd   time.Time
	DueDate     time.Time
	Amount      int64 // what this payment holds on it
	Outstanding int64
}

// PaymentResult is a recorded payment, what it paid and the tenant's totals after it.
type PaymentResult struct {
	Payment AccountPayment
	Paid    []PaidInvoice // oldest period first; empty when it all went to credit
	Totals
	// Created is false when the requestId had already recorded this payment.
	Created bool
}

// RecordPayment records a cash or transfer payment from a tenant and allocates it FIFO over
// all the tenant's invoices; what is left is credit. When the payment keeps the tenant's active
// promise, the PaymentHook closes it as kept in the same transaction. It needs payment:record
// (audited) with the amount, for the whole organization or through every lease the tenant has or
// had, since FIFO may pay any of them.
//
// The same requestId with the same input records nothing and returns the payment with Created
// false; with any other input it returns ErrRequestIDReused. Two identical requests at the same
// moment record one payment: the second waits on the first's unique key, then finds its row.
func (s *Service) RecordPayment(ctx context.Context, p authz.Principal, in PaymentInput) (PaymentResult, error) {
	in, err := ValidPayment(in, tenant.Today(s.clock))
	if err != nil {
		return PaymentResult{}, err
	}
	org := p.OrganizationID
	var out PaymentResult
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		leases, err := tenantLeases(ctx, q, org, in.TenantID, "tenantId")
		if err != nil {
			return err
		}
		amount := in.Amount
		req := authz.Request{Action: authz.ActionPaymentRecord, Resource: authz.Resource{
			Type: "tenant", ID: in.TenantID.String(), Amount: &amount,
		}}
		if err := s.authorizeThroughLeases(ctx, tx, p, req, leases); err != nil {
			return err
		}

		// The tenant's allocation lock before the insert, not only in Allocate after it: a promise
		// version made meanwhile marks the tenant's last payment seq under the same lock, so a
		// payment it does not see always gets a higher seq.
		if err := LockTenant(ctx, q, org, in.TenantID); err != nil {
			return err
		}
		row, created, err := insertManualPayment(ctx, q, org, p.UserID, in)
		if err != nil {
			return err
		}
		if created {
			if _, err := Allocate(ctx, tx, org, in.TenantID); err != nil {
				return err
			}
			if err := s.promises.PaymentRecorded(ctx, tx, org, in.TenantID); err != nil {
				return fmt.Errorf("closing the tenant's promise after a payment: %w", err)
			}
		}
		acct, err := loadAccount(ctx, q, org, in.TenantID, tenant.Today(s.clock))
		if err != nil {
			return err
		}
		if out, err = paymentResult(acct, row.ID); err != nil {
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
		return PaymentResult{}, err
	}
	return out, nil
}

// insertManualPayment inserts the payment under its requestId, or loads the one already stored
// under it and checks it is the same payment. It reports whether it inserted.
func insertManualPayment(ctx context.Context, q *dbgen.Queries, org, recordedBy uuid.UUID, in PaymentInput) (dbgen.Payment, bool, error) {
	ref := in.RequestID.String()
	var note *string
	if in.Note != "" {
		note = &in.Note
	}
	row, err := q.InsertPaymentIfNew(ctx, dbgen.InsertPaymentIfNewParams{
		ID: uuid.New(), OrganizationID: org, TenantID: in.TenantID, Amount: in.Amount, Method: in.Method,
		ReceivedAt: receivedAt(in.ReceivedOn), Source: SourceManual, SourceRef: &ref,
		RecordedByStaffUserID: uuid.NullUUID{UUID: recordedBy, Valid: true}, Note: note,
	})
	if err == nil {
		return row, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.Payment{}, false, fmt.Errorf("recording payment: %w", err)
	}
	row, err = q.GetPaymentBySourceRef(ctx, dbgen.GetPaymentBySourceRefParams{OrganizationID: org, Source: SourceManual, SourceRef: &ref})
	if err != nil {
		return dbgen.Payment{}, false, fmt.Errorf("loading the payment of a repeated request: %w", err)
	}
	if !samePayment(row, in) {
		return dbgen.Payment{}, false, ErrRequestIDReused
	}
	return row, false, nil
}

// samePayment reports whether a stored manual payment is the one in describes.
func samePayment(row dbgen.Payment, in PaymentInput) bool {
	note := ""
	if row.Note != nil {
		note = *row.Note
	}
	return row.TenantID == in.TenantID && row.Amount == in.Amount && row.Method == in.Method &&
		receivedOn(row.ReceivedAt).Equal(in.ReceivedOn) && note == in.Note
}

// paymentResult picks one payment out of its tenant's account, with the invoices it holds money
// on.
func paymentResult(acct Account, paymentID uuid.UUID) (PaymentResult, error) {
	out := PaymentResult{Totals: acct.Totals, Paid: []PaidInvoice{}}
	found := false
	for _, pay := range acct.Payments {
		if pay.ID == paymentID {
			out.Payment, found = pay, true
			break
		}
	}
	if !found {
		return PaymentResult{}, fmt.Errorf("payment %s is not in its tenant's account", paymentID)
	}
	invoices := make(map[uuid.UUID]AccountInvoice, len(acct.Invoices))
	for _, inv := range acct.Invoices {
		invoices[inv.ID] = inv
	}
	for _, a := range out.Payment.Allocations {
		inv, ok := invoices[a.InvoiceID]
		if a.Amount <= 0 || !ok {
			continue
		}
		out.Paid = append(out.Paid, PaidInvoice{
			InvoiceID: inv.ID, Kind: inv.Kind, Period: inv.Period, PeriodEnd: inv.PeriodEnd, DueDate: inv.DueDate,
			Amount: a.Amount, Outstanding: inv.Outstanding,
		})
	}
	return out, nil
}
