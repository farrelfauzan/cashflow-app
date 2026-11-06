//go:build integration

package qris_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
)

// TestSettleClosesAKeptPromise: a paid QR that reaches what the tenant promised closes the promise
// as kept in the settle's transaction; a smaller one leaves it active.
func TestSettleClosesAKeptPromise(t *testing.T) {
	s := newSuite(t)
	ctx := context.Background()
	s.env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	f := s.newFlow(t, true)
	promises := promise.NewService(promise.Deps{Store: s.env.Store, Links: f.links, Tenants: s.env.Billing, Clock: s.env.Clock})
	r := f.rent(t, "Rina Wulandari")

	date := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	amount := int64(300_000)
	_, err := promises.MakeFromLink(ctx, r.token, promise.Input{
		Kind: promise.KindSpecificDate, PromisedDate: &date, PromisedAmount: &amount, PayingNow: 100_000,
	})
	require.NoError(t, err)
	status := func() promise.Promise {
		res, err := promises.List(ctx, f.admin, r.tenantID, listing.Page{Number: 1, Size: listing.MaxPageSize})
		require.NoError(t, err)
		require.Len(t, res.Items, 1)
		return res.Items[0]
	}

	// Paying now through a QR is not yet the target (Rp100.000 now plus Rp300.000 promised).
	f.makeQR(t, r, 100_000)
	first := f.only(t, r)
	require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, first.TransactionID, 100_000))
	require.Equal(t, promise.StatusActive, status().Status)

	s.env.Clock.Set(time.Date(2026, 11, 5, 19, 45, 0, 0, clock.Jakarta))
	f.makeQR(t, r, 300_000)
	var second attemptRow
	for _, a := range f.attempts(t, r) {
		if a.TransactionID != first.TransactionID {
			second = a
		}
	}
	require.NotEmpty(t, second.TransactionID)
	require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, second.TransactionID, 300_000))
	got := status()
	require.Equal(t, promise.StatusKept, got.Status)
	require.NotNil(t, got.ClosedAt)
	require.True(t, s.env.Clock.Now().Equal(*got.ClosedAt))

	t.Run("settling the same QR again changes nothing", func(t *testing.T) {
		s.env.Clock.Advance(time.Hour)
		require.NoError(t, f.svc.SettlePaid(ctx, f.org.ID, second.TransactionID, 300_000))
		again := status()
		require.Equal(t, promise.StatusKept, again.Status)
		require.True(t, got.ClosedAt.Equal(*again.ClosedAt))
	})
}
