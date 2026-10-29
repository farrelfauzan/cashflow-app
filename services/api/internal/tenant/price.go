package tenant

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

var (
	// ErrNoPrice means a unit has no price for a term in force on the day asked about.
	ErrNoPrice = errors.New("tenant: no price in force")
	// ErrTermNotOffered means the unit does not offer the billing term asked for.
	ErrTermNotOffered = errors.New("tenant: term not offered by the unit")
)

// TermError is ErrTermNotOffered or ErrNoPrice for one term, with the input field that named
// the term, so the message can say which term and point at the field.
type TermError struct {
	Err    error // ErrTermNotOffered or ErrNoPrice
	Field  string
	Months int
}

func (e *TermError) Error() string { return fmt.Sprintf("%v: %d months", e.Err, e.Months) }

func (e *TermError) Unwrap() error { return e.Err }

// PriceInForce returns the rent of one period of a unit's term in force on day (decision 32):
// the unit_price row with the latest effective date on or before day, the latest recorded one
// on the same date, or ErrNoPrice. A period's invoice takes the price in force on the day the
// invoice is made. It does no permission check; the caller has authorized what it is part of.
func PriceInForce(ctx context.Context, q *dbgen.Queries, org, unitID uuid.UUID, term int, day time.Time) (int64, error) {
	amount, err := q.GetPriceInForce(ctx, dbgen.GetPriceInForceParams{
		OrganizationID: org, UnitID: unitID, TermMonths: int16(term), Day: calendarDay(day), //nolint:gosec // a term is 1 to 12
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNoPrice
	}
	if err != nil {
		return 0, fmt.Errorf("loading the price in force: %w", err)
	}
	return amount, nil
}

// attachPeriodRents fills in each lease's PeriodRent: the price for its unit and term in force
// on day, with one query for all of them. A lease whose term has no price keeps nil.
func attachPeriodRents(ctx context.Context, q *dbgen.Queries, org uuid.UUID, day time.Time, leases []Lease) error {
	if len(leases) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(leases))
	for _, l := range leases {
		ids = append(ids, l.UnitID)
	}
	rows, err := q.ListPricesInForce(ctx, dbgen.ListPricesInForceParams{OrganizationID: org, UnitIds: ids, Day: calendarDay(day)})
	if err != nil {
		return fmt.Errorf("loading prices in force: %w", err)
	}
	type key struct {
		unit uuid.UUID
		term int
	}
	prices := make(map[key]int64, len(rows))
	for _, r := range rows {
		prices[key{r.UnitID, int(r.TermMonths)}] = r.Amount
	}
	for i := range leases {
		if amount, ok := prices[key{leases[i].UnitID, leases[i].TermMonths}]; ok {
			leases[i].PeriodRent = &amount
		}
	}
	return nil
}
