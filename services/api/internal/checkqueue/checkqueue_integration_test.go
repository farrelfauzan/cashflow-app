//go:build integration

package checkqueue_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/checkqueue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

const linkOrigin = "https://app.test"

// wib is a moment in Asia/Jakarta.
func wib(y int, m time.Month, d, hour, minute int) time.Time {
	return time.Date(y, m, d, hour, minute, 0, 0, clock.Jakarta)
}

// day is a calendar day, as midnight UTC like a date column.
func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

// Bodies the fake scanner judges: a JPEG is clean, one carrying EICAR is infected, a PDF stored in
// place of the received photo is rejected.
var (
	cleanPhoto    = uploadtest.Image("image/jpeg", 3_000)
	infectedPhoto = append(uploadtest.Image("image/jpeg", 3_000-len(uploadtest.EICAR)), uploadtest.EICAR...)
	pdfInPlace    = append([]byte("%PDF-1.7\n"), make([]byte, 3_000-9)...)
)

// world is one organization with a kos, its built-in members, a second karyawan (Dina), and every
// source of Antrean cek wired to the fakes: payment links and transfer proofs, field cash
// deposits, receipt uploads and their scan, QRIS settlement and visit outcomes.
type world struct {
	env                               *testenv.Env
	org                               testenv.Org
	admin, supervisor, karyawan, dina authz.Principal
	kos                               property.Property
	prefix                            string
	n                                 int
	links                             *paylink.Service
	storage                           *uploadtest.Storage
	uploads                           *upload.Service
	checker                           *upload.Checker
	proofs                            *upload.Proofs
	deposits                          *deposit.Service
	visits                            *visit.Service
	qris                              *qris.Service
	queue                             *checkqueue.Service
	tokens                            map[uuid.UUID]string // a link token per invoice
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix, tokens: map[uuid.UUID]string{}}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	w.supervisor = env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	w.karyawan = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	dina, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: prefix + "099", Name: "Dina Kasir",
		Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	w.dina = env.Principal(t, w.org.ID, dina.UserID)
	w.kos, err = env.Properties.Create(ctx, w.admin, property.PropertyInput{
		Name: "Kos Melati " + name, Kind: property.KindKos, Address: "Jl. Kenanga No. 12, Depok",
	})
	require.NoError(t, err)

	w.links = paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	w.storage = uploadtest.NewStorage()
	scanQueue := &uploadtest.Queue{}
	w.uploads = upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: w.storage, Queue: scanQueue, Log: env.Log,
	})
	w.checker = upload.NewChecker(upload.CheckerDeps{
		Store: env.Store, Objects: w.storage, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
	})
	w.proofs = upload.NewProofs(upload.ProofsDeps{
		Store: env.Store, Links: w.links, Objects: w.storage, Queue: scanQueue, Clock: env.Clock, Log: env.Log,
	})
	w.deposits = deposit.NewService(env.Store, env.Authz, env.Clock, env.Log)
	w.visits = visit.NewService(env.Store, env.Billing, env.Clock)
	w.qris = qris.NewService(qris.Deps{Store: env.Store, Clock: env.Clock, Audit: env.Authz, Log: env.Log})
	w.queue = checkqueue.NewService(env.Store, env.Authz, env.Billing, env.Tenants, env.Clock)
	return w
}

// invite adds a member with the given role grants and returns their principal and membership id.
func (w *world) invite(t *testing.T, phone, name string, grants ...member.RoleGrant) (authz.Principal, uuid.UUID) {
	t.Helper()
	m, err := w.env.Members.Invite(context.Background(), w.admin, member.Invite{Phone: phone, Name: name, Roles: grants})
	require.NoError(t, err)
	return w.env.Principal(t, w.org.ID, m.UserID), m.ID
}

// rental is a tenant renting their own room in the world's kos.
type rental struct {
	tenantID, leaseID uuid.UUID
	label, phone      string // phone in E.164
}

// rent lets a new room to a new tenant monthly from start at Rp700.000, saved now, collected by
// the membership collector (uuid.Nil for none), and generates its invoices.
func (w *world) rent(t *testing.T, name string, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	tn, err := w.env.Tenants.Create(context.Background(), w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n+1)})
	require.NoError(t, err)
	r := w.lease(t, tn.ID, start, collector)
	r.phone = tn.Phone
	return r
}

// lease gives tenantID a new room monthly from start, collected by collector, and generates its
// invoices.
func (w *world) lease(t *testing.T, tenantID uuid.UUID, start time.Time, collector uuid.UUID) rental {
	t.Helper()
	ctx := context.Background()
	w.n++
	label := fmt.Sprintf("Kamar %d", w.n)
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tenantID, UnitID: unit.ID, TermMonths: 1, Terms: tenant.Terms{StartDate: start, CollectorMemberID: collector},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	return rental{tenantID: tenantID, leaseID: l.ID, label: label}
}

// account is the tenant's money account, as the admin sees it.
func (w *world) account(t *testing.T, tenantID uuid.UUID) billing.Account {
	t.Helper()
	acct, err := w.env.Billing.Account(context.Background(), w.admin, tenantID)
	require.NoError(t, err)
	return acct
}

// token is a working payment link token to the tenant's first invoice, minted once.
func (w *world) token(t *testing.T, tenantID uuid.UUID) string {
	t.Helper()
	acct := w.account(t, tenantID)
	require.NotEmpty(t, acct.Invoices)
	inv := acct.Invoices[0].ID
	if tok, ok := w.tokens[inv]; ok {
		return tok
	}
	link, err := w.links.Mint(context.Background(), w.admin, inv)
	require.NoError(t, err)
	tok, ok := strings.CutPrefix(link.URL, linkOrigin+"/t/")
	require.True(t, ok)
	w.tokens[inv] = tok
	return tok
}

// linkID is the id of the payment link behind the tenant's token.
func (w *world) linkID(t *testing.T, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	_, id, err := w.links.Resolve(context.Background(), w.token(t, tenantID))
	require.NoError(t, err)
	return id
}

// sendProof sends body as a transfer proof from the tenant's link; its photo is still pending.
func (w *world) sendProof(t *testing.T, tenantID uuid.UUID, body []byte) upload.Proof {
	t.Helper()
	proof, err := w.proofs.Send(context.Background(), w.token(t, tenantID), func() (io.Reader, error) { return bytes.NewReader(body), nil })
	require.NoError(t, err)
	return proof
}

// scan runs the upload's scan and checks it ended in status.
func (w *world) scan(t *testing.T, uploadID uuid.UUID, status string) {
	t.Helper()
	res, err := w.checker.Scan(context.Background(), w.org.ID, uploadID)
	require.NoError(t, err)
	require.Equal(t, status, res.Status)
}

// cleanProof is a transfer proof whose photo the scan found clean.
func (w *world) cleanProof(t *testing.T, tenantID uuid.UUID) upload.Proof {
	t.Helper()
	p := w.sendProof(t, tenantID, cleanPhoto)
	w.scan(t, p.UploadID, upload.StatusClean)
	return p
}

// infectedProof is a transfer proof whose photo the scan found infected.
func (w *world) infectedProof(t *testing.T, tenantID uuid.UUID) upload.Proof {
	t.Helper()
	p := w.sendProof(t, tenantID, infectedPhoto)
	w.scan(t, p.UploadID, upload.StatusInfected)
	return p
}

// rejectedProof is a transfer proof whose stored copy is not a photo by the time it is scanned.
func (w *world) rejectedProof(t *testing.T, tenantID uuid.UUID) upload.Proof {
	t.Helper()
	p := w.sendProof(t, tenantID, cleanPhoto)
	var key string
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error {
		up, err := dbgen.New(tx).GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: w.org.ID, ID: p.UploadID})
		key = up.ObjectKey
		return err
	})
	w.storage.Put(upload.IncomingKey(key), pdfInPlace)
	w.scan(t, p.UploadID, upload.StatusRejected)
	return p
}

// infectedReceipt is a receipt photo p took of r's cash that the scan found infected.
func (w *world) infectedReceipt(t *testing.T, p authz.Principal, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	c, err := w.uploads.Create(ctx, p, upload.CreateInput{
		Purpose: upload.PurposeCashReceipt, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(len(infectedPhoto)),
	})
	require.NoError(t, err)
	u, err := url.Parse(c.Put.URL)
	require.NoError(t, err)
	w.storage.Put(strings.TrimPrefix(u.Path, "/"), infectedPhoto)
	_, err = w.uploads.Complete(ctx, p, c.Upload.ID)
	require.NoError(t, err)
	w.scan(t, c.Upload.ID, upload.StatusInfected)
	return c.Upload.ID
}

// deposit has p take cash from tenantID, hand over handed, and the admin count received (below zero: not counted yet).
func (w *world) deposit(t *testing.T, p authz.Principal, tenantID uuid.UUID, cash, handed, received int64, note string) deposit.Deposit {
	t.Helper()
	ctx := context.Background()
	_, err := w.env.Billing.RecordCashPayment(ctx, p, billing.CashInput{RequestID: uuid.New(), TenantID: tenantID, Amount: cash})
	require.NoError(t, err)
	held, err := w.env.Billing.FieldCash(ctx, p)
	require.NoError(t, err)
	d, created, err := w.deposits.Record(ctx, p, deposit.Input{
		RequestID: uuid.New(), ExpectedAmount: held.Amount, ExpectedCount: held.Count, Amount: handed,
	})
	require.NoError(t, err)
	require.True(t, created)
	if received < 0 {
		return d // not counted yet
	}
	d, created, err = w.deposits.Confirm(ctx, w.admin, d.ID, deposit.ConfirmInput{ReceivedAmount: received, Note: note})
	require.NoError(t, err)
	require.True(t, created)
	return d
}

// qrisPaid makes a QRIS attempt of qr for the tenant and settles it: at reported by the webhook,
// or, when reconciledFirst, at its own amount (as the check and reconcile do) before the webhook
// reports reported.
func (w *world) qrisPaid(t *testing.T, tenantID uuid.UUID, qr, reported int64, reconciledFirst bool) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	now := w.env.Clock.Now()
	p := dbgen.InsertQrisAttemptParams{
		ID: uuid.New(), OrganizationID: w.org.ID, TenantID: tenantID, PaymentLinkID: w.linkID(t, tenantID),
		Amount: qr, TransactionID: uuid.NewString(), OrderID: "AUTOGOPAY-1793000000-1",
		QrString: "00020101021226610016ID.CO.QRIS.WWW0118936009140000000000215ID10200000000000303UME5204549953033605802ID",
		Status:   qris.AttemptPending, ExpiresAt: now.Add(15 * time.Minute), CreatedAt: now,
	}
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error { return dbgen.New(tx).InsertQrisAttempt(ctx, p) })
	if reconciledFirst {
		require.NoError(t, w.qris.SettlePaid(ctx, w.org.ID, p.TransactionID, qr))
	}
	require.NoError(t, w.qris.SettlePaid(ctx, w.org.ID, p.TransactionID, reported))
	return p.ID
}

// visitOutcome opens an undeliverable visit task for tenantID and records p's outcome on it.
func (w *world) visitOutcome(t *testing.T, p authz.Principal, tenantID uuid.UUID, in visit.Input) visit.Outcome {
	t.Helper()
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := dbgen.New(tx).OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
			ID: uuid.New(), OrganizationID: w.org.ID, TenantID: tenantID, Reason: visit.TaskUndeliverable, CreatedAt: w.env.Clock.Now(),
		})
		return err
	})
	in.RequestID = uuid.New()
	out, created, err := w.visits.Record(context.Background(), p, tenantID, in)
	require.NoError(t, err)
	require.True(t, created)
	return out
}

// newNumber is p's number_changed outcome on tenantID with the number raw.
func (w *world) newNumber(t *testing.T, p authz.Principal, tenantID uuid.UUID, raw, note string) visit.Outcome {
	t.Helper()
	return w.visitOutcome(t, p, tenantID, visit.Input{Outcome: visit.OutcomeNumberChanged, NewPhone: raw, Note: note})
}

func (w *world) mustTx(t *testing.T, fn func(ctx context.Context, tx pgx.Tx) error) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx) }))
}

// count runs a count query in the world's organization scope.
func (w *world) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error { return tx.QueryRow(ctx, sql, args...).Scan(&n) })
	return n
}

func (w *world) payments(t *testing.T) int { return w.count(t, "SELECT count(*) FROM payment") }
func (w *world) resolutions(t *testing.T) int {
	return w.count(t, "SELECT count(*) FROM check_resolution")
}

// audited counts the world's audit rows of action on resource with decision, by actor.
func (w *world) audited(t *testing.T, actor uuid.UUID, action, resource, decision string) int {
	t.Helper()
	return w.count(t, `SELECT count(*) FROM audit_log WHERE actor_staff_user_id = $1 AND action = $2 AND resource = $3 AND decision = $4`,
		actor, action, resource, decision)
}

// auditedAction counts the world's audit rows of action with decision by actor, on any resource.
func (w *world) auditedAction(t *testing.T, actor uuid.UUID, action, decision string) int {
	t.Helper()
	return w.count(t, `SELECT count(*) FROM audit_log WHERE actor_staff_user_id = $1 AND action = $2 AND decision = $3`,
		actor, action, decision)
}

// list is one page of the queue as p sees it.
func (w *world) list(t *testing.T, p authz.Principal, kind string, page listing.Page) checkqueue.Page {
	t.Helper()
	got, err := w.queue.List(context.Background(), p, kind, page)
	require.NoError(t, err)
	return got
}

// waiting is every waiting item's id, oldest first.
func (w *world) waiting(t *testing.T) []uuid.UUID {
	t.Helper()
	page := w.list(t, w.admin, "", listing.Page{Number: 1, Size: listing.MaxPageSize})
	out := make([]uuid.UUID, len(page.Items))
	for i, it := range page.Items {
		out[i] = it.ID
	}
	return out
}

// proofsWaiting is the ids the tenant's payment page lists as waiting proofs.
func (w *world) proofsWaiting(t *testing.T, tenantID uuid.UUID) []uuid.UUID {
	t.Helper()
	page, err := w.links.PublicPage(context.Background(), w.token(t, tenantID))
	require.NoError(t, err)
	out := make([]uuid.UUID, len(page.ProofsWaiting))
	for i, p := range page.ProofsWaiting {
		out[i] = p.ID
	}
	return out
}

// matchInput is a match for the proof's own tenant of amount received today, with a fresh requestId.
func (w *world) matchInput(amount int64, note string) checkqueue.MatchInput {
	return checkqueue.MatchInput{RequestID: uuid.New(), Amount: amount, ReceivedOn: tenant.Today(w.env.Clock), Note: note}
}

func requireInput(t *testing.T, err error, field string) {
	t.Helper()
	var ie *checkqueue.InputError
	require.True(t, errors.As(err, &ie), "expected an input error, got %v", err)
	require.Equal(t, field, ie.Field)
	require.NotEmpty(t, ie.Message)
}

func pgErrorOf(t *testing.T, err error, code string) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error %s, got %v", code, err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return pgErr
}

// requireCode asserts err is a Postgres error with the given SQLSTATE.
func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	_ = pgErrorOf(t, err, code)
}

func unit(label, propertyName string, active bool) checkqueue.Unit {
	return checkqueue.Unit{Label: label, PropertyName: propertyName, Active: active}
}

// TestCheckQueueList puts one item of every kind on the queue, a minute apart, and reads it back
// with each kind's details, oldest first, filtered and paged. The clock is 14 Nov 2026.
func TestCheckQueueList(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 8, 0))
	w := newWorld(t, env, "Griya Antrean", "0812771")
	kos := w.kos.Name

	// Rina rents Kamar 1 from 1 November, and rented Kamar 2 from 10 to 12 November: the active
	// unit comes first although the ended one started later.
	rina := w.rent(t, "Rina Wulandari", day(2026, 11, 1), w.karyawan.MembershipID)
	ended := w.lease(t, rina.tenantID, day(2026, 11, 10), uuid.Nil)
	_, err := env.Tenants.MoveOut(ctx, w.admin, ended.leaseID, day(2026, 11, 12))
	require.NoError(t, err)
	budi := w.rent(t, "Budi Santoso", day(2026, 11, 14), w.karyawan.MembershipID)
	hadi := w.rent(t, "Hadi Saputra", day(2026, 11, 14), w.dina.MembershipID)
	citra := w.rent(t, "Citra Lestari", day(2026, 11, 14), uuid.Nil)
	dewi := w.rent(t, "Dewi Anggraini", day(2026, 11, 14), uuid.Nil)
	eko := w.rent(t, "Eko Prasetyo", day(2026, 11, 14), w.karyawan.MembershipID)
	tick := func() time.Time {
		env.Clock.Advance(time.Minute)
		return env.Clock.Now()
	}

	t1 := tick()
	proof := w.sendProof(t, rina.tenantID, cleanPhoto)
	t2 := tick()
	short := w.deposit(t, w.karyawan, budi.tenantID, 700_000, 650_000, 650_000, "Kurang Rp50.000, sudah ditanyakan")
	t3 := tick()
	receipt := w.infectedReceipt(t, w.karyawan, rina.tenantID)
	t4 := tick()
	infectedProof := w.infectedProof(t, rina.tenantID)
	t5 := tick()
	webhookMismatch := w.qrisPaid(t, citra.tenantID, 700_000, 650_000, false)
	t6 := tick()
	reconciledMismatch := w.qrisPaid(t, dewi.tenantID, 700_000, 650_000, true)
	t7 := tick()
	number := w.newNumber(t, w.karyawan, budi.tenantID, "0812-7719-0001", "Nomor lama hilang")
	t8 := tick()
	over := w.deposit(t, w.dina, hadi.tenantID, 300_000, 300_000, 310_000, "Lebih Rp10.000")
	// Never on the queue: a deposit that matched, a QRIS payment at its QR's amount, and a visit
	// outcome that is not a new number.
	tick()
	matched := w.deposit(t, w.karyawan, eko.tenantID, 200_000, 200_000, 200_000, "")
	settled := w.qrisPaid(t, eko.tenantID, 300_000, 300_000, false)
	w.visitOutcome(t, w.karyawan, eko.tenantID, visit.Input{Outcome: visit.OutcomeNotHome})

	wantOrder := []struct {
		kind string
		id   uuid.UUID
		at   time.Time
	}{
		{checkqueue.KindTransferProof, proof.ID, t1},
		{checkqueue.KindDepositGap, short.ID, t2},
		{checkqueue.KindUploadInfected, receipt, t3},
		{checkqueue.KindUploadInfected, infectedProof.UploadID, t4},
		{checkqueue.KindQrisAmountMismatch, webhookMismatch, t5},
		{checkqueue.KindQrisAmountMismatch, reconciledMismatch, t6},
		{checkqueue.KindPhoneChanged, number.ID, t7},
		{checkqueue.KindDepositGap, over.ID, t8},
	}
	wantCounts := map[string]int{
		checkqueue.KindTransferProof: 1, checkqueue.KindDepositGap: 2, checkqueue.KindUploadInfected: 2,
		checkqueue.KindQrisAmountMismatch: 2, checkqueue.KindPhoneChanged: 1,
	}
	rinaUnits := []checkqueue.Unit{unit("Kamar 1", kos, true), unit("Kamar 2", kos, false)}

	t.Run("every kind is listed oldest first with its counts", func(t *testing.T) {
		page := w.list(t, w.admin, "", listing.FirstPage)
		require.Len(t, page.Items, len(wantOrder))
		for i, want := range wantOrder {
			it := page.Items[i]
			require.Equal(t, want.kind, it.Kind, "item %d", i)
			require.Equal(t, want.id, it.ID, "item %d", i)
			require.True(t, it.CreatedAt.Equal(want.at), "item %d arrived %s, want %s", i, it.CreatedAt, want.at)
		}
		require.Equal(t, wantCounts, page.Counts)
		require.Equal(t, 8, page.Waiting)
		require.Equal(t, 8, page.Total)
		for _, id := range []uuid.UUID{matched.ID, settled, proof.UploadID, infectedProof.ID} {
			require.NotContains(t, w.waiting(t), id, "never on the queue")
		}
	})

	t.Run("each item carries its kind's details", func(t *testing.T) {
		items := w.list(t, w.admin, "", listing.FirstPage).Items

		tp := items[0].TransferProof
		require.NotNil(t, tp)
		require.Nil(t, items[0].DepositGap)
		require.Equal(t, checkqueue.TenantRef{ID: rina.tenantID, Name: "Rina Wulandari", Units: rinaUnits}, tp.Tenant)
		require.Equal(t, proof.InvoiceID, tp.InvoiceID)
		require.Equal(t, proof.UploadID, tp.UploadID)
		require.Equal(t, upload.StatusPending, tp.UploadStatus, "still being scanned")

		gap := items[1].DepositGap
		require.NotNil(t, gap)
		require.Equal(t, checkqueue.StaffRef{ID: w.org.Karyawan.UserID, Name: w.org.Karyawan.Name}, gap.Depositor)
		require.Equal(t, int64(700_000), gap.Expected)
		require.Equal(t, int64(650_000), gap.Received)
		require.Equal(t, int64(-50_000), gap.Gap, "a shortfall is below zero")
		require.Equal(t, 1, gap.PaymentCount)
		require.Equal(t, checkqueue.StaffRef{ID: w.org.Admin.UserID, Name: w.org.Admin.Name}, gap.ConfirmedBy)
		require.True(t, gap.ConfirmedAt.Equal(t2))
		overGap := items[7].DepositGap
		require.NotNil(t, overGap)
		require.Equal(t, checkqueue.StaffRef{ID: w.dina.UserID, Name: "Dina Kasir"}, overGap.Depositor)
		require.Equal(t, int64(10_000), overGap.Gap, "money over is above zero")

		inf := items[2].UploadInfected
		require.NotNil(t, inf)
		require.Equal(t, receipt, inf.UploadID)
		require.Equal(t, upload.PurposeCashReceipt, inf.Purpose)
		require.NotNil(t, inf.Tenant)
		require.Equal(t, checkqueue.TenantRef{ID: rina.tenantID, Name: "Rina Wulandari", Units: rinaUnits}, *inf.Tenant)
		require.Equal(t, &checkqueue.StaffRef{ID: w.org.Karyawan.UserID, Name: w.org.Karyawan.Name}, inf.UploadedBy)
		require.NotNil(t, inf.ScannedAt)
		require.True(t, inf.ScannedAt.Equal(t3))

		infProof := items[3].UploadInfected
		require.NotNil(t, infProof, "an infected proof is an infected upload, not a proof to match")
		require.Equal(t, upload.PurposeTransferProof, infProof.Purpose)
		require.NotNil(t, infProof.Tenant)
		require.Equal(t, rina.tenantID, infProof.Tenant.ID)
		require.Nil(t, infProof.UploadedBy, "a tenant's proof has no staff uploader")

		hook := items[4].QrisAmountMismatch
		require.NotNil(t, hook)
		require.Equal(t, webhookMismatch, hook.AttemptID)
		require.Equal(t, checkqueue.TenantRef{ID: citra.tenantID, Name: "Citra Lestari", Units: []checkqueue.Unit{unit(citra.label, kos, true)}}, hook.Tenant)
		require.NotEmpty(t, hook.TransactionID)
		require.Equal(t, int64(700_000), hook.QRAmount)
		require.Equal(t, ptr(int64(650_000)), hook.RecordedAmount)
		require.Equal(t, ptr(int64(650_000)), hook.ReportedAmount)

		rec := items[5].QrisAmountMismatch
		require.NotNil(t, rec)
		require.Equal(t, dewi.tenantID, rec.Tenant.ID)
		require.Equal(t, int64(700_000), rec.QRAmount)
		require.Equal(t, ptr(int64(700_000)), rec.RecordedAmount, "reconcile recorded the QR's amount")
		require.Nil(t, rec.ReportedAmount, "the reported amount is only in the server log")

		ph := items[6].PhoneChanged
		require.NotNil(t, ph)
		require.Equal(t, checkqueue.TenantRef{ID: budi.tenantID, Name: "Budi Santoso", Units: []checkqueue.Unit{unit(budi.label, kos, true)}}, ph.Tenant)
		require.Equal(t, budi.phone, ph.CurrentPhone)
		require.Equal(t, "+6281277190001", ph.NewPhone)
		require.Equal(t, checkqueue.StaffRef{ID: w.org.Karyawan.UserID, Name: w.org.Karyawan.Name}, ph.RecordedBy)
		require.Equal(t, "Nomor lama hilang", ph.Note)
	})

	t.Run("a proof's upload status turns clean after its scan", func(t *testing.T) {
		w.scan(t, proof.UploadID, upload.StatusClean)
		items := w.list(t, w.admin, checkqueue.KindTransferProof, listing.FirstPage).Items
		require.Len(t, items, 1)
		require.Equal(t, upload.StatusClean, items[0].TransferProof.UploadStatus)
	})

	t.Run("a kind filter lists only that kind and keeps every count", func(t *testing.T) {
		for _, kind := range checkqueue.Kinds {
			t.Run(kind, func(t *testing.T) {
				page := w.list(t, w.admin, kind, listing.FirstPage)
				require.Equal(t, wantCounts[kind], page.Total)
				require.Len(t, page.Items, wantCounts[kind])
				for _, it := range page.Items {
					require.Equal(t, kind, it.Kind)
				}
				require.Equal(t, wantCounts, page.Counts, "counts whatever kind was asked for")
				require.Equal(t, 8, page.Waiting)
			})
		}
	})

	t.Run("pages keep the order and the total", func(t *testing.T) {
		tests := []struct {
			name string
			page listing.Page
			want []int // indexes into wantOrder
		}{
			{"the first three", listing.Page{Number: 1, Size: 3}, []int{0, 1, 2}},
			{"the next three", listing.Page{Number: 2, Size: 3}, []int{3, 4, 5}},
			{"the last two", listing.Page{Number: 3, Size: 3}, []int{6, 7}},
			{"past the end", listing.Page{Number: 4, Size: 3}, nil},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				page := w.list(t, w.admin, "", tc.page)
				require.Equal(t, 8, page.Total)
				require.NotNil(t, page.Items, "never nil")
				require.Len(t, page.Items, len(tc.want))
				for i, idx := range tc.want {
					require.Equal(t, wantOrder[idx].id, page.Items[i].ID)
				}
			})
		}
		page := w.list(t, w.admin, checkqueue.KindDepositGap, listing.Page{Number: 2, Size: 1})
		require.Equal(t, 2, page.Total)
		require.Len(t, page.Items, 1)
		require.Equal(t, over.ID, page.Items[0].ID)
	})

	t.Run("an unknown kind is refused", func(t *testing.T) {
		_, err := w.queue.List(ctx, w.admin, "bukti_lain", listing.FirstPage)
		requireInput(t, err, "kind")
	})
}

// TestCheckQueueAccess: the admin and the Supervisor read the queue; a karyawan and an admin of one
// property may not, and are audited; no organization sees another's items.
func TestCheckQueueAccess(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	a := newWorld(t, env, "Griya Akses", "0812772")
	b := newWorld(t, env, "Kos Sebelah", "0812773")
	aRina := a.rent(t, "Rina Wulandari", day(2026, 11, 14), uuid.Nil)
	bRina := b.rent(t, "Rina Kusuma", day(2026, 11, 14), uuid.Nil)
	aProof := a.cleanProof(t, aRina.tenantID)
	bProof := b.cleanProof(t, bRina.tenantID)
	joko, _ := a.invite(t, "0812772098", "Pak Joko", member.RoleGrant{Role: authz.RoleAdmin, PropertyID: a.kos.ID})

	for _, tc := range []struct {
		name string
		p    authz.Principal
	}{
		{"the admin", a.admin},
		{"the Supervisor", a.supervisor},
	} {
		t.Run(tc.name+" reads the queue", func(t *testing.T) {
			page := a.list(t, tc.p, "", listing.FirstPage)
			require.Len(t, page.Items, 1)
			require.Equal(t, aProof.ID, page.Items[0].ID)
		})
	}

	for _, tc := range []struct {
		name string
		p    authz.Principal
	}{
		{"a karyawan", a.karyawan},
		{"an admin of one property", joko},
	} {
		t.Run(tc.name+" may not, and is audited", func(t *testing.T) {
			before := a.audited(t, tc.p.UserID, authz.ActionPaymentCheck, "check_queue", "deny")
			_, err := a.queue.List(ctx, tc.p, "", listing.FirstPage)
			require.ErrorIs(t, err, authz.ErrForbidden)
			require.Equal(t, before+1, a.audited(t, tc.p.UserID, authz.ActionPaymentCheck, "check_queue", "deny"))
		})
	}

	t.Run("each organization sees only its own items", func(t *testing.T) {
		require.Equal(t, []uuid.UUID{aProof.ID}, a.waiting(t))
		require.Equal(t, []uuid.UUID{bProof.ID}, b.waiting(t))
	})

	t.Run("another organization's item cannot be answered", func(t *testing.T) {
		_, _, err := a.queue.Match(ctx, a.admin, bProof.ID, a.matchInput(700_000, ""))
		require.ErrorIs(t, err, checkqueue.ErrNotFound)
		_, err = a.queue.Dismiss(ctx, a.admin, checkqueue.KindTransferProof, bProof.ID, "Bukan pembayaran sewa")
		require.ErrorIs(t, err, checkqueue.ErrNotFound)
		require.Equal(t, []uuid.UUID{bProof.ID}, b.waiting(t))
	})
}

// TestCheckQueueMatch turns transfer proofs into transfer payments. Rina's lease started 14 Oct
// and was entered 14 Nov, so she owes two invoices of Rp700.000.
func TestCheckQueueMatch(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya Cocok", "0812774")
	rina := w.rent(t, "Rina Wulandari", day(2026, 10, 14), w.karyawan.MembershipID)
	budi := w.rent(t, "Budi Santoso", day(2026, 11, 14), uuid.Nil)
	invoices := w.account(t, rina.tenantID).Invoices
	require.Len(t, invoices, 2)
	joko, _ := w.invite(t, "0812774098", "Pak Joko", member.RoleGrant{Role: authz.RoleAdmin, PropertyID: w.kos.ID})

	first := w.cleanProof(t, rina.tenantID)
	in := w.matchInput(1_000_000, "Transfer BCA a.n. Rina")
	var res checkqueue.Resolution
	var pay billing.PaymentResult

	t.Run("a proof becomes a transfer payment, FIFO over the oldest invoices first", func(t *testing.T) {
		before := w.payments(t)
		var err error
		res, pay, err = w.queue.Match(ctx, w.admin, first.ID, in)
		require.NoError(t, err)
		require.True(t, pay.Created)
		require.Equal(t, before+1, w.payments(t))
		require.Equal(t, billing.MethodTransfer, pay.Payment.Method)
		require.Equal(t, int64(1_000_000), pay.Payment.Amount)
		require.Equal(t, "Transfer BCA a.n. Rina", pay.Payment.Note)
		require.True(t, pay.Payment.ReceivedOn.Equal(tenant.Today(env.Clock)))
		require.Len(t, pay.Paid, 2)
		require.Equal(t, invoices[0].ID, pay.Paid[0].InvoiceID)
		require.Equal(t, int64(700_000), pay.Paid[0].Amount)
		require.Equal(t, invoices[1].ID, pay.Paid[1].InvoiceID)
		require.Equal(t, int64(300_000), pay.Paid[1].Amount)
		require.Equal(t, int64(400_000), pay.Outstanding)
		require.Zero(t, pay.Credit)

		require.Equal(t, checkqueue.KindTransferProof, res.Kind)
		require.Equal(t, first.ID, res.ItemID)
		require.Equal(t, checkqueue.OutcomeMatched, res.Outcome)
		require.Equal(t, pay.Payment.ID, res.PaymentID)
		require.Equal(t, "Transfer BCA a.n. Rina", res.Note)
		require.Equal(t, w.org.Admin.UserID, res.By)
		require.True(t, res.CreatedAt.Equal(env.Clock.Now()))
		require.NotEqual(t, uuid.Nil, res.ID)

		require.NotContains(t, w.waiting(t), first.ID)
		require.NotContains(t, w.proofsWaiting(t, rina.tenantID), first.ID, "the payment page no longer lists it")
		require.Equal(t, 1, w.auditedAction(t, w.org.Admin.UserID, authz.ActionPaymentRecord, "allow"))
		require.Equal(t, 1, w.audited(t, w.org.Admin.UserID, authz.ActionPaymentCheck, "transfer_proof/"+first.ID.String(), "allow"))
	})

	t.Run("the same requestId again answers the same payment and resolution, recording nothing", func(t *testing.T) {
		before, resolved := w.payments(t), w.resolutions(t)
		again, againPay, err := w.queue.Match(ctx, w.admin, first.ID, in)
		require.NoError(t, err)
		require.False(t, againPay.Created)
		require.Equal(t, pay.Payment.ID, againPay.Payment.ID)
		require.Equal(t, res.ID, again.ID)
		require.Equal(t, res.PaymentID, again.PaymentID)
		require.Equal(t, before, w.payments(t))
		require.Equal(t, resolved, w.resolutions(t))
	})

	t.Run("an amount above what is owed becomes credit", func(t *testing.T) {
		p := w.cleanProof(t, rina.tenantID)
		_, got, err := w.queue.Match(ctx, w.admin, p.ID, w.matchInput(600_000, ""))
		require.NoError(t, err)
		require.Len(t, got.Paid, 1)
		require.Equal(t, invoices[1].ID, got.Paid[0].InvoiceID)
		require.Equal(t, int64(400_000), got.Paid[0].Amount)
		require.Zero(t, got.Outstanding)
		require.Equal(t, int64(200_000), got.Credit)
	})

	t.Run("a proof can be matched to another tenant's account", func(t *testing.T) {
		p := w.cleanProof(t, rina.tenantID)
		rinaPayments := len(w.account(t, rina.tenantID).Payments)
		mi := w.matchInput(700_000, "Rina membayar untuk Budi")
		mi.TenantID = budi.tenantID
		r, got, err := w.queue.Match(ctx, w.admin, p.ID, mi)
		require.NoError(t, err)
		require.Equal(t, p.ID, r.ItemID)
		require.Zero(t, got.Outstanding)
		acct := w.account(t, budi.tenantID)
		require.Len(t, acct.Payments, 1)
		require.Equal(t, got.Payment.ID, acct.Payments[0].ID)
		require.Equal(t, billing.MethodTransfer, acct.Payments[0].Method)
		require.Len(t, w.account(t, rina.tenantID).Payments, rinaPayments, "nothing on Rina's account")
	})

	waitingProof := w.cleanProof(t, rina.tenantID)

	t.Run("a requestId that recorded another proof's payment is refused", func(t *testing.T) {
		tests := []struct {
			name    string
			in      checkqueue.MatchInput
			wantErr error
		}{
			{"the same input", in, checkqueue.ErrRequestIDReused},
			{"another amount", checkqueue.MatchInput{RequestID: in.RequestID, Amount: 500_000, ReceivedOn: in.ReceivedOn}, billing.ErrRequestIDReused},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before, resolved := w.payments(t), w.resolutions(t)
				_, _, err := w.queue.Match(ctx, w.admin, waitingProof.ID, tc.in)
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, before, w.payments(t))
				require.Equal(t, resolved, w.resolutions(t))
				require.Contains(t, w.waiting(t), waitingProof.ID)
			})
		}
	})

	t.Run("an answered proof is ErrResolved and records no second payment", func(t *testing.T) {
		before := w.payments(t)
		_, _, err := w.queue.Match(ctx, w.admin, first.ID, w.matchInput(1_000_000, ""))
		require.ErrorIs(t, err, checkqueue.ErrResolved)
		require.Equal(t, before, w.payments(t))
	})

	t.Run("a photo not ready or refused records no payment", func(t *testing.T) {
		pending := w.sendProof(t, rina.tenantID, cleanPhoto)
		rejected := w.rejectedProof(t, rina.tenantID)
		infected := w.infectedProof(t, rina.tenantID)
		tests := []struct {
			name    string
			id      uuid.UUID
			wantErr error
		}{
			{"a photo still being scanned", pending.ID, checkqueue.ErrUploadNotReady},
			{"a rejected photo", rejected.ID, checkqueue.ErrUploadRefused},
			{"an infected photo", infected.ID, checkqueue.ErrUploadRefused},
			{"an unknown proof", uuid.New(), checkqueue.ErrNotFound},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before, resolved := w.payments(t), w.resolutions(t)
				_, _, err := w.queue.Match(ctx, w.admin, tc.id, w.matchInput(100_000, ""))
				require.ErrorIs(t, err, tc.wantErr)
				require.Equal(t, before, w.payments(t), "the payment is rolled back")
				require.Equal(t, resolved, w.resolutions(t))
			})
		}
		require.Contains(t, w.waiting(t), pending.ID, "a pending proof still waits")
	})

	t.Run("a karyawan and the Supervisor may not record the payment, and are audited", func(t *testing.T) {
		for _, p := range []authz.Principal{w.karyawan, w.supervisor} {
			before := w.payments(t)
			denied := w.auditedAction(t, p.UserID, authz.ActionPaymentRecord, "deny")
			_, _, err := w.queue.Match(ctx, p, waitingProof.ID, w.matchInput(100_000, ""))
			require.ErrorIs(t, err, authz.ErrForbidden)
			require.Equal(t, denied+1, w.auditedAction(t, p.UserID, authz.ActionPaymentRecord, "deny"))
			require.Equal(t, before, w.payments(t))
		}
		require.Contains(t, w.waiting(t), waitingProof.ID)
	})

	t.Run("an admin of one property is denied payment:check and the payment is rolled back", func(t *testing.T) {
		before, resolved := w.payments(t), w.resolutions(t)
		resource := "transfer_proof/" + waitingProof.ID.String()
		_, _, err := w.queue.Match(ctx, joko, waitingProof.ID, w.matchInput(100_000, ""))
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Equal(t, 1, w.audited(t, joko.UserID, authz.ActionPaymentCheck, resource, "deny"))
		require.Zero(t, w.auditedAction(t, joko.UserID, authz.ActionPaymentRecord, "allow"), "the allowed payment:record rolled back with it")
		require.Equal(t, before, w.payments(t))
		require.Equal(t, resolved, w.resolutions(t))
		require.Contains(t, w.waiting(t), waitingProof.ID)
	})

	t.Run("invalid input is refused", func(t *testing.T) {
		tests := []struct {
			name  string
			in    checkqueue.MatchInput
			field string
		}{
			{"no amount", checkqueue.MatchInput{RequestID: uuid.New(), ReceivedOn: tenant.Today(env.Clock)}, "amount"},
			{"received tomorrow", checkqueue.MatchInput{RequestID: uuid.New(), Amount: 1, ReceivedOn: tenant.Today(env.Clock).AddDate(0, 0, 1)}, "receivedOn"},
			{"no requestId", checkqueue.MatchInput{Amount: 1, ReceivedOn: tenant.Today(env.Clock)}, "requestId"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := w.payments(t)
				_, _, err := w.queue.Match(ctx, w.admin, waitingProof.ID, tc.in)
				var ie *billing.InputError
				require.True(t, errors.As(err, &ie), "expected an input error, got %v", err)
				require.Equal(t, tc.field, ie.Field)
				require.Equal(t, before, w.payments(t))
			})
		}
		_, _, err := w.queue.Match(ctx, w.admin, waitingProof.ID, w.matchInput(1, strings.Repeat("a", 501)))
		requireInput(t, err, "note")
	})
}

// TestCheckQueueConcurrentMatch matches the same proof twice at once with different requestIds:
// one payment and one resolution, the other answer ErrResolved. Several rounds, so both orders
// are exercised.
func TestCheckQueueConcurrentMatch(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya Bersamaan", "0812775")
	rina := w.rent(t, "Rina Wulandari", day(2026, 11, 14), uuid.Nil)

	for round := range 5 {
		proof := w.cleanProof(t, rina.tenantID)
		before, resolved := w.payments(t), w.resolutions(t)
		errs := make([]error, 2)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := range errs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				_, _, errs[i] = w.queue.Match(context.Background(), w.admin, proof.ID, w.matchInput(150_000, ""))
			}()
		}
		close(start)
		wg.Wait()

		ok := 0
		for _, err := range errs {
			if err == nil {
				ok++
				continue
			}
			require.ErrorIs(t, err, checkqueue.ErrResolved, "round %d", round)
		}
		require.Equal(t, 1, ok, "round %d: exactly one match", round)
		require.Equal(t, before+1, w.payments(t), "round %d: one payment", round)
		require.Equal(t, resolved+1, w.resolutions(t), "round %d: one resolution", round)
	}
}

// TestCheckQueueApply makes a karyawan's new number the tenant's.
func TestCheckQueueApply(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya Nomor", "0812776")
	rina := w.rent(t, "Rina Wulandari", day(2026, 11, 14), w.karyawan.MembershipID)
	budi := w.rent(t, "Budi Santoso", day(2026, 11, 14), w.karyawan.MembershipID)
	citra := w.rent(t, "Citra Lestari", day(2026, 11, 14), w.karyawan.MembershipID)
	phoneOf := func(t *testing.T, id uuid.UUID) string {
		t.Helper()
		tn, err := env.Tenants.Get(ctx, w.admin, id)
		require.NoError(t, err)
		return tn.Phone
	}
	budiNumber := w.newNumber(t, w.karyawan, budi.tenantID, "0812-7769-0001", "")

	t.Run("the new number becomes the tenant's and the item leaves the queue", func(t *testing.T) {
		managed := w.auditedAction(t, w.org.Admin.UserID, authz.ActionTenantManage, "allow")
		res, err := w.queue.Apply(ctx, w.admin, budiNumber.ID)
		require.NoError(t, err)
		require.Equal(t, checkqueue.KindPhoneChanged, res.Kind)
		require.Equal(t, budiNumber.ID, res.ItemID)
		require.Equal(t, checkqueue.OutcomeApplied, res.Outcome)
		require.Equal(t, uuid.Nil, res.PaymentID)
		require.Equal(t, w.org.Admin.UserID, res.By)
		require.Equal(t, "+6281277690001", phoneOf(t, budi.tenantID))
		require.NotContains(t, w.waiting(t), budiNumber.ID)
		require.Equal(t, 1, w.audited(t, w.org.Admin.UserID, authz.ActionPaymentCheck, "phone_changed/"+budiNumber.ID.String(), "allow"))
		require.Equal(t, managed+1, w.auditedAction(t, w.org.Admin.UserID, authz.ActionTenantManage, "allow"))
	})

	t.Run("an applied number is ErrResolved", func(t *testing.T) {
		_, err := w.queue.Apply(ctx, w.admin, budiNumber.ID)
		require.ErrorIs(t, err, checkqueue.ErrResolved)
		_, err = w.queue.Dismiss(ctx, w.admin, checkqueue.KindPhoneChanged, budiNumber.ID, "Sudah dipakai")
		require.ErrorIs(t, err, checkqueue.ErrResolved)
	})

	t.Run("a number another tenant has is refused and the item stays", func(t *testing.T) {
		taken := w.newNumber(t, w.karyawan, citra.tenantID, rina.phone, "Katanya nomor kakaknya")
		before := phoneOf(t, citra.tenantID)
		resolved := w.resolutions(t)
		_, err := w.queue.Apply(ctx, w.admin, taken.ID)
		require.ErrorIs(t, err, tenant.ErrPhoneTaken)
		require.Equal(t, before, phoneOf(t, citra.tenantID))
		require.Equal(t, resolved, w.resolutions(t))
		require.Contains(t, w.waiting(t), taken.ID)
	})

	rinaNumber := w.newNumber(t, w.karyawan, rina.tenantID, "0812-7769-0002", "")

	t.Run("applying needs tenant:manage as well as payment:check", func(t *testing.T) {
		// Sinta checks payments through a custom role but may not edit tenants.
		sinta, sintaMembership := w.invite(t, "0812776098", "Sinta Pemeriksa", member.RoleGrant{Role: authz.RoleKaryawan})
		env.GrantCustomRole(t, w.org.ID, sintaMembership, "Pemeriksa Pembayaran",
			`{"Version":"2026-10-25","Statement":[{"Effect":"Allow","Action":["payment:check"],"Resource":["*"]}]}`)
		sinta = env.Principal(t, w.org.ID, sinta.UserID)
		tests := []struct {
			name   string
			p      authz.Principal
			action string
		}{
			{"a member without tenant:manage", sinta, authz.ActionTenantManage},
			{"a karyawan", w.karyawan, authz.ActionPaymentCheck},
			{"the Supervisor", w.supervisor, authz.ActionPaymentCheck},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				denied := w.auditedAction(t, tc.p.UserID, tc.action, "deny")
				_, err := w.queue.Apply(ctx, tc.p, rinaNumber.ID)
				require.ErrorIs(t, err, authz.ErrForbidden)
				require.Equal(t, denied+1, w.auditedAction(t, tc.p.UserID, tc.action, "deny"))
				require.Equal(t, rina.phone, phoneOf(t, rina.tenantID))
				require.Contains(t, w.waiting(t), rinaNumber.ID)
			})
		}
	})

	t.Run("an outcome that is not a new number, or none, is not found", func(t *testing.T) {
		notHome := w.visitOutcome(t, w.karyawan, rina.tenantID, visit.Input{Outcome: visit.OutcomeNotHome})
		for _, id := range []uuid.UUID{notHome.ID, uuid.New()} {
			_, err := w.queue.Apply(ctx, w.admin, id)
			require.ErrorIs(t, err, checkqueue.ErrNotFound)
		}
	})
}

// TestCheckQueueDismiss takes items of every kind off the queue with a note.
func TestCheckQueueDismiss(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya Tutup", "0812777")
	rina := w.rent(t, "Rina Wulandari", day(2026, 11, 14), w.karyawan.MembershipID)
	budi := w.rent(t, "Budi Santoso", day(2026, 11, 14), w.karyawan.MembershipID)
	citra := w.rent(t, "Citra Lestari", day(2026, 11, 14), uuid.Nil)

	proof := w.cleanProof(t, rina.tenantID)
	gap := w.deposit(t, w.karyawan, budi.tenantID, 700_000, 650_000, 650_000, "Kurang Rp50.000")
	openGap := w.deposit(t, w.karyawan, budi.tenantID, 100_000, 90_000, 90_000, "Kurang Rp10.000")
	noGap := w.deposit(t, w.karyawan, rina.tenantID, 100_000, 100_000, 100_000, "")
	uncounted := w.deposit(t, w.karyawan, rina.tenantID, 50_000, 50_000, -1, "")
	infected := w.infectedReceipt(t, w.karyawan, rina.tenantID)
	mismatch := w.qrisPaid(t, citra.tenantID, 700_000, 650_000, false)
	number := w.newNumber(t, w.karyawan, budi.tenantID, "0812-7779-0001", "")

	t.Run("a note is required and bounded", func(t *testing.T) {
		for _, note := range []string{"", "   ", strings.Repeat("a", 501)} {
			_, err := w.queue.Dismiss(ctx, w.admin, checkqueue.KindTransferProof, proof.ID, note)
			requireInput(t, err, "note")
		}
		require.Contains(t, w.waiting(t), proof.ID)
	})

	t.Run("an unknown kind or item, or one never on the queue, is not found", func(t *testing.T) {
		tests := []struct {
			name string
			kind string
			id   uuid.UUID
		}{
			{"an unknown kind", "bukti_lain", proof.ID},
			{"an item under another kind", checkqueue.KindDepositGap, proof.ID},
			{"an unknown id", checkqueue.KindTransferProof, uuid.New()},
			{"a deposit with no gap", checkqueue.KindDepositGap, noGap.ID},
			{"a deposit not counted yet", checkqueue.KindDepositGap, uncounted.ID},
			{"an upload that is not infected", checkqueue.KindUploadInfected, proof.UploadID},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resolved := w.resolutions(t)
				_, err := w.queue.Dismiss(ctx, w.admin, tc.kind, tc.id, "Sudah ditangani")
				require.ErrorIs(t, err, checkqueue.ErrNotFound)
				require.Equal(t, resolved, w.resolutions(t))
			})
		}
	})

	t.Run("a karyawan and the Supervisor may not dismiss, and are audited", func(t *testing.T) {
		resource := "transfer_proof/" + proof.ID.String()
		for _, p := range []authz.Principal{w.karyawan, w.supervisor} {
			_, err := w.queue.Dismiss(ctx, p, checkqueue.KindTransferProof, proof.ID, "Bukan pembayaran sewa")
			require.ErrorIs(t, err, authz.ErrForbidden)
			require.Equal(t, 1, w.audited(t, p.UserID, authz.ActionPaymentCheck, resource, "deny"))
		}
		require.Contains(t, w.waiting(t), proof.ID)
	})

	t.Run("each kind leaves the queue with its note", func(t *testing.T) {
		tests := []struct {
			kind string
			id   uuid.UUID
		}{
			{checkqueue.KindTransferProof, proof.ID},
			{checkqueue.KindDepositGap, gap.ID},
			{checkqueue.KindUploadInfected, infected},
			{checkqueue.KindQrisAmountMismatch, mismatch},
			{checkqueue.KindPhoneChanged, number.ID},
		}
		for _, tc := range tests {
			t.Run(tc.kind, func(t *testing.T) {
				env.Clock.Advance(time.Minute)
				res, err := w.queue.Dismiss(ctx, w.admin, tc.kind, tc.id, "  Sudah ditangani admin  ")
				require.NoError(t, err)
				require.Equal(t, tc.kind, res.Kind)
				require.Equal(t, tc.id, res.ItemID)
				require.Equal(t, checkqueue.OutcomeDismissed, res.Outcome)
				require.Equal(t, "Sudah ditangani admin", res.Note)
				require.Equal(t, uuid.Nil, res.PaymentID)
				require.Equal(t, w.org.Admin.UserID, res.By)
				require.True(t, res.CreatedAt.Equal(env.Clock.Now()))
				require.NotContains(t, w.waiting(t), tc.id)
				require.Equal(t, 1, w.audited(t, w.org.Admin.UserID, authz.ActionPaymentCheck, tc.kind+"/"+tc.id.String(), "allow"))

				_, err = w.queue.Dismiss(ctx, w.admin, tc.kind, tc.id, "Lagi")
				require.ErrorIs(t, err, checkqueue.ErrResolved)
			})
		}
		require.Equal(t, []uuid.UUID{openGap.ID}, w.waiting(t), "only the other gap waits")
		require.NotContains(t, w.proofsWaiting(t, rina.tenantID), proof.ID, "the payment page no longer lists a dismissed proof")
	})

	t.Run("a dismissed proof cannot be matched", func(t *testing.T) {
		before := w.payments(t)
		_, _, err := w.queue.Match(ctx, w.admin, proof.ID, w.matchInput(700_000, ""))
		require.ErrorIs(t, err, checkqueue.ErrResolved)
		require.Equal(t, before, w.payments(t))
	})

	t.Run("a deposit shows how its gap was closed", func(t *testing.T) {
		got, err := w.deposits.Get(ctx, w.admin, gap.ID)
		require.NoError(t, err)
		require.NotNil(t, got.GapClosure)
		require.Equal(t, "Sudah ditangani admin", got.GapClosure.Note)
		require.Equal(t, w.org.Admin.UserID, got.GapClosure.ClosedBy)
		require.Equal(t, w.org.Admin.Name, got.GapClosure.ClosedByName)
		var resolvedAt time.Time
		w.mustTx(t, func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, "SELECT created_at FROM check_resolution WHERE cash_deposit_id = $1", gap.ID).Scan(&resolvedAt)
		})
		require.True(t, got.GapClosure.ClosedAt.Equal(resolvedAt))

		open, err := w.deposits.Get(ctx, w.admin, openGap.ID)
		require.NoError(t, err)
		require.Nil(t, open.GapClosure, "a gap still waiting is not closed")

		list, err := w.deposits.List(ctx, w.supervisor, deposit.Query{}, listing.FirstPage)
		require.NoError(t, err)
		closures := map[uuid.UUID]*deposit.GapClosure{}
		for _, d := range list.Items {
			closures[d.ID] = d.GapClosure
		}
		require.Len(t, closures, 4)
		require.NotNil(t, closures[gap.ID])
		require.Equal(t, "Sudah ditangani admin", closures[gap.ID].Note)
		require.Nil(t, closures[openGap.ID])
		require.Nil(t, closures[noGap.ID])
		require.Nil(t, closures[uncounted.ID])
	})
}

// TestCheckQueueQrisItemsComeOnlyFromMismatches: a QRIS attempt is on the queue only when
// AutoGoPay reported another amount than its QR. Answering or trying to answer an attempt that
// never had a mismatch must not put it there.
func TestCheckQueueQrisItemsComeOnlyFromMismatches(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya QRIS", "0812778")
	rina := w.rent(t, "Rina Wulandari", day(2026, 11, 14), uuid.Nil)
	budi := w.rent(t, "Budi Santoso", day(2026, 11, 14), uuid.Nil)
	rinaPaid := w.qrisPaid(t, rina.tenantID, 300_000, 300_000, false)
	budiPaid := w.qrisPaid(t, budi.tenantID, 300_000, 300_000, false)
	require.Empty(t, w.waiting(t))

	t.Run("a denied dismissal of a settled attempt leaves the queue empty", func(t *testing.T) {
		_, err := w.queue.Dismiss(ctx, w.karyawan, checkqueue.KindQrisAmountMismatch, rinaPaid, "Coba")
		require.ErrorIs(t, err, authz.ErrForbidden)
		require.Empty(t, w.waiting(t), "the denial's audit row must not read as a mismatch")
	})

	t.Run("the admin cannot dismiss a settled attempt that was never on the queue", func(t *testing.T) {
		resolved := w.resolutions(t)
		_, err := w.queue.Dismiss(ctx, w.admin, checkqueue.KindQrisAmountMismatch, budiPaid, "Sudah dicek")
		require.ErrorIs(t, err, checkqueue.ErrNotFound)
		require.Equal(t, resolved, w.resolutions(t))
	})
}

// TestCheckResolutionSchema checks what the table itself refuses.
func TestCheckResolutionSchema(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	a := newWorld(t, env, "Griya Skema", "0812779")
	b := newWorld(t, env, "Kos Skema", "0812780")
	aRina := a.rent(t, "Rina Wulandari", day(2026, 11, 14), uuid.Nil)
	bRina := b.rent(t, "Rina Kusuma", day(2026, 11, 14), uuid.Nil)
	aProof := a.cleanProof(t, aRina.tenantID)
	aOther := a.cleanProof(t, aRina.tenantID)
	bProof := b.cleanProof(t, bRina.tenantID)
	res, err := a.queue.Dismiss(ctx, a.admin, checkqueue.KindTransferProof, aProof.ID, "Bukan pembayaran sewa")
	require.NoError(t, err)
	_, pay, err := a.queue.Match(ctx, a.admin, aOther.ID, a.matchInput(100_000, ""))
	require.NoError(t, err)

	asApp := func(org uuid.UUID, sql string, args ...any) error {
		return env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}
	owner, err := pgx.Connect(ctx, env.URLs.Owner)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close(context.Background()) })

	t.Run("a resolution cannot be changed or removed", func(t *testing.T) {
		for _, sql := range []string{
			"UPDATE check_resolution SET note = 'diubah' WHERE id = $1",
			"DELETE FROM check_resolution WHERE id = $1",
		} {
			requireCode(t, asApp(a.org.ID, sql, res.ID), "42501")
			_, err := owner.Exec(ctx, sql, res.ID)
			require.Contains(t, pgErrorOf(t, err, "42501").Message, "append-only")
		}
		requireCode(t, asApp(a.org.ID, "TRUNCATE check_resolution"), "42501")
		_, err := owner.Exec(ctx, "TRUNCATE check_resolution")
		require.Contains(t, pgErrorOf(t, err, "42501").Message, "append-only")
		require.Equal(t, 2, a.resolutions(t))
	})

	t.Run("another organization sees none and cannot write one", func(t *testing.T) {
		require.Zero(t, b.resolutions(t))
		require.Zero(t, b.count(t, "SELECT count(*) FROM check_queue_item WHERE organization_id <> app_org_id()"))
		err := asApp(b.org.ID, `INSERT INTO check_resolution (id, organization_id, kind, transfer_proof_id, outcome, note, staff_user_id, created_at)
			VALUES ($1, $2, 'transfer_proof', $3, 'dismissed', 'x', $4, now())`, uuid.New(), a.org.ID, aProof.ID, a.org.Admin.UserID)
		require.Contains(t, pgErrorOf(t, err, "42501").Message, "row-level security")
	})

	t.Run("the row must say what it answers and how", func(t *testing.T) {
		gapItem := uuid.New() // checks run before the foreign keys
		tests := []struct {
			name       string
			kind       string
			proof      *uuid.UUID
			deposit    *uuid.UUID
			outcome    string
			payment    *uuid.UUID
			note       *string
			code       string
			constraint string
		}{
			{"two kinds' columns", checkqueue.KindTransferProof, &bProof.ID, &gapItem, "dismissed", nil, ptr("x"), "23514", "check_resolution_names_its_item"},
			{"no column of its kind", checkqueue.KindDepositGap, &bProof.ID, nil, "dismissed", nil, ptr("x"), "23514", "check_resolution_names_its_item"},
			{"matched without a payment", checkqueue.KindTransferProof, &bProof.ID, nil, "matched", nil, nil, "23514", "check_resolution_matched"},
			{"a payment on a dismissal", checkqueue.KindTransferProof, &bProof.ID, nil, "dismissed", &pay.Payment.ID, ptr("x"), "23514", "check_resolution_matched"},
			{"matched for a deposit gap", checkqueue.KindDepositGap, nil, &gapItem, "matched", &pay.Payment.ID, nil, "23514", "check_resolution_matched_proof"},
			{"applied for a proof", checkqueue.KindTransferProof, &bProof.ID, nil, "applied", nil, nil, "23514", "check_resolution_applied_phone"},
			{"dismissed without a note", checkqueue.KindTransferProof, &bProof.ID, nil, "dismissed", nil, nil, "23514", "check_resolution_dismissed_note"},
			{"an empty note", checkqueue.KindTransferProof, &bProof.ID, nil, "dismissed", nil, ptr(""), "23514", "check_resolution_note"},
			{"a note with spaces around it", checkqueue.KindTransferProof, &bProof.ID, nil, "dismissed", nil, ptr(" x "), "23514", "check_resolution_note"},
			{"an unknown outcome", checkqueue.KindTransferProof, &bProof.ID, nil, "ignored", nil, ptr("x"), "23514", "check_resolution_outcome"},
			{"another organization's proof", checkqueue.KindTransferProof, &bProof.ID, nil, "dismissed", nil, ptr("x"), "23503", ""},
			{"an answered proof", checkqueue.KindTransferProof, &aProof.ID, nil, "dismissed", nil, ptr("x"), "23505", "check_resolution_proof_once"},
			{"a payment already matched", checkqueue.KindTransferProof, &bProof.ID, nil, "matched", &pay.Payment.ID, nil, "23505", "check_resolution_payment_once"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := asApp(a.org.ID, `INSERT INTO check_resolution (id, organization_id, kind, transfer_proof_id, cash_deposit_id, outcome, payment_id, note, staff_user_id, created_at)
					VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())`,
					uuid.New(), a.org.ID, tc.kind, tc.proof, tc.deposit, tc.outcome, tc.payment, tc.note, a.org.Admin.UserID)
				pgErr := pgErrorOf(t, err, tc.code)
				if tc.constraint != "" {
					require.Equal(t, tc.constraint, pgErr.ConstraintName)
				}
			})
		}
	})

	t.Run("each kind's item must be of the same organization", func(t *testing.T) {
		bGap := b.deposit(t, b.karyawan, b.rent(t, "Budi Santoso", day(2026, 11, 14), b.karyawan.MembershipID).tenantID, 100_000, 90_000, 90_000, "Kurang")
		bInfected := b.infectedReceipt(t, b.karyawan, b.rent(t, "Hadi Saputra", day(2026, 11, 14), b.karyawan.MembershipID).tenantID)
		bAttempt := b.qrisPaid(t, bRina.tenantID, 300_000, 250_000, false)
		bCollected := b.rent(t, "Citra Lestari", day(2026, 11, 14), b.karyawan.MembershipID)
		bNumber := b.newNumber(t, b.karyawan, bCollected.tenantID, "0812-7809-0001", "")
		tests := []struct {
			column string
			kind   string
			id     uuid.UUID
		}{
			{"transfer_proof_id", checkqueue.KindTransferProof, bProof.ID},
			{"cash_deposit_id", checkqueue.KindDepositGap, bGap.ID},
			{"upload_id", checkqueue.KindUploadInfected, bInfected},
			{"qris_attempt_id", checkqueue.KindQrisAmountMismatch, bAttempt},
			{"visit_outcome_id", checkqueue.KindPhoneChanged, bNumber.ID},
		}
		for _, tc := range tests {
			t.Run(tc.kind, func(t *testing.T) {
				err := asApp(a.org.ID, `INSERT INTO check_resolution (id, organization_id, kind, `+tc.column+`, outcome, note, staff_user_id, created_at)
					VALUES ($1, $2, $3, $4, 'dismissed', 'x', $5, now())`, uuid.New(), a.org.ID, tc.kind, tc.id, a.org.Admin.UserID)
				requireCode(t, err, "23503")
				err = asApp(b.org.ID, `INSERT INTO check_resolution (id, organization_id, kind, `+tc.column+`, outcome, note, staff_user_id, created_at)
					VALUES ($1, $2, $3, $4, 'dismissed', 'x', $5, now())`, uuid.New(), b.org.ID, tc.kind, tc.id, a.org.Admin.UserID)
				requireCode(t, err, "23503") // A's admin is no member of B
			})
		}
	})
}

func ptr[T any](v T) *T { return &v }

// TestQrisMismatchesUseTheAuditIndex: the queue's QRIS branch reads the platform's mismatch rows
// through their partial index, not the organization's whole audit log, which only grows.
func TestQrisMismatchesUseTheAuditIndex(t *testing.T) {
	env := testenv.New(t)
	ctx := context.Background()
	env.Clock.Set(wib(2026, 11, 14, 9, 0))
	w := newWorld(t, env, "Griya Indeks", "0812781")
	var plan strings.Builder
	require.NoError(t, env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
		// The test database is tiny, so a sequential scan would win on cost; turning it off asks
		// whether the index can serve the join at all.
		if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "EXPLAIN SELECT item_id FROM check_queue_item WHERE organization_id = $1 AND kind = 'qris_amount_mismatch'", w.org.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			plan.WriteString(line + "\n")
		}
		return rows.Err()
	}))
	require.Contains(t, plan.String(), "audit_log_platform_payment_record", plan.String())
}
