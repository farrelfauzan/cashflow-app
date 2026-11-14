//go:build integration

package deposit_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// setNow moves the fake clock to a moment in Jakarta.
func setNow(env *testenv.Env, y int, m time.Month, dd, hour, minute int) {
	env.Clock.Set(time.Date(y, m, dd, hour, minute, 0, 0, clock.Jakarta))
}

// world is one organization with a kos, its built-in members, a second karyawan (Dina), the
// deposit service and photo uploads.
type world struct {
	env        *testenv.Env
	org        testenv.Org
	admin      authz.Principal
	karyawan   authz.Principal
	dina       authz.Principal
	supervisor authz.Principal
	kos        property.Property
	deposits   *deposit.Service
	photos     photos
	prefix     string
	n          int
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	w.supervisor = env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	dina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: prefix + "099", Name: "Dina Kasir",
		Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	w.dina = env.Principal(t, w.org.ID, dina.UserID)
	w.kos, err = env.Properties.Create(ctx, w.admin, property.PropertyInput{
		Name: "Kos Melati", Kind: property.KindKos, Address: "Jl. Kenanga No. 12, Depok",
	})
	require.NoError(t, err)
	w.deposits = deposit.NewService(env.Store, env.Authz, env.Clock, env.Log)
	w.photos = newPhotos(w)
	return w
}

// invite adds a member with the given built-in roles and returns their principal.
func (w *world) invite(t *testing.T, phone, name string, roles ...string) authz.Principal {
	t.Helper()
	grants := make([]member.RoleGrant, len(roles))
	for i, r := range roles {
		grants[i] = member.RoleGrant{Role: r}
	}
	m, err := w.env.Members.Invite(context.Background(), w.admin, member.Invite{Phone: phone, Name: name, Roles: grants})
	require.NoError(t, err)
	return w.env.Principal(t, w.org.ID, m.UserID)
}

// rental is a tenant renting their own room in the world's kos.
type rental struct {
	tenantID uuid.UUID
	label    string
}

// rent lets a new room to a new tenant monthly from today at Rp700.000, collected by collector
// (a membership id), and generates its first invoice.
func (w *world) rent(t *testing.T, name string, collector authz.Principal) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	label := fmt.Sprintf("Kamar %d", w.n)
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	_, err = w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(w.env.Clock), CollectorMemberID: collector.MembershipID},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	return rental{tenantID: tn.ID, label: label}
}

// cash records amount of field cash from r taken by p, now.
func (w *world) cash(t *testing.T, p authz.Principal, r rental, amount int64) billing.CashResult {
	t.Helper()
	res, err := w.env.Billing.RecordCashPayment(context.Background(), p, billing.CashInput{
		RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount,
	})
	require.NoError(t, err)
	return res
}

// held is the field cash p holds.
func (w *world) held(t *testing.T, p authz.Principal) billing.CashTotal {
	t.Helper()
	got, err := w.env.Billing.FieldCash(context.Background(), p)
	require.NoError(t, err)
	return got
}

// handOver is a deposit of everything p holds now, with a fresh requestId.
func (w *world) handOver(t *testing.T, p authz.Principal, amount int64, photo uuid.UUID) deposit.Input {
	t.Helper()
	h := w.held(t, p)
	return deposit.Input{
		RequestID: uuid.New(), ExpectedAmount: h.Amount, ExpectedCount: h.Count, Amount: amount, PhotoUploadID: photo,
	}
}

// record records in as p and requires a new deposit.
func (w *world) record(t *testing.T, p authz.Principal, in deposit.Input) deposit.Deposit {
	t.Helper()
	d, created, err := w.deposits.Record(context.Background(), p, in)
	require.NoError(t, err)
	require.True(t, created)
	return d
}

// confirm confirms deposit id as p with received and requires a new confirmation.
func (w *world) confirm(t *testing.T, p authz.Principal, id uuid.UUID, received int64, note string) deposit.Deposit {
	t.Helper()
	d, created, err := w.deposits.Confirm(context.Background(), p, id, deposit.ConfirmInput{ReceivedAmount: received, Note: note})
	require.NoError(t, err)
	require.True(t, created)
	return d
}

// inTx runs fn in the world's organization scope.
func (w *world) inTx(fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx, dbgen.New(tx)) })
}

func (w *world) mustTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) {
	t.Helper()
	require.NoError(t, w.inTx(fn))
}

// count counts the rows of a table in the world's organization scope.
func (w *world) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		return tx.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&n)
	})
	return n
}

// audited counts the world's audit rows for one action and decision.
func (w *world) audited(t *testing.T, action, decision string) int {
	t.Helper()
	var n int
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM audit_log WHERE action = $1 AND decision = $2", action, decision).Scan(&n)
	})
	return n
}

// covered lists the payments deposit id covers.
func (w *world) covered(t *testing.T, id uuid.UUID) []uuid.UUID {
	t.Helper()
	var out []uuid.UUID
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		rows, err := tx.Query(ctx, "SELECT payment_id FROM cash_deposit_payment WHERE deposit_id = $1", id)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		return err
	})
	return out
}

// rowCounts are the deposit tables' row counts.
type rowCounts struct{ deposits, payments, confirmations int }

func (w *world) rows(t *testing.T) rowCounts {
	t.Helper()
	return rowCounts{
		deposits:      w.count(t, "cash_deposit"),
		payments:      w.count(t, "cash_deposit_payment"),
		confirmations: w.count(t, "cash_deposit_confirmation"),
	}
}

// requireCode asserts err is a Postgres error with the given SQLSTATE.
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	_ = pgErrorOf(t, err, code)
}

// pgErrorOf asserts err is a Postgres error with the given SQLSTATE and returns it.
func pgErrorOf(t *testing.T, err error, code string) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error %s, got %v", code, err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return pgErr
}

func requireInput(t *testing.T, err error, field string) {
	t.Helper()
	var ie *deposit.InputError
	require.True(t, errors.As(err, &ie), "expected an input error, got %v", err)
	require.Equal(t, field, ie.Field)
}

// photos makes deposit and receipt photo uploads through the upload service and its scan, with
// the fake bucket and scanner.
type photos struct {
	org     uuid.UUID
	storage *uploadtest.Storage
	uploads *upload.Service
	checker *upload.Checker
}

func newPhotos(w *world) photos {
	env := w.env
	storage := uploadtest.NewStorage()
	return photos{org: w.org.ID, storage: storage,
		uploads: upload.NewService(upload.ServiceDeps{
			Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: storage, Queue: &uploadtest.Queue{}, Log: env.Log,
		}),
		checker: upload.NewChecker(upload.CheckerDeps{
			Store: env.Store, Objects: storage, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
		}),
	}
}

// Bodies the fake scanner judges: a JPEG is clean, text is rejected, EICAR is infected.
var (
	cleanPhoto    = uploadtest.Image("image/jpeg", 2_048)
	notAPhoto     = uploadtest.Image("text/plain", 64)
	infectedPhoto = []byte(uploadtest.EICAR)
)

// started starts an upload of body for purpose as p (tenantID uuid.Nil for a cash_deposit) and
// puts the file where its presigned PUT points, without completing it.
func (ph photos) started(t *testing.T, p authz.Principal, purpose string, tenantID uuid.UUID, body []byte) uuid.UUID {
	t.Helper()
	c, err := ph.uploads.Create(context.Background(), p, upload.CreateInput{
		Purpose: purpose, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(len(body)),
	})
	require.NoError(t, err)
	u, err := url.Parse(c.Put.URL)
	require.NoError(t, err)
	ph.storage.Put(strings.TrimPrefix(u.Path, "/"), body)
	return c.Upload.ID
}

// completed is a completed, not yet scanned, upload of body for purpose made by p.
func (ph photos) completed(t *testing.T, p authz.Principal, purpose string, tenantID uuid.UUID, body []byte) uuid.UUID {
	t.Helper()
	id := ph.started(t, p, purpose, tenantID, body)
	_, err := ph.uploads.Complete(context.Background(), p, id)
	require.NoError(t, err)
	return id
}

// deposit is a completed, not yet scanned, cash_deposit upload of body made by p.
func (ph photos) deposit(t *testing.T, p authz.Principal, body []byte) uuid.UUID {
	t.Helper()
	return ph.completed(t, p, upload.PurposeCashDeposit, uuid.Nil, body)
}

// scan runs the upload's scan and checks it ended in status.
func (ph photos) scan(t *testing.T, id uuid.UUID, status string) {
	t.Helper()
	res, err := ph.checker.Scan(context.Background(), ph.org, id)
	require.NoError(t, err)
	require.Equal(t, status, res.Status)
}

func TestRecordCashDeposit(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newWorld(t, env, "Griya Setoran", "0812871")
	wati := w.rent(t, "Wati Susanti", w.karyawan)
	bambang := w.rent(t, "Bambang Wijaya", w.karyawan)
	hadi := w.rent(t, "Hadi Saputra", w.dina)

	t.Run("invalid input is refused before anything is read", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			in    deposit.Input
			field string
		}{
			{name: "no requestId", in: deposit.Input{ExpectedCount: 1}, field: "requestId"},
			{name: "no payment expected", in: deposit.Input{RequestID: uuid.New()}, field: "expectedCount"},
			{name: "a negative amount", in: deposit.Input{RequestID: uuid.New(), ExpectedCount: 1, Amount: -1}, field: "amount"},
			{name: "a note too long", in: deposit.Input{RequestID: uuid.New(), ExpectedCount: 1, Note: strings.Repeat("a", 501)}, field: "note"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := w.deposits.Record(ctx, w.karyawan, tc.in)
				requireInput(t, err, tc.field)
			})
		}
	})

	t.Run("a member who holds no cash has nothing to deposit", func(t *testing.T) {
		before, allowed := w.rows(t), w.audited(t, authz.ActionCashDeposit, "allow")
		_, _, err := w.deposits.Record(ctx, w.karyawan, deposit.Input{RequestID: uuid.New(), ExpectedCount: 1})
		require.ErrorIs(t, err, deposit.ErrNothingToDeposit)
		require.Equal(t, before, w.rows(t))
		require.Equal(t, allowed, w.audited(t, authz.ActionCashDeposit, "allow"), "rolled back with the deposit")
	})

	p1 := w.cash(t, w.karyawan, wati, 300_000)
	setNow(env, 2026, 11, 14, 10, 0)
	p2 := w.cash(t, w.karyawan, bambang, 200_000)
	dinas := w.cash(t, w.dina, hadi, 700_000)
	// Cash the admin took at the office and recorded through recordPayment: no receipt, not field cash.
	byAdmin, err := env.Billing.RecordPayment(ctx, w.admin, billing.PaymentInput{
		RequestID: uuid.New(), TenantID: wati.tenantID, Amount: 50_000, Method: billing.MethodCash, ReceivedOn: tenant.Today(env.Clock),
	})
	require.NoError(t, err)
	require.Equal(t, billing.CashTotal{Amount: 500_000, Count: 2}, w.held(t, w.karyawan))

	t.Run("a stale amount or count changes nothing", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			amount int64
			count  int
		}{
			{name: "the amount is higher", amount: 500_001, count: 2},
			{name: "the amount is lower", amount: 499_999, count: 2},
			{name: "one payment is missing", amount: 500_000, count: 1},
			{name: "one payment too many", amount: 500_000, count: 3},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := w.rows(t)
				_, _, err := w.deposits.Record(ctx, w.karyawan, deposit.Input{
					RequestID: uuid.New(), ExpectedAmount: tc.amount, ExpectedCount: tc.count, Amount: tc.amount,
				})
				require.ErrorIs(t, err, deposit.ErrCashChanged)
				require.Equal(t, before, w.rows(t))
				require.Equal(t, billing.CashTotal{Amount: 500_000, Count: 2}, w.held(t, w.karyawan))
			})
		}
	})

	t.Run("a correction of one payment makes the total shown stale", func(t *testing.T) {
		_, err := env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{
			RequestID: uuid.New(), PaymentID: p2.Payment.ID, AmountDelta: ptrTo(int64(-20_000)), Reason: "salah hitung kembalian",
		})
		require.NoError(t, err)
		before := w.rows(t)
		_, _, err = w.deposits.Record(ctx, w.karyawan, deposit.Input{
			RequestID: uuid.New(), ExpectedAmount: 500_000, ExpectedCount: 2, Amount: 500_000,
		})
		require.ErrorIs(t, err, deposit.ErrCashChanged)
		require.Equal(t, before, w.rows(t))
		require.Equal(t, billing.CashTotal{Amount: 480_000, Count: 2}, w.held(t, w.karyawan))
	})

	ph := w.photos
	t.Run("a photo that cannot be this deposit's is refused and nothing is recorded", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			photo func(t *testing.T) uuid.UUID
			want  error
		}{
			{name: "an unknown upload", photo: func(*testing.T) uuid.UUID { return uuid.New() }, want: deposit.ErrPhotoNotFound},
			{name: "a cash receipt photo", want: deposit.ErrPhotoInvalid, photo: func(t *testing.T) uuid.UUID {
				return ph.completed(t, w.karyawan, upload.PurposeCashReceipt, wati.tenantID, cleanPhoto)
			}},
			{name: "another member's deposit photo", want: deposit.ErrPhotoInvalid, photo: func(t *testing.T) uuid.UUID {
				return ph.deposit(t, w.dina, cleanPhoto)
			}},
			{name: "an upload never completed", want: deposit.ErrPhotoInvalid, photo: func(t *testing.T) uuid.UUID {
				return ph.started(t, w.karyawan, upload.PurposeCashDeposit, uuid.Nil, cleanPhoto)
			}},
			{name: "a photo the scan rejected", want: deposit.ErrPhotoRefused, photo: func(t *testing.T) uuid.UUID {
				id := ph.deposit(t, w.karyawan, notAPhoto)
				ph.scan(t, id, upload.StatusRejected)
				return id
			}},
			{name: "a photo the scan found infected", want: deposit.ErrPhotoRefused, photo: func(t *testing.T) uuid.UUID {
				id := ph.deposit(t, w.karyawan, infectedPhoto)
				ph.scan(t, id, upload.StatusInfected)
				return id
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				in := w.handOver(t, w.karyawan, 480_000, tc.photo(t))
				before := w.rows(t)
				_, _, err := w.deposits.Record(ctx, w.karyawan, in)
				require.ErrorIs(t, err, tc.want)
				require.Equal(t, before, w.rows(t))
			})
		}
	})

	photo := ph.deposit(t, w.karyawan, cleanPhoto)
	setNow(env, 2026, 11, 14, 16, 30)
	first := w.handOver(t, w.karyawan, 470_000, photo)
	first.Note = "  Kurang Rp10.000, dipakai bensin  "
	var recorded deposit.Deposit

	t.Run("a deposit covers all the cash held, even when the amount differs", func(t *testing.T) {
		before := w.rows(t)
		allowed := w.audited(t, authz.ActionCashDeposit, "allow")
		recorded = w.record(t, w.karyawan, first)
		require.Equal(t, deposit.Deposit{
			ID: recorded.ID, StaffUserID: w.org.Karyawan.UserID, StaffName: w.org.Karyawan.Name,
			Amount: 470_000, ExpectedAmount: 480_000, PaymentCount: 2, Note: "Kurang Rp10.000, dipakai bensin",
			Photo:      billing.ReceiptPhoto{UploadID: photo, Status: upload.StatusPending},
			RecordedAt: recorded.RecordedAt, Status: deposit.StatusPending,
		}, recorded)
		require.True(t, recorded.RecordedAt.Equal(env.Clock.Now()))
		require.Equal(t, rowCounts{deposits: before.deposits + 1, payments: before.payments + 2}, w.rows(t))
		require.Equal(t, allowed+1, w.audited(t, authz.ActionCashDeposit, "allow"), "one audited cash:deposit")
		require.ElementsMatch(t, []uuid.UUID{p1.Payment.ID, p2.Payment.ID}, w.covered(t, recorded.ID))
		require.Equal(t, billing.CashTotal{}, w.held(t, w.karyawan), "nothing held any more")
		require.Equal(t, billing.CashTotal{Amount: 700_000, Count: 1}, w.held(t, w.dina), "Dina's cash is hers")
	})

	t.Run("the same request again records nothing and answers the deposit", func(t *testing.T) {
		before := w.rows(t)
		allowed := w.audited(t, authz.ActionCashDeposit, "allow")
		again, created, err := w.deposits.Record(ctx, w.karyawan, first)
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, recorded.ID, again.ID)
		require.Equal(t, recorded.Amount, again.Amount)
		require.Equal(t, before, w.rows(t))
		require.Equal(t, allowed, w.audited(t, authz.ActionCashDeposit, "allow"), "a repeat audits nothing")
	})

	t.Run("the same requestId with anything else is refused", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			p      authz.Principal
			change func(in *deposit.Input)
		}{
			{name: "another amount", p: w.karyawan, change: func(in *deposit.Input) { in.Amount = 480_000 }},
			{name: "another note", p: w.karyawan, change: func(in *deposit.Input) { in.Note = "Lengkap" }},
			{name: "no photo", p: w.karyawan, change: func(in *deposit.Input) { in.PhotoUploadID = uuid.Nil }},
			{name: "another expected count", p: w.karyawan, change: func(in *deposit.Input) { in.ExpectedCount = 3 }},
			{name: "another member", p: w.dina, change: func(*deposit.Input) {}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				in := first
				tc.change(&in)
				before := w.rows(t)
				_, _, err := w.deposits.Record(ctx, tc.p, in)
				require.ErrorIs(t, err, deposit.ErrRequestIDReused)
				require.Equal(t, before, w.rows(t))
			})
		}
	})

	t.Run("a photo already another deposit's is refused", func(t *testing.T) {
		w.cash(t, w.karyawan, wati, 100_000)
		before := w.rows(t)
		_, _, err := w.deposits.Record(ctx, w.karyawan, w.handOver(t, w.karyawan, 100_000, photo))
		require.ErrorIs(t, err, deposit.ErrPhotoInvalid)
		require.Equal(t, before, w.rows(t))
		require.Equal(t, billing.CashTotal{Amount: 100_000, Count: 1}, w.held(t, w.karyawan))
	})

	t.Run("a clean photo of one's own is accepted", func(t *testing.T) {
		clean := ph.deposit(t, w.karyawan, cleanPhoto)
		ph.scan(t, clean, upload.StatusClean)
		d := w.record(t, w.karyawan, w.handOver(t, w.karyawan, 100_000, clean))
		require.Equal(t, billing.ReceiptPhoto{UploadID: clean, Status: upload.StatusClean}, d.Photo)
		require.Equal(t, int64(100_000), d.ExpectedAmount)
	})

	t.Run("another karyawan's deposit covers only their own cash", func(t *testing.T) {
		d := w.record(t, w.dina, w.handOver(t, w.dina, 700_000, uuid.Nil))
		require.Equal(t, []uuid.UUID{dinas.Payment.ID}, w.covered(t, d.ID))
		require.Equal(t, billing.ReceiptPhoto{Status: billing.PhotoMissing}, d.Photo)
	})

	t.Run("cash the admin recorded through recordPayment is never covered", func(t *testing.T) {
		var n int
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, "SELECT count(*) FROM cash_deposit_payment WHERE payment_id = $1", byAdmin.Payment.ID).Scan(&n)
		})
		require.Zero(t, n)
	})

	t.Run("an admin and the Supervisor cannot record a deposit, and are audited", func(t *testing.T) {
		before := w.rows(t)
		denied := w.audited(t, authz.ActionCashDeposit, "deny")
		for _, p := range []authz.Principal{w.admin, w.supervisor} {
			_, _, err := w.deposits.Record(ctx, p, deposit.Input{RequestID: uuid.New(), ExpectedAmount: 50_000, ExpectedCount: 1, Amount: 50_000})
			require.ErrorIs(t, err, authz.ErrForbidden)
		}
		require.Equal(t, denied+2, w.audited(t, authz.ActionCashDeposit, "deny"))
		require.Equal(t, before, w.rows(t))
	})
}

func TestConfirmCashDeposit(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newWorld(t, env, "Griya Hitung", "0812872")
	// Rudi is both an admin and a karyawan: he collects cash and counts deposits.
	rudi := w.invite(t, "0812872098", "Rudi Hartono", authz.RoleAdmin, authz.RoleKaryawan)
	wati := w.rent(t, "Wati Susanti", w.karyawan)
	hadi := w.rent(t, "Hadi Saputra", w.dina)
	joko := w.rent(t, "Joko Prasetyo", rudi)

	w.cash(t, w.karyawan, wati, 300_000)
	matched := w.record(t, w.karyawan, w.handOver(t, w.karyawan, 300_000, uuid.Nil))
	w.cash(t, w.dina, hadi, 700_000)
	short := w.record(t, w.dina, w.handOver(t, w.dina, 650_000, uuid.Nil))
	w.cash(t, w.karyawan, wati, 200_000)
	over := w.record(t, w.karyawan, w.handOver(t, w.karyawan, 200_000, uuid.Nil))
	w.cash(t, rudi, joko, 400_000)
	own := w.record(t, rudi, w.handOver(t, rudi, 400_000, uuid.Nil))
	setNow(env, 2026, 11, 14, 17, 0)

	t.Run("the admin's count is recorded once with its gap", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			id       uuid.UUID
			received int64
			note     string
			status   string
			gap      int64
		}{
			{name: "matched", id: matched.ID, received: 300_000, status: deposit.StatusMatched, gap: 0},
			{name: "money missing", id: short.ID, received: 650_000, note: "Kurang Rp50.000, sudah ditanyakan", status: deposit.StatusGap, gap: -50_000},
			{name: "money over", id: over.ID, received: 210_000, note: "Lebih Rp10.000", status: deposit.StatusGap, gap: 10_000},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := w.rows(t)
				allowed := w.audited(t, authz.ActionPaymentCheck, "allow")
				d := w.confirm(t, w.admin, tc.id, tc.received, tc.note)
				require.Equal(t, tc.status, d.Status)
				require.Equal(t, &deposit.Confirmation{
					ReceivedAmount: tc.received, Gap: tc.gap, Note: tc.note,
					ConfirmedBy: w.org.Admin.UserID, ConfirmedByName: w.org.Admin.Name, ConfirmedAt: d.Confirmation.ConfirmedAt,
				}, d.Confirmation)
				require.True(t, d.Confirmation.ConfirmedAt.Equal(env.Clock.Now()))
				require.Equal(t, before.confirmations+1, w.rows(t).confirmations)
				require.Equal(t, allowed+1, w.audited(t, authz.ActionPaymentCheck, "allow"))
			})
		}
	})

	t.Run("the same count again records nothing", func(t *testing.T) {
		before := w.rows(t)
		allowed := w.audited(t, authz.ActionPaymentCheck, "allow")
		d, created, err := w.deposits.Confirm(ctx, w.admin, short.ID, deposit.ConfirmInput{ReceivedAmount: 650_000, Note: " Kurang Rp50.000, sudah ditanyakan "})
		require.NoError(t, err)
		require.False(t, created)
		require.Equal(t, deposit.StatusGap, d.Status)
		require.Equal(t, int64(-50_000), d.Confirmation.Gap)
		require.Equal(t, before, w.rows(t))
		require.Equal(t, allowed, w.audited(t, authz.ActionPaymentCheck, "allow"), "a repeat audits nothing")
	})

	t.Run("another count of a confirmed deposit is refused", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			p    authz.Principal
			in   deposit.ConfirmInput
		}{
			{name: "another amount", p: w.admin, in: deposit.ConfirmInput{ReceivedAmount: 700_000, Note: "Kurang Rp50.000, sudah ditanyakan"}},
			{name: "another note", p: w.admin, in: deposit.ConfirmInput{ReceivedAmount: 650_000, Note: "Cocok"}},
			{name: "the same count by another admin", p: rudi, in: deposit.ConfirmInput{ReceivedAmount: 650_000, Note: "Kurang Rp50.000, sudah ditanyakan"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				before := w.rows(t)
				_, _, err := w.deposits.Confirm(ctx, tc.p, short.ID, tc.in)
				require.ErrorIs(t, err, deposit.ErrAlreadyConfirmed)
				require.Equal(t, before, w.rows(t))
			})
		}
	})

	t.Run("nobody confirms their own deposit", func(t *testing.T) {
		before := w.rows(t)
		denied := w.audited(t, authz.ActionPaymentCheck, "deny")
		_, _, err := w.deposits.Confirm(ctx, rudi, own.ID, deposit.ConfirmInput{ReceivedAmount: 400_000})
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, denied+1, w.audited(t, authz.ActionPaymentCheck, "deny"))
		require.Equal(t, before, w.rows(t))

		err = w.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.InsertCashDepositConfirmation(ctx, dbgen.InsertCashDepositConfirmationParams{
				DepositID: own.ID, OrganizationID: w.org.ID, StaffUserID: rudi.UserID, ReceivedAmount: 400_000,
				ConfirmedByStaffUserID: rudi.UserID, ConfirmedAt: env.Clock.Now(),
			})
			return err
		})
		require.Contains(t, pgErrorOf(t, err, "23514").ConstraintName, "cash_deposit_confirmation_not_self")

		err = w.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.InsertCashDepositConfirmation(ctx, dbgen.InsertCashDepositConfirmationParams{
				DepositID: own.ID, OrganizationID: w.org.ID, StaffUserID: w.org.Admin.UserID, ReceivedAmount: 400_000,
				ConfirmedByStaffUserID: rudi.UserID, ConfirmedAt: env.Clock.Now(),
			})
			return err
		})
		requireCode(t, err, "23503") // the depositor named must be the deposit's own
	})

	t.Run("a karyawan and the Supervisor cannot confirm", func(t *testing.T) {
		before := w.rows(t)
		denied := w.audited(t, authz.ActionPaymentCheck, "deny")
		for _, p := range []authz.Principal{w.karyawan, w.supervisor} {
			_, _, err := w.deposits.Confirm(ctx, p, own.ID, deposit.ConfirmInput{ReceivedAmount: 400_000})
			require.ErrorIs(t, err, authz.ErrForbidden)
		}
		require.Equal(t, denied+2, w.audited(t, authz.ActionPaymentCheck, "deny"))
		require.Equal(t, before, w.rows(t))

		d := w.confirm(t, w.admin, own.ID, 400_000, "")
		require.Equal(t, deposit.StatusMatched, d.Status, "another admin may count it")
	})

	t.Run("an unknown deposit is not found", func(t *testing.T) {
		_, _, err := w.deposits.Confirm(ctx, w.admin, uuid.New(), deposit.ConfirmInput{ReceivedAmount: 1})
		require.ErrorIs(t, err, deposit.ErrNotFound)
	})

	t.Run("an invalid count is refused", func(t *testing.T) {
		_, _, err := w.deposits.Confirm(ctx, w.admin, own.ID, deposit.ConfirmInput{ReceivedAmount: -1})
		requireInput(t, err, "receivedAmount")
	})
}

// ids are the deposits' ids, in order.
func ids(items []deposit.Deposit) []uuid.UUID {
	out := make([]uuid.UUID, len(items))
	for i, d := range items {
		out[i] = d.ID
	}
	return out
}

func TestDepositReads(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	w := newWorld(t, env, "Griya Baca", "0812873")
	wati := w.rent(t, "Wati Susanti", w.karyawan)
	bambang := w.rent(t, "Bambang Wijaya", w.karyawan)
	hadi := w.rent(t, "Hadi Saputra", w.dina)

	p1 := w.cash(t, w.karyawan, wati, 300_000)
	receipt := w.photos.completed(t, w.karyawan, upload.PurposeCashReceipt, wati.tenantID, cleanPhoto)
	_, _, err := env.Billing.AttachReceiptPhoto(ctx, w.karyawan, p1.Payment.ID, receipt)
	require.NoError(t, err)
	setNow(env, 2026, 11, 14, 9, 30)
	p2 := w.cash(t, w.karyawan, bambang, 200_000)
	kos := billing.FieldUnit{PropertyName: "Kos Melati", PropertyAddress: "Jl. Kenanga No. 12, Depok"}
	unitOf := func(r rental) []billing.FieldUnit {
		u := kos
		u.UnitLabel = r.label
		return []billing.FieldUnit{u}
	}
	wantHeld := []billing.HeldPayment{
		{PaymentID: p1.Payment.ID, TenantID: wati.tenantID, TenantName: "Wati Susanti", Units: unitOf(wati), Amount: 300_000,
			RecordedAt: p1.Payment.CreatedAt, Photo: billing.ReceiptPhoto{UploadID: receipt, Status: upload.StatusPending}},
		{PaymentID: p2.Payment.ID, TenantID: bambang.tenantID, TenantName: "Bambang Wijaya", Units: unitOf(bambang), Amount: 200_000,
			RecordedAt: p2.Payment.CreatedAt, Photo: billing.ReceiptPhoto{Status: billing.PhotoMissing}},
	}
	// requireHeld compares held payments, times by instant.
	requireHeld := func(t *testing.T, want, got []billing.HeldPayment) {
		t.Helper()
		require.Len(t, got, len(want))
		for i := range want {
			require.True(t, want[i].RecordedAt.Equal(got[i].RecordedAt), "payment %d recorded at", i)
			g := got[i]
			g.RecordedAt = want[i].RecordedAt
			require.Equal(t, want[i], g)
		}
	}

	t.Run("the cash held is listed a page at a time, oldest first", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			page listing.Page
			want []billing.HeldPayment
		}{
			{name: "everything", page: listing.FirstPage, want: wantHeld},
			{name: "page 1 of 1 each", page: listing.Page{Number: 1, Size: 1}, want: wantHeld[:1]},
			{name: "page 2 of 1 each", page: listing.Page{Number: 2, Size: 1}, want: wantHeld[1:]},
			{name: "past the end", page: listing.Page{Number: 3, Size: 1}, want: []billing.HeldPayment{}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				res, err := env.Billing.UndepositedCash(ctx, w.karyawan, tc.page)
				require.NoError(t, err)
				require.Equal(t, int64(2), res.Total)
				require.Equal(t, tc.page, res.Page)
				requireHeld(t, tc.want, res.Items)
			})
		}
		res, err := env.Billing.UndepositedCash(ctx, w.dina, listing.FirstPage)
		require.NoError(t, err)
		require.Zero(t, res.Total)
		require.Empty(t, res.Items)
		_, err = env.Billing.UndepositedCash(ctx, w.admin, listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden)
	})

	setNow(env, 2026, 11, 14, 10, 0)
	k1 := w.record(t, w.karyawan, w.handOver(t, w.karyawan, 500_000, uuid.Nil))
	setNow(env, 2026, 11, 14, 11, 0)
	w.cash(t, w.karyawan, wati, 150_000)
	setNow(env, 2026, 11, 14, 12, 0)
	k2 := w.record(t, w.karyawan, w.handOver(t, w.karyawan, 150_000, uuid.Nil))
	setNow(env, 2026, 11, 14, 13, 0)
	w.cash(t, w.dina, hadi, 700_000)
	d1 := w.record(t, w.dina, w.handOver(t, w.dina, 700_000, uuid.Nil))
	setNow(env, 2026, 11, 14, 14, 0)
	w.confirm(t, w.admin, k1.ID, 500_000, "")

	t.Run("a member's own deposits, newest first", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			p    authz.Principal
			want []uuid.UUID
		}{
			{name: "Dimas", p: w.karyawan, want: []uuid.UUID{k2.ID, k1.ID}},
			{name: "Dina", p: w.dina, want: []uuid.UUID{d1.ID}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				res, err := w.deposits.Mine(ctx, tc.p, listing.FirstPage)
				require.NoError(t, err)
				require.Equal(t, tc.want, ids(res.Items))
				require.Equal(t, int64(len(tc.want)), res.Total)
			})
		}
		mine, err := w.deposits.Mine(ctx, w.karyawan, listing.FirstPage)
		require.NoError(t, err)
		require.Equal(t, deposit.StatusMatched, mine.Items[1].Status)
		require.Equal(t, deposit.StatusPending, mine.Items[0].Status)
		_, err = w.deposits.Mine(ctx, w.admin, listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden, "an admin holds no cash of their own")
	})

	t.Run("the organization's deposits by status and member", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			q    deposit.Query
			page listing.Page
			want []uuid.UUID
			tot  int64
		}{
			{name: "all", want: []uuid.UUID{d1.ID, k2.ID, k1.ID}, tot: 3},
			{name: "pending", q: deposit.Query{Status: deposit.FilterPending}, want: []uuid.UUID{d1.ID, k2.ID}, tot: 2},
			{name: "confirmed", q: deposit.Query{Status: deposit.FilterConfirmed}, want: []uuid.UUID{k1.ID}, tot: 1},
			{name: "one member", q: deposit.Query{StaffUserID: w.karyawan.UserID}, want: []uuid.UUID{k2.ID, k1.ID}, tot: 2},
			{name: "one member, pending", q: deposit.Query{StaffUserID: w.dina.UserID, Status: deposit.FilterPending}, want: []uuid.UUID{d1.ID}, tot: 1},
			{name: "one member, confirmed, none", q: deposit.Query{StaffUserID: w.dina.UserID, Status: deposit.FilterConfirmed}, want: []uuid.UUID{}, tot: 0},
			{name: "page 2 of 2 each", page: listing.Page{Number: 2, Size: 2}, want: []uuid.UUID{k1.ID}, tot: 3},
		} {
			t.Run(tc.name, func(t *testing.T) {
				page := tc.page
				if page == (listing.Page{}) {
					page = listing.FirstPage
				}
				for _, p := range []authz.Principal{w.admin, w.supervisor} {
					res, err := w.deposits.List(ctx, p, tc.q, page)
					require.NoError(t, err)
					require.Equal(t, tc.want, ids(res.Items))
					require.Equal(t, tc.tot, res.Total)
				}
			})
		}
		_, err := w.deposits.List(ctx, w.admin, deposit.Query{Status: "matched"}, listing.FirstPage)
		require.ErrorIs(t, err, deposit.ErrStatusUnknown)

		denied := w.audited(t, authz.ActionPaymentCheck, "deny")
		_, err = w.deposits.List(ctx, w.karyawan, deposit.Query{}, listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, denied+1, w.audited(t, authz.ActionPaymentCheck, "deny"))
	})

	t.Run("one deposit and its payments for the admin, the Supervisor and the depositor", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			p    authz.Principal
		}{
			{name: "the admin", p: w.admin},
			{name: "the Supervisor", p: w.supervisor},
			{name: "the depositor", p: w.karyawan},
		} {
			t.Run(tc.name, func(t *testing.T) {
				d, err := w.deposits.Get(ctx, tc.p, k1.ID)
				require.NoError(t, err)
				require.Equal(t, k1.ID, d.ID)
				require.Equal(t, deposit.StatusMatched, d.Status)
				require.Equal(t, 2, d.PaymentCount)

				res, err := w.deposits.Payments(ctx, tc.p, k1.ID, listing.FirstPage)
				require.NoError(t, err)
				require.Equal(t, int64(2), res.Total)
				requireHeld(t, wantHeld, res.Items)

				res, err = w.deposits.Payments(ctx, tc.p, k1.ID, listing.Page{Number: 2, Size: 1})
				require.NoError(t, err)
				require.Equal(t, int64(2), res.Total)
				requireHeld(t, wantHeld[1:], res.Items)
			})
		}
	})

	t.Run("another karyawan may not read it, and is audited", func(t *testing.T) {
		denied := w.audited(t, authz.ActionPaymentCheck, "deny")
		_, err := w.deposits.Get(ctx, w.dina, k1.ID)
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = w.deposits.Payments(ctx, w.dina, k1.ID, listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, denied+2, w.audited(t, authz.ActionPaymentCheck, "deny"))
	})

	t.Run("an unknown deposit is not found", func(t *testing.T) {
		_, err := w.deposits.Get(ctx, w.admin, uuid.New())
		require.ErrorIs(t, err, deposit.ErrNotFound)
		_, err = w.deposits.Payments(ctx, w.admin, uuid.New(), listing.FirstPage)
		require.ErrorIs(t, err, deposit.ErrNotFound)
	})
}

// TestDepositIsolation: the deposit tables are scoped to their organization like every business
// table, append-only, and their keys tie a covered payment to the member who recorded it as field
// cash.
func TestDepositIsolation(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 14, 9, 0)
	a := newWorld(t, env, "Griya Sejahtera", "0812874")
	b := newWorld(t, env, "Kos Bulanan Makmur", "0812875")
	ra := a.rent(t, "Rina Wulandari", a.karyawan)
	rd := a.rent(t, "Dewi Lestari", a.dina)
	rb := b.rent(t, "Budi Santoso", b.karyawan)
	a.cash(t, a.karyawan, ra, 100_000)
	b.cash(t, b.karyawan, rb, 250_000)
	da := a.record(t, a.karyawan, a.handOver(t, a.karyawan, 100_000, uuid.Nil))
	db := b.record(t, b.karyawan, b.handOver(t, b.karyawan, 250_000, uuid.Nil))
	a.confirm(t, a.admin, da.ID, 100_000, "")
	b.confirm(t, b.admin, db.ID, 240_000, "Kurang Rp10.000")

	t.Run("each organization sees only its own deposits", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			for _, table := range []string{"cash_deposit", "cash_deposit_payment", "cash_deposit_confirmation"} {
				var all, foreign int
				w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					return tx.QueryRow(ctx, fmt.Sprintf(
						"SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM %s", table)).Scan(&all, &foreign)
				})
				require.Equal(t, 1, all, table)
				require.Zero(t, foreign, table)
			}
		}
		_, err := a.deposits.Get(ctx, a.admin, db.ID)
		require.ErrorIs(t, err, deposit.ErrNotFound)
		_, err = a.deposits.Payments(ctx, a.admin, db.ID, listing.FirstPage)
		require.ErrorIs(t, err, deposit.ErrNotFound)
		_, _, err = a.deposits.Confirm(ctx, a.admin, db.ID, deposit.ConfirmInput{ReceivedAmount: 250_000})
		require.ErrorIs(t, err, deposit.ErrNotFound)
		res, err := a.deposits.List(ctx, a.admin, deposit.Query{}, listing.FirstPage)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{da.ID}, ids(res.Items))
		res, err = a.deposits.List(ctx, a.admin, deposit.Query{StaffUserID: b.karyawan.UserID}, listing.FirstPage)
		require.NoError(t, err)
		require.Empty(t, res.Items)
	})

	t.Run("no row can be written for another organization", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			insert func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error
		}{
			{name: "a deposit", insert: func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
				_, err := q.InsertCashDeposit(ctx, dbgen.InsertCashDepositParams{
					ID: uuid.New(), OrganizationID: b.org.ID, StaffUserID: b.karyawan.UserID, RequestID: uuid.New(),
					Amount: 1, ExpectedAmount: 1, PaymentCount: 1, CreatedAt: env.Clock.Now(),
				})
				return err
			}},
			{name: "a covered payment", insert: func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
				_, err := tx.Exec(ctx, `INSERT INTO cash_deposit_payment (payment_id, organization_id, deposit_id, staff_user_id)
					VALUES ($1, $2, $3, $4)`, uuid.New(), b.org.ID, db.ID, b.karyawan.UserID)
				return err
			}},
			{name: "a confirmation", insert: func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
				_, err := q.InsertCashDepositConfirmation(ctx, dbgen.InsertCashDepositConfirmationParams{
					DepositID: uuid.New(), OrganizationID: b.org.ID, StaffUserID: b.karyawan.UserID, ReceivedAmount: 1,
					ConfirmedByStaffUserID: b.admin.UserID, ConfirmedAt: env.Clock.Now(),
				})
				return err
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := a.inTx(tc.insert)
				require.Contains(t, pgErrorOf(t, err, "42501").Message, "row-level security")
			})
		}
	})

	t.Run("a deposit, its payments and its count are never changed or deleted", func(t *testing.T) {
		for _, tc := range []struct{ name, stmt string }{
			{name: "update a deposit", stmt: "UPDATE cash_deposit SET amount = 0 WHERE id = $1"},
			{name: "delete a deposit", stmt: "DELETE FROM cash_deposit WHERE id = $1"},
			{name: "truncate deposits", stmt: "TRUNCATE cash_deposit CASCADE"},
			{name: "update a covered payment", stmt: "UPDATE cash_deposit_payment SET staff_user_id = staff_user_id WHERE deposit_id = $1"},
			{name: "delete a covered payment", stmt: "DELETE FROM cash_deposit_payment WHERE deposit_id = $1"},
			{name: "truncate covered payments", stmt: "TRUNCATE cash_deposit_payment"},
			{name: "update a count", stmt: "UPDATE cash_deposit_confirmation SET received_amount = 0 WHERE deposit_id = $1"},
			{name: "delete a count", stmt: "DELETE FROM cash_deposit_confirmation WHERE deposit_id = $1"},
			{name: "truncate counts", stmt: "TRUNCATE cash_deposit_confirmation"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					var args []any
					if strings.Contains(tc.stmt, "$1") {
						args = append(args, da.ID)
					}
					_, err := tx.Exec(ctx, tc.stmt, args...)
					return err
				})
				requireCode(t, err, "42501")
			})
		}
		require.Equal(t, rowCounts{deposits: 1, payments: 1, confirmations: 1}, a.rows(t))
	})

	t.Run("a deposit covers only field cash its own member recorded", func(t *testing.T) {
		dinas := a.cash(t, a.dina, rd, 80_000)
		byAdmin, err := env.Billing.RecordPayment(ctx, a.admin, billing.PaymentInput{
			RequestID: uuid.New(), TenantID: ra.tenantID, Amount: 50_000, Method: billing.MethodCash, ReceivedOn: tenant.Today(env.Clock),
		})
		require.NoError(t, err)
		for _, tc := range []struct {
			name    string
			payment uuid.UUID
			staff   uuid.UUID
			// ownDeposit inserts a deposit of staff first, so only the payment's keys can fail;
			// otherwise the row points at Dimas's deposit.
			ownDeposit bool
		}{
			{name: "another member's payment under the depositor", payment: dinas.Payment.ID, staff: a.karyawan.UserID},
			{name: "another member's payment under its recorder in someone else's deposit", payment: dinas.Payment.ID, staff: a.dina.UserID},
			{name: "a payment without a receipt", payment: byAdmin.Payment.ID, staff: a.admin.UserID, ownDeposit: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error {
					dep := da.ID
					if tc.ownDeposit {
						row, err := q.InsertCashDeposit(ctx, dbgen.InsertCashDepositParams{
							ID: uuid.New(), OrganizationID: a.org.ID, StaffUserID: tc.staff, RequestID: uuid.New(),
							Amount: 1, ExpectedAmount: 1, PaymentCount: 1, CreatedAt: env.Clock.Now(),
						})
						if err != nil {
							return err
						}
						dep = row.ID
					}
					_, err := tx.Exec(ctx, `INSERT INTO cash_deposit_payment (payment_id, organization_id, deposit_id, staff_user_id)
						VALUES ($1, $2, $3, $4)`, tc.payment, a.org.ID, dep, tc.staff)
					return err
				})
				requireCode(t, err, "23503")
			})
		}
		require.Equal(t, billing.CashTotal{Amount: 80_000, Count: 1}, a.held(t, a.dina), "nothing was covered")
	})
}

// ptrTo is a pointer to v.
func ptrTo[T any](v T) *T { return &v }
