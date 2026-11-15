package checkqueue

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// PaymentRecorder records a payment with something else in its transaction. billing.Service
// implements it.
type PaymentRecorder interface {
	RecordPaymentWith(ctx context.Context, p authz.Principal, in billing.PaymentInput,
		inTx func(ctx context.Context, tx pgx.Tx, paymentID uuid.UUID, created bool) error) (billing.PaymentResult, error)
}

// PhoneChanger changes a tenant's number inside a transaction. tenant.Service implements it.
type PhoneChanger interface {
	ChangePhoneTx(ctx context.Context, tx pgx.Tx, p authz.Principal, id uuid.UUID, phone string) error
}

// Service reads and answers the check queue.
type Service struct {
	store    *store.Store
	authz    *authz.Service
	payments PaymentRecorder
	tenants  PhoneChanger
	clock    clock.Clock
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service, payments PaymentRecorder, tenants PhoneChanger, c clock.Clock) *Service {
	return &Service{store: st, authz: az, payments: payments, tenants: tenants, clock: c}
}

// queueResource is the queue as a whole, which reading needs for the whole organization.
var queueResource = authz.Resource{Type: "check_queue"}

// itemResource is one item, which answering needs payment:check on.
func itemResource(kind string, id uuid.UUID) authz.Resource {
	return authz.Resource{Type: kind, ID: id.String()}
}

// List returns one page of the waiting items of kind (empty for every kind), oldest first, with
// each kind's count. It needs payment:check or cash:read for the whole organization; it is a read,
// so only a denial is audited. Everything is read in one snapshot.
func (s *Service) List(ctx context.Context, p authz.Principal, kind string, page listing.Page) (Page, error) {
	if kind != "" && !ValidKind(kind) {
		return Page{}, &InputError{Field: "kind", Message: "Jenis antrean tidak dikenal."}
	}
	check := authz.Request{Action: authz.ActionPaymentCheck, Resource: queueResource}
	if !authz.Allowed(p, check) && !authz.Allowed(p, authz.Request{Action: authz.ActionCashRead, Resource: queueResource}) {
		return Page{}, s.authz.Deny(ctx, p, check)
	}
	org := p.OrganizationID
	out := Page{Items: []Item{}, Counts: map[string]int{}}
	for _, k := range Kinds {
		out.Counts[k] = 0
	}
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		counts, err := q.CountCheckQueue(ctx, org)
		if err != nil {
			return fmt.Errorf("checkqueue: counting the queue: %w", err)
		}
		for _, c := range counts {
			out.Counts[c.Kind] = int(c.Waiting)
			out.Waiting += int(c.Waiting)
		}
		out.Total = out.Waiting
		var kindArg *string
		if kind != "" {
			kindArg = &kind
			out.Total = out.Counts[kind]
		}
		rows, err := q.ListCheckQueue(ctx, dbgen.ListCheckQueueParams{
			OrganizationID: org, Kind: kindArg, PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("checkqueue: listing the queue: %w", err)
		}
		out.Items, err = loadItems(ctx, q, org, rows)
		return err
	})
	if err != nil {
		return Page{}, err
	}
	return out, nil
}

// Match turns a waiting transfer proof into a transfer payment of in.Amount received on
// in.ReceivedOn, for the proof's tenant or in.TenantID, and takes the proof off the queue in the
// payment's transaction. The payment is recorded as recordPayment records one (payment:record,
// audited; FIFO; the rest is credit; the tenant's promise hook); matching also needs payment:check
// on the proof (audited). No receipt is sent (decision 46.5: cash only).
//
// A proof whose photo is still being scanned is ErrUploadNotReady, one the scan refused
// ErrUploadRefused; one already answered is
// ErrResolved; an unknown one ErrNotFound. The same requestId again answers the same payment and
// resolution and records nothing; a requestId that recorded another payment is ErrRequestIDReused
// (or billing.ErrRequestIDReused when its input differs).
func (s *Service) Match(ctx context.Context, p authz.Principal, proofID uuid.UUID, in MatchInput) (Resolution, billing.PaymentResult, error) {
	note, err := validNote(in.Note, false)
	if err != nil {
		return Resolution{}, billing.PaymentResult{}, err
	}
	org := p.OrganizationID
	proof, err := s.loadProof(ctx, org, proofID)
	if err != nil {
		return Resolution{}, billing.PaymentResult{}, err
	}
	tenantID := in.TenantID
	if tenantID == uuid.Nil {
		tenantID = proof.TenantID
	}
	var res Resolution
	pay, err := s.payments.RecordPaymentWith(ctx, p, billing.PaymentInput{
		RequestID: in.RequestID, TenantID: tenantID, Amount: in.Amount, Method: billing.MethodTransfer,
		ReceivedOn: in.ReceivedOn, Note: note,
	}, func(ctx context.Context, tx pgx.Tx, paymentID uuid.UUID, created bool) error {
		q := dbgen.New(tx)
		if !created {
			row, err := q.GetProofResolutionByPayment(ctx, dbgen.GetProofResolutionByPaymentParams{OrganizationID: org, PaymentID: uuid.NullUUID{UUID: paymentID, Valid: true}})
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.TransferProofID.UUID != proofID) {
				return ErrRequestIDReused
			}
			if err != nil {
				return fmt.Errorf("checkqueue: loading the match of a repeated request: %w", err)
			}
			res = fromRow(row)
			return nil
		}
		if err := s.authz.Authorize(ctx, tx, p, authz.Request{Action: authz.ActionPaymentCheck, Resource: itemResource(KindTransferProof, proofID)}); err != nil {
			return err
		}
		if err := proofReady(ctx, q, org, proof.UploadID); err != nil {
			return err
		}
		if err := waiting(ctx, q, org, KindTransferProof, proofID); err != nil {
			return err
		}
		res, err = s.insert(ctx, q, org, p.UserID, KindTransferProof, proofID, OutcomeMatched, paymentID, note)
		return err
	})
	if err != nil {
		return Resolution{}, billing.PaymentResult{}, err
	}
	return res, pay, nil
}

// Apply makes the new number of a waiting phone_changed item the tenant's, and takes the item off
// the queue, in one transaction. It needs payment:check on the item (audited) and tenant:manage on
// the tenant, as editing the tenant does. A number another tenant has is tenant.ErrPhoneTaken; an
// answered item ErrResolved; an unknown one ErrNotFound.
func (s *Service) Apply(ctx context.Context, p authz.Principal, outcomeID uuid.UUID) (Resolution, error) {
	org := p.OrganizationID
	var res Resolution
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		rows, err := q.ListQueuePhoneChanges(ctx, dbgen.ListQueuePhoneChangesParams{OrganizationID: org, Ids: []uuid.UUID{outcomeID}})
		if err != nil {
			return fmt.Errorf("checkqueue: loading the new number: %w", err)
		}
		if len(rows) == 0 || rows[0].NewPhone == nil {
			return ErrNotFound
		}
		if err := s.authz.Authorize(ctx, tx, p, authz.Request{Action: authz.ActionPaymentCheck, Resource: itemResource(KindPhoneChanged, outcomeID)}); err != nil {
			return err
		}
		if err := waiting(ctx, q, org, KindPhoneChanged, outcomeID); err != nil {
			return err
		}
		if err := s.tenants.ChangePhoneTx(ctx, tx, p, rows[0].TenantID, *rows[0].NewPhone); err != nil {
			return err
		}
		res, err = s.insert(ctx, q, org, p.UserID, KindPhoneChanged, outcomeID, OutcomeApplied, uuid.Nil, "")
		return err
	})
	if err != nil {
		return Resolution{}, err
	}
	return res, nil
}

// Dismiss takes a waiting item of any kind off the queue with a note saying why: a proof that is
// not rent, a deposit gap closed, an infected file or an amount mismatch handled (the admin fixes
// the money with a correction first), a new number not used. It needs payment:check on the item
// (audited). An answered item is ErrResolved; an unknown one ErrNotFound.
func (s *Service) Dismiss(ctx context.Context, p authz.Principal, kind string, id uuid.UUID, note string) (Resolution, error) {
	if !ValidKind(kind) {
		return Resolution{}, ErrNotFound
	}
	note, err := validNote(note, true)
	if err != nil {
		return Resolution{}, err
	}
	org := p.OrganizationID
	var res Resolution
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := exists(ctx, q, org, kind, id); err != nil {
			return err
		}
		if err := s.authz.Authorize(ctx, tx, p, authz.Request{Action: authz.ActionPaymentCheck, Resource: itemResource(kind, id)}); err != nil {
			return err
		}
		if err := waiting(ctx, q, org, kind, id); err != nil {
			return err
		}
		res, err = s.insert(ctx, q, org, p.UserID, kind, id, OutcomeDismissed, uuid.Nil, note)
		return err
	})
	if err != nil {
		return Resolution{}, err
	}
	return res, nil
}

// loadProof reads a transfer proof, before the payment's transaction opens.
func (s *Service) loadProof(ctx context.Context, org, id uuid.UUID) (dbgen.ListQueueTransferProofsRow, error) {
	var out dbgen.ListQueueTransferProofsRow
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListQueueTransferProofs(ctx, dbgen.ListQueueTransferProofsParams{OrganizationID: org, Ids: []uuid.UUID{id}})
		if err != nil {
			return fmt.Errorf("checkqueue: loading the proof: %w", err)
		}
		if len(rows) == 0 {
			return ErrNotFound
		}
		out = rows[0]
		return nil
	})
	return out, err
}

// exists reports ErrNotFound unless the organization has a source row of kind with id, waiting
// or not.
func exists(ctx context.Context, q *dbgen.Queries, org uuid.UUID, kind string, id uuid.UUID) error {
	ids := []uuid.UUID{id}
	var n int
	var err error
	switch kind {
	case KindTransferProof:
		var rows []dbgen.ListQueueTransferProofsRow
		rows, err = q.ListQueueTransferProofs(ctx, dbgen.ListQueueTransferProofsParams{OrganizationID: org, Ids: ids})
		n = len(rows)
	case KindDepositGap:
		var rows []dbgen.ListQueueDepositGapsRow
		rows, err = q.ListQueueDepositGaps(ctx, dbgen.ListQueueDepositGapsParams{OrganizationID: org, Ids: ids})
		n = len(rows)
	case KindUploadInfected:
		var rows []dbgen.ListQueueInfectedUploadsRow
		rows, err = q.ListQueueInfectedUploads(ctx, dbgen.ListQueueInfectedUploadsParams{OrganizationID: org, Ids: ids})
		n = len(rows)
	case KindQrisAmountMismatch:
		var rows []dbgen.ListQueueQrisMismatchesRow
		rows, err = q.ListQueueQrisMismatches(ctx, dbgen.ListQueueQrisMismatchesParams{OrganizationID: org, Ids: ids})
		n = len(rows)
	case KindPhoneChanged:
		var rows []dbgen.ListQueuePhoneChangesRow
		rows, err = q.ListQueuePhoneChanges(ctx, dbgen.ListQueuePhoneChangesParams{OrganizationID: org, Ids: ids})
		n = len(rows)
	}
	if err != nil {
		return fmt.Errorf("checkqueue: loading the item: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// waiting reports ErrResolved unless the item is on the queue now. A source row of its kind that
// never was on it (a deposit with no gap, a clean upload, a visit outcome that is not a new
// number) is ErrNotFound.
func waiting(ctx context.Context, q *dbgen.Queries, org uuid.UUID, kind string, id uuid.UUID) error {
	_, err := q.GetCheckQueueItem(ctx, dbgen.GetCheckQueueItemParams{OrganizationID: org, Kind: kind, ItemID: id})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("checkqueue: reading the item: %w", err)
	}
	resolved, err := q.IsCheckItemResolved(ctx, dbgen.IsCheckItemResolvedParams{OrganizationID: org, Kind: kind, ItemID: id})
	if err != nil {
		return fmt.Errorf("checkqueue: reading the item's resolution: %w", err)
	}
	if resolved {
		return ErrResolved
	}
	return ErrNotFound
}

// proofReady reports ErrUploadNotReady while the proof's photo is still being scanned, and
// ErrUploadRefused once the scan refused it (an infected one waits as an upload_infected item).
func proofReady(ctx context.Context, q *dbgen.Queries, org, uploadID uuid.UUID) error {
	up, err := q.GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: org, ID: uploadID})
	if err != nil {
		return fmt.Errorf("checkqueue: loading the proof's photo: %w", err)
	}
	switch up.Status {
	case uploadClean:
		return nil
	case uploadPending:
		return ErrUploadNotReady
	default:
		return ErrUploadRefused
	}
}

// insert records a resolution. A concurrent answer to the same item makes the unique key fail,
// which is ErrResolved.
func (s *Service) insert(ctx context.Context, q *dbgen.Queries, org, by uuid.UUID, kind string, id uuid.UUID, outcome string, paymentID uuid.UUID, note string) (Resolution, error) {
	arg := dbgen.InsertCheckResolutionParams{
		ID: uuid.New(), OrganizationID: org, Kind: kind, Outcome: outcome, StaffUserID: by, CreatedAt: s.clock.Now(),
		PaymentID: uuid.NullUUID{UUID: paymentID, Valid: paymentID != uuid.Nil},
	}
	if note != "" {
		arg.Note = &note
	}
	item := uuid.NullUUID{UUID: id, Valid: true}
	switch kind {
	case KindTransferProof:
		arg.TransferProofID = item
	case KindDepositGap:
		arg.CashDepositID = item
	case KindUploadInfected:
		arg.UploadID = item
	case KindQrisAmountMismatch:
		arg.QrisAttemptID = item
	case KindPhoneChanged:
		arg.VisitOutcomeID = item
	}
	row, err := q.InsertCheckResolution(ctx, arg)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return Resolution{}, ErrResolved
	}
	if err != nil {
		return Resolution{}, fmt.Errorf("checkqueue: recording the resolution: %w", err)
	}
	return fromRow(row), nil
}

func fromRow(r dbgen.CheckResolution) Resolution {
	out := Resolution{
		ID: r.ID, Kind: r.Kind, Outcome: r.Outcome, PaymentID: r.PaymentID.UUID, By: r.StaffUserID, CreatedAt: r.CreatedAt,
	}
	for _, id := range []uuid.NullUUID{r.TransferProofID, r.CashDepositID, r.UploadID, r.QrisAttemptID, r.VisitOutcomeID} {
		if id.Valid {
			out.ItemID = id.UUID
		}
	}
	if r.Note != nil {
		out.Note = *r.Note
	}
	return out
}
