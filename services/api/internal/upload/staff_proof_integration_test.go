//go:build integration

package upload_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// Staff transfer proofs (decision 49): a member who got the photo from the tenant sends it with
// payment:proof through upload.Service, and Complete records the proof as sent by them.

// staffWorld is a proofWorld with a property of its own for extra rentals.
type staffWorld struct {
	*proofWorld
	kos    property.Property
	prefix string
	n      int
}

func newStaffWorld(t *testing.T, env *testenv.Env, name, prefix string) *staffWorld {
	t.Helper()
	w := &staffWorld{proofWorld: newProofWorld(t, env, name, prefix), prefix: prefix}
	var err error
	w.kos, err = env.Properties.Create(context.Background(), w.admin, property.PropertyInput{Name: "Kos Kenari " + name, Kind: property.KindKos})
	require.NoError(t, err)
	return w
}

// tenant creates a tenant with no lease.
func (w *staffWorld) tenant(t *testing.T, name string) uuid.UUID {
	t.Helper()
	w.n++
	tn, err := w.env.Tenants.Create(context.Background(), w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 7000+w.n)})
	require.NoError(t, err)
	return tn.ID
}

// lease lets a new room to tenantID from today, collected by the membership collector, and
// returns the lease id.
func (w *staffWorld) lease(t *testing.T, tenantID, collector uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	w.n++
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: fmt.Sprintf("Kamar K%d", w.n), Terms: testenv.Monthly(750_000)})
	require.NoError(t, err)
	l, err := w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{TenantID: tenantID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(w.env.Clock), CollectorMemberID: collector}})
	require.NoError(t, err)
	return l.ID
}

// moveOut ends the lease today.
func (w *staffWorld) moveOut(t *testing.T, leaseID uuid.UUID) {
	t.Helper()
	_, err := w.env.Tenants.MoveOut(context.Background(), w.admin, leaseID, tenant.Today(w.env.Clock))
	require.NoError(t, err)
}

// startProof starts a transfer proof upload of body's size for tenantID as p.
func (w *staffWorld) startProof(p authz.Principal, tenantID uuid.UUID, size int) (upload.Created, error) {
	return w.uploads.Create(context.Background(), p, upload.CreateInput{
		Purpose: upload.PurposeTransferProof, TenantID: tenantID, ContentType: "image/jpeg", Size: int64(size),
	})
}

// staffProof has p start, PUT and complete body as a transfer proof for tenantID, and returns
// the upload id.
func (w *staffWorld) staffProof(t *testing.T, p authz.Principal, tenantID uuid.UUID, body []byte) uuid.UUID {
	t.Helper()
	c, err := w.startProof(p, tenantID, len(body))
	require.NoError(t, err)
	w.put(t, c, body)
	_, err = w.uploads.Complete(context.Background(), p, c.Upload.ID)
	require.NoError(t, err)
	return c.Upload.ID
}

// proofsOf reads the transfer proofs behind an upload.
func (w *staffWorld) proofsOf(t *testing.T, uploadID uuid.UUID) []dbgen.TransferProof {
	t.Helper()
	var out []dbgen.TransferProof
	err := w.env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT id, organization_id, tenant_id, payment_link_id, invoice_id, upload_id,
			upload_purpose, created_at, sent_by_staff_user_id FROM transfer_proof WHERE upload_id = $1`, uploadID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tp dbgen.TransferProof
			if err := rows.Scan(&tp.ID, &tp.OrganizationID, &tp.TenantID, &tp.PaymentLinkID, &tp.InvoiceID, &tp.UploadID,
				&tp.UploadPurpose, &tp.CreatedAt, &tp.SentByStaffUserID); err != nil {
				return err
			}
			out = append(out, tp)
		}
		return rows.Err()
	})
	require.NoError(t, err)
	return out
}

// auditCount counts the organization's audit rows of action and decision by actor.
func (w *staffWorld) auditCount(t *testing.T, actor uuid.UUID, action, decision string) int {
	t.Helper()
	return w.count(t, `SELECT count(*) FROM audit_log WHERE actor_staff_user_id = $1 AND action = $2 AND decision = $3`, actor, action, decision)
}

func TestStaffProofSend(t *testing.T) {
	env := testenv.New(t)
	w := newStaffWorld(t, env, "Griya Kirim", "0812561")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 4_000)

	// Hendra moved out today: he has no active lease left.
	hendra := w.tenant(t, "Hendra Gunawan")
	w.moveOut(t, w.lease(t, hendra, w.dimas.MembershipID))

	tests := []struct {
		name   string
		p      func() authz.Principal
		tenant func() uuid.UUID
	}{
		{"a karyawan for a tenant they collect from", func() authz.Principal { return w.dimas }, func() uuid.UUID { return w.rina }},
		{"an admin for a tenant a karyawan collects from", func() authz.Principal { return w.admin }, func() uuid.UUID { return w.budi }},
		{"an admin for a tenant who moved out", func() authz.Principal { return w.admin }, func() uuid.UUID { return hendra }},
		{"an admin for a tenant who never had a lease", func() authz.Principal { return w.admin }, func() uuid.UUID { return w.eko }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, tenantID := tc.p(), tc.tenant()
			allowed := w.auditCount(t, p.UserID, authz.ActionPaymentProof, "allow")

			c, err := w.startProof(p, tenantID, len(jpeg))
			require.NoError(t, err)
			require.Equal(t, upload.PurposeTransferProof, c.Upload.Purpose)
			require.Equal(t, tenantID, c.Upload.TenantID)
			require.Equal(t, p.UserID, c.Upload.UploadedBy)
			require.Equal(t, upload.StatusPending, c.Upload.Status)
			require.Equal(t, allowed+1, w.auditCount(t, p.UserID, authz.ActionPaymentProof, "allow"), "audited on create")
			require.Empty(t, w.proofsOf(t, c.Upload.ID), "no proof before the upload is completed")

			w.put(t, c, jpeg)
			done, err := w.uploads.Complete(ctx, p, c.Upload.ID)
			require.NoError(t, err)
			require.NotNil(t, done.CompletedAt)
			require.Contains(t, w.queue.Enqueued(), c.Upload.ID)
			require.Equal(t, allowed+2, w.auditCount(t, p.UserID, authz.ActionPaymentProof, "allow"), "audited on complete")

			row := w.row(t, c.Upload.ID)
			require.Equal(t, uuid.NullUUID{UUID: p.UserID, Valid: true}, row.UploadedByStaffUserID)
			require.Equal(t, uuid.NullUUID{UUID: tenantID, Valid: true}, row.TenantID)

			proofs := w.proofsOf(t, c.Upload.ID)
			require.Len(t, proofs, 1)
			got := proofs[0]
			require.Equal(t, dbgen.TransferProof{
				ID: got.ID, OrganizationID: w.org.ID, TenantID: tenantID, UploadID: c.Upload.ID,
				UploadPurpose: upload.PurposeTransferProof, CreatedAt: got.CreatedAt,
				SentByStaffUserID: uuid.NullUUID{UUID: p.UserID, Valid: true},
			}, got, "sent by the member, with no link or invoice")
			require.True(t, got.CreatedAt.Equal(env.Clock.Now()))

			_, err = w.uploads.Complete(ctx, p, c.Upload.ID)
			require.NoError(t, err)
			require.Equal(t, proofs, w.proofsOf(t, c.Upload.ID), "completing again records no second proof")
		})
	}
}

func TestStaffProofRefusals(t *testing.T) {
	env := testenv.New(t)
	w := newStaffWorld(t, env, "Griya Tolak Staf", "0812562")
	other := newWorld(t, env, "Kos Lain Staf", "0812563")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 2_000)

	hendra := w.tenant(t, "Hendra Gunawan")
	w.moveOut(t, w.lease(t, hendra, w.dimas.MembershipID))
	// Indah rents two rooms: Dimas collects one, Sari the other.
	indah := w.tenant(t, "Indah Permata")
	w.lease(t, indah, w.dimas.MembershipID)
	w.lease(t, indah, w.sari.MembershipID)
	// Joko is an admin of another property only.
	kenanga, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Kenanga", Kind: property.KindKontrakan})
	require.NoError(t, err)
	jokoPhone := "0812562098"
	_, err = env.Members.Invite(ctx, w.admin, member.Invite{Phone: jokoPhone, Name: "Pak Joko",
		Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: kenanga.ID}}})
	require.NoError(t, err)
	joko := env.Principal(t, w.org.ID, env.UserID(t, jokoPhone))

	t.Run("a member without payment:proof on the tenant starts nothing", func(t *testing.T) {
		tests := []struct {
			name   string
			p      authz.Principal
			tenant uuid.UUID
		}{
			{"a karyawan for a tenant another karyawan collects from", w.dimas, w.budi},
			{"a karyawan for a tenant who moved out", w.dimas, hendra},
			{"a karyawan for a tenant who never had a lease", w.dimas, w.eko},
			{"a karyawan who collects only one of the tenant's active leases", w.dimas, indah},
			{"the Supervisor", w.supervisor, w.rina},
			{"an admin of another property", joko, w.rina},
			{"an admin of another property for a tenant with no lease", joko, w.eko},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := w.counts(t)
				_, err := w.startProof(tc.p, tc.tenant, len(jpeg))
				requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentProof)
				audit := env.AuditLog(t, w.org.ID)[0]
				require.Equal(t, tc.p.UserID, audit.ActorStaffUserID.UUID)
				require.Contains(t, audit.Resource, "tenant/"+tc.tenant.String())
				require.Equal(t, before, w.counts(t), "no upload or proof recorded")
			})
		}
	})

	t.Run("missing tenantId is an input error on tenantId", func(t *testing.T) {
		before := w.counts(t)
		_, err := w.startProof(w.dimas, uuid.Nil, len(jpeg))
		var input *upload.InputError
		require.ErrorAs(t, err, &input)
		require.Equal(t, "tenantId", input.Field)
		require.NotEmpty(t, input.Message)
		require.Equal(t, before, w.counts(t))
	})

	t.Run("a tenant of another organization is not found", func(t *testing.T) {
		_, err := w.startProof(w.admin, other.rina, len(jpeg))
		require.ErrorIs(t, err, upload.ErrTenantNotFound)
	})

	t.Run("only the member who started it completes it", func(t *testing.T) {
		c, err := w.startProof(w.dimas, w.rina, len(jpeg))
		require.NoError(t, err)
		w.put(t, c, jpeg)
		for _, p := range []authz.Principal{w.sari, w.admin, w.supervisor} {
			_, err := w.uploads.Complete(ctx, p, c.Upload.ID)
			requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentProof)
			require.Equal(t, p.UserID, env.AuditLog(t, w.org.ID)[0].ActorStaffUserID.UUID)
		}
		require.Nil(t, w.row(t, c.Upload.ID).CompletedAt)
		require.Empty(t, w.proofsOf(t, c.Upload.ID))
		require.NotContains(t, w.queue.Enqueued(), c.Upload.ID)

		_, err = other.uploads.Complete(ctx, other.dimas, c.Upload.ID)
		require.ErrorIs(t, err, upload.ErrNotFound, "another organization's member does not find it")
	})

	t.Run("a karyawan who lost the tenant before completing is denied", func(t *testing.T) {
		fajar := w.tenant(t, "Fajar Nugroho")
		lease := w.lease(t, fajar, w.dimas.MembershipID)
		c, err := w.startProof(w.dimas, fajar, len(jpeg))
		require.NoError(t, err)
		w.put(t, c, jpeg)
		w.moveOut(t, lease)
		_, err = w.uploads.Complete(ctx, w.dimas, c.Upload.ID)
		requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentProof)
		require.Nil(t, w.row(t, c.Upload.ID).CompletedAt)
		require.Empty(t, w.proofsOf(t, c.Upload.ID))
	})
}

func TestStaffProofRead(t *testing.T) {
	env := testenv.New(t)
	w := newStaffWorld(t, env, "Griya Baca Staf", "0812564")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 2_500)

	id := w.staffProof(t, w.dimas, w.rina, jpeg)

	t.Run("while pending, the sender, the admin and the Supervisor read it; another karyawan does not", func(t *testing.T) {
		for _, p := range []authz.Principal{w.dimas, w.admin, w.supervisor} {
			got, err := w.uploads.Get(ctx, p, id)
			require.NoError(t, err)
			require.Equal(t, upload.StatusPending, got.Status)
			require.Equal(t, w.dimas.UserID, got.UploadedBy)
			require.Empty(t, got.DownloadURL)
		}
		_, err := w.uploads.Get(ctx, w.sari, id)
		requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentCheck)
		require.Equal(t, w.sari.UserID, env.AuditLog(t, w.org.ID)[0].ActorStaffUserID.UUID)
	})

	t.Run("once clean, the readers get a download link", func(t *testing.T) {
		res, err := w.checker.Scan(ctx, w.org.ID, id)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, res.Status)
		for _, p := range []authz.Principal{w.dimas, w.admin, w.supervisor} {
			got, err := w.uploads.Get(ctx, p, id)
			require.NoError(t, err)
			require.Contains(t, got.DownloadURL, upload.StoredKey(w.row(t, id).ObjectKey))
		}
		_, err = w.uploads.Get(ctx, w.sari, id)
		requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentCheck)
	})

	t.Run("a sender who lost the tenant no longer reads it", func(t *testing.T) {
		gita := w.tenant(t, "Gita Savitri")
		lease := w.lease(t, gita, w.dimas.MembershipID)
		gitaProof := w.staffProof(t, w.dimas, gita, jpeg)
		_, err := w.uploads.Get(ctx, w.dimas, gitaProof)
		require.NoError(t, err)
		w.moveOut(t, lease)
		_, err = w.uploads.Get(ctx, w.dimas, gitaProof)
		require.ErrorIs(t, err, authz.ErrForbidden)
		_, err = w.uploads.Get(ctx, w.admin, gitaProof)
		require.NoError(t, err, "the admin still reads it")
	})
}

func TestStaffProofSchema(t *testing.T) {
	env := testenv.New(t)
	a := newStaffWorld(t, env, "Griya Skema Staf", "0812565")
	b := newStaffWorld(t, env, "Kos Skema Staf", "0812566")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 1_000)

	rinaInvoice := a.invoice(t, a.rina)
	rinaLink, _ := a.mint(t, rinaInvoice)
	staffUpload := a.staffProof(t, a.dimas, a.rina, jpeg)
	staffProofID := a.proofsOf(t, staffUpload)[0].ID
	bUpload := b.staffProof(t, b.dimas, b.rina, jpeg)
	bProofID := b.proofsOf(t, bUpload)[0].ID
	receipt := a.uploadAndScan(t, jpeg, len(jpeg))

	asApp := func(org uuid.UUID, sql string, args ...any) error {
		return env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, sql, args...)
			return err
		})
	}
	requireCode := func(t *testing.T, err error, code string) *pgconn.PgError {
		t.Helper()
		var pgErr *pgconn.PgError
		require.True(t, errors.As(err, &pgErr), "expected a Postgres error %s, got %v", code, err)
		require.Equal(t, code, pgErr.Code, pgErr.Message)
		return pgErr
	}
	keyN := 0
	// newUpload records a completed transfer proof upload for rina, by uploader (nil for none).
	newUpload := func(t *testing.T, uploader *uuid.UUID) uuid.UUID {
		t.Helper()
		keyN++
		id := uuid.New()
		key := "uploads/" + a.org.ID.String() + "/" + strings.Repeat("0", 28) + fmt.Sprintf("%04x", keyN)
		require.NoError(t, asApp(a.org.ID, `INSERT INTO upload (id, organization_id, purpose, tenant_id, object_key, declared_size,
			uploaded_by_staff_user_id, created_at, completed_at) VALUES ($1, $2, 'transfer_proof', $3, $4, 10, $5, now(), now())`,
			id, a.org.ID, a.rina, key, uploader))
		return id
	}
	// insert writes a transfer proof for rina in org's scope, naming organization a.
	insert := func(org uuid.UUID, link, invoice, sender *uuid.UUID, uploadID uuid.UUID) error {
		return asApp(org, `INSERT INTO transfer_proof (id, organization_id, tenant_id, payment_link_id, invoice_id, upload_id,
			sent_by_staff_user_id, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
			uuid.New(), a.org.ID, a.rina, link, invoice, uploadID, sender)
	}
	dimas, sari := a.dimas.UserID, a.sari.UserID

	t.Run("a proof names its link and invoice, or its sender, exactly as its upload was sent", func(t *testing.T) {
		tests := []struct {
			name     string
			link     *uuid.UUID
			invoice  *uuid.UUID
			sender   *uuid.UUID
			uploader *uuid.UUID
			wantCode string
			wantMsg  string
		}{
			{name: "a staff proof by its uploader", sender: &dimas, uploader: &dimas},
			{name: "a link proof of an upload with no uploader", link: &rinaLink, invoice: &rinaInvoice},
			{name: "both a link and a sender", link: &rinaLink, invoice: &rinaInvoice, sender: &dimas, uploader: &dimas, wantCode: "23514", wantMsg: "transfer_proof_one_sender"},
			{name: "neither a link nor a sender", wantCode: "23514", wantMsg: "transfer_proof_one_sender"},
			{name: "a link without its invoice", link: &rinaLink, wantCode: "23514", wantMsg: "transfer_proof_link_invoice"},
			{name: "an invoice without a link", invoice: &rinaInvoice, sender: &dimas, uploader: &dimas, wantCode: "23514", wantMsg: "transfer_proof_link_invoice"},
			{name: "a sender who is not the uploader", sender: &sari, uploader: &dimas, wantCode: "23514", wantMsg: "not sent by its upload's uploader"},
			{name: "a sender for an upload with no uploader", sender: &dimas, wantCode: "23514", wantMsg: "not sent by its upload's uploader"},
			{name: "a link proof whose upload has a staff uploader", link: &rinaLink, invoice: &rinaInvoice, uploader: &dimas, wantCode: "23514", wantMsg: "not sent by its upload's uploader"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := insert(a.org.ID, tc.link, tc.invoice, tc.sender, newUpload(t, tc.uploader))
				if tc.wantCode == "" {
					require.NoError(t, err)
					return
				}
				pgErr := requireCode(t, err, tc.wantCode)
				require.Contains(t, pgErr.Message+" "+pgErr.ConstraintName, tc.wantMsg)
			})
		}
	})

	t.Run("a staff proof of an upload that is not a transfer proof is refused by the key", func(t *testing.T) {
		requireCode(t, insert(a.org.ID, nil, nil, &dimas, receipt), "23503")
	})

	t.Run("a second proof of the same upload is refused, and the staff insert skips it", func(t *testing.T) {
		requireCode(t, insert(a.org.ID, nil, nil, &dimas, staffUpload), "23505")
		err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
			n, err := dbgen.New(tx).InsertStaffTransferProof(ctx, dbgen.InsertStaffTransferProofParams{
				ID: uuid.New(), OrganizationID: a.org.ID, TenantID: a.rina, UploadID: staffUpload, SentByStaffUserID: dimas, CreatedAt: env.Clock.Now(),
			})
			require.Zero(t, n)
			return err
		})
		require.NoError(t, err)
		require.Len(t, a.proofsOf(t, staffUpload), 1)
	})

	t.Run("another organization's staff proof is invisible and none can be written for it", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, a.org.ID, func(tx pgx.Tx) error {
			var foreign, theirs int
			require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE organization_id <> app_org_id()), count(*) FILTER (WHERE id = $1)
				FROM transfer_proof`, bProofID).Scan(&foreign, &theirs))
			require.Zero(t, foreign)
			require.Zero(t, theirs)
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, b.count(t, "SELECT count(*) FROM transfer_proof WHERE id = $1", bProofID), "B sees its own")
		require.Zero(t, b.count(t, "SELECT count(*) FROM transfer_proof WHERE id = $1", staffProofID))

		// B's scope writing a staff proof that names organization A and A's upload: the sender
		// trigger sees no such upload in B's scope.
		pgErr := requireCode(t, insert(b.org.ID, nil, nil, &dimas, newUpload(t, &dimas)), "23503")
		require.Contains(t, pgErr.Message, "names no upload of its organization")
		// Nor through B's own upload, named under organization A.
		requireCode(t, insert(b.org.ID, nil, nil, &b.dimas.UserID, bUpload), "23503")
		require.Len(t, a.proofsOf(t, staffUpload), 1)
	})
}
