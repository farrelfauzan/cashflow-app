//go:build integration

package upload_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

const proofLinkOrigin = "https://app.test"

// proofWorld is a world whose tenants can send transfer proofs from their payment links.
type proofWorld struct {
	*world
	links  *paylink.Service
	proofs *upload.Proofs
}

func newProofWorld(t *testing.T, env *testenv.Env, name, prefix string) *proofWorld {
	t.Helper()
	w := &proofWorld{world: newWorld(t, env, name, prefix)}
	w.links = paylink.NewService(env.Store, env.Authz, env.Clock, proofLinkOrigin)
	w.proofs = w.newProofs(0)
	return w
}

// newProofs returns a Proofs on the world's fakes holding at most inFlight proofs at once (0 is
// the default).
func (w *proofWorld) newProofs(inFlight int) *upload.Proofs {
	return upload.NewProofs(upload.ProofsDeps{
		Store: w.env.Store, Links: w.links, Objects: w.storage, Queue: w.queue, Clock: w.env.Clock, Log: w.env.Log,
		InFlight: inFlight,
	})
}

// invoice generates the organization's invoices and returns tenantID's first one.
func (w *proofWorld) invoice(t *testing.T, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	_, err := w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	acct, err := w.env.Billing.Account(ctx, w.admin, tenantID)
	require.NoError(t, err)
	require.NotEmpty(t, acct.Invoices)
	return acct.Invoices[0].ID
}

// mint makes a payment link to invoiceID and returns its id and token.
func (w *proofWorld) mint(t *testing.T, invoiceID uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	link, err := w.links.Mint(context.Background(), w.admin, invoiceID)
	require.NoError(t, err)
	token, ok := strings.CutPrefix(link.URL, proofLinkOrigin+"/t/")
	require.True(t, ok)
	return link.ID, token
}

// send sends body as a transfer proof through token and requires it to succeed.
func (w *proofWorld) send(t *testing.T, token string, body []byte) upload.Proof {
	t.Helper()
	proof, err := w.proofs.Send(context.Background(), token, opener(bytes.NewReader(body)))
	require.NoError(t, err)
	return proof
}

// count runs a count query in the organization's scope.
func (w *proofWorld) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	err := w.env.Store.WithOrgTx(context.Background(), w.org.ID, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), sql, args...).Scan(&n)
	})
	require.NoError(t, err)
	return n
}

// proofCounts is how many transfer proofs, transfer proof uploads and bucket PUTs exist.
type proofCounts struct{ proofs, uploads, puts int }

func (w *proofWorld) counts(t *testing.T) proofCounts {
	t.Helper()
	puts := 0
	for _, c := range w.storage.Calls() {
		if strings.HasPrefix(c, "put ") {
			puts++
		}
	}
	return proofCounts{
		proofs:  w.count(t, "SELECT count(*) FROM transfer_proof"),
		uploads: w.count(t, "SELECT count(*) FROM upload WHERE purpose = 'transfer_proof'"),
		puts:    puts,
	}
}

// waiting returns the ids the payment page behind token lists as waiting proofs.
func (w *proofWorld) waiting(t *testing.T, token string) []uuid.UUID {
	t.Helper()
	page, err := w.links.PublicPage(context.Background(), token)
	require.NoError(t, err)
	require.NotNil(t, page.ProofsWaiting, "never nil")
	out := make([]uuid.UUID, len(page.ProofsWaiting))
	for i, p := range page.ProofsWaiting {
		out[i] = p.ID
	}
	return out
}

// untouchedReader fails the test if anything reads it.
type untouchedReader struct {
	mu   sync.Mutex
	read bool
}

func (r *untouchedReader) Read([]byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.read = true
	return 0, io.EOF
}

func (r *untouchedReader) wasRead() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.read
}

// gateReader says it was entered on its first Read, then waits for release before reading r.
type gateReader struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
	r       io.Reader
}

func (g *gateReader) Read(p []byte) (int, error) {
	g.once.Do(func() { close(g.entered) })
	<-g.release
	return g.r.Read(p)
}

func TestProofSend(t *testing.T) {
	env := testenv.New(t)
	w := newProofWorld(t, env, "Griya Bukti", "0812871")
	ctx := context.Background()
	rinaInvoice := w.invoice(t, w.rina)
	rinaLink, rinaToken := w.mint(t, rinaInvoice)

	// Joko is an admin of another property only.
	other, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kontrakan Kenanga", Kind: property.KindKontrakan})
	require.NoError(t, err)
	jokoPhone := "0812871098"
	_, err = env.Members.Invite(ctx, w.admin, member.Invite{Phone: jokoPhone, Name: "Pak Joko",
		Roles: []member.RoleGrant{{Role: authz.RoleAdmin, PropertyID: other.ID}}})
	require.NoError(t, err)
	joko := env.Principal(t, w.org.ID, env.UserID(t, jokoPhone))

	t.Run("a proof is stored, recorded pending, queued, scanned, and read by those who check it", func(t *testing.T) {
		body := uploadtest.Image("image/png", 40_000)
		proof := w.send(t, rinaToken, body)
		require.Equal(t, w.rina, proof.TenantID)
		require.Equal(t, rinaInvoice, proof.InvoiceID)
		require.NotEqual(t, uuid.Nil, proof.ID)
		require.True(t, proof.CreatedAt.Equal(env.Clock.Now()), "%s", proof.CreatedAt)

		row := w.row(t, proof.UploadID)
		require.Equal(t, upload.PurposeTransferProof, row.Purpose)
		require.Equal(t, uuid.NullUUID{UUID: w.rina, Valid: true}, row.TenantID)
		require.False(t, row.UploadedByStaffUserID.Valid, "no staff uploader")
		require.Equal(t, upload.StatusPending, row.Status)
		require.EqualValues(t, len(body), row.DeclaredSize)
		require.NotNil(t, row.CompletedAt)
		require.True(t, row.CompletedAt.Equal(row.CreatedAt), "completed when recorded")
		require.Nil(t, row.ContentType, "the scan sets the type")

		incoming := upload.IncomingKey(row.ObjectKey)
		require.Equal(t, []string{incoming}, w.storage.Keys(), "the only object is the proof's")
		stored, ok := w.storage.Object(incoming)
		require.True(t, ok)
		require.Equal(t, body, stored)
		require.Equal(t, []string{"put " + incoming + " image/png"}, w.storage.Calls())

		var tp dbgen.TransferProof
		err := env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT id, organization_id, tenant_id, payment_link_id, invoice_id, upload_id, upload_purpose, created_at,
				sent_by_staff_user_id FROM transfer_proof WHERE id = $1`, proof.ID).Scan(&tp.ID, &tp.OrganizationID, &tp.TenantID, &tp.PaymentLinkID,
				&tp.InvoiceID, &tp.UploadID, &tp.UploadPurpose, &tp.CreatedAt, &tp.SentByStaffUserID)
		})
		require.NoError(t, err)
		require.Equal(t, dbgen.TransferProof{
			ID: proof.ID, OrganizationID: w.org.ID, TenantID: w.rina,
			PaymentLinkID: uuid.NullUUID{UUID: rinaLink, Valid: true}, InvoiceID: uuid.NullUUID{UUID: rinaInvoice, Valid: true},
			UploadID: proof.UploadID, UploadPurpose: upload.PurposeTransferProof, CreatedAt: tp.CreatedAt,
		}, tp, "sent from the link, by no member")
		require.True(t, tp.CreatedAt.Equal(proof.CreatedAt))

		require.Equal(t, []uuid.UUID{proof.UploadID}, w.queue.Enqueued())

		got, err := w.uploads.Get(ctx, w.admin, proof.UploadID)
		require.NoError(t, err)
		require.Empty(t, got.DownloadURL, "pending: no link")

		res, err := w.checker.Scan(ctx, w.org.ID, proof.UploadID)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusClean, Changed: true}, res)

		readers := []struct {
			name string
			p    authz.Principal
		}{
			{"the organization's admin", w.admin},
			{"the Supervisor", w.supervisor},
		}
		for _, r := range readers {
			t.Run(r.name+" reads it", func(t *testing.T) {
				got, err := w.uploads.Get(ctx, r.p, proof.UploadID)
				require.NoError(t, err)
				require.Equal(t, upload.StatusClean, got.Status)
				require.Equal(t, "image/png", got.ContentType)
				require.Equal(t, uuid.Nil, got.UploadedBy)
				require.Contains(t, got.DownloadURL, upload.StoredKey(row.ObjectKey))
				require.Contains(t, got.DownloadURL, "filename=bukti-"+proof.UploadID.String()+".png")
			})
		}

		deniedReaders := []struct {
			name string
			p    authz.Principal
		}{
			{"an admin of another property", joko},
			{"the tenant's own collector", w.dimas},
			{"another karyawan", w.sari},
		}
		for _, r := range deniedReaders {
			t.Run(r.name+" may not read it", func(t *testing.T) {
				_, err := w.uploads.Get(ctx, r.p, proof.UploadID)
				requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentCheck)
				require.Equal(t, r.p.UserID, env.AuditLog(t, w.org.ID)[0].ActorStaffUserID.UUID)
			})
		}

		t.Run("nobody completes a tenant's transfer proof", func(t *testing.T) {
			// Completing is checked as sending (payment:proof, decision 49); a link's proof has no
			// staff uploader, so even those who may send one are denied.
			for _, p := range []authz.Principal{w.admin, w.supervisor, w.dimas} {
				_, err := w.uploads.Complete(ctx, p, proof.UploadID)
				requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentProof)
			}
		})
	})

	t.Run("an EICAR file that looks like a JPEG is received and the scan finds it infected", func(t *testing.T) {
		body := append(uploadtest.Image("image/jpeg", 2048), uploadtest.EICAR...)
		proof := w.send(t, rinaToken, body)
		res, err := w.checker.Scan(ctx, w.org.ID, proof.UploadID)
		require.NoError(t, err)
		require.Equal(t, upload.Result{Status: upload.StatusInfected, Changed: true}, res)
		got, err := w.uploads.Get(ctx, w.admin, proof.UploadID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusInfected, got.Status)
		require.Empty(t, got.DownloadURL)
		_, ok := w.storage.Object(upload.QuarantineKey(w.row(t, proof.UploadID).ObjectKey))
		require.True(t, ok)
	})

	t.Run("only members with payment:proof start a transfer proof upload", func(t *testing.T) {
		// Decision 49: the admin and the tenant's collector may (TestStaffProof covers them); the
		// Supervisor may not.
		_, err := w.uploads.Create(ctx, w.supervisor, upload.CreateInput{
			Purpose: upload.PurposeTransferProof, TenantID: w.rina, ContentType: "image/jpeg", Size: 100,
		})
		requireForbidden(t, env, w.org.ID, err, authz.ActionPaymentProof)
	})
}

func TestProofRefusals(t *testing.T) {
	env := testenv.New(t)
	w := newProofWorld(t, env, "Griya Tolak", "0812872")
	ctx := context.Background()
	_, rinaToken := w.mint(t, w.invoice(t, w.rina))

	t.Run("a link that does not work is refused before the file is read", func(t *testing.T) {
		revokedInvoice := w.invoice(t, w.budi)
		_, revoked := w.mint(t, revokedInvoice)
		_, err := w.links.RevokeAll(ctx, w.admin, revokedInvoice)
		require.NoError(t, err)

		tests := []struct {
			name  string
			token string
		}{
			{"malformed", "not-a-token"},
			{"well-formed but unknown", strings.Repeat("A", 43)},
			{"revoked", revoked},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := w.counts(t)
				file := &untouchedReader{}
				_, err := w.proofs.Send(ctx, tc.token, opener(file))
				require.ErrorIs(t, err, paylink.ErrLinkNotFound)
				require.False(t, file.wasRead(), "the file was read")
				require.Equal(t, before, w.counts(t), "nothing stored or recorded")
			})
		}
	})

	t.Run("a file that is not an acceptable photo is refused with nothing stored", func(t *testing.T) {
		pdf := append([]byte("%PDF-1.7\n"), make([]byte, 3000)...)
		tests := []struct {
			name    string
			body    []byte
			wantErr error
			input   bool // an *upload.InputError on the file field
		}{
			{name: "empty", body: nil, input: true},
			{name: "one byte over the limit", body: uploadtest.Image("image/jpeg", int(upload.MaxSize)+1), wantErr: upload.ErrTooLarge},
			{name: "text", body: uploadtest.Image("text/plain", 500), wantErr: upload.ErrTypeNotAllowed},
			{name: "a PDF", body: pdf, wantErr: upload.ErrTypeNotAllowed},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := w.counts(t)
				_, err := w.proofs.Send(ctx, rinaToken, opener(bytes.NewReader(tc.body)))
				if tc.input {
					var input *upload.InputError
					require.ErrorAs(t, err, &input)
					require.Equal(t, "file", input.Field)
					require.NotEmpty(t, input.Message)
				} else {
					require.ErrorIs(t, err, tc.wantErr)
				}
				require.Equal(t, before, w.counts(t), "nothing stored or recorded")
			})
		}
	})

	t.Run("a photo of exactly the limit is received", func(t *testing.T) {
		before := w.counts(t)
		body := uploadtest.Image("image/webp", int(upload.MaxSize))
		proof := w.send(t, rinaToken, body)
		require.EqualValues(t, upload.MaxSize, w.row(t, proof.UploadID).DeclaredSize)
		stored, ok := w.storage.Object(upload.IncomingKey(w.row(t, proof.UploadID).ObjectKey))
		require.True(t, ok)
		require.Equal(t, body, stored)
		require.Equal(t, proofCounts{before.proofs + 1, before.uploads + 1, before.puts + 1}, w.counts(t))
	})

	t.Run("a storage failure records nothing", func(t *testing.T) {
		before := w.counts(t)
		w.storage.Err = uploadtest.ErrDown
		_, err := w.proofs.Send(ctx, rinaToken, opener(bytes.NewReader(uploadtest.Image("image/jpeg", 1000))))
		w.storage.Err = nil
		require.ErrorIs(t, err, uploadtest.ErrDown)
		after := w.counts(t)
		require.Equal(t, before.proofs, after.proofs)
		require.Equal(t, before.uploads, after.uploads)
		require.Equal(t, before.puts+1, after.puts, "one PUT tried")
	})

	t.Run("a link expires 30 days after its invoice is cancelled, and is refused before the file is read", func(t *testing.T) {
		start := env.Clock.Now()
		t.Cleanup(func() { env.Clock.Set(start) })
		invoice := w.invoice(t, w.budi)
		_, token := w.mint(t, invoice)
		zero := int64(0)
		_, err := env.Billing.CreateCorrection(ctx, w.admin, billing.CorrectionInput{
			RequestID: uuid.New(), InvoiceID: invoice, NewAmount: &zero, Reason: "sewa tidak diperpanjang",
		})
		require.NoError(t, err)
		// The correction is stamped by the database's clock, so move well past 30 days from now.
		env.Clock.Set(time.Now().Add(paylink.LinkLifetimeAfterPaid + time.Hour).In(clock.Jakarta))

		before := w.counts(t)
		file := &untouchedReader{}
		_, err = w.proofs.Send(ctx, token, opener(file))
		require.ErrorIs(t, err, paylink.ErrLinkExpired)
		require.False(t, file.wasRead())
		require.Equal(t, before, w.counts(t))
	})
}

func TestProofSlots(t *testing.T) {
	env := testenv.New(t)
	w := newProofWorld(t, env, "Griya Antre", "0812873")
	ctx := context.Background()
	_, token := w.mint(t, w.invoice(t, w.rina))
	proofs := w.newProofs(1)

	body := uploadtest.Image("image/jpeg", 5000)
	gate := &gateReader{entered: make(chan struct{}), release: make(chan struct{}), r: bytes.NewReader(body)}
	type sent struct {
		proof upload.Proof
		err   error
	}
	done := make(chan sent, 1)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(gate.release) }) }
	go func() {
		p, err := proofs.Send(ctx, token, opener(gate))
		done <- sent{p, err}
	}()
	// Whatever happens below, let the first Send finish so no goroutine outlives the test.
	t.Cleanup(func() {
		release()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("the first Send never returned")
		}
	})
	<-gate.entered // the first Send holds the one slot and is reading

	before := w.counts(t)
	short, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	second := &untouchedReader{}
	_, err := proofs.Send(short, token, opener(second))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, second.wasRead(), "it waited for a slot without reading")
	require.Equal(t, before, w.counts(t), "the second stored nothing")

	release()
	var first sent
	select {
	case first = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the first Send never returned")
	}
	done <- first // for the cleanup
	require.NoError(t, first.err)
	stored, ok := w.storage.Object(upload.IncomingKey(w.row(t, first.proof.UploadID).ObjectKey))
	require.True(t, ok)
	require.Equal(t, body, stored)

	// The slot is free again.
	_, err = proofs.Send(ctx, token, opener(bytes.NewReader(body)))
	require.NoError(t, err)
}

func TestProofsWaitingOnThePaymentPage(t *testing.T) {
	env := testenv.New(t)
	w := newProofWorld(t, env, "Griya Tunggu", "0812874")
	ctx := context.Background()
	rinaInvoice := w.invoice(t, w.rina)
	_, first := w.mint(t, rinaInvoice)
	_, second := w.mint(t, rinaInvoice)
	_, budi := w.mint(t, w.invoice(t, w.budi))
	jpeg := uploadtest.Image("image/jpeg", 3000)

	t.Run("none yet is an empty list", func(t *testing.T) {
		require.Empty(t, w.waiting(t, first))
		require.Empty(t, w.waiting(t, budi))
	})

	// Rina sends six proofs a minute apart, alternating between her two links; Budi sends one.
	var rina []upload.Proof // oldest first
	for i := range 6 {
		token := first
		if i%2 == 1 {
			token = second
		}
		rina = append(rina, w.send(t, token, jpeg))
		env.Clock.Advance(time.Minute)
	}
	budiProof := w.send(t, budi, jpeg)
	ids := func(ps ...upload.Proof) []uuid.UUID {
		out := make([]uuid.UUID, len(ps))
		for i, p := range ps {
			out[i] = p.ID
		}
		return out
	}

	t.Run("the newest five from any of the tenant's links, newest first", func(t *testing.T) {
		want := ids(rina[5], rina[4], rina[3], rina[2], rina[1])
		require.Equal(t, want, w.waiting(t, first))
		require.Equal(t, want, w.waiting(t, second), "the same from either link")
		page, err := w.links.PublicPage(ctx, first)
		require.NoError(t, err)
		require.Len(t, page.ProofsWaiting, paylink.MaxProofsWaiting)
		require.True(t, page.ProofsWaiting[0].CreatedAt.Equal(rina[5].CreatedAt))
	})

	t.Run("another tenant's proofs are their own", func(t *testing.T) {
		require.Equal(t, ids(budiProof), w.waiting(t, budi))
		require.NotContains(t, w.waiting(t, first), budiProof.ID)
	})

	t.Run("a clean proof still waits", func(t *testing.T) {
		res, err := w.checker.Scan(ctx, w.org.ID, rina[3].UploadID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, res.Status)
		require.Contains(t, w.waiting(t, first), rina[3].ID)
	})

	t.Run("an infected or rejected proof no longer waits", func(t *testing.T) {
		// The newest turns out infected: the stored copy holds EICAR by the time it is scanned.
		key := w.row(t, rina[5].UploadID).ObjectKey
		w.storage.Put(upload.IncomingKey(key), append(uploadtest.Image("image/jpeg", 3000-len(uploadtest.EICAR)), uploadtest.EICAR...))
		res, err := w.checker.Scan(ctx, w.org.ID, rina[5].UploadID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusInfected, res.Status)
		require.Equal(t, ids(rina[4], rina[3], rina[2], rina[1], rina[0]), w.waiting(t, first), "the oldest is back in the five")

		// The next one is rejected: its stored copy is not the photo that was received.
		key = w.row(t, rina[4].UploadID).ObjectKey
		w.storage.Put(upload.IncomingKey(key), append([]byte("%PDF-1.7\n"), make([]byte, 3000-9)...))
		res, err = w.checker.Scan(ctx, w.org.ID, rina[4].UploadID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusRejected, res.Status)
		require.Equal(t, ids(rina[3], rina[2], rina[1], rina[0]), w.waiting(t, second))
	})
}

func TestProofSchema(t *testing.T) {
	env := testenv.New(t)
	a := newProofWorld(t, env, "Griya Skema Bukti", "0812875")
	b := newProofWorld(t, env, "Kos Skema Bukti", "0812876")
	ctx := context.Background()
	jpeg := uploadtest.Image("image/jpeg", 1000)

	rinaInvoice := a.invoice(t, a.rina)
	rinaLink, rinaToken := a.mint(t, rinaInvoice)
	budiInvoice := a.invoice(t, a.budi)
	budiLink, budiToken := a.mint(t, budiInvoice)
	rinaProof := a.send(t, rinaToken, jpeg)
	budiProof := a.send(t, budiToken, jpeg)
	receipt := a.uploadAndScan(t, jpeg, len(jpeg)) // a cash_receipt of Rina's
	_, bToken := b.mint(t, b.invoice(t, b.rina))
	b.send(t, bToken, jpeg)

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
	keyN := 0
	newKey := func(org uuid.UUID) string {
		keyN++
		return "uploads/" + org.String() + "/" + strings.Repeat("0", 30) + fmt.Sprintf("%02x", keyN)
	}
	// newProofUpload records a transfer proof upload for tenantID with no proof behind it yet.
	newProofUpload := func(t *testing.T, tenantID uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		require.NoError(t, asApp(a.org.ID, `INSERT INTO upload (id, organization_id, purpose, tenant_id, object_key, declared_size, created_at, completed_at)
			VALUES ($1, $2, 'transfer_proof', $3, $4, 10, now(), now())`, id, a.org.ID, tenantID, newKey(a.org.ID)))
		return id
	}
	insertProof := func(org, tenantID, linkID, invoiceID, uploadID uuid.UUID) error {
		return env.Store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
			_, err := dbgen.New(tx).InsertTransferProof(ctx, dbgen.InsertTransferProofParams{
				ID: uuid.New(), OrganizationID: a.org.ID, TenantID: tenantID, PaymentLinkID: linkID, InvoiceID: invoiceID,
				UploadID: uploadID, CreatedAt: env.Clock.Now(),
			})
			return err
		})
	}

	t.Run("another organization sees none of the proofs and cannot write one", func(t *testing.T) {
		err := env.Store.WithOrgTx(ctx, b.org.ID, func(tx pgx.Tx) error {
			var all, foreign, mine int
			require.NoError(t, tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE organization_id <> app_org_id()),
				count(*) FILTER (WHERE id = ANY($1)) FROM transfer_proof`, []uuid.UUID{rinaProof.ID, budiProof.ID}).Scan(&all, &foreign, &mine))
			require.Equal(t, 1, all, "only B's own")
			require.Zero(t, foreign)
			require.Zero(t, mine)
			return nil
		})
		require.NoError(t, err)
		// The sender trigger runs before RLS's check and finds no upload of A in B's scope.
		pgErr := pgError(t, insertProof(b.org.ID, a.rina, rinaLink, rinaInvoice, newProofUpload(t, a.rina)), "23503")
		require.Contains(t, pgErr.Message, "names no upload of its organization")
	})

	t.Run("a proof cannot be changed or removed", func(t *testing.T) {
		for _, sql := range []string{
			"UPDATE transfer_proof SET created_at = now() WHERE id = $1",
			"DELETE FROM transfer_proof WHERE id = $1",
		} {
			requireCode(t, asApp(a.org.ID, sql, rinaProof.ID), "42501")
			pgErr := pgError(t, asOwner(sql, rinaProof.ID), "42501")
			require.Contains(t, pgErr.Message, "append-only")
		}
		requireCode(t, asApp(a.org.ID, "TRUNCATE transfer_proof"), "42501")
		// check_resolution references transfer_proof (S6-6), so even the owner's TRUNCATE is
		// refused for the foreign key before the append-only trigger runs.
		pgErr := pgError(t, asOwner("TRUNCATE transfer_proof"), "0A000")
		require.Contains(t, pgErr.Message, "foreign key")
		require.Equal(t, 2, a.count(t, "SELECT count(*) FROM transfer_proof"))
	})

	t.Run("a proof names its tenant's own proof upload and a link of that tenant and invoice", func(t *testing.T) {
		tests := []struct {
			name     string
			tenant   uuid.UUID
			link     uuid.UUID
			invoice  uuid.UUID
			upload   func(t *testing.T) uuid.UUID
			wantCode string
		}{
			{"all of Rina's", a.rina, rinaLink, rinaInvoice, func(t *testing.T) uuid.UUID { return newProofUpload(t, a.rina) }, ""},
			{"another tenant's upload", a.rina, rinaLink, rinaInvoice, func(t *testing.T) uuid.UUID { return newProofUpload(t, a.budi) }, "23503"},
			// A receipt has a staff uploader, so the sender trigger refuses it before the key does.
			{"an upload that is not a transfer proof", a.rina, rinaLink, rinaInvoice, func(*testing.T) uuid.UUID { return receipt }, "23514"},
			{"an upload already behind a proof", a.rina, rinaLink, rinaInvoice, func(*testing.T) uuid.UUID { return rinaProof.UploadID }, "23505"},
			{"another tenant's link", a.rina, budiLink, budiInvoice, func(t *testing.T) uuid.UUID { return newProofUpload(t, a.rina) }, "23503"},
			{"another invoice than the link's", a.rina, rinaLink, budiInvoice, func(t *testing.T) uuid.UUID { return newProofUpload(t, a.rina) }, "23503"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := insertProof(a.org.ID, tc.tenant, tc.link, tc.invoice, tc.upload(t))
				if tc.wantCode == "" {
					require.NoError(t, err)
					return
				}
				requireCode(t, err, tc.wantCode)
			})
		}
	})

	t.Run("only a transfer proof upload may have no staff uploader, and it has a tenant", func(t *testing.T) {
		insert := func(purpose string, tenantID, uploader *uuid.UUID) error {
			return asApp(a.org.ID, `INSERT INTO upload (id, organization_id, purpose, tenant_id, object_key, declared_size, uploaded_by_staff_user_id, created_at)
				VALUES ($1, $2, $3, $4, $5, 10, $6, now())`, uuid.New(), a.org.ID, purpose, tenantID, newKey(a.org.ID), uploader)
		}
		rina, dimas := a.rina, a.dimas.UserID
		tests := []struct {
			name     string
			purpose  string
			tenant   *uuid.UUID
			uploader *uuid.UUID
			wantCode string
		}{
			{"a transfer proof with a tenant and no uploader", upload.PurposeTransferProof, &rina, nil, ""},
			{"a transfer proof with a staff uploader (decision 49)", upload.PurposeTransferProof, &rina, &dimas, ""},
			{"a transfer proof without a tenant", upload.PurposeTransferProof, nil, nil, "23514"},
			{"a receipt without an uploader", upload.PurposeCashReceipt, &rina, nil, "23514"},
			{"a deposit without an uploader", upload.PurposeCashDeposit, nil, nil, "23514"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				err := insert(tc.purpose, tc.tenant, tc.uploader)
				if tc.wantCode == "" {
					require.NoError(t, err)
					return
				}
				requireCode(t, err, tc.wantCode)
			})
		}
	})

	t.Run("the uploader of a pending upload never changes", func(t *testing.T) {
		pendingReceipt := a.receipt(t, a.dimas, a.rina, len(jpeg)).Upload.ID
		pendingProof := a.send(t, rinaToken, jpeg).UploadID
		tests := []struct {
			name string
			id   uuid.UUID
			set  string
		}{
			{"a receipt's uploader to nobody", pendingReceipt, "uploaded_by_staff_user_id = NULL"},
			{"a receipt's uploader to someone else", pendingReceipt, "uploaded_by_staff_user_id = '" + a.admin.UserID.String() + "'"},
			{"a proof's missing uploader to someone", pendingProof, "uploaded_by_staff_user_id = '" + a.dimas.UserID.String() + "'"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				require.Equal(t, upload.StatusPending, a.row(t, tc.id).Status)
				pgErr := pgError(t, asOwner("UPDATE upload SET "+tc.set+" WHERE id = $1", tc.id), "42501")
				require.Contains(t, pgErr.Message, "cannot change what it is")
				pgErr = pgError(t, asApp(a.org.ID, "UPDATE upload SET "+tc.set+" WHERE id = $1", tc.id), "42501")
				require.Contains(t, pgErr.Message, "permission denied")
			})
		}
	})
}

// opener hands Send a file that is already open.
func opener(r io.Reader) func() (io.Reader, error) {
	return func() (io.Reader, error) { return r, nil }
}
