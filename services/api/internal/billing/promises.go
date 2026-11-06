package billing

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ActivePromise is the newest version of a tenant's active payment promise, as staff see it on
// Detail penyewa and Penagihan.
type ActivePromise struct {
	ID             uuid.UUID // the promise
	Version        int
	Kind           string
	PromisedDate   time.Time // a calendar day, midnight UTC
	PromisedAmount *int64    // nil: everything owed after PayingNow
	PayingNow      int64
	Source         string
	Note           string
	// CreatedBy is the member who made the version; nil from the tenant page.
	CreatedBy *StaffRef
	CreatedAt time.Time
	// CoveredInvoiceIDs are the invoices the version covers, oldest period first.
	CoveredInvoiceIDs []uuid.UUID
}

// Covers reports whether the promise's newest version covers invoice.
func (a *ActivePromise) Covers(invoice uuid.UUID) bool {
	if a == nil {
		return false
	}
	for _, id := range a.CoveredInvoiceIDs {
		if id == invoice {
			return true
		}
	}
	return false
}

// PromiseDates are the days a promise made now may name, both included (Asia/Jakarta, midnight
// UTC).
type PromiseDates struct {
	Earliest, Latest time.Time
}

// TenantPromises is what Detail penyewa shows about a tenant's promises.
type TenantPromises struct {
	Active *ActivePromise // nil when the tenant has no active promise
	Broken int            // promises closed broken, ever
	// Dates is what a promise made now may say, nil when none can be made; NextInvoiceDueDate the
	// day a with_next_invoice promise would take, nil when that kind is not offered. Both are the
	// payment page's values.
	Dates              *PromiseDates
	NextInvoiceDueDate *time.Time
}

// TenantPromiseSummary is what Penagihan shows about the promises of a row's tenant.
type TenantPromiseSummary struct {
	Active *ActivePromise // nil when the tenant has no active promise
	Broken int
}

// PromiseReader reads tenants' payment promises for the staff views, inside the caller's
// transaction scoped to org. promise.Closer implements it; package promise imports billing, so
// billing reaches it only through this hook, as with PaymentHook. It checks no permission: the
// caller has authorized the read it rides on.
type PromiseReader interface {
	// TenantPromisesTx reads one tenant's promises; outstanding is what the tenant owes now and
	// today the current day in Asia/Jakarta.
	TenantPromisesTx(ctx context.Context, tx pgx.Tx, org, tenantID uuid.UUID, outstanding int64, today time.Time) (TenantPromises, error)
	// PromiseSummariesTx reads the promises of the given tenants, by tenant; a tenant with no
	// promise has no entry. leaseIDs, when not nil, keeps only the covered invoices of those
	// leases.
	PromiseSummariesTx(ctx context.Context, tx pgx.Tx, org uuid.UUID, tenantIDs, leaseIDs []uuid.UUID) (map[uuid.UUID]TenantPromiseSummary, error)
}

// Promises is the promise package's hook into billing: closing a promise a payment keeps, and
// reading promises for the staff views. promise.Closer implements it.
type Promises interface {
	PaymentHook
	PromiseReader
}
