package property

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// The messages shared by the unit and price inputs.
const (
	termMessage  = "Jangka sewa harus 1 sampai 60 bulan."
	priceMessage = "Harga sewa harus lebih dari Rp0."
	// customTermMessage is for a second term outside 1, 3, 6 and 12 months (decision 33).
	customTermMessage = "Selain 1, 3, 6 dan 12 bulan, unit hanya bisa menawarkan satu jangka sewa lain."
)

// ListUnitPrices returns a unit's whole price history (decision 32), the latest effective date
// first and, on the same date, the latest recorded first. It needs unit:read on the unit's
// property.
func (s *Service) ListUnitPrices(ctx context.Context, p authz.Principal, unitID uuid.UUID) ([]UnitPrice, error) {
	var out []UnitPrice
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		u, err := getUnit(ctx, q, p.OrganizationID, unitID)
		if err != nil {
			return err
		}
		if err := s.authz.Authorize(ctx, tx, p, unitRequest(authz.ActionUnitRead, u.PropertyID, unitID)); err != nil {
			return err
		}
		out, err = listPrices(ctx, q, p.OrganizationID, unitID)
		return err
	})
	return out, err
}

// CreateUnitPrice records a unit's new price for one of its terms from a day on: today in
// Asia/Jakarta or later (else ErrPriceTooEarly). The unit must offer the term (else a
// *tenant.TermError on months with tenant.ErrTermNotOffered). It needs unit:manage on the
// unit's property, which is audited. Rows are never changed: a later price is a new row.
func (s *Service) CreateUnitPrice(ctx context.Context, p authz.Principal, unitID uuid.UUID, in PriceInput) (UnitPrice, error) {
	var out UnitPrice
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		out, err = s.CreateUnitPriceTx(ctx, tx, p, unitID, in)
		return err
	})
	return out, err
}

// CreateUnitPriceTx is CreateUnitPrice inside the caller's organization transaction.
func (s *Service) CreateUnitPriceTx(ctx context.Context, tx pgx.Tx, p authz.Principal, unitID uuid.UUID, in PriceInput) (UnitPrice, error) {
	in, err := ValidPrice(in)
	if err != nil {
		return UnitPrice{}, err
	}
	q := dbgen.New(tx)
	u, err := getUnit(ctx, q, p.OrganizationID, unitID)
	if err != nil {
		return UnitPrice{}, err
	}
	if err := s.authz.Authorize(ctx, tx, p, unitRequest(authz.ActionUnitManage, u.PropertyID, unitID)); err != nil {
		return UnitPrice{}, err
	}
	if !slices.Contains(u.Terms, int16(in.Months)) { //nolint:gosec // a valid term is 1 to 60
		return UnitPrice{}, &tenant.TermError{Err: tenant.ErrTermNotOffered, Field: "months", Months: in.Months}
	}
	if in.EffectiveFrom.Before(tenant.Today(s.clock)) {
		return UnitPrice{}, ErrPriceTooEarly
	}
	id, err := insertPrice(ctx, q, p, unitID, in)
	if err != nil {
		return UnitPrice{}, err
	}
	rows, err := listPrices(ctx, q, p.OrganizationID, unitID)
	if err != nil {
		return UnitPrice{}, err
	}
	for _, r := range rows {
		if r.ID == id {
			return r, nil
		}
	}
	return UnitPrice{}, errors.New("property: the new price is missing from the history")
}

// ValidPrice checks a price input and returns it with the date as a calendar day. Errors are
// *InputError.
func ValidPrice(in PriceInput) (PriceInput, error) {
	if !in.EffectiveFrom.IsZero() {
		in.EffectiveFrom = time.Date(in.EffectiveFrom.Year(), in.EffectiveFrom.Month(), in.EffectiveFrom.Day(), 0, 0, 0, 0, time.UTC)
	}
	switch {
	case !tenant.ValidTerm(in.Months):
		return in, &InputError{Field: "months", Message: termMessage}
	case in.Amount <= 0:
		return in, &InputError{Field: "amount", Message: priceMessage}
	case in.EffectiveFrom.IsZero():
		return in, &InputError{Field: "effectiveFrom", Message: "Tanggal mulai berlaku wajib diisi."}
	}
	return in, nil
}

// recordPrices adds a price effective today for each term whose given price differs from the
// one in force today, or has none. Terms without a price are left alone.
func recordPrices(ctx context.Context, q *dbgen.Queries, p authz.Principal, unitID uuid.UUID, terms []TermInput, today time.Time) error {
	if !slices.ContainsFunc(terms, func(t TermInput) bool { return t.Price != nil }) {
		return nil
	}
	rows, err := q.ListPricesInForce(ctx, dbgen.ListPricesInForceParams{
		OrganizationID: p.OrganizationID, UnitIds: []uuid.UUID{unitID}, Day: today,
	})
	if err != nil {
		return fmt.Errorf("loading prices in force: %w", err)
	}
	inForce := map[int]int64{}
	for _, r := range rows {
		inForce[int(r.TermMonths)] = r.Amount
	}
	for _, t := range terms {
		if t.Price == nil {
			continue
		}
		if current, ok := inForce[t.Months]; ok && current == *t.Price {
			continue
		}
		if _, err := insertPrice(ctx, q, p, unitID, PriceInput{Months: t.Months, Amount: *t.Price, EffectiveFrom: today}); err != nil {
			return err
		}
	}
	return nil
}

func insertPrice(ctx context.Context, q *dbgen.Queries, p authz.Principal, unitID uuid.UUID, in PriceInput) (uuid.UUID, error) {
	r, err := q.CreateUnitPrice(ctx, dbgen.CreateUnitPriceParams{
		ID: uuid.New(), OrganizationID: p.OrganizationID, UnitID: unitID,
		TermMonths: int16(in.Months), //nolint:gosec // a valid term is 1 to 60
		Amount:     in.Amount, EffectiveFrom: in.EffectiveFrom,
		CreatedByStaffUserID: uuid.NullUUID{UUID: p.UserID, Valid: p.UserID != uuid.Nil},
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("saving unit price: %w", err)
	}
	return r.ID, nil
}

func listPrices(ctx context.Context, q *dbgen.Queries, org, unitID uuid.UUID) ([]UnitPrice, error) {
	rows, err := q.ListUnitPrices(ctx, dbgen.ListUnitPricesParams{OrganizationID: org, UnitID: unitID})
	if err != nil {
		return nil, fmt.Errorf("listing unit prices: %w", err)
	}
	out := make([]UnitPrice, len(rows))
	for i, r := range rows {
		out[i] = UnitPrice{
			ID: r.ID, Months: int(r.TermMonths), Amount: r.Amount, EffectiveFrom: r.EffectiveFrom,
			RecordedAt: r.CreatedAt, RecordedByName: r.CreatedByName,
		}
	}
	return out, nil
}

// attachPrices fills in each unit's terms with the price in force on day and the next one
// after it, with one query each for all the units.
func attachPrices(ctx context.Context, q *dbgen.Queries, org uuid.UUID, day time.Time, units []Unit) error {
	if len(units) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(units))
	for i, u := range units {
		ids[i] = u.ID
	}
	type key struct {
		unit   uuid.UUID
		months int
	}
	inForce, err := q.ListPricesInForce(ctx, dbgen.ListPricesInForceParams{OrganizationID: org, UnitIds: ids, Day: day})
	if err != nil {
		return fmt.Errorf("loading prices in force: %w", err)
	}
	prices := map[key]int64{}
	for _, r := range inForce {
		prices[key{r.UnitID, int(r.TermMonths)}] = r.Amount
	}
	upcoming, err := q.ListNextPrices(ctx, dbgen.ListNextPricesParams{OrganizationID: org, UnitIds: ids, Day: day})
	if err != nil {
		return fmt.Errorf("loading next prices: %w", err)
	}
	next := map[key]ScheduledPrice{}
	for _, r := range upcoming {
		next[key{r.UnitID, int(r.TermMonths)}] = ScheduledPrice{Amount: r.Amount, EffectiveFrom: r.EffectiveFrom}
	}
	for i := range units {
		u := &units[i]
		for j := range u.Terms {
			term := &u.Terms[j]
			k := key{u.ID, term.Months}
			if amount, ok := prices[k]; ok {
				term.Price = &amount
			}
			if sp, ok := next[k]; ok {
				term.Next = &sp
			}
		}
	}
	return nil
}

// termMonths is the offered months as the unit row stores them.
func termMonths(terms []TermInput) []int16 {
	out := make([]int16, len(terms))
	for i, t := range terms {
		out[i] = int16(t.Months) //nolint:gosec // a valid term is 1 to 60
	}
	return out
}

func getUnit(ctx context.Context, q *dbgen.Queries, org, id uuid.UUID) (dbgen.Unit, error) {
	u, err := q.GetUnit(ctx, dbgen.GetUnitParams{OrganizationID: org, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	if err != nil {
		return u, fmt.Errorf("loading unit: %w", err)
	}
	return u, nil
}
