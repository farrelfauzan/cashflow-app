//go:build integration

package billing_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// TestFieldAppChecksActiveLeasesOnly (decision 47): the field app acts on a tenant through the
// tenant's active leases only. Budi rented a room Andi collects from 1 Nov 2026, moved out on
// 10 Nov and now rents a room Sari collects; the field app is used on 14 Nov 2026 (Asia/Jakarta).
// Budi still owes for the room he left, which the admin's paths keep counting.
func TestFieldAppChecksActiveLeasesOnly(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 1, 9, 0)
	w := newFieldWorld(t, env, "0812868", "0812-8680-0099")
	invite := func(phone, name string) authz.Principal {
		t.Helper()
		m, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: phone, Name: name, Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		return env.Principal(t, w.org.ID, m.UserID)
	}
	andi := invite("0812-8680-0097", "Andi Saputra")
	sari := invite("0812-8680-0096", "Sari Wulandari")

	budiBefore := w.collected(t, "Budi Santoso", day(2026, 11, 1), andi.MembershipID)
	wati := w.collected(t, "Wati Susanti", day(2026, 11, 1), andi.MembershipID) // moves out, rents nothing now
	indra := w.collected(t, "Indra Gunawan", day(2026, 11, 1), andi.MembershipID)
	w.generate(t)
	setNow(env, 2026, 11, 10, 9, 0)
	w.moveOut(t, budiBefore.renter, day(2026, 11, 10))
	w.moveOut(t, wati.renter, day(2026, 11, 10))
	budi := w.secondLeaseCollected(t, budiBefore.renter, day(2026, 11, 10), sari.MembershipID)
	w.secondLeaseCollected(t, indra.renter, day(2026, 11, 10), sari.MembershipID) // Indra rents a second room, Sari's
	w.generate(t)
	setNow(env, 2026, 11, 14, 9, 0)

	promises := promise.NewService(promise.Deps{Store: env.Store, Tenants: w.billing, Clock: env.Clock})
	ph := newPhotos(w)
	fieldNames := func(t *testing.T, p authz.Principal) []string {
		t.Helper()
		items, err := listing.All(func(page listing.Page) (listing.Result[billing.FieldTenant], error) {
			return w.billing.FieldToday(ctx, p, page)
		})
		require.NoError(t, err)
		out := []string{}
		for _, it := range items {
			out = append(out, it.TenantName)
		}
		return out
	}

	t.Run("the new collector lists the moved tenant and the old one does not", func(t *testing.T) {
		require.Equal(t, []string{"Budi Santoso"}, fieldNames(t, sari), "not Indra: one of his rooms is Andi's")
		require.Empty(t, fieldNames(t, andi), "Budi and Wati left his rooms; Indra also rents Sari's")
	})

	var sariCash uuid.UUID
	t.Run("the new collector records cash, its photo and a promise", func(t *testing.T) {
		up := ph.receipt(t, sari, budi, cleanPhoto)
		_, err := ph.uploads.Get(ctx, sari, up)
		require.NoError(t, err)
		allowed := w.audited(t, authz.ActionCashRecord, "allow")
		res, err := w.billing.RecordCashPayment(ctx, sari,
			billing.CashInput{RequestID: uuid.New(), TenantID: budi.tenantID, Amount: 500_000, PhotoUploadID: up})
		require.NoError(t, err)
		require.True(t, res.Created)
		sariCash = res.Payment.ID
		require.EqualValues(t, 500_000, res.Paid[0].Amount, "FIFO still pays the room he left first")
		require.Equal(t, allowed+1, w.audited(t, authz.ActionCashRecord, "allow"))
		require.Equal(t, billing.ReceiptPhoto{UploadID: up, Status: upload.StatusPending}, res.Photo)

		p, err := promises.MakeFromField(ctx, sari, budi.tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptrTo(day(2026, 11, 20))})
		require.NoError(t, err)
		require.Equal(t, promise.SourceKaryawan, p.Latest().Source)
	})

	t.Run("the old collector can do none of it, and each refusal is audited", func(t *testing.T) {
		payments, links := w.count(t, "payment"), w.count(t, "payment_receipt_photo")
		sariUpload := ph.receipt(t, sari, budi, cleanPhoto)
		denied := w.denials(t)
		for _, tc := range []struct {
			name   string
			action string
			do     func() error
		}{
			{name: "record cash", action: authz.ActionCashRecord, do: func() error {
				_, err := w.billing.RecordCashPayment(ctx, andi, w.cashIn(t, andi, budi, 100_000))
				return err
			}},
			{name: "start a receipt photo upload", action: authz.ActionCashRecord, do: func() error {
				_, err := ph.uploads.Create(ctx, andi, upload.CreateInput{
					Purpose: upload.PurposeCashReceipt, TenantID: budi.tenantID, ContentType: "image/jpeg", Size: int64(len(cleanPhoto)),
				})
				return err
			}},
			{name: "read the new collector's receipt photo", action: authz.ActionCashRecord, do: func() error {
				_, err := ph.uploads.Get(ctx, andi, sariUpload)
				return err
			}},
			{name: "attach a photo to the new collector's cash", action: authz.ActionCashRecord, do: func() error {
				_, _, err := w.billing.AttachReceiptPhoto(ctx, andi, sariCash, sariUpload)
				return err
			}},
			{name: "make a field promise", action: authz.ActionPromiseCreate, do: func() error {
				_, err := promises.MakeFromField(ctx, andi, budi.tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptrTo(day(2026, 11, 21))})
				return err
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := w.audited(t, tc.action, "deny")
				require.ErrorIs(t, tc.do(), authz.ErrForbidden)
				require.Equal(t, before+1, w.audited(t, tc.action, "deny"), "audited as "+tc.action)
			})
		}
		require.Equal(t, denied+5, w.denials(t))
		require.Equal(t, payments, w.count(t, "payment"))
		require.Equal(t, links, w.count(t, "payment_receipt_photo"))
	})

	t.Run("the admin still records payments through every lease, ended ones included", func(t *testing.T) {
		res := w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: budi.tenantID, Amount: 50_000, Method: billing.MethodTransfer, ReceivedOn: day(2026, 11, 14)})
		require.True(t, res.Created)
		w.record(t, billing.PaymentInput{RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 50_000, Method: billing.MethodTransfer, ReceivedOn: day(2026, 11, 14)})
	})

	t.Run("a karyawan whose only lease with the tenant ended cannot record cash", func(t *testing.T) {
		for _, p := range []authz.Principal{andi, sari, w.karyawan} {
			_, err := w.billing.RecordCashPayment(ctx, p, w.cashIn(t, p, wati, 50_000))
			require.ErrorIs(t, err, authz.ErrForbidden, "a tenant with no active lease needs the organization-wide grant")
		}
	})

	t.Run("a tenant with two active leases of different collectors is neither's", func(t *testing.T) {
		require.NotContains(t, fieldNames(t, andi), "Indra Gunawan")
		require.NotContains(t, fieldNames(t, sari), "Indra Gunawan")
		for _, p := range []authz.Principal{andi, sari} {
			_, err := w.billing.RecordCashPayment(ctx, p, w.cashIn(t, p, indra, 50_000))
			require.ErrorIs(t, err, authz.ErrForbidden)
		}
	})
}

// secondLeaseCollected gives r's tenant a monthly lease of a new room in the world's kos from
// start, collected by the member collector, saved now (the fake clock).
func (w fieldWorld) secondLeaseCollected(t *testing.T, r renter, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: fmt.Sprintf("Kamar %d", w.n), Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: r.tenantID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: start, CollectorMemberID: collector},
	})
	require.NoError(t, err)
	return rental{renter: renter{tenantID: r.tenantID, leaseID: l.ID}, unitID: unit.ID}
}

// denials counts the world's denied audit rows, whatever the action.
func (w fieldWorld) denials(t *testing.T) int {
	t.Helper()
	n := 0
	for _, row := range w.env.AuditLog(t, w.org.ID) {
		if row.Decision == "deny" {
			n++
		}
	}
	return n
}
