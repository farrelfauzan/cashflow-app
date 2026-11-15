package visit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// errRepeat rolls back the transaction of a repeated request; the caller answers what it loaded.
var errRepeat = errors.New("visit: repeated request")

// Tenants is what the visit service needs from billing: the field app's checks on a tenant and the
// leases a member may act through. billing.Service implements it.
type Tenants interface {
	AuthorizeTenantInField(ctx context.Context, tx pgx.Tx, p authz.Principal, tenantID uuid.UUID, action string) error
	VisibleLeaseIDs(ctx context.Context, tx pgx.Tx, p authz.Principal, action, resourceType string) ([]uuid.UUID, error)
}

// Service lists a member's visits and records their outcomes.
type Service struct {
	store   *store.Store
	tenants Tenants
	clock   clock.Clock
}

// NewService returns a Service.
func NewService(st *store.Store, tenants Tenants, clk clock.Clock) *Service {
	return &Service{store: st, tenants: tenants, clock: clk}
}

// today is the Asia/Jakarta day of now, as [start, end).
func (s *Service) today() (day, start, end time.Time) {
	day = tenant.Today(s.clock)
	start = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, clock.Jakarta)
	return day, start, start.AddDate(0, 0, 1)
}

// List is one page of member p's visit list ("Kunjungan hari ini"): the tenants p collects from on
// an active lease who have an open visit task, then those p recorded an outcome on today (done),
// as ListFieldVisits orders them. It needs visit:read: the collection is authorized like the
// dashboard's (a member with no access at all is denied and audited), then a tenant is listed only
// when p may read visits through each of its active leases (decision 47). A member with the
// organization-wide grant still sees only the tenants they collect from. Total counts the rows p
// may see.
func (s *Service) List(ctx context.Context, p authz.Principal, page listing.Page) (listing.Result[Row], error) {
	org := p.OrganizationID
	now := s.clock.Now()
	day, start, end := s.today()
	out := listing.Result[Row]{Items: []Row{}, Page: page}
	err := s.store.WithOrgSnapshot(ctx, org, func(tx pgx.Tx) error {
		leaseIDs, err := s.tenants.VisibleLeaseIDs(ctx, tx, p, authz.ActionVisitRead, "tenant")
		if err != nil {
			return err
		}
		q := dbgen.New(tx)
		total, err := q.CountFieldVisits(ctx, dbgen.CountFieldVisitsParams{
			OrganizationID: org, CollectorID: p.UserID, LeaseIds: leaseIDs, DayStart: start, DayEnd: end,
		})
		if err != nil {
			return fmt.Errorf("counting the visit list: %w", err)
		}
		rows, err := q.ListFieldVisits(ctx, dbgen.ListFieldVisitsParams{
			OrganizationID: org, Today: day, CollectorID: p.UserID, LeaseIds: leaseIDs, DayStart: start, DayEnd: end,
			PageLimit: page.Limit(), PageOffset: page.Offset(),
		})
		if err != nil {
			return fmt.Errorf("listing the visit list: %w", err)
		}
		if out.Items, err = s.rows(ctx, tx, p, rows, now, start, end); err != nil {
			return err
		}
		out.Total = total
		return nil
	})
	if err != nil {
		return listing.Result[Row]{}, err
	}
	return out, nil
}

// rows completes a page of the visit list with each tenant's units, reasons, latest outcome today
// and unlinked cash.
func (s *Service) rows(ctx context.Context, tx pgx.Tx, p authz.Principal, page []dbgen.ListFieldVisitsRow, now, start, end time.Time) ([]Row, error) {
	out := make([]Row, 0, len(page))
	if len(page) == 0 {
		return out, nil
	}
	org := p.OrganizationID
	q := dbgen.New(tx)
	ids := make([]uuid.UUID, len(page))
	for i, r := range page {
		ids[i] = r.TenantID
	}
	collector := uuid.NullUUID{UUID: p.UserID, Valid: true}
	unitRows, err := q.ListFieldTodayUnits(ctx, dbgen.ListFieldTodayUnitsParams{OrganizationID: org, TenantIds: ids, CollectorID: collector})
	if err != nil {
		return nil, fmt.Errorf("listing the visit list's units: %w", err)
	}
	units := map[uuid.UUID][]billing.FieldUnit{}
	for _, u := range unitRows {
		units[u.TenantID] = append(units[u.TenantID], billing.FieldUnit{UnitLabel: u.UnitLabel, PropertyName: u.PropertyName, PropertyAddress: u.PropertyAddress})
	}
	taskRows, err := q.ListOpenVisitTasksOfTenants(ctx, dbgen.ListOpenVisitTasksOfTenantsParams{OrganizationID: org, TenantIds: ids})
	if err != nil {
		return nil, fmt.Errorf("listing the visit list's open tasks: %w", err)
	}
	tasks := map[uuid.UUID][]string{}
	for _, t := range taskRows {
		tasks[t.TenantID] = append(tasks[t.TenantID], t.Reason)
	}
	outcomeRows, err := q.ListLatestVisitOutcomes(ctx, dbgen.ListLatestVisitOutcomesParams{
		OrganizationID: org, TenantIds: ids, StaffUserID: p.UserID, DayStart: start, DayEnd: end,
	})
	if err != nil {
		return nil, fmt.Errorf("listing today's visit outcomes: %w", err)
	}
	latest := map[uuid.UUID]*Outcome{}
	for _, o := range outcomeRows {
		got, err := withResolved(ctx, q, org, o)
		if err != nil {
			return nil, err
		}
		latest[o.TenantID] = &got
	}
	cashRows, err := q.ListUnlinkedCashToday(ctx, dbgen.ListUnlinkedCashTodayParams{
		OrganizationID: org, TenantIds: ids, StaffUserID: p.UserID, DayStart: start, DayEnd: end,
	})
	if err != nil {
		return nil, fmt.Errorf("listing today's unlinked cash: %w", err)
	}
	cashIDs := make([]uuid.UUID, len(cashRows))
	for i, c := range cashRows {
		cashIDs[i] = c.ID
	}
	photos, err := billing.ReceiptPhotosTx(ctx, tx, org, cashIDs)
	if err != nil {
		return nil, err
	}
	cash := map[uuid.UUID][]billing.FieldCashPayment{}
	for _, c := range cashRows {
		cash[c.TenantID] = append(cash[c.TenantID], billing.FieldCashPayment{
			PaymentID: c.ID, Amount: c.Net, RecordedAt: c.CreatedAt, Photo: photos[c.ID],
		})
	}
	for _, r := range page {
		row := Row{
			TenantID: r.TenantID, TenantName: r.TenantName, Units: units[r.TenantID], Reasons: []Reason{},
			Outstanding: r.Outstanding, Overdue: r.Overdue, Status: StatusDone, LastOutcome: latest[r.TenantID],
			UnlinkedCashToday: cash[r.TenantID],
		}
		if row.Units == nil {
			row.Units = []billing.FieldUnit{}
		}
		if row.UnlinkedCashToday == nil {
			row.UnlinkedCashToday = []billing.FieldCashPayment{}
		}
		if r.Outstanding > 0 {
			due := r.OldestUnpaidDueDate
			row.OldestUnpaidDueDate = &due
		}
		if r.Open {
			row.Status = StatusOpen
		}
		for _, task := range tasks[r.TenantID] {
			reason, err := s.reason(ctx, q, org, r.TenantID, task, row.OldestUnpaidDueDate, now)
			if err != nil {
				return nil, err
			}
			row.Reasons = append(row.Reasons, reason)
		}
		out = append(out, row)
	}
	return out, nil
}

// reason is the client's reason for an open task of a tenant.
func (s *Service) reason(ctx context.Context, q *dbgen.Queries, org, tenantID uuid.UUID, task string, oldestDue *time.Time, now time.Time) (Reason, error) {
	r := Reason{Code: clientReason(task)}
	switch task {
	case TaskH7:
		if oldestDue != nil {
			if fr, ok := billing.FieldReasonAt(*oldestDue, now); ok && fr.Code == billing.FieldOverdue {
				r.DaysOverdue = fr.Days
			}
		}
	case TaskBrokenPromises:
		n, err := q.CountTenantBrokenPromisesSinceVisit(ctx, dbgen.CountTenantBrokenPromisesSinceVisitParams{OrganizationID: org, TenantID: tenantID})
		if err != nil {
			return Reason{}, fmt.Errorf("counting the tenant's broken promises: %w", err)
		}
		r.BrokenPromises = int(n)
	}
	return r, nil
}

// Record records an outcome member p reports on tenantID after visiting them. It needs visit:record
// on the tenant through each of its active leases (decision 47), inside the transaction; a missing
// tenant is a *billing.NotFoundError. The tenant must have an open visit task (ErrNotOpen).
//
// paid names a field cash payment p took from the tenant today (Asia/Jakarta), and promise a promise
// version p made from the field for the tenant today; each backs one outcome
// (ErrPaymentInvalid, ErrPromiseInvalid otherwise). number_changed carries the tenant's new
// WhatsApp number for the admin to apply (ErrInvalidPhone when it is not one). The outcome resolves
// the open tasks it answers: paid and promise the h7 and broken_promises ones, number_changed the
// undeliverable one, not_home none. The tenant's open tasks are locked, so two outcomes take turns.
//
// The same requestId with the same input from the same member records nothing and answers the
// outcome with created false; anything else under it is ErrRequestIDReused.
func (s *Service) Record(ctx context.Context, p authz.Principal, tenantID uuid.UUID, in Input) (Outcome, bool, error) {
	in, err := validInput(in)
	if err != nil {
		return Outcome{}, false, err
	}
	if in.NewPhone != "" {
		if in.NewPhone, err = phone.Normalize(in.NewPhone); err != nil {
			return Outcome{}, false, ErrInvalidPhone
		}
	}
	org := p.OrganizationID
	_, start, end := s.today()
	var out Outcome
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		if err := s.tenants.AuthorizeTenantInField(ctx, tx, p, tenantID, authz.ActionVisitRecord); err != nil {
			return err
		}
		q := dbgen.New(tx)
		if done, err := repeated(ctx, q, p, tenantID, in); err != nil || done != nil {
			if err == nil {
				out = *done
				err = errRepeat
			}
			return err
		}
		tasks, err := q.LockOpenVisitTasksOfTenant(ctx, dbgen.LockOpenVisitTasksOfTenantParams{OrganizationID: org, TenantID: tenantID})
		if err != nil {
			return fmt.Errorf("locking the tenant's visit tasks: %w", err)
		}
		if len(tasks) == 0 {
			return ErrNotOpen
		}
		if err := checkRefs(ctx, q, p, tenantID, in, start, end); err != nil {
			return err
		}
		row, err := q.InsertVisitOutcome(ctx, dbgen.InsertVisitOutcomeParams{
			ID: uuid.New(), OrganizationID: org, TenantID: tenantID, StaffUserID: p.UserID, RequestID: in.RequestID,
			Outcome: in.Outcome, PaymentID: nullUUID(in.PaymentID), PromiseVersionID: nullUUID(in.PromiseVersionID),
			NewPhone: optional(in.NewPhone), Note: optional(in.Note), CreatedAt: s.clock.Now(),
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// Recorded under this requestId by a request that committed after ours looked.
			done, err := repeated(ctx, q, p, tenantID, in)
			if err != nil {
				return err
			}
			if done == nil {
				return fmt.Errorf("visit outcome under request %s neither inserted nor found", in.RequestID)
			}
			out = *done
			return errRepeat
		case isUniqueViolation(err, "visit_outcome_payment_once"):
			return ErrPaymentInvalid
		case isUniqueViolation(err, "visit_outcome_promise_once"):
			return ErrPromiseInvalid
		case err != nil:
			return fmt.Errorf("recording the visit outcome: %w", err)
		}
		out = fromRow(row)
		out.Resolved = []string{}
		for _, t := range tasks {
			if !answers(in.Outcome, t.Reason) {
				continue
			}
			if err := q.InsertVisitOutcomeTask(ctx, dbgen.InsertVisitOutcomeTaskParams{
				VisitTaskID: t.ID, OrganizationID: org, TenantID: tenantID, OutcomeID: row.ID,
			}); err != nil {
				return fmt.Errorf("linking the resolved visit task: %w", err)
			}
			if _, err := q.ResolveVisitTask(ctx, dbgen.ResolveVisitTaskParams{OrganizationID: org, ID: t.ID, ResolvedAt: &row.CreatedAt}); err != nil {
				return fmt.Errorf("resolving the visit task: %w", err)
			}
			out.Resolved = append(out.Resolved, clientReason(t.Reason))
		}
		return nil
	})
	if errors.Is(err, errRepeat) {
		return out, false, nil
	}
	if err != nil {
		return Outcome{}, false, err
	}
	return out, true, nil
}

// answers reports whether outcome resolves a task of reason.
func answers(outcome, reason string) bool {
	for _, r := range resolves[outcome] {
		if r == reason {
			return true
		}
	}
	return false
}

// repeated returns the outcome already recorded under in.RequestID when it is the one asked for,
// nil when there is none, and ErrRequestIDReused when it is another.
func repeated(ctx context.Context, q *dbgen.Queries, p authz.Principal, tenantID uuid.UUID, in Input) (*Outcome, error) {
	row, err := q.GetVisitOutcomeByRequestID(ctx, dbgen.GetVisitOutcomeByRequestIDParams{OrganizationID: p.OrganizationID, RequestID: in.RequestID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("loading the outcome of a repeated request: %w", err)
	}
	same := row.StaffUserID == p.UserID && row.TenantID == tenantID && row.Outcome == in.Outcome &&
		row.PaymentID.UUID == in.PaymentID && row.PromiseVersionID.UUID == in.PromiseVersionID &&
		deref(row.NewPhone) == in.NewPhone && deref(row.Note) == in.Note
	if !same {
		return nil, ErrRequestIDReused
	}
	got, err := withResolved(ctx, q, p.OrganizationID, row)
	if err != nil {
		return nil, err
	}
	return &got, nil
}

// checkRefs checks the payment or promise version an outcome names.
func checkRefs(ctx context.Context, q *dbgen.Queries, p authz.Principal, tenantID uuid.UUID, in Input, start, end time.Time) error {
	org := p.OrganizationID
	inDay := func(t time.Time) bool { return !t.Before(start) && t.Before(end) }
	switch in.Outcome {
	case OutcomePaid:
		pay, err := q.GetVisitPayment(ctx, dbgen.GetVisitPaymentParams{OrganizationID: org, ID: in.PaymentID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPaymentInvalid
		}
		if err != nil {
			return fmt.Errorf("loading the outcome's payment: %w", err)
		}
		if pay.TenantID != tenantID || pay.RecordedByStaffUserID.UUID != p.UserID || pay.Method != billing.MethodCash ||
			!pay.FieldCash || !inDay(pay.ReceivedAt) {
			return ErrPaymentInvalid
		}
	case OutcomePromise:
		v, err := q.GetVisitPromiseVersion(ctx, dbgen.GetVisitPromiseVersionParams{OrganizationID: org, ID: in.PromiseVersionID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPromiseInvalid
		}
		if err != nil {
			return fmt.Errorf("loading the outcome's promise version: %w", err)
		}
		if v.TenantID != tenantID || v.Source != promise.SourceKaryawan || v.CreatedByStaffUserID.UUID != p.UserID || !inDay(v.CreatedAt) {
			return ErrPromiseInvalid
		}
	}
	return nil
}

// withResolved is an outcome row with the reasons of the tasks it resolved.
func withResolved(ctx context.Context, q *dbgen.Queries, org uuid.UUID, row dbgen.VisitOutcome) (Outcome, error) {
	out := fromRow(row)
	reasons, err := q.ListVisitOutcomeTaskReasons(ctx, dbgen.ListVisitOutcomeTaskReasonsParams{OrganizationID: org, OutcomeID: row.ID})
	if err != nil {
		return Outcome{}, fmt.Errorf("listing what the outcome resolved: %w", err)
	}
	out.Resolved = make([]string, len(reasons))
	for i, r := range reasons {
		out.Resolved[i] = clientReason(r)
	}
	return out, nil
}

func fromRow(r dbgen.VisitOutcome) Outcome {
	return Outcome{
		ID: r.ID, TenantID: r.TenantID, Outcome: r.Outcome, PaymentID: r.PaymentID.UUID, PromiseVersionID: r.PromiseVersionID.UUID,
		NewPhone: deref(r.NewPhone), Note: deref(r.Note), RecordedAt: r.CreatedAt,
	}
}

func nullUUID(id uuid.UUID) uuid.NullUUID { return uuid.NullUUID{UUID: id, Valid: id != uuid.Nil} }

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func isUniqueViolation(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}
