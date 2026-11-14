//go:build integration

package upload_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// world is one organization with uploads wired to the fakes: Dimas (the org's karyawan)
// collects from Rina, Sari (a second karyawan) from Budi, and Eko has no lease.
type world struct {
	env        *testenv.Env
	org        testenv.Org
	admin      authz.Principal
	supervisor authz.Principal
	dimas      authz.Principal
	sari       authz.Principal
	rina       uuid.UUID
	budi       uuid.UUID
	eko        uuid.UUID
	storage    *uploadtest.Storage
	scanner    *uploadtest.Scanner
	queue      *uploadtest.Queue
	uploads    *upload.Service
	checker    *upload.Checker
}

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{env: env, org: env.CreateOrg(t, name, prefix),
		storage: uploadtest.NewStorage(), scanner: &uploadtest.Scanner{}, queue: &uploadtest.Queue{}}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	w.supervisor = env.Principal(t, w.org.ID, w.org.Supervisor.UserID)
	w.dimas = env.Principal(t, w.org.ID, w.org.Karyawan.UserID)
	sariPhone := prefix + "099"
	sari, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: sariPhone, Name: "Mbak Sari", Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)
	w.sari = env.Principal(t, w.org.ID, env.UserID(t, sariPhone))

	kos, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kos Melati " + name, Kind: property.KindKos})
	require.NoError(t, err)
	n := 0
	rent := func(tenantName string, collector uuid.UUID) uuid.UUID {
		n++
		tn, err := env.Tenants.Create(ctx, w.admin, tenant.Input{Name: tenantName, Phone: fmt.Sprintf("%s%04d", prefix, 5000+n)})
		require.NoError(t, err)
		if collector == uuid.Nil {
			return tn.ID
		}
		unit, err := env.Properties.CreateUnit(ctx, w.admin, kos.ID, property.UnitInput{Label: fmt.Sprintf("Kamar %d", n), Terms: testenv.Monthly(700_000)})
		require.NoError(t, err)
		_, err = env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1,
			Terms: tenant.Terms{StartDate: tenant.Today(env.Clock), CollectorMemberID: collector}})
		require.NoError(t, err)
		return tn.ID
	}
	w.rina = rent("Rina Wulandari", w.dimas.MembershipID)
	w.budi = rent("Budi Santoso", sari.ID)
	w.eko = rent("Eko Prasetyo", uuid.Nil)

	w.uploads = upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: w.storage, Queue: w.queue, Log: env.Log,
	})
	w.checker = upload.NewChecker(upload.CheckerDeps{
		Store: env.Store, Objects: w.storage, Scanner: w.scanner, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
	})
	return w
}

// receipt starts a JPEG receipt upload of size bytes for tenantID as p.
func (w *world) receipt(t *testing.T, p authz.Principal, tenantID uuid.UUID, size int) upload.Created {
	t.Helper()
	c, err := w.uploads.Create(context.Background(), p, upload.CreateInput{
		Purpose: upload.PurposeCashReceipt, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(size),
	})
	require.NoError(t, err)
	return c
}

// put stores body where c's presigned PUT points, as the phone would.
func (w *world) put(t *testing.T, c upload.Created, body []byte) {
	t.Helper()
	u, err := url.Parse(c.Put.URL)
	require.NoError(t, err)
	w.storage.Put(strings.TrimPrefix(u.Path, "/"), body)
}

// row reads an upload from the database.
func (w *world) row(t *testing.T, id uuid.UUID) dbgen.Upload {
	t.Helper()
	var row dbgen.Upload
	err := w.env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		var err error
		row, err = dbgen.New(tx).GetUpload(context.Background(), dbgen.GetUploadParams{OrganizationID: w.org.ID, ID: id})
		return err
	})
	require.NoError(t, err)
	return row
}

// uploadAndScan runs the whole flow for body as Dimas for Rina, and returns the upload id.
func (w *world) uploadAndScan(t *testing.T, body []byte, declared int) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	c := w.receipt(t, w.dimas, w.rina, declared)
	w.put(t, c, body)
	_, err := w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
	require.NoError(t, err)
	_, err = w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
	require.NoError(t, err)
	return c.Upload.ID
}

func requireForbidden(t *testing.T, env *testenv.Env, org uuid.UUID, err error, action string) {
	t.Helper()
	require.ErrorIs(t, err, authz.ErrForbidden)
	audit := env.AuditLog(t, org)
	require.Equal(t, "deny", audit[0].Decision)
	require.Equal(t, action, audit[0].Action)
}

func hasAudit(rows []dbgen.AuditLog, action, resource, decision string) bool {
	for _, r := range rows {
		if r.Action == action && r.Resource == resource && r.Decision == decision {
			return true
		}
	}
	return false
}

func TestUploadFlow(t *testing.T) {
	env := testenv.New(t)
	w := newWorld(t, env, "Griya Unggah", "0812881")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 50_000)

	t.Run("a clean receipt photo becomes readable, and a pending one has no link", func(t *testing.T) {
		c := w.receipt(t, w.dimas, w.rina, len(jpeg))
		require.Equal(t, upload.StatusPending, c.Upload.Status)
		require.Equal(t, w.rina, c.Upload.TenantID)
		require.Equal(t, map[string]string{"Content-Type": "image/jpeg"}, c.Put.Headers)
		require.Equal(t, env.Clock.Now().Add(upload.PutURLTTL), c.ExpiresAt)
		row := w.row(t, c.Upload.ID)
		require.Regexp(t, `^uploads/`+w.org.ID.String()+`/[0-9a-f]{32}$`, row.ObjectKey)
		require.Contains(t, c.Put.URL, upload.IncomingKey(row.ObjectKey))

		got, err := w.uploads.Get(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusPending, got.Status)
		require.Empty(t, got.DownloadURL, "a pending upload has no link")

		w.put(t, c, jpeg)
		done, err := w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		require.NotNil(t, done.CompletedAt)
		require.Contains(t, w.queue.Enqueued(), c.Upload.ID)

		got, err = w.uploads.Get(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		require.Empty(t, got.DownloadURL, "still pending until the scan")

		res, err := w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusClean, Changed: true}, res)

		for _, p := range []authz.Principal{w.dimas, w.admin} {
			got, err = w.uploads.Get(ctx, p, c.Upload.ID)
			require.NoError(t, err)
			require.Equal(t, upload.StatusClean, got.Status)
			require.Equal(t, "image/jpeg", got.ContentType)
			require.NotNil(t, got.ScannedAt)
			require.Contains(t, got.DownloadURL, upload.StoredKey(row.ObjectKey))
			require.Contains(t, got.DownloadURL, "filename=bukti-"+c.Upload.ID.String()+".jpg")
			require.Equal(t, env.Clock.Now().Add(upload.GetURLTTL), got.DownloadExpiresAt)
		}
		_, ok := w.storage.Object(upload.StoredKey(row.ObjectKey))
		require.True(t, ok)
		_, ok = w.storage.Object(upload.IncomingKey(row.ObjectKey))
		require.False(t, ok, "the scan took the file out of incoming/")
	})

	t.Run("EICAR is infected, quarantined, flagged and never readable", func(t *testing.T) {
		body := append(uploadtest.Image("image/jpeg", 2048), uploadtest.EICAR...)
		id := w.uploadAndScan(t, body, len(body))
		got, err := w.uploads.Get(ctx, w.admin, id)
		require.NoError(t, err)
		require.Equal(t, upload.StatusInfected, got.Status)
		require.Empty(t, got.DownloadURL)
		key := w.row(t, id).ObjectKey
		_, ok := w.storage.Object(upload.QuarantineKey(key))
		require.True(t, ok)
		_, ok = w.storage.Object(upload.StoredKey(key))
		require.False(t, ok)
		require.True(t, hasAudit(env.AuditLog(t, w.org.ID), "upload:scan", "upload_infected/"+id.String(), "allow"))
	})

	t.Run("a PDF named .jpg is rejected and quarantined", func(t *testing.T) {
		pdf := append([]byte("%PDF-1.7\n"), make([]byte, 3000)...)
		id := w.uploadAndScan(t, pdf, len(pdf))
		got, err := w.uploads.Get(ctx, w.admin, id)
		require.NoError(t, err)
		require.Equal(t, upload.StatusRejected, got.Status)
		require.Equal(t, "application/pdf", got.ContentType)
		require.Empty(t, got.DownloadURL)
		_, ok := w.storage.Object(upload.QuarantineKey(w.row(t, id).ObjectKey))
		require.True(t, ok)
		require.True(t, hasAudit(env.AuditLog(t, w.org.ID), "upload:scan", "upload_rejected/"+id.String(), "allow"))
	})

	t.Run("a file of another size than declared is rejected", func(t *testing.T) {
		id := w.uploadAndScan(t, jpeg[:len(jpeg)-1], len(jpeg))
		got, err := w.uploads.Get(ctx, w.admin, id)
		require.NoError(t, err)
		require.Equal(t, upload.StatusRejected, got.Status)
		require.Empty(t, got.DownloadURL)
	})

	t.Run("a rerun of a finished scan changes nothing", func(t *testing.T) {
		id := w.uploadAndScan(t, jpeg, len(jpeg))
		before := w.row(t, id)
		calls, scans := len(w.storage.Calls()), w.scanner.Scanned()
		env.Clock.Advance(time.Minute)
		res, err := w.checker.Scan(ctx, w.org.ID, id)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusClean}, res)
		require.Equal(t, before, w.row(t, id))
		require.Len(t, w.storage.Calls(), calls, "no storage call")
		require.Equal(t, scans, w.scanner.Scanned(), "no scan")

		_, err = w.uploads.Complete(ctx, w.dimas, id)
		require.NoError(t, err)
		require.Equal(t, before, w.row(t, id), "completing again changes nothing")
	})

	t.Run("a storage outage leaves it pending for a retry", func(t *testing.T) {
		c := w.receipt(t, w.dimas, w.rina, len(jpeg))
		w.put(t, c, jpeg)
		_, err := w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		w.scanner.Err = uploadtest.ErrDown
		_, err = w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		w.scanner.Err = nil
		require.ErrorIs(t, err, uploadtest.ErrDown)
		require.Equal(t, upload.StatusPending, w.row(t, c.Upload.ID).Status)
		res, err := w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, res.Status)
	})

	t.Run("a file that never arrives is waited for, then rejected", func(t *testing.T) {
		c := w.receipt(t, w.dimas, w.rina, len(jpeg))
		_, err := w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		res, err := w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusPending, Waiting: true}, res)
		env.Clock.Advance(upload.MissingAfter)
		res, err = w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusRejected, Changed: true}, res)
		require.Nil(t, w.row(t, c.Upload.ID).ContentType)
	})

	t.Run("an upload never completed is never scanned", func(t *testing.T) {
		c := w.receipt(t, w.dimas, w.rina, len(jpeg))
		w.put(t, c, jpeg)
		res, err := w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusPending, Waiting: true}, res)
	})

	t.Run("the reconcile picks up a lost enqueue", func(t *testing.T) {
		// Earlier cases left uploads pending; this one checks only its own.
		w.queue.Err = errors.New("redis down")
		c := w.receipt(t, w.dimas, w.rina, len(jpeg))
		w.put(t, c, jpeg)
		_, err := w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		w.queue.Err = nil
		require.NoError(t, err, "the upload is completed even when the enqueue fails")
		require.NotContains(t, w.queue.Enqueued(), c.Upload.ID)

		stale, err := w.checker.Stale(ctx, w.org.ID)
		require.NoError(t, err)
		require.NotContains(t, stale, c.Upload.ID, "not stale yet")

		env.Clock.Advance(upload.StaleAfter + time.Second)
		stale, err = w.checker.Stale(ctx, w.org.ID)
		require.NoError(t, err)
		require.Contains(t, stale, c.Upload.ID)

		enq := &recordingEnqueuer{}
		n, err := uploadtask.Reconcile(ctx, orgList{w.org.ID}, w.checker, enq, env.Clock.Now())
		require.NoError(t, err)
		require.Equal(t, len(stale), n)
		require.Contains(t, enq.tasks, uploadtask.ReconcileTaskID(c.Upload.ID, env.Clock.Now()))

		_, err = w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		stale, err = w.checker.Stale(ctx, w.org.ID)
		require.NoError(t, err)
		require.NotContains(t, stale, c.Upload.ID, "scanned uploads are not stale")
	})

	t.Run("two scans of one upload take turns: one decides, the other changes nothing", func(t *testing.T) {
		c := w.receipt(t, w.dimas, w.rina, len(jpeg))
		w.put(t, c, jpeg)
		_, err := w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		gate := &gateScanner{entered: make(chan struct{}, 2), release: make(chan struct{})}
		checker := upload.NewChecker(upload.CheckerDeps{
			Store: env.Store, Objects: w.storage, Scanner: gate, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
		})
		results := make(chan upload.Result, 2)
		errs := make(chan error, 2)
		scan := func() {
			res, err := checker.Scan(ctx, w.org.ID, c.Upload.ID)
			results <- res
			errs <- err
		}
		go scan()
		<-gate.entered // the first run holds the row lock and is scanning
		go scan()
		time.Sleep(200 * time.Millisecond) // the second run waits on the lock meanwhile
		// A second PUT while the first run scans must never be what the second run reads.
		w.put(t, c, append(uploadtest.Image("image/jpeg", len(jpeg)-len(uploadtest.EICAR)), uploadtest.EICAR...))
		close(gate.release)
		var changed int
		for range 2 {
			require.NoError(t, <-errs)
			res := <-results
			require.Equal(t, upload.StatusClean, res.Status)
			if res.Changed {
				changed++
			}
		}
		require.Equal(t, 1, changed)
		require.Len(t, gate.entered, 0, "the second run never scanned")
		stored, ok := w.storage.Object(upload.StoredKey(w.row(t, c.Upload.ID).ObjectKey))
		require.True(t, ok)
		require.Equal(t, jpeg, stored, "the scanned copy is the one served")
	})

	t.Run("a deposit photo", func(t *testing.T) {
		c, err := w.uploads.Create(ctx, w.dimas, upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/png", Size: 900})
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, c.Upload.TenantID)
		w.put(t, c, uploadtest.Image("image/png", 900))
		_, err = w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		_, err = w.checker.Scan(ctx, w.org.ID, c.Upload.ID)
		require.NoError(t, err)
		got, err := w.uploads.Get(ctx, w.dimas, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, got.Status)
		require.Contains(t, got.DownloadURL, ".png")
		got, err = w.uploads.Get(ctx, w.admin, c.Upload.ID)
		require.NoError(t, err)
		require.NotEmpty(t, got.DownloadURL)
		_, err = w.uploads.Get(ctx, w.sari, c.Upload.ID)
		requireForbidden(t, env, w.org.ID, err, "cash:deposit")
	})
}

func TestUploadPermissions(t *testing.T) {
	env := testenv.New(t)
	w := newWorld(t, env, "Griya Izin", "0812882")
	other := newWorld(t, env, "Kos Lain", "0812883")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 1000)
	receipt := func(tenantID uuid.UUID) upload.CreateInput {
		return upload.CreateInput{Purpose: upload.PurposeCashReceipt, TenantID: tenantID, ContentType: "image/jpeg", Size: 1000}
	}
	mine := w.receipt(t, w.dimas, w.rina, len(jpeg))

	t.Run("a karyawan uploads a receipt only for a tenant they collect from", func(t *testing.T) {
		_, err := w.uploads.Create(ctx, w.dimas, receipt(w.budi))
		requireForbidden(t, env, w.org.ID, err, "cash:record")
		_, err = w.uploads.Create(ctx, w.sari, receipt(w.budi))
		require.NoError(t, err)
	})

	t.Run("a tenant with no lease needs an organization-wide grant", func(t *testing.T) {
		_, err := w.uploads.Create(ctx, w.dimas, receipt(w.eko))
		requireForbidden(t, env, w.org.ID, err, "cash:record")
	})

	t.Run("roles without cash:record or cash:deposit cannot upload", func(t *testing.T) {
		for _, p := range []authz.Principal{w.admin, w.supervisor} {
			_, err := w.uploads.Create(ctx, p, receipt(w.rina))
			requireForbidden(t, env, w.org.ID, err, "cash:record")
			_, err = w.uploads.Create(ctx, p, upload.CreateInput{Purpose: upload.PurposeCashDeposit, ContentType: "image/jpeg", Size: 10})
			requireForbidden(t, env, w.org.ID, err, "cash:deposit")
		}
	})

	t.Run("an allowed upload is audited in its transaction", func(t *testing.T) {
		require.True(t, hasAudit(env.AuditLog(t, w.org.ID), "cash:record", auditPath(t, env, w, w.rina), "allow"))
	})

	t.Run("a tenant of another organization is not found", func(t *testing.T) {
		_, err := w.uploads.Create(ctx, w.dimas, receipt(other.rina))
		require.ErrorIs(t, err, upload.ErrTenantNotFound)
	})

	t.Run("only the uploader completes", func(t *testing.T) {
		_, err := w.uploads.Complete(ctx, w.sari, mine.Upload.ID)
		requireForbidden(t, env, w.org.ID, err, "cash:record")
		_, err = w.uploads.Complete(ctx, w.admin, mine.Upload.ID)
		requireForbidden(t, env, w.org.ID, err, "cash:record")
		require.Nil(t, w.row(t, mine.Upload.ID).CompletedAt)
		_, err = w.uploads.Complete(ctx, w.dimas, mine.Upload.ID)
		require.NoError(t, err)
	})

	t.Run("an admin and the Supervisor read any upload, another karyawan none of Dimas's", func(t *testing.T) {
		w.put(t, mine, jpeg)
		_, err := w.checker.Scan(ctx, w.org.ID, mine.Upload.ID)
		require.NoError(t, err)

		got, err := w.uploads.Get(ctx, w.admin, mine.Upload.ID)
		require.NoError(t, err)
		require.NotEmpty(t, got.DownloadURL)

		_, err = w.uploads.Get(ctx, w.sari, mine.Upload.ID)
		requireForbidden(t, env, w.org.ID, err, "cash:record")

		// The Supervisor has no payment:check but reads everything, cash:read included.
		got, err = w.uploads.Get(ctx, w.supervisor, mine.Upload.ID)
		require.NoError(t, err)
		require.NotEmpty(t, got.DownloadURL)
	})

	t.Run("another organization's upload is not found", func(t *testing.T) {
		_, err := other.uploads.Get(ctx, other.admin, mine.Upload.ID)
		require.ErrorIs(t, err, upload.ErrNotFound)
		_, err = other.uploads.Complete(ctx, other.dimas, mine.Upload.ID)
		require.ErrorIs(t, err, upload.ErrNotFound)
		_, err = other.checker.Scan(ctx, other.org.ID, mine.Upload.ID)
		require.ErrorIs(t, err, upload.ErrNotFound)
	})
}

// auditPath is the resource path of a cash:record check on tenantID through its one lease.
func auditPath(t *testing.T, env *testenv.Env, w *world, tenantID uuid.UUID) string {
	t.Helper()
	var path string
	err := env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		leases, err := dbgen.New(tx).ListTenantLeaseScopes(context.Background(), dbgen.ListTenantLeaseScopesParams{OrganizationID: w.org.ID, TenantID: tenantID})
		if err != nil {
			return err
		}
		require.Len(t, leases, 1)
		path = authz.Resource{Type: "tenant", ID: tenantID.String(), PropertyID: leases[0].PropertyID}.Path()
		return nil
	})
	require.NoError(t, err)
	return path
}

func TestUploadSchema(t *testing.T) {
	env := testenv.New(t)
	a := newWorld(t, env, "Griya Skema", "0812884")
	b := newWorld(t, env, "Kos Skema", "0812885")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 1000)

	clean := a.uploadAndScan(t, jpeg, len(jpeg))
	pendingUpload := a.receipt(t, a.dimas, a.rina, len(jpeg))
	pending := pendingUpload.Upload.ID
	bUpload := b.receipt(t, b.dimas, b.rina, len(jpeg))

	asApp := func(org uuid.UUID, sql string, args ...any) error {
		return env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}
	owner, err := pgx.Connect(ctx, env.URLs.Owner)
	require.NoError(t, err)
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	asOwner := func(sql string, args ...any) error {
		_, err := owner.Exec(ctx, sql, args...)
		return err
	}
	// pgError checks err is a Postgres error with code and returns it, for its message.
	pgError := func(t *testing.T, err error, code string) *pgconn.PgError {
		t.Helper()
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
		require.Equal(t, code, pgErr.Code, pgErr.Message)
		return pgErr
	}
	requireCode := func(t *testing.T, err error, code string) {
		t.Helper()
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
		require.Equal(t, code, pgErr.Code, pgErr.Message)
	}

	t.Run("a scanned upload is final, for the app and for the owner", func(t *testing.T) {
		for _, set := range []string{"status = 'infected'", "status = 'pending', scanned_at = NULL", "content_type = 'image/png'", "completed_at = now()"} {
			err := asApp(a.org.ID, "UPDATE upload SET "+set+" WHERE id = $1", clean)
			requireCode(t, err, "42501")
			err = asOwner("UPDATE upload SET "+set+" WHERE id = $1", clean)
			pgErr := pgError(t, err, "42501")
			require.Contains(t, pgErr.Message, "already scanned")
		}
	})

	t.Run("the app role cannot change what an upload is", func(t *testing.T) {
		for _, set := range []string{"object_key = object_key || 'x'", "declared_size = 1", "tenant_id = NULL", "purpose = 'cash_deposit'",
			"uploaded_by_staff_user_id = uploaded_by_staff_user_id", "created_at = now()", "organization_id = organization_id"} {
			err := asApp(a.org.ID, "UPDATE upload SET "+set+" WHERE id = $1", pending)
			pgErr := pgError(t, err, "42501")
			require.Contains(t, pgErr.Message, "permission denied", set)
		}
	})

	t.Run("nor can the owner", func(t *testing.T) {
		for _, set := range []string{"declared_size = declared_size + 1", "object_key = object_key || 'x'", "created_at = created_at + interval '1 second'"} {
			err := asOwner("UPDATE upload SET "+set+" WHERE id = $1", pending)
			pgErr := pgError(t, err, "42501")
			require.Contains(t, pgErr.Message, "cannot change what it is", set)
		}
	})

	t.Run("completed_at is set once", func(t *testing.T) {
		require.NoError(t, asApp(a.org.ID, "UPDATE upload SET completed_at = now() WHERE id = $1", pending))
		err := asApp(a.org.ID, "UPDATE upload SET completed_at = now() + interval '1 hour' WHERE id = $1", pending)
		pgErr := pgError(t, err, "42501")
		require.Contains(t, pgErr.Message, "already completed")
	})

	t.Run("the app role cannot delete or truncate an upload", func(t *testing.T) {
		requireCode(t, asApp(a.org.ID, "DELETE FROM upload WHERE id = $1", pending), "42501")
		requireCode(t, asApp(a.org.ID, "TRUNCATE upload"), "42501")
	})

	t.Run("a clean upload needs an allowed type and a scan time", func(t *testing.T) {
		requireCode(t, asApp(a.org.ID, "UPDATE upload SET status = 'clean', content_type = 'image/gif', scanned_at = now() WHERE id = $1", pending), "23514")
		requireCode(t, asApp(a.org.ID, "UPDATE upload SET status = 'clean', content_type = 'image/jpeg' WHERE id = $1", pending), "23514")
	})

	insert := func(org uuid.UUID, purpose string, tenantID uuid.NullUUID, key string, uploader uuid.UUID) error {
		return env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).InsertUpload(ctx, dbgen.InsertUploadParams{
				ID: uuid.New(), OrganizationID: org, Purpose: purpose, TenantID: tenantID, ObjectKey: key,
				DeclaredSize: 10, UploadedByStaffUserID: uploader, CreatedAt: env.Clock.Now(),
			})
			return err
		})
	}
	key := func(org uuid.UUID) string { return "uploads/" + org.String() + "/" + strings.Repeat("ab", 16) }

	t.Run("the key sits under the organization's own prefix", func(t *testing.T) {
		requireCode(t, insert(a.org.ID, upload.PurposeCashDeposit, uuid.NullUUID{}, key(b.org.ID), a.dimas.UserID), "23514")
		requireCode(t, insert(a.org.ID, upload.PurposeCashDeposit, uuid.NullUUID{}, "uploads/"+a.org.ID.String()+"/../x", a.dimas.UserID), "23514")
		require.NoError(t, insert(a.org.ID, upload.PurposeCashDeposit, uuid.NullUUID{}, key(a.org.ID), a.dimas.UserID))
	})

	t.Run("a receipt has a tenant and a deposit none", func(t *testing.T) {
		k := "uploads/" + a.org.ID.String() + "/" + strings.Repeat("cd", 16)
		requireCode(t, insert(a.org.ID, upload.PurposeCashReceipt, uuid.NullUUID{}, k, a.dimas.UserID), "23514")
		requireCode(t, insert(a.org.ID, upload.PurposeCashDeposit, uuid.NullUUID{UUID: a.rina, Valid: true}, k, a.dimas.UserID), "23514")
		requireCode(t, insert(a.org.ID, "selfie", uuid.NullUUID{}, k, a.dimas.UserID), "23514")
	})

	t.Run("the tenant and the uploader belong to the organization", func(t *testing.T) {
		k := "uploads/" + a.org.ID.String() + "/" + strings.Repeat("ef", 16)
		requireCode(t, insert(a.org.ID, upload.PurposeCashReceipt, uuid.NullUUID{UUID: b.rina, Valid: true}, k, a.dimas.UserID), "23503")
		requireCode(t, insert(a.org.ID, upload.PurposeCashDeposit, uuid.NullUUID{}, k, b.dimas.UserID), "23503")
	})

	t.Run("each organization sees only its own uploads", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
			var all, foreign int
			require.NoError(t, tx.QueryRow(ctx, "SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()) FROM upload").Scan(&all, &foreign))
			require.Positive(t, all)
			require.Zero(t, foreign)
			var n int
			require.NoError(t, tx.QueryRow(ctx, "SELECT count(*) FROM upload WHERE id = $1", bUpload.Upload.ID).Scan(&n))
			require.Zero(t, n)
			_, err := dbgen.New(tx).GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: b.org.ID, ID: bUpload.Upload.ID})
			require.ErrorIs(t, err, pgx.ErrNoRows)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("no upload can be written for another organization", func(t *testing.T) {
		// Organization A's scope writing a row that names organization B.
		err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).InsertUpload(ctx, dbgen.InsertUploadParams{
				ID: uuid.New(), OrganizationID: b.org.ID, Purpose: upload.PurposeCashDeposit,
				ObjectKey: "uploads/" + b.org.ID.String() + "/" + strings.Repeat("12", 16), DeclaredSize: 10,
				UploadedByStaffUserID: b.dimas.UserID, CreatedAt: env.Clock.Now(),
			})
			return err
		})
		pgErr := pgError(t, err, "42501")
		require.Contains(t, pgErr.Message, "row-level security")

		// Nor can A's scope change B's upload: it does not see it.
		err = env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
			n, err := dbgen.New(tx).MarkUploadCompleted(ctx, dbgen.MarkUploadCompletedParams{
				OrganizationID: b.org.ID, ID: bUpload.Upload.ID, CompletedAt: env.Clock.Now(),
			})
			require.Zero(t, n)
			return err
		})
		require.NoError(t, err)
	})
}

// orgList is a fixed uploadtask.OrgLister.
type orgList []uuid.UUID

func (o orgList) ActiveOrganizationIDs(context.Context) ([]uuid.UUID, error) { return o, nil }

// recordingEnqueuer is a fake uploadtask.Enqueuer.
type recordingEnqueuer struct{ tasks []string }

func (e *recordingEnqueuer) EnqueueScanAs(_ context.Context, _, _ uuid.UUID, taskID string) error {
	e.tasks = append(e.tasks, taskID)
	return nil
}

// gateScanner says it entered, then waits for release before calling the content clean.
type gateScanner struct {
	entered chan struct{}
	release chan struct{}
}

func (g *gateScanner) Scan(ctx context.Context, r io.Reader) (upload.Verdict, error) {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return upload.Verdict{}, err
	}
	g.entered <- struct{}{}
	select {
	case <-g.release:
	case <-ctx.Done():
		return upload.Verdict{}, ctx.Err()
	}
	return upload.Verdict{Clean: true}, nil
}
