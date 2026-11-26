package demoseed

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/deposit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/qris"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/scan/devscan"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/secret"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/visit"
)

// CaseDeps is what SeedCases needs. It builds the domain services itself, each on the clock its
// step needs, from these.
type CaseDeps struct {
	Store *store.Store
	Authz *authz.Service
	Orgs  *org.Service
	// AutoGoPay reads the demo AutoGoPay key that seed-demo stored, for the QRIS cases.
	AutoGoPay *autogopay.Service
	// Files is the upload storage; nil skips the cases that need a photo (see CaseSummary).
	Files *CaseFiles
	Log   *slog.Logger
}

// FileStorage is the bucket: s3store.Store in development, uploadtest.Storage in tests.
type FileStorage interface {
	upload.Presigner
	upload.ObjectWriter
	upload.Objects
}

// CaseFiles is how the cases that need a photo reach storage.
type CaseFiles struct {
	Storage FileStorage
	// Put does the client's PUT of a staff photo to its presigned URL, as the browser does.
	Put func(ctx context.Context, put upload.PresignedPut, body []byte) error
}

// CaseSummary is what SeedCases did. A second run on the same day records nothing new.
type CaseSummary struct {
	Organization string
	Invoices     int // invoices created
	Payments     int // payments recorded, QRIS and cash included
	Promises     int // tenants given a promise
	VisitTasks   int // visit tasks opened
	Outcomes     int // visit outcomes recorded
	Uploads      int // photos stored and scanned
	Deposits     int // cash deposits recorded
	// FilesSkipped is true when Files was nil, so the cases that need a photo (cash, deposits,
	// transfer proofs, the infected upload) were not seeded.
	FilesSkipped bool
}

// How far back the case organization's earlier money happens, in days before today.
const (
	pastBillingAgo = 10 // invoices due by then are made then, so a payment can overpay them
	creditAgo      = 10 // Vina's overpayment
	partlyMadeAgo  = 9  // Gita's promise, for partlyDateAgo
	partlyPaidAgo  = 8  // her part payment
	partlyDateAgo  = 7
	partlyCloseAgo = 6 // closed partly kept
	keptMadeAgo    = 1 // Fikri's promise, kept today
)

// The amounts of the cases, in rupiah.
const (
	creditAmount      = 300_000 // Vina's overpayment beyond what she owed
	partlyPromised    = 600_000
	partlyPaid        = 300_000
	mismatchQR        = 500_000
	mismatchPaid      = 450_000
	depositExactCash  = 500_000
	depositGapCash    = 400_000
	depositGapCounted = 350_000 // the admin counts Rp50.000 short
)

// SeedCases bills the case organization (OrgKasus) and records each of its cases through the
// app's own services, as its admin, its karyawan, its tenants' payment page and the platform's
// daily jobs would. Run seed-demo first. The steps, in order:
//
//  1. invoices due 10 days ago are made on a clock 10 days back, and Vina overpays them then;
//  2. Gita's promise is made, part paid and closed partly kept, Citra's two promises broken, and
//     Fikri's made yesterday, all on clocks in the recent past;
//  3. today's invoices are made, which applies Vina's credit to her next one;
//  4. the admin records transfers (Rizky's and Salsabila's earlier invoices, all of Fikri's debt,
//     which keeps his promise) and Eka's promise, and the QRIS payments go through the payment
//     page and the AutoGoPay webhook's settle path against a stand-in gateway;
//  5. Dedi's house lease is moved out six days ago, owing;
//  6. the daily reminder run opens the h7 visit tasks, and Nanda gets an undeliverable one (nothing
//     opens those before slice 5);
//  7. Yoga records today's visit outcomes;
//  8. with Files, the cases that need a photo: cash with its receipt photo, three deposits (one
//     counted exactly, one short, one waiting), the infected upload and two transfer proofs.
//
// Each step is skipped when what it makes exists: payments by their fixed requestId, promises when
// the tenant has one, deposits by requestId, uploads when the tenant has one of that purpose. Its
// dates are relative to today, so a run on a later day adds nothing and the cases drift; reseed
// after dev-reset for a fresh demo.
func SeedCases(ctx context.Context, d CaseDeps, c clock.Clock) (CaseSummary, error) {
	k, err := newCaseSeeder(ctx, d, c)
	if err != nil {
		return CaseSummary{Organization: OrgKasus}, err
	}
	for _, step := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"past invoices and credit", k.past},
		{"promises", k.promises},
		{"today's invoices", k.invoices},
		{"payments", k.payments},
		{"move-out", k.moveOut},
		{"visit tasks", k.visitTasks},
		{"visit outcomes", k.outcomes},
		{"photos and deposits", k.files},
	} {
		if err := step.run(ctx); err != nil {
			return k.sum, fmt.Errorf("seeding the cases of %s, %s: %w", OrgKasus, step.name, err)
		}
	}
	return k.sum, nil
}

// caseSeeder seeds the case organization.
type caseSeeder struct {
	d     CaseDeps
	g     *generator // pays and breaks promises as GenerateInvoices does, as the admin
	clock clock.Clock
	today time.Time
	org   uuid.UUID
	admin authz.Principal
	yoga  authz.Principal
	dina  authz.Principal
	sum   CaseSummary
}

func newCaseSeeder(ctx context.Context, d CaseDeps, c clock.Clock) (*caseSeeder, error) {
	spec := kasusOrg(c)
	k := &caseSeeder{d: d, clock: c, today: tenant.Today(c), sum: CaseSummary{Organization: OrgKasus, FilesSkipped: d.Files == nil}}
	var err error
	if k.admin, err = k.principal(ctx, spec.Actor); err != nil {
		if errors.Is(err, org.ErrNotFound) {
			return nil, fmt.Errorf("the demo organization %s does not exist; run seed-demo first: %w", OrgKasus, err)
		}
		return nil, err
	}
	k.org = k.admin.OrganizationID
	if k.yoga, err = k.principal(ctx, phoneYoga); err != nil {
		return nil, err
	}
	if k.dina, err = k.principal(ctx, phoneDina); err != nil {
		return nil, err
	}
	k.g = &generator{
		d: GenerateDeps{
			Store: d.Store, Authz: d.Authz, Orgs: d.Orgs, Billing: k.billing(c),
			Promises: func(pc clock.Clock) (*promise.Service, *promise.Closer) {
				return k.promiseService(pc), promise.NewCloser(d.Store, pc, d.Log)
			},
		},
		spec: spec, clock: c, today: k.today, p: k.admin,
	}
	return k, nil
}

// principal loads the member with the phone in the case organization.
func (k *caseSeeder) principal(ctx context.Context, number string) (authz.Principal, error) {
	m, err := k.d.Orgs.FindForMember(ctx, number, OrgKasus)
	if err != nil {
		return authz.Principal{}, fmt.Errorf("finding %s in %s: %w", phone.Mask(number), OrgKasus, err)
	}
	return k.d.Authz.LoadPrincipal(ctx, m.OrganizationID, m.UserID)
}

func (k *caseSeeder) billing(c clock.Clock) *billing.Service {
	return billing.NewService(k.d.Store, k.d.Authz, c, k.d.Log, promise.NewCloser(k.d.Store, c, k.d.Log))
}

func (k *caseSeeder) promiseService(c clock.Clock) *promise.Service {
	return promise.NewService(promise.Deps{Store: k.d.Store, Tenants: k.billing(c), Clock: c, Log: k.d.Log})
}

func (k *caseSeeder) ago(days int) clock.Clock { return Behind(k.clock, days) }

func (k *caseSeeder) day(offset int) time.Time { return k.today.AddDate(0, 0, offset) }

// tenant is the id of the case organization's tenant called name.
func (k *caseSeeder) tenant(ctx context.Context, name string) (uuid.UUID, error) {
	number, err := k.g.phoneOf(name)
	if err != nil {
		return uuid.Nil, err
	}
	return k.g.tenantID(ctx, number)
}

func (k *caseSeeder) paid(recorded bool, err error) error {
	if recorded {
		k.sum.Payments++
	}
	return err
}

// past makes the invoices due by pastBillingAgo days ago on a clock that far back, and records
// Vina's overpayment then: everything she owed plus creditAmount, which stays credit until her
// next invoice is made today.
func (k *caseSeeder) past(ctx context.Context) error {
	res, err := k.billing(k.ago(pastBillingAgo)).GenerateOrg(ctx, k.org)
	k.sum.Invoices += res.Invoices
	if err != nil {
		return err
	}
	overpay := func(a billing.Account) int64 {
		if a.Outstanding == 0 {
			return 0
		}
		return a.Outstanding + creditAmount
	}
	return k.paid(k.g.payOn(ctx, k.billing(k.ago(creditAgo)), k.day(-creditAgo), KasusCredit, "credit",
		overpay, billing.MethodTransfer, "Transfer lebih bayar (data demo)"))
}

// promises makes the promises whose story is in the past: Gita's, partly kept; Citra's two, broken
// (GenerateInvoices' brokenPromises); Fikri's, made yesterday and kept by today's payment.
func (k *caseSeeder) promises(ctx context.Context) error {
	gita, err := k.tenant(ctx, KasusPromisePartly)
	if err != nil {
		return err
	}
	has, err := k.g.hasPromise(ctx, gita)
	if err != nil {
		return err
	}
	if !has {
		date, amount := k.day(-partlyDateAgo), int64(partlyPromised)
		_, err := k.promiseService(k.ago(partlyMadeAgo)).MakeForTenant(ctx, k.admin, gita, promise.Input{
			Kind: promise.KindSpecificDate, PromisedDate: &date, PromisedAmount: &amount,
			Note: "Janji bayar Rp600.000, baru dibayar separuh (data demo)",
		})
		if err != nil {
			return fmt.Errorf("making %s's promise: %w", KasusPromisePartly, err)
		}
		k.sum.Promises++
		part := func(billing.Account) int64 { return partlyPaid }
		if err := k.paid(k.g.payOn(ctx, k.billing(k.ago(partlyPaidAgo)), k.day(-partlyPaidAgo), KasusPromisePartly,
			"partly", part, billing.MethodTransfer, "Bayar sebagian dari janji (data demo)")); err != nil {
			return fmt.Errorf("paying part of %s's promise: %w", KasusPromisePartly, err)
		}
		res, err := promise.NewCloser(k.d.Store, k.ago(partlyCloseAgo), k.d.Log).CloseDue(ctx, k.org)
		if err != nil {
			return fmt.Errorf("closing Gita's promise: %w", err)
		}
		if res.PartiallyKept != 1 {
			return fmt.Errorf("closing Gita's promise: %d closed partly kept, want 1", res.PartiallyKept)
		}
	}

	n, err := k.g.brokenPromises(ctx)
	k.sum.Promises += n
	if err != nil {
		return err
	}

	return k.promise(ctx, KasusPromiseKept, k.ago(keptMadeAgo), 2, "Janji lunas, ditepati (data demo)")
}

// promise gives the tenant a promise for all they owe, made on clock made for the day days after
// today, unless they have one.
func (k *caseSeeder) promise(ctx context.Context, name string, made clock.Clock, days int, note string) error {
	id, err := k.tenant(ctx, name)
	if err != nil {
		return err
	}
	has, err := k.g.hasPromise(ctx, id)
	if err != nil || has {
		return err
	}
	date := k.day(days)
	if _, err := k.promiseService(made).MakeForTenant(ctx, k.admin, id, promise.Input{
		Kind: promise.KindSpecificDate, PromisedDate: &date, Note: note,
	}); err != nil {
		return fmt.Errorf("%s's promise: %w", name, err)
	}
	k.sum.Promises++
	return nil
}

// invoices makes today's invoices, as the daily run would; Vina's credit pays part of her next one.
func (k *caseSeeder) invoices(ctx context.Context) error {
	res, err := k.billing(k.clock).GenerateOrg(ctx, k.org)
	k.sum.Invoices += res.Invoices
	return err
}

// payments records today's transfers, Eka's active promise and the QRIS payments.
func overdue(a billing.Account) int64 { return a.Overdue }

func (k *caseSeeder) payments(ctx context.Context) error {
	for _, p := range []struct {
		name, label string
		amount      func(billing.Account) int64
		note        string
	}{
		{KasusDueSoon, "earlier", overdue, "Transfer tagihan bulan lalu (data demo)"},
		{KasusNotDue, "earlier", overdue, "Transfer tagihan bulan lalu (data demo)"},
		{KasusPromiseKept, "kept", fullAmount, "Transfer lunas sesuai janji (data demo)"},
	} {
		if err := k.paid(k.g.pay(ctx, p.name, p.label, p.amount, billing.MethodTransfer, p.note)); err != nil {
			return fmt.Errorf("payment of %s: %w", p.name, err)
		}
	}
	if err := k.promise(ctx, KasusPromiseActive, k.clock, 3, "Janji bayar lunas 3 hari lagi (data demo)"); err != nil {
		return err
	}
	half := func(a billing.Account) int64 { return a.Outstanding / 2 / 1000 * 1000 }
	for _, q := range []struct {
		name string
		qr   func(billing.Account) int64
		paid func(int64) int64
	}{
		{KasusQRISFull, fullAmount, same},
		{KasusQRISPart, half, same},
		{KasusQRISMismatch, func(billing.Account) int64 { return mismatchQR }, func(int64) int64 { return mismatchPaid }},
	} {
		if err := k.qris(ctx, q.name, q.qr, q.paid); err != nil {
			return fmt.Errorf("QRIS of %s: %w", q.name, err)
		}
	}
	return nil
}

func same(n int64) int64 { return n }

// qris pays through the tenant's payment page: a link to their oldest owing invoice, a QR of
// qr(account) from the stand-in gateway, and the webhook's settle at paid(that amount). A tenant
// with a QRIS payment already is skipped.
func (k *caseSeeder) qris(ctx context.Context, name string, qr func(billing.Account) int64, paid func(int64) int64) error {
	id, err := k.tenant(ctx, name)
	if err != nil {
		return err
	}
	bill := k.billing(k.clock)
	acct, err := bill.Account(ctx, k.admin, id)
	if err != nil {
		return err
	}
	var invoiceID uuid.UUID
	for _, p := range acct.Payments {
		if p.Method == billing.MethodQRIS {
			return nil
		}
	}
	for _, inv := range acct.Invoices {
		if inv.Outstanding > 0 {
			invoiceID = inv.ID
			break
		}
	}
	amount := qr(acct)
	if invoiceID == uuid.Nil || amount <= 0 {
		return nil
	}
	links := paylink.NewService(k.d.Store, k.d.Authz, k.clock, "")
	link, err := links.Mint(ctx, k.admin, invoiceID)
	if err != nil {
		return err
	}
	gw := &standInGateway{clock: k.clock}
	svc := qris.NewService(qris.Deps{
		Store: k.d.Store, Links: links, Clock: k.clock, Gateway: gw, Keys: k.d.AutoGoPay,
		Limiter: allowAll{}, Audit: k.d.Authz, Log: k.d.Log,
	})
	if _, err := svc.MakeQR(ctx, linkToken(link.URL), amount); err != nil {
		return fmt.Errorf("making the QR: %w", err)
	}
	if gw.made == "" {
		return errors.New("the QR was not generated")
	}
	if err := svc.SettlePaid(ctx, k.org, gw.made, paid(amount)); err != nil {
		return fmt.Errorf("settling the QR: %w", err)
	}
	k.sum.Payments++
	return nil
}

// linkToken is the token at the end of a payment link's URL. It is a secret: it is never logged.
func linkToken(url string) string { return url[strings.LastIndex(url, "/")+1:] }

// standInGateway answers like the AutoGoPay stand-in: every QR is generated pending. It records
// the transaction id it made, for the settle.
type standInGateway struct {
	clock clock.Clock
	made  string
}

func (g *standInGateway) Generate(_ context.Context, _ secret.Value, amount int64) (autogopay.QRIS, error) {
	now := g.clock.Now().In(clock.Jakarta)
	g.made = "DEMO-" + uuid.NewString()
	return autogopay.QRIS{
		TransactionID: g.made, OrderID: g.made, Amount: amount, QRString: "00020101021226DEMO" + g.made,
		Status: autogopay.StatusPending, CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute),
	}, nil
}

func (g *standInGateway) Status(_ context.Context, _ secret.Value, id string) (autogopay.Transaction, error) {
	return autogopay.Transaction{ID: id, Status: autogopay.StatusPending}, nil
}

func (g *standInGateway) Cancel(_ context.Context, _ secret.Value, id string) (autogopay.Transaction, error) {
	return autogopay.Transaction{ID: id, Status: autogopay.StatusCancelled}, nil
}

// allowAll is a limiter that never limits: the seed makes one QR per tenant.
type allowAll struct{}

func (allowAll) Allow(context.Context, string, int, time.Duration) (bool, error) { return true, nil }

// moveOut ends Dedi's house lease six days ago, once it is billed, so he moves out owing.
func (k *caseSeeder) moveOut(ctx context.Context) error {
	id, err := k.tenant(ctx, KasusEndedLease)
	if err != nil {
		return err
	}
	tenants := tenant.NewService(k.d.Store, k.d.Authz, k.clock, k.billing(k.clock), nil)
	tn, err := tenants.Get(ctx, k.admin, id)
	if err != nil {
		return err
	}
	for _, ls := range k.g.spec.Tenants {
		if ls.Name != KasusEndedLease {
			continue
		}
		for _, spec := range ls.Leases {
			if spec.EndOn.IsZero() {
				continue
			}
			for _, l := range tn.Leases {
				if l.PropertyName == spec.Property && l.UnitLabel == spec.Unit && l.Status == tenant.StatusActive {
					if _, err := tenants.MoveOut(ctx, k.admin, l.ID, spec.EndOn); err != nil {
						return fmt.Errorf("moving %s out of %s: %w", KasusEndedLease, spec.Unit, err)
					}
				}
			}
		}
	}
	return nil
}

// unsent leaves the reminders the daily run records unsent: the worker's sweep sends them.
type unsent struct{}

func (unsent) EnqueueSend(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// visitTasks runs today's reminder decisions, which open the h7 visit tasks, and opens Nanda's
// undeliverable task with the query the h7 one uses: nothing opens one before slice 5, when a
// WhatsApp send fails.
func (k *caseSeeder) visitTasks(ctx context.Context) error {
	res, err := reminder.NewDecider(k.d.Store, k.clock, k.d.Log, unsent{}).DecideOrg(ctx, k.org)
	k.sum.VisitTasks += res.VisitTasks
	if err != nil {
		return fmt.Errorf("deciding today's reminders: %w", err)
	}
	id, err := k.tenant(ctx, KasusUndeliverable)
	if err != nil {
		return err
	}
	return k.d.Store.WithOrgTx(ctx, k.org, func(tx pgx.Tx) error {
		n, err := dbgen.New(tx).OpenVisitTask(ctx, dbgen.OpenVisitTaskParams{
			ID: uuid.New(), OrganizationID: k.org, TenantID: id, Reason: visit.TaskUndeliverable, CreatedAt: k.clock.Now(),
		})
		k.sum.VisitTasks += int(n)
		return err
	})
}

// caseNamespace derives the request ids of the case organization's outcomes, cash, deposits and
// proofs, so a second run repeats them.
var caseNamespace = uuid.NewSHA1(uuid.NameSpaceOID, []byte("cashflow-app demo cases"))

func caseRequest(label string) uuid.UUID {
	return uuid.NewSHA1(caseNamespace, []byte(OrgKasus+"|"+label))
}

// outcomes records Yoga's visits today: a promise from the field for Irfan (done), not home for
// Joko (still open) and a new number for Kevin (for the admin's queue).
func (k *caseSeeder) outcomes(ctx context.Context) error {
	visits := visit.NewService(k.d.Store, k.billing(k.clock), k.clock)
	irfan, err := k.tenant(ctx, KasusVisitDone)
	if err != nil {
		return err
	}
	has, err := k.g.hasPromise(ctx, irfan)
	if err != nil {
		return err
	}
	if !has {
		date := k.day(4)
		p, err := k.promiseService(k.clock).MakeFromField(ctx, k.yoga, irfan, promise.Input{
			Kind: promise.KindSpecificDate, PromisedDate: &date, Note: "Janji di tempat saat kunjungan (data demo)",
		})
		if err != nil {
			return fmt.Errorf("making %s's promise: %w", KasusVisitDone, err)
		}
		k.sum.Promises++
		if err := k.outcome(ctx, visits, irfan, visit.Input{
			RequestID: caseRequest("visit|irfan"), Outcome: visit.OutcomePromise, PromiseVersionID: p.Latest().ID,
		}); err != nil {
			return fmt.Errorf("recording %s's visit: %w", KasusVisitDone, err)
		}
	}
	for _, o := range []struct {
		name string
		in   visit.Input
	}{
		{KasusVisitNotHome, visit.Input{RequestID: caseRequest("visit|joko"), Outcome: visit.OutcomeNotHome, Note: "Kamar terkunci, kata tetangga pulang kampung"}},
		{KasusNumberChanged, visit.Input{RequestID: caseRequest("visit|kevin"), Outcome: visit.OutcomeNumberChanged, NewPhone: kasusNewNumber}},
	} {
		id, err := k.tenant(ctx, o.name)
		if err != nil {
			return err
		}
		if err := k.outcome(ctx, visits, id, o.in); err != nil {
			return fmt.Errorf("%s's visit: %w", o.name, err)
		}
	}
	return nil
}

func (k *caseSeeder) outcome(ctx context.Context, visits *visit.Service, tenantID uuid.UUID, in visit.Input) error {
	_, created, err := visits.Record(ctx, k.yoga, tenantID, in)
	if created {
		k.sum.Outcomes++
	}
	return err
}

// files seeds the cases that need a photo, when there is storage: the cash Yoga and Dina took
// with its receipt photo, their deposits (Yoga's first counted exactly, his second short; Dina's
// waiting), the infected upload and the two transfer proofs. Each photo is scanned at once with
// the development scanner, which flags only the EICAR test file: the seed's photos are a plain
// JPEG, so any scanner answers the same.
func (k *caseSeeder) files(ctx context.Context) error {
	if k.d.Files == nil {
		return nil
	}
	f, err := newFileCases(k)
	if err != nil {
		return err
	}
	for _, step := range []func(context.Context) error{
		func(ctx context.Context) error {
			return f.cash(ctx, k.yoga, KasusDepositExact, "cash|oki", fixed(depositExactCash))
		},
		func(ctx context.Context) error { return f.deposit(ctx, k.yoga, "deposit|exact", counted(0)) },
		func(ctx context.Context) error {
			return f.cash(ctx, k.yoga, KasusDepositGap, "cash|putri", fixed(depositGapCash))
		},
		func(ctx context.Context) error {
			return f.deposit(ctx, k.yoga, "deposit|gap", counted(depositGapCash-depositGapCounted))
		},
		func(ctx context.Context) error { return f.cash(ctx, k.yoga, KasusCashToday, "cash|wahyu", fullAmount) },
		func(ctx context.Context) error { return f.cash(ctx, k.dina, KasusCashOther, "cash|yusuf", fullAmount) },
		func(ctx context.Context) error { return f.deposit(ctx, k.dina, "deposit|waiting", nil) },
		f.infected,
		f.staffProof,
		f.tenantProof,
	} {
		if err := step(ctx); err != nil {
			return err
		}
	}
	return nil
}

func fixed(n int64) func(billing.Account) int64 { return func(billing.Account) int64 { return n } }

// counted is how much less than it holds the admin counts in a deposit.
func counted(short int64) *int64 { return &short }

// fileCases holds the services of the photo cases.
type fileCases struct {
	k        *caseSeeder
	photo    []byte // demoPhoto
	uploads  *upload.Service
	checker  *upload.Checker
	proofs   *upload.Proofs
	deposits *deposit.Service
	links    *paylink.Service
}

func newFileCases(k *caseSeeder) (*fileCases, error) {
	photo, err := demoPhoto()
	if err != nil {
		return nil, err
	}
	st := k.d.Files.Storage
	links := paylink.NewService(k.d.Store, k.d.Authz, k.clock, "")
	return &fileCases{
		k: k, photo: photo, links: links,
		uploads: upload.NewService(upload.ServiceDeps{
			Store: k.d.Store, Authz: k.d.Authz, Clock: k.clock, Presigner: st, Queue: scannedHere{}, Log: k.d.Log,
		}),
		checker: upload.NewChecker(upload.CheckerDeps{
			Store: k.d.Store, Objects: st, Scanner: devscan.Scanner{}, Audit: k.d.Authz, Clock: k.clock, Log: k.d.Log,
		}),
		proofs: upload.NewProofs(upload.ProofsDeps{
			Store: k.d.Store, Links: links, Objects: st, Queue: scannedHere{}, Clock: k.clock, Log: k.d.Log,
		}),
		deposits: deposit.NewService(k.d.Store, k.d.Authz, k.clock, k.d.Log),
	}, nil
}

// scannedHere queues nothing: the seed scans each upload itself, right after it is completed.
type scannedHere struct{}

func (scannedHere) EnqueueScan(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// cash records cash member p took from the tenant today, amount(account), with a receipt photo,
// unless the payment under label exists.
func (f *fileCases) cash(ctx context.Context, p authz.Principal, name, label string, amount func(billing.Account) int64) error {
	k := f.k
	id, err := k.tenant(ctx, name)
	if err != nil {
		return err
	}
	requestID := caseRequest(label)
	if stored, err := k.g.stored(ctx, id, requestID); err != nil || stored {
		return err
	}
	bill := k.billing(k.clock)
	acct, err := bill.Account(ctx, k.admin, id)
	if err != nil {
		return err
	}
	due := amount(acct)
	if due <= 0 {
		return nil
	}
	photo, err := f.upload(ctx, p, upload.PurposeCashReceipt, id, f.photo)
	if err != nil {
		return fmt.Errorf("%s's receipt photo: %w", name, err)
	}
	if _, err := bill.RecordCashPayment(ctx, p, billing.CashInput{
		RequestID: requestID, TenantID: id, Amount: due, Note: "Tunai diterima saat kunjungan (data demo)", PhotoUploadID: photo,
	}); err != nil {
		return fmt.Errorf("%s's cash: %w", name, err)
	}
	k.sum.Payments++
	return nil
}

// deposit hands over all the cash member p holds, unless the deposit under label exists. With
// short, the admin then counts it, short that much (0: exactly); nil leaves it waiting.
func (f *fileCases) deposit(ctx context.Context, p authz.Principal, label string, short *int64) error {
	k := f.k
	requestID := caseRequest(label)
	var exists bool
	err := k.d.Store.WithOrgTx(ctx, k.org, func(tx pgx.Tx) error {
		_, err := dbgen.New(tx).GetCashDepositByRequestID(ctx, dbgen.GetCashDepositByRequestIDParams{OrganizationID: k.org, RequestID: requestID})
		switch {
		case err == nil:
			exists = true
		case !errors.Is(err, pgx.ErrNoRows):
			return err
		}
		return nil
	})
	if err != nil || exists {
		return err
	}
	held, err := k.billing(k.clock).FieldCash(ctx, p)
	if err != nil {
		return err
	}
	if held.Amount <= 0 {
		return nil
	}
	dep, _, err := f.deposits.Record(ctx, p, deposit.Input{
		RequestID: requestID, ExpectedAmount: held.Amount, ExpectedCount: held.Count, Amount: held.Amount,
		Note: "Setoran tunai hari ini (data demo)",
	})
	if err != nil {
		return fmt.Errorf("recording the deposit: %w", err)
	}
	k.sum.Deposits++
	if short == nil {
		return nil
	}
	note := "Dihitung pas (data demo)"
	if *short > 0 {
		note = "Uang kurang saat dihitung (data demo)"
	}
	if _, _, err := f.deposits.Confirm(ctx, k.admin, dep.ID, deposit.ConfirmInput{ReceivedAmount: held.Amount - *short, Note: note}); err != nil {
		return fmt.Errorf("counting the deposit: %w", err)
	}
	return nil
}

// hasUpload reports whether the tenant has an upload of the purpose.
func (f *fileCases) hasUpload(ctx context.Context, tenantID uuid.UUID, purpose string) (bool, error) {
	var n int64
	err := f.k.d.Store.WithOrgTx(ctx, f.k.org, func(tx pgx.Tx) error {
		var err error
		n, err = dbgen.New(tx).CountTenantUploads(ctx, dbgen.CountTenantUploadsParams{
			OrganizationID: f.k.org, TenantID: uuid.NullUUID{UUID: tenantID, Valid: true}, Purpose: purpose,
		})
		return err
	})
	return n > 0, err
}

// infected is Yoga's receipt photo for Agus that holds the EICAR test file: the scan marks it
// infected, so it waits in Antrean cek as "File ditolak". No payment uses it.
func (f *fileCases) infected(ctx context.Context) error {
	return f.once(ctx, KasusOverdue, upload.PurposeCashReceipt, func(id uuid.UUID) error {
		body := append(slices.Clone(f.photo), devscan.EICAR...)
		_, err := f.upload(ctx, f.k.yoga, upload.PurposeCashReceipt, id, body)
		return err
	})
}

// staffProof is a transfer proof Yoga sent for Bima, waiting for the admin's check.
func (f *fileCases) staffProof(ctx context.Context) error {
	return f.once(ctx, KasusStaffProof, upload.PurposeTransferProof, func(id uuid.UUID) error {
		_, err := f.upload(ctx, f.k.yoga, upload.PurposeTransferProof, id, f.photo)
		return err
	})
}

// tenantProof is a transfer proof Zahra sent from her payment page, waiting for the admin's check.
func (f *fileCases) tenantProof(ctx context.Context) error {
	k := f.k
	return f.once(ctx, KasusTenantProof, upload.PurposeTransferProof, func(id uuid.UUID) error {
		acct, err := k.billing(k.clock).Account(ctx, k.admin, id)
		if err != nil {
			return err
		}
		var invoiceID uuid.UUID
		for _, inv := range acct.Invoices {
			if inv.Outstanding > 0 {
				invoiceID = inv.ID
				break
			}
		}
		if invoiceID == uuid.Nil {
			return nil
		}
		link, err := f.links.Mint(ctx, k.admin, invoiceID)
		if err != nil {
			return err
		}
		body := f.photo
		proof, err := f.proofs.Send(ctx, linkToken(link.URL), caseRequest("proof|zahra"), func() (io.Reader, error) {
			return bytes.NewReader(body), nil
		})
		if err != nil {
			return fmt.Errorf("sending the proof: %w", err)
		}
		return f.scan(ctx, proof.UploadID)
	})
}

// once runs make for the tenant unless they have an upload of the purpose.
func (f *fileCases) once(ctx context.Context, name, purpose string, make func(tenantID uuid.UUID) error) error {
	id, err := f.k.tenant(ctx, name)
	if err != nil {
		return err
	}
	has, err := f.hasUpload(ctx, id, purpose)
	if err != nil || has {
		return err
	}
	if err := make(id); err != nil {
		return fmt.Errorf("%s's %s: %w", name, purpose, err)
	}
	return nil
}

// upload stores body as member p's upload for the tenant, as the field app does (start, PUT,
// complete), and scans it; it returns the upload's id.
func (f *fileCases) upload(ctx context.Context, p authz.Principal, purpose string, tenantID uuid.UUID, body []byte) (uuid.UUID, error) {
	created, err := f.uploads.Create(ctx, p, upload.CreateInput{
		Purpose: purpose, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(len(body)),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("starting the upload: %w", err)
	}
	if err := f.k.d.Files.Put(ctx, created.Put, body); err != nil {
		return uuid.Nil, fmt.Errorf("putting the file: %w", err)
	}
	if _, err := f.uploads.Complete(ctx, p, created.Upload.ID); err != nil {
		return uuid.Nil, fmt.Errorf("completing the upload: %w", err)
	}
	return created.Upload.ID, f.scan(ctx, created.Upload.ID)
}

func (f *fileCases) scan(ctx context.Context, id uuid.UUID) error {
	if _, err := f.checker.Scan(ctx, f.k.org, id); err != nil {
		return fmt.Errorf("scanning upload %s: %w", id, err)
	}
	f.k.sum.Uploads++
	return nil
}

// demoPhoto is a small JPEG of a plain cream square, the palette's background: a real image the
// browser shows.
func demoPhoto() ([]byte, error) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	cream := color.RGBA{R: 0xF5, G: 0xF3, B: 0xEE, A: 0xFF}
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, cream)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		return nil, fmt.Errorf("encoding the demo photo: %w", err)
	}
	return buf.Bytes(), nil
}
