package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

const (
	maxChargeTypeNameLength = 50
	// maxLeaseCharges bounds one lease's fixed extras; a real lease has a handful.
	maxLeaseCharges = 20
)

// ErrChargeTypeTaken means another charge type in the organization already has the name.
var ErrChargeTypeTaken = errors.New("tenant: charge type name already used")

// ChargeType is a kind of extra charge on top of rent (listrik, air, Wi-Fi, ...), shared by the
// whole organization. A metered type is billed from usage recorded each month, never as a fixed
// lease charge.
type ChargeType struct {
	ID        uuid.UUID
	Name      string
	Metered   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ChargeTypeInput is the input for adding a charge type.
type ChargeTypeInput struct {
	Name    string
	Metered bool
}

// Charge is a fixed monthly extra on a lease, in rupiah.
type Charge struct {
	ChargeTypeID uuid.UUID
	Name         string
	Amount       int64
}

// ChargeInput is one fixed extra to set on a lease.
type ChargeInput struct {
	ChargeTypeID uuid.UUID
	Amount       int64
}

// MeteredType is a metered charge type a lease expects an amount for every month ("Listrik
// dicatat dari meteran tiap bulan"). It carries no amount: amounts are recorded per month.
type MeteredType struct {
	ChargeTypeID uuid.UUID
	Name         string
}

// LeaseChargesInput is the extras of a lease: its whole set of fixed extras and, when
// MeteredTypes is not nil, its whole set of expected metered types (nil leaves them unchanged).
type LeaseChargesInput struct {
	Charges      []ChargeInput
	MeteredTypes *[]uuid.UUID
}

// chargeTypeRequest is lease:manage on the organization's charge types. It names no property,
// so only an organization-wide grant allows it. Creating a type uses it, so a member limited to
// some properties cannot add the types every property shares (decision 15). Listing is open to
// anyone with any lease:manage access (decision 22) and uses this request only to record a
// denial.
func chargeTypeRequest(id uuid.UUID) authz.Request {
	res := authz.Resource{Type: "charge-type"}
	if id != uuid.Nil {
		res.ID = id.String()
	}
	return authz.Request{Action: authz.ActionLeaseManage, Resource: res}
}

// ListChargeTypes returns the organization's charge types by name, the list a lease form picks
// from. Anyone who may manage leases, in the whole organization or only in some properties, may
// read it: a property-limited admin sets charges on their own leases and needs the names, which
// reveal nothing about other properties. Without lease:manage the request is refused through
// Check, which records the denial; allowed reads write no audit row.
func (s *Service) ListChargeTypes(ctx context.Context, p authz.Principal) ([]ChargeType, error) {
	if authz.AccessTo(p, authz.ActionLeaseManage) == authz.AccessDeny {
		if err := s.authz.Check(ctx, p, chargeTypeRequest(uuid.Nil)); err != nil {
			return nil, err
		}
	}
	var out []ChargeType
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListChargeTypes(ctx, p.OrganizationID)
		if err != nil {
			return fmt.Errorf("listing charge types: %w", err)
		}
		out = make([]ChargeType, len(rows))
		for i, r := range rows {
			out[i] = toChargeType(r)
		}
		return nil
	})
	return out, err
}

// CreateChargeType adds a charge type. It needs lease:manage for the whole organization and is
// audited.
func (s *Service) CreateChargeType(ctx context.Context, p authz.Principal, in ChargeTypeInput) (ChargeType, error) {
	in, err := ValidChargeType(in)
	if err != nil {
		return ChargeType{}, err
	}
	var out ChargeType
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		id := uuid.New()
		if err := s.authz.Authorize(ctx, tx, p, chargeTypeRequest(id)); err != nil {
			return err
		}
		r, err := dbgen.New(tx).CreateChargeType(ctx, dbgen.CreateChargeTypeParams{
			ID: id, OrganizationID: p.OrganizationID, Name: in.Name, Metered: in.Metered,
		})
		if isUniqueViolation(err, "charge_type_name_unique") {
			return ErrChargeTypeTaken
		}
		if err != nil {
			return fmt.Errorf("creating charge type: %w", err)
		}
		out = toChargeType(r)
		return nil
	})
	return out, err
}

// SetLeaseCharges replaces the whole set of fixed extras on an active lease, and its expected
// metered types when in.MeteredTypes is set; an empty set removes them all. It needs
// lease:manage on the lease, like a rent change, and is audited the same way. Fixed extras are
// lease terms, not money rows: a change applies from the next invoice generated, because each
// invoice snapshots the lease's charges into its own lines. The metered types only say which
// monthly amounts to expect; amounts already recorded stay and are billed whatever the set.
func (s *Service) SetLeaseCharges(ctx context.Context, p authz.Principal, leaseID uuid.UUID, in LeaseChargesInput) (Lease, error) {
	if err := ValidCharges(in.Charges); err != nil {
		return Lease{}, err
	}
	if in.MeteredTypes != nil {
		if err := ValidMeteredTypes(*in.MeteredTypes); err != nil {
			return Lease{}, err
		}
	}
	var out Lease
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := s.loadActiveForChange(ctx, tx, q, p, leaseID); err != nil {
			return err
		}
		if err := checkChargeTypes(ctx, q, p.OrganizationID, in.Charges); err != nil {
			return err
		}
		if err := replaceCharges(ctx, q, p.OrganizationID, leaseID, in.Charges); err != nil {
			return err
		}
		if in.MeteredTypes != nil {
			if err := checkMeteredTypes(ctx, q, p.OrganizationID, *in.MeteredTypes); err != nil {
				return err
			}
			if err := replaceMeteredTypes(ctx, q, p.OrganizationID, leaseID, *in.MeteredTypes); err != nil {
				return err
			}
		}
		var err error
		out, err = loadLease(ctx, q, p.OrganizationID, leaseID, Today(s.clock))
		return err
	})
	return out, err
}

// replaceCharges makes the lease's fixed extras exactly in.
func replaceCharges(ctx context.Context, q *dbgen.Queries, org, leaseID uuid.UUID, in []ChargeInput) error {
	// Never nil: a NULL array would make the delete keep every row.
	keep := make([]uuid.UUID, 0, len(in))
	for _, c := range in {
		keep = append(keep, c.ChargeTypeID)
	}
	err := q.DeleteLeaseChargesExcept(ctx, dbgen.DeleteLeaseChargesExceptParams{OrganizationID: org, LeaseID: leaseID, Keep: keep})
	if err != nil {
		return fmt.Errorf("removing lease charges: %w", err)
	}
	for _, c := range in {
		err := q.UpsertLeaseCharge(ctx, dbgen.UpsertLeaseChargeParams{
			ID: uuid.New(), OrganizationID: org, LeaseID: leaseID, ChargeTypeID: c.ChargeTypeID, Amount: c.Amount,
		})
		if err != nil {
			return fmt.Errorf("setting lease charge: %w", err)
		}
	}
	return nil
}

// replaceMeteredTypes makes the lease's expected metered types exactly ids.
func replaceMeteredTypes(ctx context.Context, q *dbgen.Queries, org, leaseID uuid.UUID, ids []uuid.UUID) error {
	// Never nil: a NULL array would make the delete keep every row.
	keep := append(make([]uuid.UUID, 0, len(ids)), ids...)
	err := q.DeleteLeaseMeteredTypesExcept(ctx, dbgen.DeleteLeaseMeteredTypesExceptParams{OrganizationID: org, LeaseID: leaseID, Keep: keep})
	if err != nil {
		return fmt.Errorf("removing lease metered types: %w", err)
	}
	for _, id := range ids {
		err := q.AddLeaseMeteredType(ctx, dbgen.AddLeaseMeteredTypeParams{
			ID: uuid.New(), OrganizationID: org, LeaseID: leaseID, ChargeTypeID: id,
		})
		if err != nil {
			return fmt.Errorf("adding lease metered type: %w", err)
		}
	}
	return nil
}

// checkMeteredTypes makes sure every id names a metered charge type of the organization. Errors
// are *InputError on meteredChargeTypeIds[i].
func checkMeteredTypes(ctx context.Context, q *dbgen.Queries, org uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.GetChargeTypes(ctx, dbgen.GetChargeTypesParams{OrganizationID: org, Ids: ids})
	if err != nil {
		return fmt.Errorf("loading charge types: %w", err)
	}
	byID := make(map[uuid.UUID]dbgen.ChargeType, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for i, id := range ids {
		ct, ok := byID[id]
		switch {
		case !ok:
			return &InputError{Field: meteredField(i), Message: "Jenis biaya tidak ditemukan."}
		case !ct.Metered:
			return &InputError{
				Field:   meteredField(i),
				Message: "Biaya " + ct.Name + " bukan biaya meteran. Isi jumlahnya sebagai biaya tetap.",
			}
		}
	}
	return nil
}

// checkChargeTypes makes sure every charge names a charge type of the organization that is not
// metered. Errors are *InputError on the charge's chargeTypeId.
func checkChargeTypes(ctx context.Context, q *dbgen.Queries, org uuid.UUID, in []ChargeInput) error {
	if len(in) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(in))
	for i, c := range in {
		ids[i] = c.ChargeTypeID
	}
	rows, err := q.GetChargeTypes(ctx, dbgen.GetChargeTypesParams{OrganizationID: org, Ids: ids})
	if err != nil {
		return fmt.Errorf("loading charge types: %w", err)
	}
	byID := make(map[uuid.UUID]dbgen.ChargeType, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	for i, c := range in {
		ct, ok := byID[c.ChargeTypeID]
		switch {
		case !ok:
			return &InputError{Field: chargeField(i, "chargeTypeId"), Message: "Jenis biaya tidak ditemukan."}
		case ct.Metered:
			return &InputError{
				Field:   chargeField(i, "chargeTypeId"),
				Message: "Biaya " + ct.Name + " dicatat dari meteran tiap bulan, jadi tidak bisa dijadikan biaya tetap.",
			}
		}
	}
	return nil
}

// ValidChargeType checks a charge type input and returns it with the name trimmed. Errors are
// *InputError.
func ValidChargeType(in ChargeTypeInput) (ChargeTypeInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	switch {
	case in.Name == "":
		return in, &InputError{Field: "name", Message: "Nama biaya wajib diisi."}
	case len([]rune(in.Name)) > maxChargeTypeNameLength:
		return in, &InputError{Field: "name", Message: fmt.Sprintf("Nama biaya paling panjang %d huruf.", maxChargeTypeNameLength)}
	}
	return in, nil
}

// ValidCharges checks the shape of a lease's fixed extras: at most maxLeaseCharges, each with a
// charge type named once and an amount above Rp0. Whether the types exist is checked against
// the database. Errors are *InputError.
func ValidCharges(in []ChargeInput) error {
	if len(in) > maxLeaseCharges {
		return &InputError{Field: "charges", Message: fmt.Sprintf("Paling banyak %d biaya tambahan per sewa.", maxLeaseCharges)}
	}
	seen := make(map[uuid.UUID]bool, len(in))
	for i, c := range in {
		switch {
		case c.ChargeTypeID == uuid.Nil:
			return &InputError{Field: chargeField(i, "chargeTypeId"), Message: "Jenis biaya wajib dipilih."}
		case seen[c.ChargeTypeID]:
			return &InputError{Field: chargeField(i, "chargeTypeId"), Message: "Jenis biaya ini sudah ada di daftar."}
		case c.Amount <= 0:
			return &InputError{Field: chargeField(i, "amount"), Message: "Jumlah biaya harus lebih dari Rp0."}
		}
		seen[c.ChargeTypeID] = true
	}
	return nil
}

func chargeField(i int, name string) string { return fmt.Sprintf("charges[%d].%s", i, name) }

// ValidMeteredTypes checks the shape of a lease's expected metered types: at most
// maxLeaseCharges, each named once. Whether they exist and are metered is checked against the
// database. Errors are *InputError.
func ValidMeteredTypes(ids []uuid.UUID) error {
	if len(ids) > maxLeaseCharges {
		return &InputError{Field: "meteredChargeTypeIds", Message: fmt.Sprintf("Paling banyak %d biaya meteran per sewa.", maxLeaseCharges)}
	}
	seen := make(map[uuid.UUID]bool, len(ids))
	for i, id := range ids {
		switch {
		case id == uuid.Nil:
			return &InputError{Field: meteredField(i), Message: "Jenis biaya wajib dipilih."}
		case seen[id]:
			return &InputError{Field: meteredField(i), Message: "Jenis biaya ini sudah ada di daftar."}
		}
		seen[id] = true
	}
	return nil
}

func meteredField(i int) string { return fmt.Sprintf("meteredChargeTypeIds[%d]", i) }

// attachCharges fills in the fixed extras of each lease, by name. Every lease gets a non-nil
// slice.
func attachCharges(ctx context.Context, q *dbgen.Queries, org uuid.UUID, leases []Lease) error {
	if len(leases) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(leases))
	for i, l := range leases {
		ids[i] = l.ID
	}
	rows, err := q.ListLeaseCharges(ctx, dbgen.ListLeaseChargesParams{OrganizationID: org, LeaseIds: ids})
	if err != nil {
		return fmt.Errorf("loading lease charges: %w", err)
	}
	byLease := map[uuid.UUID][]Charge{}
	for _, r := range rows {
		byLease[r.LeaseID] = append(byLease[r.LeaseID], Charge{ChargeTypeID: r.ChargeTypeID, Name: r.ChargeTypeName, Amount: r.Amount})
	}
	for i := range leases {
		if c, ok := byLease[leases[i].ID]; ok {
			leases[i].Charges = c
		} else {
			leases[i].Charges = []Charge{}
		}
	}
	return nil
}

// attachMeteredTypes fills in the expected metered types of each lease, by name. Every lease gets
// a non-nil slice.
func attachMeteredTypes(ctx context.Context, q *dbgen.Queries, org uuid.UUID, leases []Lease) error {
	if len(leases) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(leases))
	for i, l := range leases {
		ids[i] = l.ID
	}
	rows, err := q.ListLeaseMeteredTypes(ctx, dbgen.ListLeaseMeteredTypesParams{OrganizationID: org, LeaseIds: ids})
	if err != nil {
		return fmt.Errorf("loading lease metered types: %w", err)
	}
	byLease := map[uuid.UUID][]MeteredType{}
	for _, r := range rows {
		byLease[r.LeaseID] = append(byLease[r.LeaseID], MeteredType{ChargeTypeID: r.ChargeTypeID, Name: r.ChargeTypeName})
	}
	for i := range leases {
		if m, ok := byLease[leases[i].ID]; ok {
			leases[i].MeteredChargeTypes = m
		} else {
			leases[i].MeteredChargeTypes = []MeteredType{}
		}
	}
	return nil
}

func toChargeType(r dbgen.ChargeType) ChargeType {
	return ChargeType{ID: r.ID, Name: r.Name, Metered: r.Metered, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
