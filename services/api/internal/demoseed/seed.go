// Package demoseed fills a development database with three demo organizations: their staff,
// properties, units, tenants and leases. cmd/admin runs it as "seed-demo", and only in
// development.
//
// Every write goes through the domain services, acting as each organization's org-wide Admin
// (the organization itself is created by org.Service.Create, like the admin command does), so
// the same validation, permission checks and audit rows apply as in the app.
//
// It is safe to run again. Each record is matched before it is created: an organization by name
// among its Admin's own memberships, members by phone, properties by name, units by label in
// their property, charge types by name ignoring case, tenants by phone and leases by an active
// lease of that tenant on the unit. A record that already exists is left exactly as it is, even
// when its values differ; fixed extras are set only on a lease that has none, and a member the
// demo marks disabled is disabled only by the run that invites them.
package demoseed

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Deps are the services the seed writes through.
type Deps struct {
	Store      *store.Store
	Authz      *authz.Service
	Orgs       *org.Service
	Members    *member.Service
	Properties *property.Service
	Tenants    *tenant.Service
}

// Login is one staff member who can sign in to one organization.
type Login struct {
	Phone        string // E.164
	Name         string
	Organization string
	Roles        []string // role names, in display order
	// PropertyLimits lists "Role: Property" for each role limited to a property.
	PropertyLimits []string
	// Disabled members cannot sign in; they are listed so the demo's people are all there.
	Disabled bool
}

// Result is what the seed leaves in the database.
type Result struct {
	// Logins has every member of the demo organizations, organization by organization.
	Logins []Login
	// Notes says what was skipped because existing data differs from the demo contents.
	Notes []string
}

// Run seeds the demo organizations. c decides "today" for move-out rules and the booking that
// starts on the first day of next month, both in Asia/Jakarta.
func Run(ctx context.Context, d Deps, c clock.Clock) (Result, error) {
	var res Result
	for _, o := range demoOrgs(c) {
		s := &seeder{d: d, spec: o}
		if err := s.run(ctx); err != nil {
			return Result{}, fmt.Errorf("seeding %s: %w", o.Name, err)
		}
		logins, err := s.logins(ctx)
		if err != nil {
			return Result{}, fmt.Errorf("listing logins of %s: %w", o.Name, err)
		}
		res.Logins = append(res.Logins, logins...)
		res.Notes = append(res.Notes, s.notes...)
	}
	return res, nil
}

// seeder seeds one organization.
type seeder struct {
	d     Deps
	spec  orgSpec
	p     authz.Principal                 // the org-wide Admin who makes the changes
	props map[string]uuid.UUID            // property name -> id
	types map[string]uuid.UUID            // lower-case charge type name -> id
	units map[string]map[string]uuid.UUID // property name -> unit label -> id
	staff map[string]uuid.UUID            // E.164 phone -> membership id
	// disabled has the E.164 phones of members who were disabled before this run.
	disabled map[string]bool
	// toDisable are the members this run invited that the demo marks disabled.
	toDisable []uuid.UUID
	notes     []string
}

func (s *seeder) note(format string, args ...any) {
	s.notes = append(s.notes, s.spec.Name+": "+fmt.Sprintf(format, args...))
}

func (s *seeder) run(ctx context.Context) error {
	if err := s.ensureOrganization(ctx); err != nil {
		return err
	}
	if err := s.ensureProperties(ctx); err != nil {
		return err
	}
	if err := s.ensureMembers(ctx); err != nil {
		return err
	}
	if err := s.ensureChargeTypes(ctx); err != nil {
		return err
	}
	for _, t := range s.spec.Tenants {
		if err := s.ensureTenant(ctx, t); err != nil {
			return fmt.Errorf("tenant %s: %w", t.Name, err)
		}
	}
	// Last, so their leases could still name them as collector.
	for _, id := range s.toDisable {
		if _, err := s.d.Members.SetStatus(ctx, s.p, id, member.StatusDisabled); err != nil {
			return fmt.Errorf("disabling a member: %w", err)
		}
	}
	return nil
}

// ensureOrganization finds the organization among its Admin's own memberships, or creates it
// with its founders, and loads the Admin's principal.
func (s *seeder) ensureOrganization(ctx context.Context) error {
	m, err := s.d.Orgs.FindForMember(ctx, s.spec.Actor, s.spec.Name)
	if errors.Is(err, org.ErrNotFound) {
		founders := make([]org.NewMember, len(s.spec.Founders))
		for i, f := range s.spec.Founders {
			founders[i] = org.NewMember{Phone: f.Phone, Name: f.Name, Roles: roleKeys(f.Grants)}
		}
		if _, err := s.d.Orgs.Create(ctx, org.New{Name: s.spec.Name, DisplayName: s.spec.Name, Members: founders}); err != nil {
			return fmt.Errorf("creating organization: %w", err)
		}
		m, err = s.d.Orgs.FindForMember(ctx, s.spec.Actor, s.spec.Name)
	}
	if err != nil {
		return fmt.Errorf("finding organization: %w", err)
	}
	s.p, err = s.d.Authz.LoadPrincipal(ctx, m.OrganizationID, m.UserID)
	if err != nil {
		return fmt.Errorf("loading the acting admin: %w", err)
	}
	return nil
}

// ensureProperties finds or creates every property and unit in one transaction.
func (s *seeder) ensureProperties(ctx context.Context) error {
	s.props = map[string]uuid.UUID{}
	s.units = map[string]map[string]uuid.UUID{}
	return s.d.Store.WithOrgTx(ctx, s.p.OrganizationID, func(tx pgx.Tx) error {
		for _, action := range []string{authz.ActionPropertyRead, authz.ActionUnitRead} {
			req := authz.Request{Action: action, Resource: authz.Resource{Type: strings.TrimSuffix(action, ":read")}}
			if err := s.d.Authz.Authorize(ctx, tx, s.p, req); err != nil {
				return err
			}
		}
		for _, ps := range s.spec.Properties {
			prop, err := property.FindByName(ctx, tx, s.p.OrganizationID, ps.Name)
			if errors.Is(err, property.ErrNotFound) {
				prop, err = s.d.Properties.CreateTx(ctx, tx, s.p, property.PropertyInput{
					Name: ps.Name, Kind: ps.Kind, Address: ps.Address,
				})
			}
			if err != nil {
				return fmt.Errorf("property %s: %w", ps.Name, err)
			}
			s.props[ps.Name] = prop.ID
			s.units[ps.Name] = map[string]uuid.UUID{}
			for _, label := range ps.Units {
				u, err := property.FindUnit(ctx, tx, s.p.OrganizationID, prop.ID, label)
				if errors.Is(err, property.ErrNotFound) {
					rent := ps.Rent
					u, err = s.d.Properties.CreateUnitTx(ctx, tx, s.p, prop.ID, property.UnitInput{Label: label, DefaultRent: &rent})
				}
				if err != nil {
					return fmt.Errorf("unit %s in %s: %w", label, ps.Name, err)
				}
				s.units[ps.Name][label] = u.ID
			}
		}
		return nil
	})
}

// ensureChargeTypes finds every charge type by name, ignoring case, and adds the missing ones.
func (s *seeder) ensureChargeTypes(ctx context.Context) error {
	existing, err := s.d.Tenants.ListChargeTypes(ctx, s.p)
	if err != nil {
		return err
	}
	s.types = map[string]uuid.UUID{}
	for _, ct := range existing {
		s.types[strings.ToLower(ct.Name)] = ct.ID
	}
	for _, spec := range s.spec.ChargeTypes {
		if _, ok := s.types[strings.ToLower(spec.Name)]; ok {
			continue
		}
		ct, err := s.d.Tenants.CreateChargeType(ctx, s.p, tenant.ChargeTypeInput{Name: spec.Name, Metered: spec.Metered})
		if err != nil {
			return fmt.Errorf("charge type %s: %w", spec.Name, err)
		}
		s.types[strings.ToLower(spec.Name)] = ct.ID
	}
	return nil
}

// ensureCharges sets the spec's fixed extras on a running lease that has none yet. A lease
// whose extras were set or cleared since keeps them.
func (s *seeder) ensureCharges(ctx context.Context, tenantID, leaseID uuid.UUID, ls leaseSpec) error {
	if len(ls.Charges) == 0 || leaseID == uuid.Nil {
		return nil
	}
	tn, err := s.d.Tenants.Get(ctx, s.p, tenantID)
	if err != nil {
		return err
	}
	for _, l := range tn.Leases {
		if l.ID == leaseID && len(l.Charges) > 0 {
			return nil
		}
	}
	in := make([]tenant.ChargeInput, len(ls.Charges))
	for i, c := range ls.Charges {
		id, ok := s.types[strings.ToLower(c.Type)]
		if !ok {
			return fmt.Errorf("unknown charge type %q", c.Type)
		}
		in[i] = tenant.ChargeInput{ChargeTypeID: id, Amount: c.Amount}
	}
	if _, err := s.d.Tenants.SetLeaseCharges(ctx, s.p, leaseID, in); err != nil {
		return fmt.Errorf("extras on %s %s: %w", ls.Property, ls.Unit, err)
	}
	return nil
}

// ensureMembers invites every staff member who is missing, with their roles and property
// limits in one call, so a member is created with exactly their roles or not at all. An existing
// member keeps their roles.
func (s *seeder) ensureMembers(ctx context.Context) error {
	existing, err := s.allMembers(ctx)
	if err != nil {
		return err
	}
	s.staff = map[string]uuid.UUID{}
	s.disabled = map[string]bool{}
	for _, m := range existing {
		s.staff[m.Phone] = m.ID
		s.disabled[m.Phone] = m.Status == member.StatusDisabled
	}
	for _, sp := range slices.Concat(s.spec.Founders, s.spec.Invited) {
		number, err := phone.Normalize(sp.Phone)
		if err != nil {
			return fmt.Errorf("member %s: %w", sp.Name, err)
		}
		if _, ok := s.staff[number]; ok {
			continue
		}
		grants := make([]member.RoleGrant, len(sp.Grants))
		for i, g := range sp.Grants {
			grants[i] = member.RoleGrant{Role: g.Role}
			if g.Property != "" {
				id, ok := s.props[g.Property]
				if !ok {
					return fmt.Errorf("member %s: unknown property %q", sp.Name, g.Property)
				}
				grants[i].PropertyID = id
			}
		}
		m, err := s.d.Members.Invite(ctx, s.p, member.Invite{Phone: sp.Phone, Name: sp.Name, Roles: grants})
		if err != nil {
			return fmt.Errorf("inviting %s: %w", sp.Name, err)
		}
		s.staff[number] = m.ID
		if sp.Disabled {
			s.toDisable = append(s.toDisable, m.ID)
		}
	}
	return nil
}

// ensureTenant finds or creates the tenant and their running leases in one transaction, then
// records any move-out, which the tenant service does in a transaction of its own.
func (s *seeder) ensureTenant(ctx context.Context, ts tenantSpec) error {
	var tenantID uuid.UUID
	running := make([]uuid.UUID, len(ts.Leases)) // lease ids by spec index; uuid.Nil when skipped
	err := s.d.Store.WithOrgTx(ctx, s.p.OrganizationID, func(tx pgx.Tx) error {
		req := authz.Request{Action: authz.ActionTenantRead, Resource: authz.Resource{Type: "tenant"}}
		if err := s.d.Authz.Authorize(ctx, tx, s.p, req); err != nil {
			return err
		}
		number, err := phone.Normalize(ts.Phone)
		if err != nil {
			return err
		}
		tn, err := tenant.FindByPhone(ctx, tx, s.p.OrganizationID, number)
		if errors.Is(err, tenant.ErrNotFound) {
			tn, err = s.d.Tenants.CreateTx(ctx, tx, s.p, tenant.Input{Name: ts.Name, Phone: ts.Phone})
		}
		if err != nil {
			return err
		}
		tenantID = tn.ID
		for i, ls := range ts.Leases {
			if !ls.MovedOut.IsZero() {
				continue
			}
			if running[i], err = s.ensureActiveLease(ctx, tx, tenantID, ls); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i, ls := range ts.Leases {
		if err := s.ensureCharges(ctx, tenantID, running[i], ls); err != nil {
			return err
		}
	}
	for _, ls := range ts.Leases {
		if ls.MovedOut.IsZero() {
			continue
		}
		if err := s.ensureEndedLease(ctx, tenantID, ls); err != nil {
			return err
		}
	}
	return nil
}

// ensureActiveLease returns the tenant's active lease on the unit, creating it when the unit
// is free. When another tenant holds the unit it notes that and returns uuid.Nil.
func (s *seeder) ensureActiveLease(ctx context.Context, tx pgx.Tx, tenantID uuid.UUID, ls leaseSpec) (uuid.UUID, error) {
	unitID, ok := s.units[ls.Property][ls.Unit]
	if !ok {
		return uuid.Nil, fmt.Errorf("unknown unit %s in %s", ls.Unit, ls.Property)
	}
	current, err := tenant.ActiveLeaseForUnit(ctx, tx, s.p.OrganizationID, unitID)
	switch {
	case err == nil && current.TenantID == tenantID:
		return current.ID, nil
	case err == nil:
		s.note("%s %s already has another tenant; lease skipped", ls.Property, ls.Unit)
		return uuid.Nil, nil
	case !errors.Is(err, tenant.ErrNotFound):
		return uuid.Nil, err
	}
	terms := tenant.Terms{StartDate: ls.Start, RentAmount: ls.Rent, DepositAmount: ls.Deposit}
	if ls.Collector != "" {
		number, err := phone.Normalize(ls.Collector)
		if err != nil {
			return uuid.Nil, err
		}
		id, ok := s.staff[number]
		switch {
		case !ok:
			return uuid.Nil, fmt.Errorf("collector %s is not a member", phone.Mask(number))
		case s.disabled[number]:
			// Only an active member can be made collector of a new lease.
			s.note("%s %s: collector %s is disabled; lease created without a collector", ls.Property, ls.Unit, phone.Mask(number))
		default:
			terms.CollectorMemberID = id
		}
	}
	l, err := s.d.Tenants.CreateLeaseTx(ctx, tx, s.p, tenant.NewLease{TenantID: tenantID, UnitID: unitID, Terms: terms})
	if err != nil {
		return uuid.Nil, fmt.Errorf("lease on %s %s: %w", ls.Property, ls.Unit, err)
	}
	return l.ID, nil
}

// ensureEndedLease leaves the tenant with an ended lease on the unit. It is skipped when they
// already have one; otherwise it reuses or creates their active lease there and moves it out.
func (s *seeder) ensureEndedLease(ctx context.Context, tenantID uuid.UUID, ls leaseSpec) error {
	unitID, ok := s.units[ls.Property][ls.Unit]
	if !ok {
		return fmt.Errorf("unknown unit %s in %s", ls.Unit, ls.Property)
	}
	tn, err := s.d.Tenants.Get(ctx, s.p, tenantID)
	if err != nil {
		return err
	}
	for _, l := range tn.Leases {
		if l.UnitID == unitID && l.Status == tenant.StatusEnded {
			return nil
		}
	}
	var leaseID uuid.UUID
	err = s.d.Store.WithOrgTx(ctx, s.p.OrganizationID, func(tx pgx.Tx) error {
		leaseID, err = s.ensureActiveLease(ctx, tx, tenantID, ls)
		return err
	})
	if err != nil || leaseID == uuid.Nil {
		return err
	}
	if _, err := s.d.Tenants.MoveOut(ctx, s.p, leaseID, ls.MovedOut); err != nil {
		return fmt.Errorf("moving out of %s %s: %w", ls.Property, ls.Unit, err)
	}
	return nil
}

// allMembers is every member of the organization, by name.
func (s *seeder) allMembers(ctx context.Context) ([]member.Member, error) {
	return listing.All(func(pg listing.Page) (listing.Result[member.Member], error) {
		return s.d.Members.List(ctx, s.p, member.ListQuery{Page: pg})
	})
}

// logins lists the organization's members as they are now, with role and property names.
func (s *seeder) logins(ctx context.Context) ([]Login, error) {
	props, err := listing.All(func(pg listing.Page) (listing.Result[property.Property], error) {
		return s.d.Properties.List(ctx, s.p, property.ListQuery{Page: pg})
	})
	if err != nil {
		return nil, err
	}
	propName := map[uuid.UUID]string{}
	for _, p := range props {
		propName[p.ID] = p.Name
	}
	members, err := s.allMembers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Login, 0, len(members))
	for _, m := range members {
		l := Login{Phone: m.Phone, Name: m.Name, Organization: s.spec.Name, Disabled: m.Status == member.StatusDisabled}
		for _, r := range m.Roles {
			if !slices.Contains(l.Roles, r.Name) {
				l.Roles = append(l.Roles, r.Name)
			}
			if r.PropertyID != uuid.Nil {
				l.PropertyLimits = append(l.PropertyLimits, r.Name+": "+propName[r.PropertyID])
			}
		}
		out = append(out, l)
	}
	slices.SortFunc(out, func(a, b Login) int { return strings.Compare(a.Phone, b.Phone) })
	return out, nil
}

func roleKeys(grants []grantSpec) []string {
	var out []string
	for _, g := range grants {
		if !slices.Contains(out, g.Role) {
			out = append(out, g.Role)
		}
	}
	return out
}
