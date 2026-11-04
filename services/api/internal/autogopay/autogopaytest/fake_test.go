package autogopaytest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay/autogopaytest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
)

// qrisGateway is what a consumer of the client will declare. The real client and the fake must
// both satisfy it.
type qrisGateway interface {
	Generate(ctx context.Context, key secret.Value, amount int64) (autogopay.QRIS, error)
	Status(ctx context.Context, key secret.Value, transactionID string) (autogopay.Transaction, error)
	Cancel(ctx context.Context, key secret.Value, transactionID string) (autogopay.Transaction, error)
}

var (
	_ qrisGateway = (*autogopay.Client)(nil)
	_ qrisGateway = (*autogopaytest.Fake)(nil)
)

var (
	keyA = secret.NewValue("org-a-test-key")
	keyB = secret.NewValue("org-b-test-key")
	// 11:39:49 WIB on 4 Nov 2026.
	start = time.Date(2026, 11, 4, 11, 39, 49, 0, clock.Jakarta)
)

func newFake(t *testing.T) (*autogopaytest.Fake, *clock.Fake) {
	t.Helper()
	clk := clock.NewFake(start)
	return autogopaytest.New(clk), clk
}

func TestGenerate(t *testing.T) {
	t.Parallel()
	f, _ := newFake(t)
	q1, err := f.Generate(t.Context(), keyA, 150_000)
	require.NoError(t, err)
	q2, err := f.Generate(t.Context(), keyB, 75_000)
	require.NoError(t, err)

	require.Equal(t, "00000000-0000-4000-8000-000000000001", q1.TransactionID)
	require.Equal(t, "00000000-0000-4000-8000-000000000002", q2.TransactionID)
	require.Equal(t, autogopay.StatusPending, q1.Status)
	require.Equal(t, int64(150_000), q1.Amount)
	require.NotEmpty(t, q1.QRString)
	require.True(t, q1.CreatedAt.Equal(start))
	require.Equal(t, clock.Jakarta, q1.CreatedAt.Location())
	require.Equal(t, autogopaytest.QRValidity, q1.ExpiresAt.Sub(q1.CreatedAt))

	require.Equal(t, []autogopaytest.Call{
		{Method: autogopaytest.MethodGenerate, Key: "org-a-test-key", Amount: 150_000},
		{Method: autogopaytest.MethodGenerate, Key: "org-b-test-key", Amount: 75_000},
	}, f.Calls())
}

func TestRefusalsMatchTheClient(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		call    func(*autogopaytest.Fake) error
		wantErr error
	}{
		{name: "amount 0", wantErr: autogopay.ErrAmountOutOfRange, call: func(f *autogopaytest.Fake) error {
			_, err := f.Generate(context.Background(), keyA, 0)
			return err
		}},
		{name: "amount above the limit", wantErr: autogopay.ErrAmountOutOfRange, call: func(f *autogopaytest.Fake) error {
			_, err := f.Generate(context.Background(), keyA, autogopay.MaxAmount+1)
			return err
		}},
		{name: "no key", wantErr: autogopay.ErrInvalidAPIKey, call: func(f *autogopaytest.Fake) error {
			_, err := f.Status(context.Background(), secret.Value{}, "x")
			return err
		}},
		{name: "empty id", wantErr: autogopay.ErrInvalidTransactionID, call: func(f *autogopaytest.Fake) error {
			_, err := f.Cancel(context.Background(), keyA, "")
			return err
		}},
		{name: "unknown id", wantErr: autogopay.ErrTransactionNotFound, call: func(f *autogopaytest.Fake) error {
			_, err := f.Status(context.Background(), keyA, "00000000-0000-4000-8000-0000000000ff")
			return err
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, _ := newFake(t)
			require.ErrorIs(t, tc.call(f), tc.wantErr)
		})
	}
}

func TestLifecycle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		act        func(*testing.T, *autogopaytest.Fake, *clock.Fake, string)
		cancel     bool
		wantStatus autogopay.TransactionStatus
	}{
		{name: "new QR is pending", act: func(*testing.T, *autogopaytest.Fake, *clock.Fake, string) {}, wantStatus: autogopay.StatusPending},
		{name: "paid QR is paid", act: func(t *testing.T, f *autogopaytest.Fake, _ *clock.Fake, id string) { must(t, f.Pay(id)) },
			wantStatus: autogopay.StatusPaid},
		{name: "QR expires after 15 minutes", act: func(_ *testing.T, _ *autogopaytest.Fake, c *clock.Fake, _ string) { c.Advance(15 * time.Minute) },
			wantStatus: autogopay.StatusExpired},
		{name: "QR is still pending just before expiry", act: func(_ *testing.T, _ *autogopaytest.Fake, c *clock.Fake, _ string) {
			c.Advance(15*time.Minute - time.Second)
		},
			wantStatus: autogopay.StatusPending},
		{name: "Expire forces expiry", act: func(t *testing.T, f *autogopaytest.Fake, _ *clock.Fake, id string) { must(t, f.Expire(id)) },
			wantStatus: autogopay.StatusExpired},
		{name: "cancel of a pending QR cancels it", act: func(*testing.T, *autogopaytest.Fake, *clock.Fake, string) {}, cancel: true,
			wantStatus: autogopay.StatusCancelled},
		{name: "cancel of a paid QR reports paid", act: func(t *testing.T, f *autogopaytest.Fake, _ *clock.Fake, id string) { must(t, f.Pay(id)) },
			cancel: true, wantStatus: autogopay.StatusPaid},
		{name: "cancel of an expired QR reports expired", act: func(_ *testing.T, _ *autogopaytest.Fake, c *clock.Fake, _ string) { c.Advance(time.Hour) },
			cancel: true, wantStatus: autogopay.StatusExpired},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, clk := newFake(t)
			q, err := f.Generate(t.Context(), keyA, 50_000)
			require.NoError(t, err)
			tc.act(t, f, clk, q.TransactionID)

			call := f.Status
			if tc.cancel {
				call = f.Cancel
			}
			got, err := call(t.Context(), keyA, q.TransactionID)
			require.NoError(t, err)
			require.Equal(t, autogopay.Transaction{ID: q.TransactionID, Status: tc.wantStatus, Time: q.CreatedAt}, got)

			// The state sticks: a later status reads the same.
			again, err := f.Status(t.Context(), keyA, q.TransactionID)
			require.NoError(t, err)
			require.Equal(t, tc.wantStatus, again.Status)
		})
	}
}

func TestFailNext(t *testing.T) {
	t.Parallel()
	f, _ := newFake(t)
	f.FailNext(autogopaytest.MethodGenerate, autogopay.ErrUnavailable)

	_, err := f.Generate(t.Context(), keyA, 10_000)
	require.ErrorIs(t, err, autogopay.ErrUnavailable)
	q, err := f.Generate(t.Context(), keyA, 10_000)
	require.NoError(t, err, "the failure happens once")
	require.Equal(t, "00000000-0000-4000-8000-000000000001", q.TransactionID, "a failed generate makes no QR")

	f.FailNext(autogopaytest.MethodStatus, autogopay.ErrRejected)
	_, err = f.Cancel(t.Context(), keyA, q.TransactionID)
	require.NoError(t, err, "a failure for status does not hit cancel")
	_, err = f.Status(t.Context(), keyA, q.TransactionID)
	require.ErrorIs(t, err, autogopay.ErrRejected)

	require.Len(t, f.Calls(), 4, "failed calls are recorded too")
}

func TestPayAndExpireUnknownID(t *testing.T) {
	t.Parallel()
	f, _ := newFake(t)
	require.ErrorIs(t, f.Pay("nope"), autogopaytest.ErrUnknownTransaction)
	require.ErrorIs(t, f.Expire("nope"), autogopaytest.ErrUnknownTransaction)
}

func must(t *testing.T, err error) {
	t.Helper()
	require.NoError(t, err)
}
