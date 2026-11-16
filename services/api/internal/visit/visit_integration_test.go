//go:build integration

package visit_test

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// setNow moves the fake clock to a moment in Jakarta.
func setNow(env *testenv.Env, y int, m time.Month, dd, hour, minute int) {
	env.Clock.Set(wib(y, m, dd, hour, minute))
}

// wib is a moment in Asia/Jakarta.
func wib(y int, m time.Month, dd, hour, minute int) time.Time {
	return time.Date(y, m, dd, hour, minute, 0, 0, clock.Jakarta)
}

// day is a calendar day, as midnight UTC like a date column.
func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// world is one organization with a kos, its built-in members, a second karyawan (Dina), and the
// promise and visit services.
type world struct {
	env                               *testenv.Env
	org                               testenv.Org
	admin, karyawan, dina, supervisor authz.Principal
	kos                               property.Property
	promises                          *promise.Service
	visits                            *visit.Service
	storage                           *uploadtest.Storage
	uploads                           *upload.Service
	prefix                            string
	n                                 int
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix, storage: uploadtest.NewStorage()}
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
	w.promises = promise.NewService(promise.Deps{Store: env.Store, Tenants: env.Billing, Clock: env.Clock})
	w.visits = visit.NewService(env.Store, env.Billing, env.Clock)
	w.uploads = upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: w.storage, Queue: &uploadtest.Queue{}, Log: env.Log,
	})
	return w
}

// rental is a tenant renting their own room in the world's kos.
type rental struct {
	tenantID, leaseID, unitID uuid.UUID
	label                     string
}

// rent lets a new room to a new tenant monthly from start at Rp700.000, saved now (the fake
// clock), collected by the member collector (uuid.Nil for none), and generates its invoices.
func (w *world) rent(t *testing.T, name string, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	label := fmt.Sprintf("Kamar %d", w.n)
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	r := rental{tenantID: tn.ID, unitID: unit.ID, label: label}
	r.leaseID = w.lease(t, tn.ID, unit.ID, start, collector)
	return r
}

// another gives r's tenant a second monthly lease on a new room from start, collected by
// collector, and generates its invoices.
func (w *world) another(t *testing.T, r rental, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	w.n++
	label := fmt.Sprintf("Kamar %d", w.n)
	unit, err := w.env.Properties.CreateUnit(context.Background(), w.admin, w.kos.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	return rental{tenantID: r.tenantID, unitID: unit.ID, label: label, leaseID: w.lease(t, r.tenantID, unit.ID, start, collector)}
}

func (w *world) lease(t *testing.T, tenantID, unitID uuid.UUID, start time.Time, collector uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tenantID, UnitID: unitID, TermMonths: 1, Terms: tenant.Terms{StartDate: start, CollectorMemberID: collector},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	return l.ID
}

// handOver makes collector (a member id) the collector of r's lease, as the admin.
func (w *world) handOver(t *testing.T, r rental, start time.Time, collector uuid.UUID) {
	t.Helper()
	_, err := w.env.Tenants.UpdateLease(context.Background(), w.admin, r.leaseID, tenant.Terms{StartDate: start, CollectorMemberID: collector})
	require.NoError(t, err)
}

// openTask opens a visit task of reason for tenantID, made at at, as the reminder rules and the
// promise closer do.
func (w *world) openTask(t *testing.T, tenantID uuid.UUID, reason string, at time.Time) uuid.UUID {
	t.Helper()
	id := uuid.New()
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		n, err := q.OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{ID: id, OrganizationID: w.org.ID, TenantID: tenantID, Reason: reason, CreatedAt: at})
		if err == nil && n != 1 {
			err = fmt.Errorf("a %s task is already open", reason)
		}
		return err
	})
	return id
}

// brokenPromise stores a promise of tenantID that broke at closedAt.
func (w *world) brokenPromise(t *testing.T, tenantID uuid.UUID, closedAt time.Time) {
	t.Helper()
	w.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
		id := uuid.New()
		if err := q.InsertPaymentPromise(ctx, dbgen.InsertPaymentPromiseParams{
			ID: id, OrganizationID: w.org.ID, TenantID: tenantID, CreatedAt: closedAt.AddDate(0, 0, -3),
		}); err != nil {
			return err
		}
		_, err := q.ClosePromise(ctx, dbgen.ClosePromiseParams{Status: promise.StatusBroken, ClosedAt: &closedAt, OrganizationID: w.org.ID, ID: id})
		return err
	})
}

// cash records amount of field cash from r taken by p, now, with a fresh photo.
func (w *world) cash(t *testing.T, p authz.Principal, r rental, amount int64) billing.CashResult {
	t.Helper()
	return w.cashWith(t, p, r, amount, w.env.CashPhoto(t, w.org.ID, p.UserID, r.tenantID))
}

// cashWith records amount of field cash from r taken by p, now, with photo (decision 50).
func (w *world) cashWith(t *testing.T, p authz.Principal, r rental, amount int64, photo uuid.UUID) billing.CashResult {
	t.Helper()
	res, err := w.env.Billing.RecordCashPayment(context.Background(), p, billing.CashInput{
		RequestID: uuid.New(), TenantID: r.tenantID, Amount: amount, PhotoUploadID: photo,
	})
	require.NoError(t, err)
	return res
}

// fieldPromise is p's field promise for r to pay by on; it returns the version saved.
func (w *world) fieldPromise(t *testing.T, p authz.Principal, r rental, on time.Time) uuid.UUID {
	t.Helper()
	got, err := w.promises.MakeFromField(context.Background(), p, r.tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &on})
	require.NoError(t, err)
	return got.Latest().ID
}

// photo is a receipt photo of r uploaded by p, completed and still being scanned.
func (w *world) photo(t *testing.T, p authz.Principal, r rental) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	body := uploadtest.Image("image/jpeg", 2_048)
	c, err := w.uploads.Create(ctx, p, upload.CreateInput{
		Purpose: upload.PurposeCashReceipt, TenantID: r.tenantID, ContentType: "image/jpeg", Size: int64(len(body)),
	})
	require.NoError(t, err)
	u, err := url.Parse(c.Put.URL)
	require.NoError(t, err)
	w.storage.Put(strings.TrimPrefix(u.Path, "/"), body)
	_, err = w.uploads.Complete(ctx, p, c.Upload.ID)
	require.NoError(t, err)
	return c.Upload.ID
}

// task is a visit task as stored, with the outcome that resolved it.
type task struct {
	Reason     string
	ResolvedAt *time.Time
	OutcomeID  *uuid.UUID
}

// tasks are the tenant's visit tasks, oldest first.
func (w *world) tasks(t *testing.T, tenantID uuid.UUID) []task {
	t.Helper()
	var out []task
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		rows, err := tx.Query(ctx, `SELECT v.reason, v.resolved_at, ot.outcome_id
			FROM visit_task v LEFT JOIN visit_outcome_task ot ON ot.visit_task_id = v.id
			WHERE v.tenant_id = $1 ORDER BY v.created_at, v.id`, tenantID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tk task
			if err := rows.Scan(&tk.Reason, &tk.ResolvedAt, &tk.OutcomeID); err != nil {
				return err
			}
			out = append(out, tk)
		}
		return rows.Err()
	})
	return out
}

// open are the reasons of the tenant's open visit tasks, oldest first.
func (w *world) open(t *testing.T, tenantID uuid.UUID) []string {
	t.Helper()
	out := []string{}
	for _, tk := range w.tasks(t, tenantID) {
		if tk.ResolvedAt == nil {
			out = append(out, tk.Reason)
		}
	}
	return out
}

func (w *world) inTx(fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx, dbgen.New(tx)) })
}

func (w *world) mustTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx, q *dbgen.Queries) error) {
	t.Helper()
	require.NoError(t, w.inTx(fn))
}

// count counts the rows of a table in the world's organization.
func (w *world) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n)
	})
	return n
}

// audited counts the world's audit rows for one action and decision.
func (w *world) audited(t *testing.T, action, decision string) int {
	t.Helper()
	n := 0
	for _, row := range w.env.AuditLog(t, w.org.ID) {
		if row.Action == action && row.Decision == decision {
			n++
		}
	}
	return n
}

// requireCode asserts err is a Postgres error with the given SQLSTATE and returns it (as a value,
// so a caller that only asserts has nothing to check).
func requireCode(t *testing.T, err error, code string) pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error %s, got %v", code, err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return *pgErr
}

func requireInput(t *testing.T, err error, field string) {
	t.Helper()
	var ie *visit.InputError
	require.True(t, errors.As(err, &ie), "expected an input error, got %v", err)
	require.Equal(t, field, ie.Field)
	require.NotEmpty(t, ie.Message)
}

func paid(paymentID uuid.UUID) visit.Input {
	return visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomePaid, PaymentID: paymentID}
}

func promised(versionID uuid.UUID) visit.Input {
	return visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomePromise, PromiseVersionID: versionID}
}

func notHome() visit.Input {
	return visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNotHome}
}

// TestRecordVisitOutcome covers "Catat hasil kunjungan". Every lease starts on 1 Nov 2026 and is
// saved that day, so its first invoice (Rp700.000) is due on 1 Nov; the outcomes are recorded on
// 14 Nov 2026 in Asia/Jakarta, when that debt is 13 days old. Mas Dimas is the org's karyawan.
func TestRecordVisitOutcome(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 1, 9, 0)
	w := newWorld(t, env, "Griya Kunjungan", "0812871")
	dimas := w.karyawan.MembershipID
	start := day(2026, 11, 1)
	agus := w.rent(t, "Agus Setiawan", start, dimas)
	budi := w.rent(t, "Budi Hartono", start, dimas)
	citra := w.rent(t, "Citra Lestari", start, dimas)
	dewi := w.rent(t, "Dewi Anggraini", start, dimas)
	eko := w.rent(t, "Eko Prasetyo", start, dimas) // never on the visit list
	fajar := w.rent(t, "Fajar Nugroho", start, dimas)
	gita := w.rent(t, "Gita Permata", start, w.dina.MembershipID) // Dina's, handed to Mas Dimas today
	hadi := w.rent(t, "Hadi Susanto", start, dimas)               // owes since 1 Nov, never visited

	for _, r := range []rental{agus, budi, citra} {
		w.openTask(t, r.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))
	}
	w.openTask(t, agus.tenantID, visit.TaskBrokenPromises, wib(2026, 11, 10, 6, 0))
	w.openTask(t, budi.tenantID, visit.TaskBrokenPromises, wib(2026, 11, 10, 6, 0))
	w.openTask(t, agus.tenantID, visit.TaskUndeliverable, wib(2026, 11, 12, 6, 0))
	w.openTask(t, budi.tenantID, visit.TaskUndeliverable, wib(2026, 11, 12, 6, 0))
	w.openTask(t, citra.tenantID, visit.TaskUndeliverable, wib(2026, 11, 12, 6, 0))
	for _, r := range []rental{dewi, fajar, gita} {
		w.openTask(t, r.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))
	}

	// Yesterday Mas Dimas took cash from Fajar and wrote down his promise.
	setNow(env, 2026, 11, 13, 16, 0)
	fajarCashYesterday := w.cash(t, w.karyawan, fajar, 50_000).Payment.ID
	fajarPromiseYesterday := w.fieldPromise(t, w.karyawan, fajar, day(2026, 11, 20))

	setNow(env, 2026, 11, 14, 9, 0)
	// This morning Dina took cash from Gita and wrote down her promise, then the admin handed
	// Gita's room to Mas Dimas.
	gitaCashByDina := w.cash(t, w.dina, gita, 100_000).Payment.ID
	gitaPromiseByDina := w.fieldPromise(t, w.dina, gita, day(2026, 11, 21))
	w.handOver(t, gita, start, dimas)

	record := func(p authz.Principal, r rental, in visit.Input) (visit.Outcome, bool, error) {
		return w.visits.Record(ctx, p, r.tenantID, in)
	}
	var agusCash uuid.UUID

	t.Run("paid with own field cash today resolves h7 and broken_promises, not undeliverable", func(t *testing.T) {
		agusCash = w.cash(t, w.karyawan, agus, 300_000).Payment.ID
		links := w.count(t, "visit_outcome_task")
		in := paid(agusCash)
		in.Note = "  Dibayar sebagian, sisanya akhir bulan  "
		out, created, err := record(w.karyawan, agus, in)
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, visit.OutcomePaid, out.Outcome)
		require.Equal(t, agus.tenantID, out.TenantID)
		require.Equal(t, agusCash, out.PaymentID)
		require.Equal(t, uuid.Nil, out.PromiseVersionID)
		require.Empty(t, out.NewPhone)
		require.Equal(t, "Dibayar sebagian, sisanya akhir bulan", out.Note, "trimmed")
		require.True(t, out.RecordedAt.Equal(env.Clock.Now()))
		require.Equal(t, []string{visit.ReasonOverdue, visit.ReasonBrokenPromises}, out.Resolved)

		require.Equal(t, []string{visit.TaskUndeliverable}, w.open(t, agus.tenantID))
		for _, tk := range w.tasks(t, agus.tenantID) {
			if tk.Reason == visit.TaskUndeliverable {
				require.Nil(t, tk.OutcomeID)
				continue
			}
			require.NotNil(t, tk.ResolvedAt, tk.Reason)
			require.True(t, tk.ResolvedAt.Equal(out.RecordedAt), "resolved when the outcome was recorded")
			require.NotNil(t, tk.OutcomeID, tk.Reason)
			require.Equal(t, out.ID, *tk.OutcomeID)
		}
		require.Equal(t, links+2, w.count(t, "visit_outcome_task"))

		var staff uuid.UUID
		var note *string
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, "SELECT staff_user_id, note FROM visit_outcome WHERE id = $1", out.ID).Scan(&staff, &note)
		})
		require.Equal(t, w.org.Karyawan.UserID, staff)
		require.NotNil(t, note)
		require.Equal(t, "Dibayar sebagian, sisanya akhir bulan", *note)
	})

	var budiVersion uuid.UUID
	t.Run("promise with own field promise today resolves h7 and broken_promises, not undeliverable", func(t *testing.T) {
		budiVersion = w.fieldPromise(t, w.karyawan, budi, day(2026, 11, 25))
		out, created, err := record(w.karyawan, budi, promised(budiVersion))
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, visit.OutcomePromise, out.Outcome)
		require.Equal(t, budiVersion, out.PromiseVersionID)
		require.Equal(t, uuid.Nil, out.PaymentID)
		require.Equal(t, []string{visit.ReasonOverdue, visit.ReasonBrokenPromises}, out.Resolved)
		require.Equal(t, []string{visit.TaskUndeliverable}, w.open(t, budi.tenantID))
	})

	t.Run("number_changed stores the number in E.164 and resolves only undeliverable", func(t *testing.T) {
		before, err := env.Tenants.Get(ctx, w.admin, citra.tenantID)
		require.NoError(t, err)
		out, created, err := record(w.karyawan, citra, visit.Input{
			RequestID: uuid.New(), Outcome: visit.OutcomeNumberChanged, NewPhone: " 0812-3456-7890 ", Note: "Nomor lama hilang",
		})
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, "+6281234567890", out.NewPhone)
		require.Equal(t, []string{visit.ReasonUndeliverable}, out.Resolved)
		require.Equal(t, []string{visit.TaskH7}, w.open(t, citra.tenantID))

		var stored *string
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
			return tx.QueryRow(ctx, "SELECT new_phone FROM visit_outcome WHERE id = $1", out.ID).Scan(&stored)
		})
		require.NotNil(t, stored)
		require.Equal(t, "+6281234567890", *stored)
		after, err := env.Tenants.Get(ctx, w.admin, citra.tenantID)
		require.NoError(t, err)
		require.Equal(t, before.Phone, after.Phone, "the admin applies the number, not the karyawan")
	})

	t.Run("not_home resolves nothing and the visit stays open", func(t *testing.T) {
		outcomes, links := w.count(t, "visit_outcome"), w.count(t, "visit_outcome_task")
		out, created, err := record(w.karyawan, dewi, notHome())
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, visit.OutcomeNotHome, out.Outcome)
		require.NotNil(t, out.Resolved)
		require.Empty(t, out.Resolved)
		require.Equal(t, []string{visit.TaskH7}, w.open(t, dewi.tenantID))
		require.Equal(t, outcomes+1, w.count(t, "visit_outcome"))
		require.Equal(t, links, w.count(t, "visit_outcome_task"))

		_, created, err = record(w.karyawan, dewi, notHome())
		require.NoError(t, err, "a second visit to an open task is another outcome")
		require.True(t, created)
	})

	t.Run("a tenant with no open visit is refused", func(t *testing.T) {
		ekoCash := w.cash(t, w.karyawan, eko, 20_000).Payment.ID
		outcomes := w.count(t, "visit_outcome")
		for _, tc := range []struct {
			name string
			in   visit.Input
		}{
			{name: "not home", in: notHome()},
			{name: "paid with the tenant's own cash today", in: paid(ekoCash)},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := record(w.karyawan, eko, tc.in)
				require.ErrorIs(t, err, visit.ErrNotOpen)
			})
		}
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
	})

	t.Run("a payment that cannot back a paid outcome is refused", func(t *testing.T) {
		otherTenants := w.cash(t, w.karyawan, eko, 25_000).Payment.ID
		byAdmin, err := env.Billing.RecordPayment(ctx, w.admin, billing.PaymentInput{
			RequestID: uuid.New(), TenantID: fajar.tenantID, Amount: 40_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14),
		})
		require.NoError(t, err)
		outcomes := w.count(t, "visit_outcome")
		for _, tc := range []struct {
			name      string
			r         rental
			paymentID uuid.UUID
		}{
			{name: "another tenant's cash", r: fajar, paymentID: otherTenants},
			{name: "cash another member took (before the room was handed over)", r: gita, paymentID: gitaCashByDina},
			{name: "cash the admin recorded with recordPayment (no receipt)", r: fajar, paymentID: byAdmin.Payment.ID},
			{name: "the member's own cash from yesterday", r: fajar, paymentID: fajarCashYesterday},
			{name: "an unknown payment", r: fajar, paymentID: uuid.New()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := record(w.karyawan, tc.r, paid(tc.paymentID))
				require.ErrorIs(t, err, visit.ErrPaymentInvalid)
				require.Equal(t, []string{visit.TaskH7}, w.open(t, tc.r.tenantID))
			})
		}
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
	})

	t.Run("a promise that cannot back a promise outcome is refused", func(t *testing.T) {
		otherTenants := w.fieldPromise(t, w.karyawan, eko, day(2026, 11, 20))
		adminMade, err := w.promises.MakeForTenant(ctx, w.admin, fajar.tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: ptr(day(2026, 11, 22))})
		require.NoError(t, err)
		require.Equal(t, promise.SourceAdmin, adminMade.Latest().Source)
		outcomes := w.count(t, "visit_outcome")
		for _, tc := range []struct {
			name      string
			r         rental
			versionID uuid.UUID
		}{
			{name: "a promise the admin recorded", r: fajar, versionID: adminMade.Latest().ID},
			{name: "a promise another member made (before the room was handed over)", r: gita, versionID: gitaPromiseByDina},
			{name: "another tenant's promise", r: fajar, versionID: otherTenants},
			{name: "the member's own promise from yesterday", r: fajar, versionID: fajarPromiseYesterday},
			{name: "an unknown promise version", r: fajar, versionID: uuid.New()},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := record(w.karyawan, tc.r, promised(tc.versionID))
				require.ErrorIs(t, err, visit.ErrPromiseInvalid)
				require.Equal(t, []string{visit.TaskH7}, w.open(t, tc.r.tenantID))
			})
		}
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
	})

	t.Run("cash or a promise another outcome names already is refused", func(t *testing.T) {
		w.openTask(t, agus.tenantID, visit.TaskH7, env.Clock.Now())
		w.openTask(t, budi.tenantID, visit.TaskH7, env.Clock.Now())
		outcomes := w.count(t, "visit_outcome")
		_, _, err := record(w.karyawan, agus, paid(agusCash))
		require.ErrorIs(t, err, visit.ErrPaymentInvalid)
		_, _, err = record(w.karyawan, budi, promised(budiVersion))
		require.ErrorIs(t, err, visit.ErrPromiseInvalid)
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
		require.Equal(t, []string{visit.TaskUndeliverable, visit.TaskH7}, w.open(t, agus.tenantID))

		// Fresh cash and a fresh promise version do back an outcome.
		out, _, err := record(w.karyawan, agus, paid(w.cash(t, w.karyawan, agus, 100_000).Payment.ID))
		require.NoError(t, err)
		require.Equal(t, []string{visit.ReasonOverdue}, out.Resolved)
		out, _, err = record(w.karyawan, budi, promised(w.fieldPromise(t, w.karyawan, budi, day(2026, 11, 26))))
		require.NoError(t, err)
		require.Equal(t, []string{visit.ReasonOverdue}, out.Resolved)
		require.Equal(t, []string{visit.TaskUndeliverable}, w.open(t, agus.tenantID))
	})

	t.Run("a number that is not a WhatsApp number is refused", func(t *testing.T) {
		w.openTask(t, citra.tenantID, visit.TaskUndeliverable, env.Clock.Now())
		outcomes := w.count(t, "visit_outcome")
		for _, number := range []string{"021-5551234", "0812-3456-789x", "08123"} {
			_, _, err := record(w.karyawan, citra, visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNumberChanged, NewPhone: number})
			require.ErrorIs(t, err, visit.ErrInvalidPhone, number)
		}
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
	})

	t.Run("bad input names its field", func(t *testing.T) {
		someID := uuid.New()
		for _, tc := range []struct {
			name  string
			in    visit.Input
			field string
		}{
			{name: "no requestId", in: visit.Input{Outcome: visit.OutcomeNotHome}, field: "requestId"},
			{name: "an unknown outcome", in: visit.Input{RequestID: uuid.New(), Outcome: "kabur"}, field: "outcome"},
			{name: "paid without a payment", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomePaid}, field: "paymentId"},
			{name: "a payment on not_home", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNotHome, PaymentID: someID}, field: "paymentId"},
			{name: "a payment on promise", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomePromise, PromiseVersionID: someID, PaymentID: someID}, field: "paymentId"},
			{name: "promise without a version", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomePromise}, field: "promiseVersionId"},
			{name: "a promise version on paid", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomePaid, PaymentID: someID, PromiseVersionID: someID}, field: "promiseVersionId"},
			{name: "number_changed without a number", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNumberChanged, NewPhone: "   "}, field: "newPhone"},
			{name: "a number on not_home", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNotHome, NewPhone: "081234567890"}, field: "newPhone"},
			{name: "a note over 500 letters", in: visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNotHome, Note: strings.Repeat("é", 501)}, field: "note"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := record(w.karyawan, dewi, tc.in)
				requireInput(t, err, tc.field)
			})
		}
		_, _, err := record(w.karyawan, dewi, visit.Input{RequestID: uuid.New(), Outcome: visit.OutcomeNotHome, Note: strings.Repeat("é", 500)})
		require.NoError(t, err, "500 letters is the limit, not 500 bytes")
	})

	t.Run("the same request records once; another body under its requestId is refused", func(t *testing.T) {
		gitaCash := w.cash(t, w.karyawan, gita, 600_000).Payment.ID
		in := paid(gitaCash)
		in.Note = "Lunas bulan November"
		first, created, err := record(w.karyawan, gita, in)
		require.NoError(t, err)
		require.True(t, created)
		require.Equal(t, []string{visit.ReasonOverdue}, first.Resolved)
		require.Empty(t, w.open(t, gita.tenantID))
		outcomes, links := w.count(t, "visit_outcome"), w.count(t, "visit_outcome_task")

		again, created, err := record(w.karyawan, gita, in)
		require.NoError(t, err, "a retry after the task is resolved still answers the outcome")
		require.False(t, created)
		require.Equal(t, first, again)

		for _, tc := range []struct {
			name string
			r    rental
			in   visit.Input
		}{
			{name: "another note", r: gita, in: visit.Input{RequestID: in.RequestID, Outcome: visit.OutcomePaid, PaymentID: gitaCash, Note: "Lunas"}},
			{name: "another outcome", r: gita, in: visit.Input{RequestID: in.RequestID, Outcome: visit.OutcomeNotHome, Note: in.Note}},
			{name: "another tenant", r: dewi, in: visit.Input{RequestID: in.RequestID, Outcome: visit.OutcomePaid, PaymentID: gitaCash, Note: in.Note}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := record(w.karyawan, tc.r, tc.in)
				require.ErrorIs(t, err, visit.ErrRequestIDReused)
			})
		}
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
		require.Equal(t, links, w.count(t, "visit_outcome_task"))
	})

	t.Run("only the tenant's collector records, and refusals are audited", func(t *testing.T) {
		outcomes := w.count(t, "visit_outcome")
		denied := w.audited(t, authz.ActionVisitRecord, "deny")
		for _, tc := range []struct {
			name string
			p    authz.Principal
		}{
			{name: "another karyawan", p: w.dina},
			{name: "the admin", p: w.admin},
			{name: "the Supervisor", p: w.supervisor},
		} {
			t.Run(tc.name, func(t *testing.T) {
				_, _, err := record(tc.p, dewi, notHome())
				require.ErrorIs(t, err, authz.ErrForbidden)
			})
		}
		require.Equal(t, denied+3, w.audited(t, authz.ActionVisitRecord, "deny"))
		require.Equal(t, outcomes, w.count(t, "visit_outcome"))
	})

	t.Run("an unknown tenant is not found", func(t *testing.T) {
		_, _, err := w.visits.Record(ctx, w.karyawan, uuid.New(), notHome())
		var nf *billing.NotFoundError
		require.ErrorAs(t, err, &nf)
		require.Empty(t, nf.Field)
	})

	t.Run("the next day the reminder rules do not reopen a resolved h7 for the same debt", func(t *testing.T) {
		setNow(env, 2026, 11, 15, 7, 0)
		t.Cleanup(func() { setNow(env, 2026, 11, 14, 9, 0) })
		_, err := reminder.NewDecider(env.Store, env.Clock, env.Log, nil).DecideOrg(ctx, w.org.ID)
		require.NoError(t, err)
		require.Equal(t, []string{visit.TaskUndeliverable}, w.open(t, agus.tenantID), "Agus still owes Rp300.000 from 1 Nov, but was visited for it")
		require.Equal(t, []string{visit.TaskH7}, w.open(t, hadi.tenantID), "Hadi's debt of 1 Nov was never visited")
		require.Equal(t, []string{visit.TaskH7}, w.open(t, dewi.tenantID), "not home: the same task stays open")
	})
}

func ptr[T any](v T) *T { return &v }

// TestListFieldVisits covers "Kunjungan hari ini". Leases start on 1 Nov 2026 and are saved that
// day (first invoice of Rp700.000 due 1 Nov); the list is read on 14 Nov 2026 in Asia/Jakarta.
func TestListFieldVisits(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 1, 9, 0)
	w := newWorld(t, env, "Griya Daftar", "0812872")
	dimas, dina := w.karyawan.MembershipID, w.dina.MembershipID
	start := day(2026, 11, 1)
	agus := w.rent(t, "Agus Setiawan", start, dimas)
	budi := w.rent(t, "Budi Hartono", start, dimas)
	citra := w.rent(t, "Citra Lestari", start, dimas)
	dewi := w.rent(t, "Dewi Anggraini", start, dimas)
	joko := w.rent(t, "Joko Widodo", start, dimas)
	eka := w.rent(t, "Eka Putri", start, dina)
	fajar := w.rent(t, "Fajar Nugroho", start, dimas)
	w.another(t, fajar, start, dina)        // Fajar also rents a room Dina collects
	w.rent(t, "Gita Permata", start, dimas) // nothing open, never visited
	hana := w.rent(t, "Hana Safitri", start, dina)

	// Hana moved on 13 Nov from Dina's room to one Mas Dimas collects.
	setNow(env, 2026, 11, 13, 9, 0)
	_, err := env.Tenants.MoveOut(ctx, w.admin, hana.leaseID, day(2026, 11, 13))
	require.NoError(t, err)
	hanaNow := w.another(t, hana, day(2026, 11, 13), dimas)

	w.brokenPromise(t, agus.tenantID, wib(2026, 11, 5, 0, 5))
	w.brokenPromise(t, agus.tenantID, wib(2026, 11, 11, 0, 5))
	w.openTask(t, agus.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))
	w.openTask(t, agus.tenantID, visit.TaskBrokenPromises, wib(2026, 11, 11, 0, 5))
	w.openTask(t, citra.tenantID, visit.TaskH7, wib(2026, 11, 9, 6, 0))
	w.openTask(t, budi.tenantID, visit.TaskUndeliverable, wib(2026, 11, 13, 8, 0))
	w.openTask(t, hana.tenantID, visit.TaskH7, wib(2026, 11, 13, 12, 0))
	w.openTask(t, dewi.tenantID, visit.TaskH7, wib(2026, 11, 10, 6, 0))
	w.openTask(t, joko.tenantID, visit.TaskH7, wib(2026, 11, 10, 6, 0))
	w.openTask(t, eka.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))
	w.openTask(t, fajar.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))

	setNow(env, 2026, 11, 14, 8, 0)
	agusPhoto := w.photo(t, w.karyawan, agus)
	agusCash1 := w.cashWith(t, w.karyawan, agus, 200_000, agusPhoto)
	setNow(env, 2026, 11, 14, 8, 30)
	agusCash2 := w.cash(t, w.karyawan, agus, 100_000)
	setNow(env, 2026, 11, 14, 9, 0)
	dewiCash := w.cash(t, w.karyawan, dewi, 700_000)
	_, _, err = w.visits.Record(ctx, w.karyawan, dewi.tenantID, paid(dewiCash.Payment.ID))
	require.NoError(t, err)
	setNow(env, 2026, 11, 14, 9, 30)
	citraOutcome, _, err := w.visits.Record(ctx, w.karyawan, citra.tenantID, notHome())
	require.NoError(t, err)
	setNow(env, 2026, 11, 14, 10, 0)
	jokoOutcome, _, err := w.visits.Record(ctx, w.karyawan, joko.tenantID, promised(w.fieldPromise(t, w.karyawan, joko, day(2026, 11, 20))))
	require.NoError(t, err)

	all := func(t *testing.T, p authz.Principal) []visit.Row {
		t.Helper()
		items, err := listing.All(func(page listing.Page) (listing.Result[visit.Row], error) {
			res, err := w.visits.List(ctx, p, page)
			return res.Result, err
		})
		require.NoError(t, err)
		return items
	}
	names := func(rows []visit.Row) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.TenantName
		}
		return out
	}
	byName := func(rows []visit.Row, name string) visit.Row {
		for _, r := range rows {
			if r.TenantName == name {
				return r
			}
		}
		require.FailNow(t, "not listed", name)
		return visit.Row{}
	}
	noCash := []billing.FieldCashPayment{}

	t.Run("open visits oldest first, then today's visits latest first", func(t *testing.T) {
		rows := all(t, w.karyawan)
		require.Equal(t, []string{"Agus Setiawan", "Citra Lestari", "Budi Hartono", "Hana Safitri", "Joko Widodo", "Dewi Anggraini"}, names(rows),
			"not Eka (Dina's), not Fajar (one of his rooms is Dina's), not Gita (nothing open, not visited)")
		require.Equal(t, []string{visit.StatusOpen, visit.StatusOpen, visit.StatusOpen, visit.StatusOpen, visit.StatusDone, visit.StatusDone},
			[]string{rows[0].Status, rows[1].Status, rows[2].Status, rows[3].Status, rows[4].Status, rows[5].Status})
	})

	t.Run("one row per tenant with each open reason, days overdue and broken promises", func(t *testing.T) {
		row := byName(all(t, w.karyawan), "Agus Setiawan")
		require.Equal(t, agus.tenantID, row.TenantID)
		require.Equal(t, []visit.Reason{
			{Code: visit.ReasonOverdue, DaysOverdue: 13},
			{Code: visit.ReasonBrokenPromises, BrokenPromises: 2},
		}, row.Reasons)
		require.EqualValues(t, 400_000, row.Outstanding)
		require.EqualValues(t, 400_000, row.Overdue)
		require.NotNil(t, row.OldestUnpaidDueDate)
		require.Equal(t, day(2026, 11, 1), *row.OldestUnpaidDueDate)
		require.Equal(t, []billing.FieldUnit{{UnitLabel: agus.label, PropertyName: "Kos Melati", PropertyAddress: "Jl. Kenanga No. 12, Depok"}}, row.Units)
		require.Nil(t, row.LastOutcome)

		budiRow := byName(all(t, w.karyawan), "Budi Hartono")
		require.Equal(t, []visit.Reason{{Code: visit.ReasonUndeliverable}}, budiRow.Reasons)
	})

	t.Run("unlinked cash today is listed oldest first, with its photo", func(t *testing.T) {
		row := byName(all(t, w.karyawan), "Agus Setiawan")
		require.Equal(t, []billing.FieldCashPayment{
			{PaymentID: agusCash1.Payment.ID, Amount: 200_000, RecordedAt: agusCash1.Payment.CreatedAt,
				Photo: billing.ReceiptPhoto{UploadID: agusPhoto, Status: upload.StatusPending}},
			{PaymentID: agusCash2.Payment.ID, Amount: 100_000, RecordedAt: agusCash2.Payment.CreatedAt,
				Photo: agusCash2.Photo},
		}, row.UnlinkedCashToday)
		require.Equal(t, noCash, byName(all(t, w.karyawan), "Dewi Anggraini").UnlinkedCashToday, "named by her paid outcome")
	})

	t.Run("not home keeps the visit open with its latest outcome", func(t *testing.T) {
		row := byName(all(t, w.karyawan), "Citra Lestari")
		require.Equal(t, visit.StatusOpen, row.Status)
		require.Equal(t, []visit.Reason{{Code: visit.ReasonOverdue, DaysOverdue: 13}}, row.Reasons)
		require.NotNil(t, row.LastOutcome)
		require.Equal(t, citraOutcome.ID, row.LastOutcome.ID)
		require.Equal(t, visit.OutcomeNotHome, row.LastOutcome.Outcome)
		require.Empty(t, row.LastOutcome.Resolved)
	})

	t.Run("a paid or promised visit is done with no reasons left", func(t *testing.T) {
		rows := all(t, w.karyawan)
		dewiRow := byName(rows, "Dewi Anggraini")
		require.Equal(t, visit.StatusDone, dewiRow.Status)
		require.Empty(t, dewiRow.Reasons)
		require.Zero(t, dewiRow.Outstanding)
		require.Nil(t, dewiRow.OldestUnpaidDueDate, "owes nothing")
		require.NotNil(t, dewiRow.LastOutcome)
		require.Equal(t, visit.OutcomePaid, dewiRow.LastOutcome.Outcome)
		require.Equal(t, dewiCash.Payment.ID, dewiRow.LastOutcome.PaymentID)
		require.EqualValues(t, 700_000, dewiRow.LastOutcome.Amount, "what the cash counts")
		require.Zero(t, byName(rows, "Joko Widodo").LastOutcome.Amount, "a promise has none")
		require.Equal(t, []string{visit.ReasonOverdue}, dewiRow.LastOutcome.Resolved)

		jokoRow := byName(rows, "Joko Widodo")
		require.Equal(t, visit.StatusDone, jokoRow.Status)
		require.Equal(t, jokoOutcome.ID, jokoRow.LastOutcome.ID)
		require.EqualValues(t, 700_000, jokoRow.Outstanding, "a promise pays nothing yet")
	})

	t.Run("a tenant who moved to the member's room is listed by the new collector only", func(t *testing.T) {
		row := byName(all(t, w.karyawan), "Hana Safitri")
		require.Equal(t, []billing.FieldUnit{{UnitLabel: hanaNow.label, PropertyName: "Kos Melati", PropertyAddress: "Jl. Kenanga No. 12, Depok"}}, row.Units,
			"only the room he collects")
		require.Equal(t, []string{"Eka Putri"}, names(all(t, w.dina)), "Hana's ended lease is not Dina's any more; Fajar's other room is Mas Dimas's")
	})

	t.Run("pages never overlap and total counts open and done rows", func(t *testing.T) {
		var got []string
		for n := 1; n <= 3; n++ {
			res, err := w.visits.List(ctx, w.karyawan, listing.Page{Number: n, Size: 4})
			require.NoError(t, err)
			require.EqualValues(t, 6, res.Total)
			require.EqualValues(t, 4, res.OpenTotal, "Agus, Citra (not home), Budi and Hana, on every page")
			got = append(got, names(res.Items)...)
		}
		require.Equal(t, []string{"Agus Setiawan", "Citra Lestari", "Budi Hartono", "Hana Safitri", "Joko Widodo", "Dewi Anggraini"}, got)
	})

	t.Run("members who collect from nobody see an empty list", func(t *testing.T) {
		for _, p := range []authz.Principal{w.admin, w.supervisor} {
			res, err := w.visits.List(ctx, p, listing.FirstPage)
			require.NoError(t, err)
			require.Empty(t, res.Items)
			require.Zero(t, res.Total)
			require.Zero(t, res.OpenTotal)
		}
	})

	t.Run("a member without visit:read is refused and audited", func(t *testing.T) {
		nina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: "0812-8720-0098", Name: "Nina Tamu",
			Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
		require.NoError(t, err)
		env.GrantCustomRole(t, w.org.ID, nina.ID, "Tanpa kunjungan", `{"Version":"2026-10-25","Statement":[`+
			`{"Effect":"Deny","Action":["visit:read"],"Resource":["*"]}]}`)
		before := w.audited(t, authz.ActionVisitRead, "deny")
		_, err = w.visits.List(ctx, env.Principal(t, w.org.ID, nina.UserID), listing.FirstPage)
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, before+1, w.audited(t, authz.ActionVisitRead, "deny"))
	})

	t.Run("the next day today's visits and cash are gone and the days move on", func(t *testing.T) {
		setNow(env, 2026, 11, 15, 0, 0)
		t.Cleanup(func() { setNow(env, 2026, 11, 14, 10, 0) })
		rows := all(t, w.karyawan)
		require.Equal(t, []string{"Agus Setiawan", "Citra Lestari", "Budi Hartono", "Hana Safitri"}, names(rows))
		res, err := w.visits.List(ctx, w.karyawan, listing.Page{Number: 1, Size: 1})
		require.NoError(t, err)
		require.EqualValues(t, 4, res.Total)
		require.EqualValues(t, 4, res.OpenTotal, "every row is open again")
		require.Equal(t, visit.Reason{Code: visit.ReasonOverdue, DaysOverdue: 14}, rows[0].Reasons[0])
		require.Equal(t, noCash, rows[0].UnlinkedCashToday)
		require.Nil(t, rows[1].LastOutcome, "yesterday's not home is not today's")
	})

	// Near the end, since it changes what Dewi paid; before the next one, which moves to the next day.
	t.Run("a paid outcome's amount is what its cash counts after corrections", func(t *testing.T) {
		newAmount := int64(650_000)
		_, err := env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{
			RequestID: uuid.New(), PaymentID: dewiCash.Payment.ID, NewAmount: &newAmount, Reason: "Uang kurang Rp50.000 saat disetor",
		})
		require.NoError(t, err)
		require.EqualValues(t, 650_000, byName(all(t, w.karyawan), "Dewi Anggraini").LastOutcome.Amount)
	})

	// Last, since it records outcomes the subtests above do not expect.
	t.Run("the newest field promise of today that no outcome names is offered for Janji baru", func(t *testing.T) {
		promiseOf := func(name string) *visit.TodayPromise {
			t.Helper()
			return byName(all(t, w.karyawan), name).UnlinkedPromiseToday
		}
		require.Nil(t, promiseOf("Agus Setiawan"), "no promise yet")
		require.Nil(t, promiseOf("Joko Widodo"), "named by his promise outcome")

		setNow(env, 2026, 11, 14, 10, 30)
		first := w.fieldPromise(t, w.karyawan, agus, day(2026, 11, 20))
		got := promiseOf("Agus Setiawan")
		require.NotNil(t, got)
		require.Equal(t, first, got.VersionID)
		require.Equal(t, 1, got.Version)
		require.Equal(t, promise.KindSpecificDate, got.Kind)
		require.Equal(t, day(2026, 11, 20), got.PromisedDate)
		require.Nil(t, got.PromisedAmount, "everything owed")
		require.True(t, got.CreatedAt.Equal(wib(2026, 11, 14, 10, 30)), got.CreatedAt)
		promiseID := got.PromiseID

		setNow(env, 2026, 11, 14, 10, 35)
		second := w.fieldPromise(t, w.karyawan, agus, day(2026, 11, 22))
		got = promiseOf("Agus Setiawan")
		require.Equal(t, second, got.VersionID, "the edit is the newest")
		require.Equal(t, 2, got.Version)
		require.Equal(t, promiseID, got.PromiseID, "the same promise")

		setNow(env, 2026, 11, 14, 10, 40)
		adminDay := day(2026, 11, 23)
		_, err := w.promises.MakeForTenant(ctx, w.admin, agus.tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &adminDay})
		require.NoError(t, err)
		require.Equal(t, second, promiseOf("Agus Setiawan").VersionID, "the admin's version is not the member's field promise")

		_, _, err = w.visits.Record(ctx, w.karyawan, agus.tenantID, promised(second))
		require.NoError(t, err)
		require.Nil(t, promiseOf("Agus Setiawan"), "once the newest is named, the older one of the day is not offered")

		budiVersion := w.fieldPromise(t, w.karyawan, budi, day(2026, 11, 21))
		require.Equal(t, budiVersion, promiseOf("Budi Hartono").VersionID)
		setNow(env, 2026, 11, 15, 8, 0)
		require.Nil(t, promiseOf("Budi Hartono"), "yesterday's promise is not today's")
	})
}

// TestVisitIsolation: visit_outcome and visit_outcome_task are scoped to their organization like
// every business table, append-only, and their keys and checks hold whatever the service does.
func TestVisitIsolation(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	setNow(env, 2026, 11, 1, 9, 0)
	a := newWorld(t, env, "Griya Sejahtera", "0812873")
	b := newWorld(t, env, "Kos Bulanan Sejahtera", "0812874")
	start := day(2026, 11, 1)
	ra := a.rent(t, "Rina Wulandari", start, a.karyawan.MembershipID)
	rb := b.rent(t, "Budi Santoso", start, b.karyawan.MembershipID)
	other := a.rent(t, "Sari Lain", start, a.karyawan.MembershipID)
	dinas := a.rent(t, "Tono Dina", start, a.dina.MembershipID)
	taskA := a.openTask(t, ra.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))
	b.openTask(t, rb.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))
	otherTask := a.openTask(t, other.tenantID, visit.TaskH7, wib(2026, 11, 8, 6, 0))

	setNow(env, 2026, 11, 14, 9, 0)
	payA := a.cash(t, a.karyawan, ra, 100_000).Payment.ID
	payB := b.cash(t, b.karyawan, rb, 100_000).Payment.ID
	outA, _, err := a.visits.Record(ctx, a.karyawan, ra.tenantID, paid(payA))
	require.NoError(t, err)
	outB, _, err := b.visits.Record(ctx, b.karyawan, rb.tenantID, paid(payB))
	require.NoError(t, err)

	t.Run("each organization sees only its own outcomes and links", func(t *testing.T) {
		for _, w := range []*world{a, b} {
			for _, table := range []string{"visit_outcome", "visit_outcome_task"} {
				var all, foreign int
				w.mustTx(t, func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					return tx.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM "+table).Scan(&all, &foreign)
				})
				require.Equal(t, 1, all, table)
				require.Zero(t, foreign, table)
			}
		}
		a.mustTx(t, func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.GetVisitOutcomeByRequestID(ctx, dbgen.GetVisitOutcomeByRequestIDParams{OrganizationID: b.org.ID, RequestID: outB.ID})
			require.ErrorIs(t, err, pgx.ErrNoRows)
			reasons, err := q.ListVisitOutcomeTaskReasons(ctx, dbgen.ListVisitOutcomeTaskReasonsParams{OrganizationID: b.org.ID, OutcomeID: outB.ID})
			require.NoError(t, err)
			require.Empty(t, reasons)
			return nil
		})
	})

	t.Run("another organization's tenant is not found and not listed", func(t *testing.T) {
		_, _, err := a.visits.Record(ctx, a.karyawan, rb.tenantID, notHome())
		var nf *billing.NotFoundError
		require.ErrorAs(t, err, &nf)
		res, err := a.visits.List(ctx, a.karyawan, listing.FirstPage)
		require.NoError(t, err)
		for _, row := range res.Items {
			require.NotEqual(t, rb.tenantID, row.TenantID)
		}
		_, _, err = a.visits.Record(ctx, a.karyawan, other.tenantID, paid(payB))
		require.ErrorIs(t, err, visit.ErrPaymentInvalid, "another organization's payment is unknown here")
	})

	t.Run("nothing can be written for another organization", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.InsertVisitOutcome(ctx, dbgen.InsertVisitOutcomeParams{
				ID: uuid.New(), OrganizationID: b.org.ID, TenantID: rb.tenantID, StaffUserID: b.org.Karyawan.UserID,
				RequestID: uuid.New(), Outcome: visit.OutcomeNotHome, CreatedAt: env.Clock.Now(),
			})
			return err
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
		err = a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertVisitOutcomeTask(ctx, dbgen.InsertVisitOutcomeTaskParams{
				VisitTaskID: uuid.New(), OrganizationID: b.org.ID, TenantID: rb.tenantID, OutcomeID: outB.ID,
			})
		})
		require.Contains(t, requireCode(t, err, "42501").Message, "row-level security")
	})

	t.Run("outcomes and their links are never changed or removed", func(t *testing.T) {
		for _, tc := range []struct{ name, stmt string }{
			{name: "update an outcome", stmt: "UPDATE visit_outcome SET note = 'diubah' WHERE id = $1"},
			{name: "delete an outcome", stmt: "DELETE FROM visit_outcome WHERE id = $1"},
			{name: "truncate outcomes", stmt: "TRUNCATE visit_outcome CASCADE"},
			{name: "update a link", stmt: "UPDATE visit_outcome_task SET outcome_id = outcome_id WHERE outcome_id = $1"},
			{name: "delete a link", stmt: "DELETE FROM visit_outcome_task WHERE outcome_id = $1"},
			{name: "truncate links", stmt: "TRUNCATE visit_outcome_task"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, tx pgx.Tx, _ *dbgen.Queries) error {
					if strings.HasPrefix(tc.stmt, "TRUNCATE") {
						_, err := tx.Exec(ctx, tc.stmt)
						return err
					}
					_, err := tx.Exec(ctx, tc.stmt, outA.ID)
					return err
				})
				requireCode(t, err, "42501")
			})
		}
		require.Equal(t, 1, a.count(t, "visit_outcome"))
		require.Equal(t, 1, a.count(t, "visit_outcome_task"))
	})

	adminCash, err := env.Billing.RecordPayment(ctx, a.admin, billing.PaymentInput{
		RequestID: uuid.New(), TenantID: other.tenantID, Amount: 10_000, Method: billing.MethodCash, ReceivedOn: day(2026, 11, 14),
	})
	require.NoError(t, err)
	dinaCash := a.cash(t, a.dina, dinas, 50_000).Payment.ID
	raVersion := a.fieldPromise(t, a.karyawan, ra, day(2026, 11, 20))
	insert := func(row dbgen.InsertVisitOutcomeParams) error {
		row.ID, row.OrganizationID, row.RequestID, row.CreatedAt = uuid.New(), a.org.ID, uuid.New(), env.Clock.Now()
		if row.StaffUserID == uuid.Nil {
			row.StaffUserID = a.org.Karyawan.UserID
		}
		return a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			_, err := q.InsertVisitOutcome(ctx, row)
			return err
		})
	}
	some := func(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: true} }

	t.Run("an outcome names only the tenant's own field cash, taken by its member, and the tenant's promise", func(t *testing.T) {
		freshA := a.cash(t, a.karyawan, ra, 10_000).Payment.ID
		for _, tc := range []struct {
			name string
			row  dbgen.InsertVisitOutcomeParams
		}{
			{name: "another tenant's payment", row: dbgen.InsertVisitOutcomeParams{TenantID: other.tenantID, Outcome: visit.OutcomePaid, PaymentID: some(freshA)}},
			{name: "a payment another member recorded", row: dbgen.InsertVisitOutcomeParams{TenantID: dinas.tenantID, Outcome: visit.OutcomePaid, PaymentID: some(dinaCash)}},
			{name: "a payment without a receipt", row: dbgen.InsertVisitOutcomeParams{TenantID: other.tenantID, StaffUserID: a.org.Admin.UserID,
				Outcome: visit.OutcomePaid, PaymentID: some(adminCash.Payment.ID)}},
			{name: "another tenant's promise version", row: dbgen.InsertVisitOutcomeParams{TenantID: other.tenantID, Outcome: visit.OutcomePromise, PromiseVersionID: some(raVersion)}},
			{name: "an unknown tenant", row: dbgen.InsertVisitOutcomeParams{TenantID: uuid.New(), Outcome: visit.OutcomeNotHome}},
			{name: "a staff user who is not a member", row: dbgen.InsertVisitOutcomeParams{TenantID: ra.tenantID, StaffUserID: b.org.Karyawan.UserID, Outcome: visit.OutcomeNotHome}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				requireCode(t, insert(tc.row), "23503")
			})
		}
		require.NoError(t, insert(dbgen.InsertVisitOutcomeParams{TenantID: ra.tenantID, Outcome: visit.OutcomePaid, PaymentID: some(freshA)}),
			"the tenant's own field cash taken by the member is accepted")
	})

	t.Run("a link names a task of the outcome's own tenant", func(t *testing.T) {
		err := a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertVisitOutcomeTask(ctx, dbgen.InsertVisitOutcomeTaskParams{
				VisitTaskID: otherTask, OrganizationID: a.org.ID, TenantID: ra.tenantID, OutcomeID: outA.ID,
			})
		})
		requireCode(t, err, "23503")
		err = a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertVisitOutcomeTask(ctx, dbgen.InsertVisitOutcomeTaskParams{
				VisitTaskID: otherTask, OrganizationID: a.org.ID, TenantID: other.tenantID, OutcomeID: outA.ID,
			})
		})
		requireCode(t, err, "23503")
		err = a.inTx(func(ctx context.Context, _ pgx.Tx, q *dbgen.Queries) error {
			return q.InsertVisitOutcomeTask(ctx, dbgen.InsertVisitOutcomeTaskParams{
				VisitTaskID: taskA, OrganizationID: a.org.ID, TenantID: ra.tenantID, OutcomeID: outA.ID,
			})
		})
		requireCode(t, err, "23505") // a task is resolved by one outcome
	})

	t.Run("an outcome carries exactly what its kind needs", func(t *testing.T) {
		freshA := a.cash(t, a.karyawan, ra, 10_000).Payment.ID
		text := func(s string) *string { return &s }
		for _, tc := range []struct {
			name       string
			row        dbgen.InsertVisitOutcomeParams
			constraint string
		}{
			{name: "paid without a payment", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomePaid},
				constraint: "visit_outcome_paid_names_payment"},
			{name: "not_home with a payment", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomeNotHome, PaymentID: some(freshA)},
				constraint: "visit_outcome_paid_names_payment"},
			{name: "promise without a version", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomePromise},
				constraint: "visit_outcome_promise_names_version"},
			{name: "number_changed without a number", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomeNumberChanged},
				constraint: "visit_outcome_number_names_phone"},
			{name: "a number on not_home", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomeNotHome, NewPhone: text("+6281234567890")},
				constraint: "visit_outcome_number_names_phone"},
			{name: "a number not in E.164", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomeNumberChanged, NewPhone: text("081234567890")},
				constraint: "visit_outcome_new_phone_e164"},
			{name: "an empty note", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomeNotHome, Note: text("")},
				constraint: "visit_outcome_note"},
			{name: "an untrimmed note", row: dbgen.InsertVisitOutcomeParams{Outcome: visit.OutcomeNotHome, Note: text(" ada ")},
				constraint: "visit_outcome_note"},
			{name: "an unknown outcome", row: dbgen.InsertVisitOutcomeParams{Outcome: "kabur"},
				constraint: "visit_outcome_kind"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.row.TenantID = ra.tenantID
				require.Equal(t, tc.constraint, requireCode(t, insert(tc.row), "23514").ConstraintName)
			})
		}
	})
}
