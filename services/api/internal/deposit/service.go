package deposit

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// errRepeat rolls back the transaction of a repeated request, its audit row included; the caller
// answers what it loaded.
var errRepeat = errors.New("deposit: repeated request")

// Service records, lists and confirms deposits.
type Service struct {
	store *store.Store
	authz *authz.Service
	clock clock.Clock
	log   *slog.Logger
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service, clk clock.Clock, log *slog.Logger) *Service {
	return &Service{store: st, authz: az, clock: clk, log: log}
}

// ownCash is the check on a member's own cash: cash:deposit on a record they own (the karyawan's
// "self").
func ownCash(p authz.Principal, owner uuid.UUID, amount *int64) authz.Request {
	return authz.Request{Action: authz.ActionCashDeposit, Resource: authz.Resource{
		Type: "cash", ID: owner.String(), OwnerID: owner, Amount: amount,
	}}
}

// Record records that member p hands over the field cash they hold (billing.FieldCash), all of it
// at once: every field cash payment p recorded that no deposit covers yet. in.ExpectedAmount and
// in.ExpectedCount must still be that cash, net of corrections (ErrCashChanged otherwise, nothing
// recorded); p must hold some (ErrNothingToDeposit). It needs cash:deposit on p's own cash with
// in.Amount, audited, in the same transaction.
//
// in.PhotoUploadID, when set, must be a completed cash_deposit upload by p (ErrPhotoInvalid), not
// refused by the scan (ErrPhotoRefused), and not another deposit's photo (ErrPhotoInvalid); an
// upload not in the organization is ErrPhotoNotFound.
//
// The same requestId with the same input from the same member records nothing and answers the
// deposit with created false (its transaction, audit row included, rolled back); anything else
// under it is ErrRequestIDReused.
func (s *Service) Record(ctx context.Context, p authz.Principal, in Input) (Deposit, bool, error) {
	in, err := validInput(in)
	if err != nil {
		return Deposit{}, false, err
	}
	org := p.OrganizationID
	var out Deposit
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		amount := in.Amount
		if err := s.authz.Authorize(ctx, tx, p, ownCash(p, p.UserID, &amount)); err != nil {
			return err
		}
		done, err := s.repeated(ctx, q, p, in)
		if err != nil {
			return err
		}
		if done != nil {
			if out, err = getDeposit(ctx, q, org, done.ID); err != nil {
				return err
			}
			return errRepeat
		}
		if in.PhotoUploadID != uuid.Nil {
			if err := checkPhoto(ctx, q, p, in.PhotoUploadID); err != nil {
				return err
			}
		}
		row, err := q.InsertCashDeposit(ctx, dbgen.InsertCashDepositParams{
			ID: uuid.New(), OrganizationID: org, StaffUserID: p.UserID, RequestID: in.RequestID, Amount: in.Amount,
			ExpectedAmount: in.ExpectedAmount, PaymentCount: int32(in.ExpectedCount), //nolint:gosec // at most maxCount
			Note: optional(in.Note), PhotoUploadID: nullUUID(in.PhotoUploadID), CreatedAt: s.clock.Now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// Recorded under this requestId meanwhile, by a request that committed after ours
			// looked: answer it as a repeat, or refuse the reuse.
			return s.raced(ctx, q, p, in, &out)
		}
		if isUniqueViolation(err, "cash_deposit_photo_once") {
			return ErrPhotoInvalid
		}
		if err != nil {
			return fmt.Errorf("recording the deposit: %w", err)
		}
		covered, err := q.CoverUndepositedCash(ctx, dbgen.CoverUndepositedCashParams{
			OrganizationID: org, DepositID: row.ID, StaffUserID: p.UserID,
		})
		if err != nil {
			return fmt.Errorf("covering the member's cash: %w", err)
		}
		if len(covered) == 0 {
			return ErrNothingToDeposit
		}
		var total int64
		for _, c := range covered {
			total += c.Net
		}
		if total != in.ExpectedAmount || len(covered) != in.ExpectedCount {
			return ErrCashChanged
		}
		out, err = getDeposit(ctx, q, org, row.ID)
		return err
	})
	if errors.Is(err, errRepeat) {
		return out, false, nil
	}
	if err != nil {
		return Deposit{}, false, err
	}
	return out, true, nil
}

// repeated returns the deposit already recorded under in.RequestID when it is the one in asks
// for, nil when there is none, and ErrRequestIDReused when it is another.
func (s *Service) repeated(ctx context.Context, q *dbgen.Queries, p authz.Principal, in Input) (*dbgen.CashDeposit, error) {
	row, err := q.GetCashDepositByRequestID(ctx, dbgen.GetCashDepositByRequestIDParams{OrganizationID: p.OrganizationID, RequestID: in.RequestID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading the deposit of a repeated request: %w", err)
	}
	same := row.StaffUserID == p.UserID && row.Amount == in.Amount && row.ExpectedAmount == in.ExpectedAmount &&
		int(row.PaymentCount) == in.ExpectedCount && deref(row.Note) == in.Note &&
		row.PhotoUploadID.UUID == in.PhotoUploadID
	if !same {
		return nil, ErrRequestIDReused
	}
	return &row, nil
}

// raced handles a requestId recorded by a concurrent request after this one looked.
func (s *Service) raced(ctx context.Context, q *dbgen.Queries, p authz.Principal, in Input, out *Deposit) error {
	done, err := s.repeated(ctx, q, p, in)
	if err != nil {
		return err
	}
	if done == nil {
		return fmt.Errorf("deposit under request %s neither inserted nor found", in.RequestID)
	}
	if *out, err = getDeposit(ctx, q, p.OrganizationID, done.ID); err != nil {
		return err
	}
	return errRepeat
}

// checkPhoto checks that upload id can be p's deposit photo.
func checkPhoto(ctx context.Context, q *dbgen.Queries, p authz.Principal, id uuid.UUID) error {
	up, err := q.GetUpload(ctx, dbgen.GetUploadParams{OrganizationID: p.OrganizationID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPhotoNotFound
	}
	if err != nil {
		return fmt.Errorf("loading the deposit photo: %w", err)
	}
	if up.Purpose != upload.PurposeCashDeposit || up.UploadedByStaffUserID.UUID != p.UserID || up.CompletedAt == nil {
		return ErrPhotoInvalid
	}
	if up.Status == upload.StatusRejected || up.Status == upload.StatusInfected {
		return ErrPhotoRefused
	}
	return nil
}

// Mine is one page of member p's own deposits, newest first. It needs cash:deposit on p's own
// cash; an allowed read is not audited, a denial is.
func (s *Service) Mine(ctx context.Context, p authz.Principal, page listing.Page) (listing.Result[Deposit], error) {
	if err := s.authz.Check(ctx, p, ownCash(p, p.UserID, nil)); err != nil {
		return listing.Result[Deposit]{}, err
	}
	return s.list(ctx, p.OrganizationID, Query{StaffUserID: p.UserID}, page)
}

// List is one page of the organization's deposits, newest first, filtered by q. It needs
// payment:check (the admin) or cash:read (the Supervisor) for the whole organization: a deposit
// spans properties. An allowed read is not audited; a denial is. A status outside FilterPending
// and FilterConfirmed is ErrStatusUnknown.
func (s *Service) List(ctx context.Context, p authz.Principal, q Query, page listing.Page) (listing.Result[Deposit], error) {
	if q.Status != "" && q.Status != FilterPending && q.Status != FilterConfirmed {
		return listing.Result[Deposit]{}, ErrStatusUnknown
	}
	if err := s.checkReadAll(ctx, p, authz.Resource{Type: "cash_deposit"}); err != nil {
		return listing.Result[Deposit]{}, err
	}
	return s.list(ctx, p.OrganizationID, q, page)
}

func (s *Service) list(ctx context.Context, org uuid.UUID, filter Query, page listing.Page) (listing.Result[Deposit], error) {
	out := listing.Result[Deposit]{Items: []Deposit{}, Page: page}
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		staff := nullUUID(filter.StaffUserID)
		status := optional(filter.Status)
		total, err := q.CountCashDeposits(ctx, dbgen.CountCashDepositsParams{OrganizationID: org, StaffUserID: staff, Status: status})
		if err != nil {
			return fmt.Errorf("counting deposits: %w", err)
		}
		rows, err := q.ListCashDeposits(ctx, dbgen.ListCashDepositsParams{
			OrganizationID: org, StaffUserID: staff, Status: status, PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("listing deposits: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, fromRow(r))
		}
		out.Total = total
		return nil
	})
	if err != nil {
		return listing.Result[Deposit]{}, err
	}
	return out, nil
}

// checkReadAll allows payment:check or cash:read on res, without auditing an allow; a denial is
// audited as payment:check.
func (s *Service) checkReadAll(ctx context.Context, p authz.Principal, res authz.Resource) error {
	check := authz.Request{Action: authz.ActionPaymentCheck, Resource: res}
	if authz.Allowed(p, check) || authz.Allowed(p, authz.Request{Action: authz.ActionCashRead, Resource: res}) {
		return nil
	}
	return s.authz.Deny(ctx, p, check)
}

// Get is one deposit. The admin (payment:check), the Supervisor (cash:read) and the member who
// deposited it (cash:deposit on their own cash) may read it; anyone else is denied, audited. A
// deposit not in the organization is ErrNotFound.
func (s *Service) Get(ctx context.Context, p authz.Principal, id uuid.UUID) (Deposit, error) {
	var out Deposit
	err := s.store.WithOrgSnapshot(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		if out, err = getDeposit(ctx, dbgen.New(tx), p.OrganizationID, id); err != nil {
			return err
		}
		return s.checkRead(ctx, p, out)
	})
	if err != nil {
		return Deposit{}, err
	}
	return out, nil
}

// Payments is one page of the payments deposit id covers, oldest recorded first, with their
// tenants' units collected by the depositor and their photos. It is checked as Get; the total is
// the deposit's payment count.
func (s *Service) Payments(ctx context.Context, p authz.Principal, id uuid.UUID, page listing.Page) (listing.Result[billing.HeldPayment], error) {
	org := p.OrganizationID
	out := listing.Result[billing.HeldPayment]{Items: []billing.HeldPayment{}, Page: page}
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		dep, err := getDeposit(ctx, q, org, id)
		if err != nil {
			return err
		}
		if err := s.checkRead(ctx, p, dep); err != nil {
			return err
		}
		rows, err := q.ListCashDepositPayments(ctx, dbgen.ListCashDepositPaymentsParams{
			OrganizationID: org, DepositID: id, PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("listing the deposit's payments: %w", err)
		}
		for _, r := range rows {
			out.Items = append(out.Items, billing.HeldPayment{
				PaymentID: r.ID, TenantID: r.TenantID, TenantName: r.TenantName, Amount: r.Net, RecordedAt: r.CreatedAt,
			})
		}
		out.Total = int64(dep.PaymentCount)
		return billing.DescribeHeldPaymentsTx(ctx, tx, org, dep.StaffUserID, out.Items)
	})
	if err != nil {
		return listing.Result[billing.HeldPayment]{}, err
	}
	return out, nil
}

// checkRead allows reading deposit d as Get describes.
func (s *Service) checkRead(ctx context.Context, p authz.Principal, d Deposit) error {
	res := authz.Resource{Type: "cash_deposit", ID: d.ID.String(), OwnerID: d.StaffUserID}
	if authz.Allowed(p, authz.Request{Action: authz.ActionPaymentCheck, Resource: res}) ||
		authz.Allowed(p, authz.Request{Action: authz.ActionCashRead, Resource: res}) ||
		authz.Allowed(p, ownCash(p, d.StaffUserID, nil)) {
		return nil
	}
	return s.authz.Deny(ctx, p, authz.Request{Action: authz.ActionPaymentCheck, Resource: res})
}

// Confirm records an admin's count of deposit id, once: payment:check on the deposit with the
// received amount, for the whole organization, audited, in the same transaction. Nobody confirms
// their own deposit (denied, audited). The same count again records nothing and answers the
// deposit with created false (rolled back, audit row included); another count of a deposit
// already confirmed is ErrAlreadyConfirmed. A deposit not in the organization is ErrNotFound.
func (s *Service) Confirm(ctx context.Context, p authz.Principal, id uuid.UUID, in ConfirmInput) (Deposit, bool, error) {
	in, err := validConfirm(in)
	if err != nil {
		return Deposit{}, false, err
	}
	org := p.OrganizationID
	var out Deposit
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		dep, err := getDeposit(ctx, q, org, id)
		if err != nil {
			return err
		}
		received := in.ReceivedAmount
		req := authz.Request{Action: authz.ActionPaymentCheck, Resource: authz.Resource{
			Type: "cash_deposit", ID: id.String(), OwnerID: dep.StaffUserID, Amount: &received,
		}}
		if dep.StaffUserID == p.UserID {
			return s.authz.Deny(ctx, p, req)
		}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		if dep.Confirmation == nil {
			n, err := q.InsertCashDepositConfirmation(ctx, dbgen.InsertCashDepositConfirmationParams{
				DepositID: id, OrganizationID: org, StaffUserID: dep.StaffUserID, ReceivedAmount: in.ReceivedAmount,
				Note: optional(in.Note), ConfirmedByStaffUserID: p.UserID, ConfirmedAt: s.clock.Now(),
			})
			if err != nil {
				return fmt.Errorf("confirming the deposit: %w", err)
			}
			if out, err = getDeposit(ctx, q, org, id); err != nil {
				return err
			}
			if n == 1 {
				return nil
			}
			dep = out // confirmed meanwhile by another request
		}
		c := dep.Confirmation
		if c == nil || c.ReceivedAmount != in.ReceivedAmount || c.Note != in.Note || c.ConfirmedBy != p.UserID {
			return ErrAlreadyConfirmed
		}
		out = dep
		return errRepeat
	})
	if errors.Is(err, errRepeat) {
		return out, false, nil
	}
	if err != nil {
		return Deposit{}, false, err
	}
	return out, true, nil
}

func getDeposit(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) (Deposit, error) {
	row, err := q.GetCashDeposit(ctx, dbgen.GetCashDepositParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return Deposit{}, ErrNotFound
	}
	if err != nil {
		return Deposit{}, fmt.Errorf("loading deposit %s: %w", id, err)
	}
	return fromRow(dbgen.ListCashDepositsRow(row)), nil
}

func nullUUID(id uuid.UUID) uuid.NullUUID {
	return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil}
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
