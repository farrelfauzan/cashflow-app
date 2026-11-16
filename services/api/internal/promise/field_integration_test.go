//go:build integration

package promise_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

// rentCollected is rent for a tenant whose lease the member collector collects, with no link.
func (w *world) rentCollected(t *testing.T, tenantName string, collector uuid.UUID) renter {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{
		Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(700_000),
	})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: tenantName, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	_, err = w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: tenant.Today(w.env.Clock), CollectorMemberID: collector},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	return renter{tenantID: tn.ID}
}

func TestMakeFromField(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(time.November, 14, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812966")
	karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	own := w.rentCollected(t, "Wati Susanti", karyawan.MembershipID)
	notOwn := w.rentCollected(t, "Bambang Wijaya", uuid.Nil)
	audited := func(decision string) int {
		n := 0
		for _, row := range env.AuditLog(t, w.org.ID) {
			if row.Action == authz.ActionPromiseCreate && row.Decision == decision {
				n++
			}
		}
		return n
	}

	t.Run("a karyawan's promise from the field is stored with source karyawan", func(t *testing.T) {
		allowed := audited("allow")
		in := onDay(day(2026, 11, 20))
		in.Note = "Gajian tanggal 20"
		p, err := w.promises.MakeFromField(ctx, karyawan, own.tenantID, in)
		require.NoError(t, err)
		v := p.Latest()
		require.Equal(t, promise.SourceKaryawan, v.Source)
		require.Equal(t, "Gajian tanggal 20", v.Note)
		require.NotNil(t, v.CreatedBy)
		require.Equal(t, karyawan.MembershipID, v.CreatedBy.MemberID)
		require.Equal(t, allowed+1, audited("allow"))
		require.Equal(t, promise.SourceKaryawan, w.latest(t, own).Latest().Source, "as stored")
	})

	t.Run("an edit from the field is the next version, still karyawan", func(t *testing.T) {
		p, err := w.promises.MakeFromField(ctx, karyawan, own.tenantID, onDay(day(2026, 11, 22)))
		require.NoError(t, err)
		require.Len(t, p.Versions, 2)
		require.Equal(t, promise.SourceKaryawan, p.Latest().Source)
	})

	t.Run("only for a tenant the karyawan collects from; the refusal is audited", func(t *testing.T) {
		denied := audited("deny")
		_, err := w.promises.MakeFromField(ctx, karyawan, notOwn.tenantID, onDay(day(2026, 11, 20)))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, denied+1, audited("deny"))
		require.Empty(t, w.list(t, notOwn))
	})

	t.Run("MakeForTenant still records admin", func(t *testing.T) {
		p, err := w.promises.MakeForTenant(ctx, w.admin, notOwn.tenantID, onDay(day(2026, 11, 20)))
		require.NoError(t, err)
		require.Equal(t, promise.SourceAdmin, p.Latest().Source)
	})
}

// TestCashKeepsAPromise: cash a karyawan records runs the promise hook like recordPayment, so a
// payment that keeps the tenant's promise closes it as kept at once, and a repeated request
// counts once.
func TestCashKeepsAPromise(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(time.November, 14, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812967")
	karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	wati := w.rentCollected(t, "Wati Susanti", karyawan.MembershipID)
	_, err := w.promises.MakeFromField(ctx, karyawan, wati.tenantID, amountOn(day(2026, 11, 20), 300_000, 0))
	require.NoError(t, err)

	in := billing.CashInput{RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 200_000,
		PhotoUploadID: env.CashPhoto(t, w.org.ID, karyawan.UserID, wati.tenantID)}
	for range 2 {
		_, err := env.Billing.RecordCashPayment(ctx, karyawan, in)
		require.NoError(t, err)
	}
	w.requireStatus(t, wati, promise.StatusActive, time.Time{})

	env.Clock.Set(wib(time.November, 15, 10, 30))
	_, err = env.Billing.RecordCashPayment(ctx, karyawan, billing.CashInput{RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 100_000,
		PhotoUploadID: env.CashPhoto(t, w.org.ID, karyawan.UserID, wati.tenantID)})
	require.NoError(t, err)
	w.requireStatus(t, wati, promise.StatusKept, wib(time.November, 15, 10, 30))
}

// TestFieldPromiseOffer: what "Catat janji bayar" shows before it saves is what Detail penyewa
// shows the admin, for a tenant the karyawan collects from, and nothing for anyone else.
func TestFieldPromiseOffer(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(time.November, 14, 9, 0))
	w := newWorld(t, env, "Griya Janji", "0812969")
	karyawan := env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	own := w.rentCollected(t, "Wati Susanti", karyawan.MembershipID)
	notOwn := w.rentCollected(t, "Bambang Wijaya", uuid.Nil)
	asAdmin := func(t *testing.T, r renter) billing.TenantPromises {
		t.Helper()
		acct, err := env.Billing.Account(ctx, w.admin, r.tenantID)
		require.NoError(t, err)
		return acct.Promises
	}
	readDenials := func() int {
		n := 0
		for _, row := range env.AuditLog(t, w.org.ID) {
			if row.Action == authz.ActionInvoiceRead && row.Decision == "deny" {
				n++
			}
		}
		return n
	}

	t.Run("before any promise: the dates and the next due date, as the admin sees them", func(t *testing.T) {
		got, err := env.Billing.FieldPromiseOffer(ctx, karyawan, own.tenantID)
		require.NoError(t, err)
		require.Nil(t, got.Active)
		require.NotNil(t, got.Dates)
		require.Equal(t, day(2026, 11, 15), got.Dates.Earliest, "tomorrow")
		require.NotNil(t, got.NextInvoiceDueDate, "the monthly lease bills again on 14 Dec")
		want := asAdmin(t, own)
		require.Equal(t, want.Dates, got.Dates)
		require.Equal(t, want.NextInvoiceDueDate, got.NextInvoiceDueDate)
	})

	t.Run("with an active promise: its newest version, which saving replaces", func(t *testing.T) {
		_, err := w.promises.MakeFromField(ctx, karyawan, own.tenantID, onDay(day(2026, 11, 20)))
		require.NoError(t, err)
		got, err := env.Billing.FieldPromiseOffer(ctx, karyawan, own.tenantID)
		require.NoError(t, err)
		require.NotNil(t, got.Active)
		require.Equal(t, 1, got.Active.Version)
		require.Equal(t, day(2026, 11, 20), got.Active.PromisedDate)
		want := asAdmin(t, own)
		require.Equal(t, want.Active, got.Active)
		require.Equal(t, want.Dates, got.Dates)
		require.Equal(t, want.NextInvoiceDueDate, got.NextInvoiceDueDate)
	})

	t.Run("not for a tenant the karyawan does not collect from; the refusal is audited", func(t *testing.T) {
		denied := readDenials()
		_, err := env.Billing.FieldPromiseOffer(ctx, karyawan, notOwn.tenantID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, denied+1, readDenials())
	})

	t.Run("a tenant not in the organization is not found", func(t *testing.T) {
		_, err := env.Billing.FieldPromiseOffer(ctx, karyawan, uuid.New())
		var nf *billing.NotFoundError
		require.ErrorAs(t, err, &nf)
	})
}
