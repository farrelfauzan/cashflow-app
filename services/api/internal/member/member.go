// Package member manages the staff members of an organization: listing them, adding them by
// phone number, changing their roles and disabling or re-enabling them.
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
	// ErrLastAdmin means the change would leave the organization without an active Admin for
	// the whole organization.
	ErrLastAdmin = errors.New("member: last active admin")
)

// Membership statuses. They match the CHECK constraint on membership.status.
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
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
	// organization. PropertyName is that property's name, for showing the role.
	PropertyID   uuid.UUID
	PropertyName string
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
	// Invitation is nil for a member who was never invited (an organization founder).
	Invitation *Invitation
	// FirstSignInAt is when the member first opened this organization after signing in, or nil.
	// It is per membership, so it says nothing about the person's other organizations.
	FirstSignInAt *time.Time
	// CollectorLeaseCount is the number of active leases in the organization that name the
	// member as collector. A disabled member keeps them until an admin picks another collector.
	CollectorLeaseCount int64
}

// CollectorCandidate is an active member a lease's collector can be picked from.
type CollectorCandidate struct {
	MemberID uuid.UUID
	Name     string
	// Karyawan is true when the member holds the karyawan role anywhere in the organization.
	Karyawan bool
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
	inv   Invitations
}

// NewService returns a Service. inv sends the WhatsApp invitations.
func NewService(st *store.Store, az *authz.Service, inv Invitations) *Service {
	return &Service{store: st, authz: az, inv: inv}
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
				Invitation:    invitationFrom(r.InvitationState, r.InvitationLastSentAt),
				FirstSignInAt: r.FirstSignInAt, CollectorLeaseCount: r.CollectorLeaseCount,
			}
		}
		return nil
	})
	return out, err
}

// OwnRoles returns the principal's own roles in their organization, with property names, for
// GET /orgs/{orgId}/me. Everyone may read their own roles, so it checks no permission.
func (s *Service) OwnRoles(ctx context.Context, p authz.Principal) ([]RoleRef, error) {
	var out []RoleRef
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		var err error
		out, err = membershipRoles(ctx, dbgen.New(tx), p.OrganizationID, p.MembershipID)
		return err
	})
	return out, err
}

// CollectorCandidates lists the active members, karyawan first, for the collector picker on
// the lease form. Any member who may manage some lease may list them, including one whose roles
// are limited to some properties: a narrow exception to decision 15 (decision 27, like decision
// 22 for charge types). It returns names only, no phones or roles. Listing writes no allowed
// audit row; a refusal is audited.
func (s *Service) CollectorCandidates(ctx context.Context, p authz.Principal) ([]CollectorCandidate, error) {
	if authz.AccessTo(p, authz.ActionLeaseManage) == authz.AccessDeny {
		req := authz.Request{Action: authz.ActionLeaseManage, Resource: authz.Resource{Type: "member"}}
		if err := s.authz.Check(ctx, p, req); err != nil {
			return nil, err
		}
	}
	var out []CollectorCandidate
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListCollectorCandidates(ctx, p.OrganizationID)
		if err != nil {
			return fmt.Errorf("listing collector candidates: %w", err)
		}
		out = make([]CollectorCandidate, len(rows))
		for i, r := range rows {
			out[i] = CollectorCandidate{MemberID: r.ID, Name: r.Name, Karyawan: r.Karyawan}
		}
		return nil
	})
	return out, err
}

// RecordSignIn records that the principal opened their organization, the first time only, so
// the members screen can tell an invitee who never signed in. It is the member's own row and
// changes no access, so it needs no permission.
func (s *Service) RecordSignIn(ctx context.Context, p authz.Principal) error {
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		now := s.inv.Clock.Now()
		return dbgen.New(tx).SetMembershipFirstSignIn(ctx, dbgen.SetMembershipFirstSignInParams{
			OrganizationID: p.OrganizationID, ID: p.MembershipID, FirstSignInAt: &now,
		})
	})
	if err != nil {
		return fmt.Errorf("recording first sign-in: %w", err)
	}
	return nil
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
//
// Step 3 marks the invitation not_sent. Once it has committed, the invitation is sent over
// WhatsApp and the outcome recorded (see deliver). A send that fails or is rate limited never
// undoes the invite: the member is returned with the invitation not_sent. A number that is
// already a member fails in step 3 with ErrAlreadyMember, so nothing is sent.
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
	var (
		m      Member
		params map[string]string
	)
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
		if err := setInvitation(ctx, q, p.OrganizationID, membershipID, InvitationNotSent, nil); err != nil {
			return err
		}
		if m, err = load(ctx, q, p.OrganizationID, membershipID); err != nil {
			return err
		}
		params, err = invitationParams(ctx, q, p, m)
		return err
	})
	if err != nil {
		return Member{}, err
	}

	// The invite has committed, so a failed send only shows on the member as not_sent.
	m, err = s.deliver(ctx, p.OrganizationID, m, params)
	if NeedsOperator(err) {
		s.inv.Log.ErrorContext(ctx, "member invitation not sent", "organization_id", p.OrganizationID,
			"membership_id", m.ID, "reason", failureReason(err))
	}
	return m, nil
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
//
// Taking the Admin role for the whole organization away from the last active member who holds
// it fails with ErrLastAdmin; keeping Admin limited to a property does not count. It takes the
// same lock as SetStatus, so a demotion and a disable cannot together remove the last Admin.
func (s *Service) SetRoles(ctx context.Context, p authz.Principal, membershipID uuid.UUID, grants []RoleGrant) (Member, error) {
	grants, err := validGrants(grants)
	if err != nil {
		return Member{}, err
	}
	var m Member
	err = s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.LockOrganizationMembers(ctx, p.OrganizationID); err != nil {
			return fmt.Errorf("locking memberships: %w", err)
		}
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
		if current.Status == StatusActive && holdsOrgAdmin(current) && !grantsOrgAdmin(grants) {
			if err := refuseLastAdmin(ctx, q, p.OrganizationID, membershipID); err != nil {
				return err
			}
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

// SetStatus disables or re-enables a member. It needs the same permissions as removing every
// built-in role the member holds: member:manage on that member, which nobody has on themselves,
// and role:assign on each of those roles, so an Admin cannot disable or re-enable a Supervisor.
//
// Roles are kept, so re-enabling restores the same access. A disabled member's sessions stop
// working in this organization at once, because authz.LoadPrincipal loads active memberships
// only; their other organizations are untouched.
//
// Disabling the last active member who holds the Admin role for the whole organization fails
// with ErrLastAdmin. Setting the status the member already has changes nothing, after the same
// permission check.
func (s *Service) SetStatus(ctx context.Context, p authz.Principal, membershipID uuid.UUID, status string) (Member, error) {
	if status != StatusActive && status != StatusDisabled {
		return Member{}, &InputError{Field: "status", Message: "Status harus active atau disabled."}
	}
	var m Member
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		// Taken before reading anything, so status and role changes in one organization run one
		// after the other and the second sees the first's result. Without it, two admins
		// disabling or demoting each other at once would each still count the other as active.
		if err := q.LockOrganizationMembers(ctx, p.OrganizationID); err != nil {
			return fmt.Errorf("locking memberships: %w", err)
		}
		current, err := load(ctx, q, p.OrganizationID, membershipID)
		if err != nil {
			return err
		}
		for _, req := range statusRequests(current) {
			if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
				return err
			}
		}
		if current.Status == status {
			m = current
			return nil
		}
		if status == StatusDisabled && holdsOrgAdmin(current) {
			if err := refuseLastAdmin(ctx, q, p.OrganizationID, membershipID); err != nil {
				return err
			}
		}
		n, err := q.SetMembershipStatus(ctx, dbgen.SetMembershipStatusParams{
			OrganizationID: p.OrganizationID, ID: membershipID, Status: status,
		})
		if err != nil {
			return fmt.Errorf("setting member status: %w", err)
		}
		if n != 1 {
			return ErrNotFound
		}
		m, err = load(ctx, q, p.OrganizationID, membershipID)
		return err
	})
	return m, err
}

// statusRequests returns member:manage on the member and role:assign on each built-in role
// the member holds, once per role.
func statusRequests(m Member) []authz.Request {
	reqs := []authz.Request{{Action: authz.ActionMemberManage, Resource: authz.Resource{
		Type: "member", ID: m.ID.String(), OwnerID: m.UserID,
	}}}
	var roles []string
	for _, r := range m.Roles {
		if r.Builtin && !slices.Contains(roles, r.Key) {
			roles = append(roles, r.Key)
		}
	}
	for _, r := range roles {
		reqs = append(reqs, roleAssign(r))
	}
	return reqs
}

// refuseLastAdmin returns ErrLastAdmin when no active member other than membershipID holds the
// Admin role for the whole organization. Callers hold LockOrganizationMembers, so the count
// cannot change before they commit.
func refuseLastAdmin(ctx context.Context, q *dbgen.Queries, org, membershipID uuid.UUID) error {
	others, err := q.CountOtherActiveOrgAdmins(ctx, dbgen.CountOtherActiveOrgAdminsParams{
		OrganizationID: org, ExcludedMembershipID: membershipID,
	})
	if err != nil {
		return fmt.Errorf("counting admins: %w", err)
	}
	if others == 0 {
		return ErrLastAdmin
	}
	return nil
}

// grantsOrgAdmin reports whether the grants include the Admin role for the whole organization.
func grantsOrgAdmin(grants []RoleGrant) bool {
	return slices.ContainsFunc(grants, func(g RoleGrant) bool {
		return g.Role == authz.RoleAdmin && g.PropertyID == uuid.Nil
	})
}

// holdsOrgAdmin reports whether the member holds the Admin role for the whole organization.
func holdsOrgAdmin(m Member) bool {
	return slices.ContainsFunc(m.Roles, func(r RoleRef) bool {
		return r.Builtin && r.Key == authz.RoleAdmin && r.PropertyID == uuid.Nil
	})
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
	roles, err := membershipRoles(ctx, q, org, membershipID)
	if err != nil {
		return Member{}, err
	}
	return Member{
		ID: r.ID, UserID: r.StaffUserID, Name: r.Name, Phone: r.Phone, Status: r.Status, CreatedAt: r.CreatedAt,
		Invitation: invitationFrom(r.InvitationState, r.InvitationLastSentAt), FirstSignInAt: r.FirstSignInAt,
		CollectorLeaseCount: r.CollectorLeaseCount, Roles: roles,
	}, nil
}

// membershipRoles reads one membership's roles, sorted for display.
func membershipRoles(ctx context.Context, q *dbgen.Queries, org, membershipID uuid.UUID) ([]RoleRef, error) {
	rows, err := q.ListRoleAssignmentsForMembership(ctx, dbgen.ListRoleAssignmentsForMembershipParams{
		OrganizationID: org, MembershipID: membershipID,
	})
	if err != nil {
		return nil, fmt.Errorf("loading member roles: %w", err)
	}
	var roles []RoleRef
	for _, row := range rows {
		roles = append(roles, roleRef(row.BuiltinRole, row.RoleID, row.RoleName, row.PropertyID, row.PropertyName))
	}
	sortRoles(roles)
	return roles, nil
}

func rolesByMembership(ctx context.Context, q *dbgen.Queries, org uuid.UUID) (map[uuid.UUID][]RoleRef, error) {
	rows, err := q.ListOrgRoleAssignments(ctx, org)
	if err != nil {
		return nil, fmt.Errorf("listing role assignments: %w", err)
	}
	out := map[uuid.UUID][]RoleRef{}
	for _, r := range rows {
		out[r.MembershipID] = append(out[r.MembershipID], roleRef(r.BuiltinRole, r.RoleID, r.RoleName, r.PropertyID, r.PropertyName))
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

func roleRef(builtin *string, roleID uuid.NullUUID, roleName *string, propertyID uuid.NullUUID, propertyName *string) RoleRef {
	ref := RoleRef{PropertyID: propertyID.UUID}
	if propertyName != nil {
		ref.PropertyName = *propertyName
	}
	switch {
	case builtin != nil:
		ref.Key, ref.Name, ref.Builtin = *builtin, *builtin, true
		if r, ok := authz.BuiltinRole(*builtin); ok {
			ref.Key, ref.Name = r.Key, r.Name
		}
	default:
		ref.Key = "custom:" + roleID.UUID.String()
		if roleName != nil {
			ref.Name = *roleName
		}
	}
	return ref
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
