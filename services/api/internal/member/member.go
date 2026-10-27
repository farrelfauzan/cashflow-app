// Package member manages the staff members of an organization: listing them, adding them by
// phone number and changing their roles.
package member

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
)

const maxNameLength = 100

var (
	// ErrNotFound means there is no such member in this organization.
	ErrNotFound = errors.New("member: not found")
	// ErrAlreadyMember means the phone number already belongs to a member.
	ErrAlreadyMember = errors.New("member: already a member")
)

// InputError names the field that is wrong. Message is safe to show to the user.
type InputError struct {
	Field   string
	Message string
}

func (e *InputError) Error() string { return fmt.Sprintf("member: %s: %s", e.Field, e.Message) }

// RoleRef names a role held by a member.
type RoleRef struct {
	Key     string
	Name    string
	Builtin bool
	// PropertyID is the property the role is limited to, or uuid.Nil for the whole
	// organization.
	PropertyID uuid.UUID
}

// RoleGrant is a built-in role to hold, optionally limited to one property.
type RoleGrant struct {
	Role string
	// PropertyID limits the role to one property; uuid.Nil means the whole organization.
	PropertyID uuid.UUID
}

// Member is a staff member of one organization.
type Member struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	Phone     string
	Status    string
	Roles     []RoleRef
	CreatedAt time.Time
}

// Invite is the input for adding a member.
type Invite struct {
	Phone string
	Name  string
	// Roles are the member's built-in roles, each optionally limited to one property.
	Roles []RoleGrant
}

// Service manages members.
type Service struct {
	store *store.Store
	authz *authz.Service
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service) *Service {
	return &Service{store: st, authz: az}
}

// List returns every member of the principal's organization.
func (s *Service) List(ctx context.Context, p authz.Principal) ([]Member, error) {
	var out []Member
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		req := authz.Request{Action: authz.ActionMemberRead, Resource: authz.Resource{Type: "member"}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		q := dbgen.New(tx)
		rows, err := q.ListMembers(ctx, p.OrganizationID)
		if err != nil {
			return fmt.Errorf("listing members: %w", err)
		}
		roles, err := rolesByMembership(ctx, q, p.OrganizationID)
		if err != nil {
			return err
		}
		out = make([]Member, len(rows))
		for i, r := range rows {
			out[i] = Member{
				ID: r.ID, UserID: r.StaffUserID, Name: r.Name, Phone: r.Phone,
				Status: r.Status, CreatedAt: r.CreatedAt, Roles: roles[r.ID],
			}
		}
		return nil
	})
	return out, err
}

// Invite adds a staff member by phone number with built-in roles, each optionally limited to
// one property. It needs member:invite and role:assign on every role granted, so an Admin can
// add Admins and Karyawan but not a Supervisor. A property limit narrows the stored grant only;
// it is not part of the check, and a principal whose roles are all limited to properties
// cannot invite anyone.
//
// The staff user lives outside any organization and can only be written in the auth scope, so
// an invite takes three steps:
//  1. Validate the input, check every permission and every property limit. A denial or an
//     invalid grant stops here and writes nothing except the audited denial.
//  2. Find or create the staff user in the auth scope.
//  3. In one organization transaction, authorize again (auditing the allowed actions), create
//     the membership and every role assignment. The member gets every grant or no membership,
//     and never holds a broader role than the one asked for.
//
// A failure in step 3 (such as ErrAlreadyMember or a property removed in between) can leave a
// staff user from step 2 with no membership. That row grants nothing: access comes only from
// memberships and their roles.
func (s *Service) Invite(ctx context.Context, p authz.Principal, in Invite) (Member, error) {
	number, err := phone.Normalize(in.Phone)
	if err != nil {
		return Member{}, &InputError{Field: "phone", Message: "Nomor WhatsApp tidak valid."}
	}
	name, err := validName(in.Name)
	if err != nil {
		return Member{}, err
	}
	grants, err := validGrants(in.Roles)
	if err != nil {
		return Member{}, err
	}
	reqs := inviteRequests(grants)

	// Step 1. Check records denials only; the allowed actions are audited in step 3.
	for _, req := range reqs {
		if err := s.authz.Check(ctx, p, req); err != nil {
			return Member{}, err
		}
	}
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		return checkProperties(ctx, dbgen.New(tx), p.OrganizationID, grants)
	})
	if err != nil {
		return Member{}, err
	}

	// Step 2.
	var user dbgen.StaffUser
	err = s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		user, err = dbgen.New(tx).UpsertStaffUser(ctx, dbgen.UpsertStaffUserParams{ID: uuid.New(), Phone: number, Name: name})
		return err
	})
	if err != nil {
		return Member{}, fmt.Errorf("finding or creating staff user: %w", err)
	}

	// Step 3.
	var m Member
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		for _, req := range reqs {
			if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
				return err
			}
		}
		q := dbgen.New(tx)
		if err := checkProperties(ctx, q, p.OrganizationID, grants); err != nil {
			return err
		}
		membershipID, err := Add(ctx, q, p.OrganizationID, user.ID, grants)
		if err != nil {
			return err
		}
		m, err = load(ctx, q, p.OrganizationID, membershipID)
		return err
	})
	return m, err
}

// inviteRequests returns member:invite and role:assign on each granted role, once per role
// even when it is limited to several properties. Like SetRoles, the property limit is not in
// the request.
func inviteRequests(grants []RoleGrant) []authz.Request {
	reqs := []authz.Request{{Action: authz.ActionMemberInvite, Resource: authz.Resource{Type: "member"}}}
	var roles []string
	for _, g := range grants {
		if !slices.Contains(roles, g.Role) {
			roles = append(roles, g.Role)
		}
	}
	for _, r := range roles {
		reqs = append(reqs, roleAssign(r))
	}
	return reqs
}

func roleAssign(role string) authz.Request {
	return authz.Request{Action: authz.ActionRoleAssign, Resource: authz.Resource{Type: "role", ID: role}}
}

// SetRoles replaces a member's built-in roles. It needs member:manage on that member, which
// nobody has on themselves, and role:assign on every role it adds or removes, including a role
// whose property limit changes.
func (s *Service) SetRoles(ctx context.Context, p authz.Principal, membershipID uuid.UUID, grants []RoleGrant) (Member, error) {
	grants, err := validGrants(grants)
	if err != nil {
		return Member{}, err
	}
	var m Member
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := load(ctx, q, p.OrganizationID, membershipID)
		if err != nil {
			return err
		}
		manage := authz.Request{Action: authz.ActionMemberManage, Resource: authz.Resource{
			Type: "member", ID: membershipID.String(), OwnerID: current.UserID,
		}}
		if err := s.authz.Authorize(ctx, tx, p, manage); err != nil {
			return err
		}
		for _, changed := range changedBuiltinRoles(current.Roles, grants) {
			if err := s.authz.Authorize(ctx, tx, p, roleAssign(changed)); err != nil {
				return err
			}
		}
		if err := checkProperties(ctx, q, p.OrganizationID, grants); err != nil {
			return err
		}

		if err := q.DeleteBuiltinRoleAssignments(ctx, dbgen.DeleteBuiltinRoleAssignmentsParams{
			OrganizationID: p.OrganizationID, MembershipID: membershipID,
		}); err != nil {
			return fmt.Errorf("removing roles: %w", err)
		}
		if err := assignBuiltinRoles(ctx, q, p.OrganizationID, membershipID, grants); err != nil {
			return err
		}
		m, err = load(ctx, q, p.OrganizationID, membershipID)
		return err
	})
	return m, err
}

// checkProperties makes sure every property limit names a property of this organization.
func checkProperties(ctx context.Context, q *dbgen.Queries, org uuid.UUID, grants []RoleGrant) error {
	for _, g := range grants {
		if g.PropertyID == uuid.Nil {
			continue
		}
		ok, err := property.Exists(ctx, q, org, g.PropertyID)
		if err != nil {
			return err
		}
		if !ok {
			return &InputError{Field: "roles", Message: "Properti tidak ditemukan."}
		}
	}
	return nil
}

// Add creates a membership with built-in roles inside an organization transaction, so the
// membership and every role assignment commit together. It is shared with organization setup.
// The caller has already validated the grants (validGrants), checked their properties and
// authorized the change.
func Add(ctx context.Context, q *dbgen.Queries, org, userID uuid.UUID, grants []RoleGrant) (uuid.UUID, error) {
	m, err := q.CreateMembership(ctx, dbgen.CreateMembershipParams{ID: uuid.New(), OrganizationID: org, StaffUserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrAlreadyMember
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating membership: %w", err)
	}
	if err := assignBuiltinRoles(ctx, q, org, m.ID, grants); err != nil {
		return uuid.Nil, err
	}
	return m.ID, nil
}

func assignBuiltinRoles(ctx context.Context, q *dbgen.Queries, org, membershipID uuid.UUID, grants []RoleGrant) error {
	for _, g := range grants {
		err := q.CreateBuiltinRoleAssignment(ctx, dbgen.CreateBuiltinRoleAssignmentParams{
			ID: uuid.New(), OrganizationID: org, MembershipID: membershipID, BuiltinRole: &g.Role,
			PropertyID: uuid.NullUUID{UUID: g.PropertyID, Valid: g.PropertyID != uuid.Nil},
		})
		if err != nil {
			return fmt.Errorf("assigning role %s: %w", g.Role, err)
		}
	}
	return nil
}

// changedBuiltinRoles returns the built-in roles that are added or removed, or whose property
// limits change, each once.
func changedBuiltinRoles(current []RoleRef, next []RoleGrant) []string {
	var had []RoleGrant
	for _, r := range current {
		if r.Builtin {
			had = append(had, RoleGrant{Role: r.Key, PropertyID: r.PropertyID})
		}
	}
	var changed []string
	for _, k := range authz.BuiltinRoleKeys() {
		if !slices.Equal(grantsFor(had, k), grantsFor(next, k)) {
			changed = append(changed, k)
		}
	}
	return changed
}

// grantsFor returns the property limits of one role, sorted.
func grantsFor(grants []RoleGrant, role string) []uuid.UUID {
	var out []uuid.UUID
	for _, g := range grants {
		if g.Role == role {
			out = append(out, g.PropertyID)
		}
	}
	slices.SortFunc(out, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	return out
}

func load(ctx context.Context, q *dbgen.Queries, org, membershipID uuid.UUID) (Member, error) {
	r, err := q.GetMembership(ctx, dbgen.GetMembershipParams{OrganizationID: org, ID: membershipID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, ErrNotFound
	}
	if err != nil {
		return Member{}, fmt.Errorf("loading member: %w", err)
	}
	rows, err := q.ListRoleAssignmentsForMembership(ctx, dbgen.ListRoleAssignmentsForMembershipParams{
		OrganizationID: org, MembershipID: membershipID,
	})
	if err != nil {
		return Member{}, fmt.Errorf("loading member roles: %w", err)
	}
	m := Member{ID: r.ID, UserID: r.StaffUserID, Name: r.Name, Phone: r.Phone, Status: r.Status, CreatedAt: r.CreatedAt}
	for _, row := range rows {
		m.Roles = append(m.Roles, roleRef(row.BuiltinRole, row.RoleID, row.RoleName, row.PropertyID))
	}
	sortRoles(m.Roles)
	return m, nil
}

func rolesByMembership(ctx context.Context, q *dbgen.Queries, org uuid.UUID) (map[uuid.UUID][]RoleRef, error) {
	rows, err := q.ListOrgRoleAssignments(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("listing role assignments: %w", err)
	}
	out := map[uuid.UUID][]RoleRef{}
	for _, r := range rows {
		out[r.MembershipID] = append(out[r.MembershipID], roleRef(r.BuiltinRole, r.RoleID, r.RoleName, r.PropertyID))
	}
	for _, roles := range out {
		sortRoles(roles)
	}
	return out, nil
}

func sortRoles(roles []RoleRef) {
	// The property id only breaks ties, so the same role limited to several properties keeps a
	// stable order.
	authz.SortRoles(roles, func(r RoleRef) string { return r.Key },
		func(r RoleRef) string { return r.Name + "/" + r.PropertyID.String() })
}

func roleRef(builtin *string, roleID uuid.NullUUID, roleName *string, propertyID uuid.NullUUID) RoleRef {
	if builtin != nil {
		if r, ok := authz.BuiltinRole(*builtin); ok {
			return RoleRef{Key: r.Key, Name: r.Name, Builtin: true, PropertyID: propertyID.UUID}
		}
		return RoleRef{Key: *builtin, Name: *builtin, Builtin: true, PropertyID: propertyID.UUID}
	}
	name := ""
	if roleName != nil {
		name = *roleName
	}
	return RoleRef{Key: "custom:" + roleID.UUID.String(), Name: name, PropertyID: propertyID.UUID}
}

func validName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", &InputError{Field: "name", Message: "Nama wajib diisi."}
	}
	if len([]rune(name)) > maxNameLength {
		return "", &InputError{Field: "name", Message: fmt.Sprintf("Nama paling panjang %d huruf.", maxNameLength)}
	}
	return name, nil
}

// validGrants checks the roles and returns each (role, property) pair once, in display order.
// A role held for the whole organization cannot also be limited to a property.
func validGrants(grants []RoleGrant) ([]RoleGrant, error) {
	keys := make([]string, len(grants))
	for i, g := range grants {
		keys[i] = g.Role
	}
	roles, err := validRoles(keys)
	if err != nil {
		return nil, err
	}
	var out []RoleGrant
	for _, role := range roles {
		limits := grantsFor(grants, role)
		limits = slices.Compact(limits)
		if len(limits) > 1 && limits[0] == uuid.Nil {
			// uuid.Nil sorts first: the role is given both org-wide and for a property.
			r, _ := authz.BuiltinRole(role)
			return nil, &InputError{Field: "roles", Message: fmt.Sprintf("Peran %s sudah berlaku untuk semua properti.", r.Name)}
		}
		for _, id := range limits {
			out = append(out, RoleGrant{Role: role, PropertyID: id})
		}
	}
	return out, nil
}

func validRoles(keys []string) ([]string, error) {
	roles, err := authz.NormalizeBuiltinRoles(keys)
	if err != nil {
		return nil, &InputError{Field: "roles", Message: "Peran tidak dikenal."}
	}
	if len(roles) == 0 {
		return nil, &InputError{Field: "roles", Message: "Pilih minimal satu peran."}
	}
	return roles, nil
}
