package authz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

var (
	// ErrForbidden means the principal may not perform the action.
	ErrForbidden = errors.New("authz: forbidden")
	// ErrNotMember means the staff user has no active membership in the organization.
	ErrNotMember = errors.New("authz: not a member of this organization")
)

// Service loads principals from the database and records decisions in the audit log.
type Service struct {
	store *store.Store
	clock clock.Clock
	log   *slog.Logger
}

// NewService returns a Service.
func NewService(st *store.Store, c clock.Clock, log *slog.Logger) *Service {
	return &Service{store: st, clock: c, log: log}
}

// LoadPrincipal returns the staff user's principal in the organization, or ErrNotMember.
// A custom role whose stored policy fails validation grants nothing and is logged.
func (s *Service) LoadPrincipal(ctx context.Context, org, user uuid.UUID) (Principal, error) {
	var p Principal
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		m, err := q.GetActiveMembershipForUser(ctx, dbgen.GetActiveMembershipForUserParams{
			OrganizationID: org, StaffUserID: user,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotMember
		}
		if err != nil {
			return fmt.Errorf("loading membership: %w", err)
		}
		rows, err := q.ListRoleAssignmentsForMembership(ctx, dbgen.ListRoleAssignmentsForMembershipParams{
			OrganizationID: org, MembershipID: m.ID,
		})
		if err != nil {
			return fmt.Errorf("loading role assignments: %w", err)
		}
		p = Principal{UserID: user, OrganizationID: org, MembershipID: m.ID}
		for _, row := range rows {
			role, ok := s.roleFromRow(ctx, row)
			if !ok {
				continue
			}
			if row.PropertyID.Valid {
				role.Property = row.PropertyID.UUID
				if !slices.Contains(p.Properties, role.Property) {
					p.Properties = append(p.Properties, role.Property)
				}
			}
			p.Roles = append(p.Roles, role)
		}
		SortRoles(p.Roles, func(r Role) string { return r.Key }, func(r Role) string { return r.Name })
		return nil
	})
	return p, err
}

func (s *Service) roleFromRow(ctx context.Context, row dbgen.ListRoleAssignmentsForMembershipRow) (Role, bool) {
	if row.BuiltinRole != nil {
		r, ok := BuiltinRole(*row.BuiltinRole)
		if !ok {
			s.log.ErrorContext(ctx, "unknown built-in role in assignment", "role", *row.BuiltinRole)
		}
		return r, ok
	}
	if !row.RoleID.Valid || row.RoleName == nil {
		s.log.ErrorContext(ctx, "role assignment without a role", "assignment", row.ID)
		return Role{}, false
	}
	policy, err := ParsePolicy(row.RolePolicy)
	if err != nil {
		s.log.ErrorContext(ctx, "custom role has an invalid policy and grants nothing",
			"role", row.RoleID.UUID, "error", err)
		return Role{}, false
	}
	return Role{Key: "custom:" + row.RoleID.UUID.String(), Name: *row.RoleName, Policy: policy}, true
}

// Authorize checks req inside the caller's transaction tx.
//
// A denial is written to the audit log in its own transaction, so it survives the caller's
// rollback, and Authorize returns an error wrapping ErrForbidden. An allowed sensitive action is
// written in tx, so the record commits only together with the change it describes.
func (s *Service) Authorize(ctx context.Context, tx pgx.Tx, p Principal, req Request) error {
	d := Evaluate(p, req)
	if !d.Allowed {
		s.recordDenial(ctx, p, req, d)
		return fmt.Errorf("%w: %s on %s (%s)", ErrForbidden, req.Action, req.Resource.Path(), d.Reason)
	}
	if IsSensitive(req.Action) {
		if err := s.insert(ctx, tx, p.OrganizationID, uuid.NullUUID{UUID: p.UserID, Valid: true}, req, d); err != nil {
			return err
		}
	}
	return nil
}

// AuthorizeList checks req for a whole collection, such as listing properties, inside tx.
//
// It returns all = true when the principal may act on every record, with the same auditing as
// Authorize. It returns all = false when the principal may act only on some records, for
// example a role limited to one property: the caller then keeps the records for which Allowed
// is true. When no record can be allowed, the denial is recorded and the error wraps
// ErrForbidden.
func (s *Service) AuthorizeList(ctx context.Context, tx pgx.Tx, p Principal, req Request) (all bool, err error) {
	if Evaluate(p, req).Allowed {
		return true, s.Authorize(ctx, tx, p, req)
	}
	if AccessTo(p, req.Action) == AccessConditional {
		return false, nil
	}
	return false, s.Authorize(ctx, tx, p, req)
}

// Allowed reports whether req is allowed, without auditing. Use it only to filter the records
// of a collection that AuthorizeList has already checked.
func Allowed(p Principal, req Request) bool { return Evaluate(p, req).Allowed }

// Check is Authorize for code that has no transaction open yet. Denials are recorded; allowed
// actions are not, so call Authorize again inside the transaction that makes the change.
func (s *Service) Check(ctx context.Context, p Principal, req Request) error {
	d := Evaluate(p, req)
	if !d.Allowed {
		s.recordDenial(ctx, p, req, d)
		return fmt.Errorf("%w: %s on %s (%s)", ErrForbidden, req.Action, req.Resource.Path(), d.Reason)
	}
	return nil
}

// RecordSystemAction writes an audit row for a change made by the platform itself, such as
// creating an organization from the admin command, where there is no acting staff member.
func (s *Service) RecordSystemAction(ctx context.Context, tx pgx.Tx, org uuid.UUID, action string, res Resource) error {
	d := Decision{Allowed: true, Reason: ReasonAllowed, Policy: "system"}
	return s.insert(ctx, tx, org, uuid.NullUUID{}, Request{Action: action, Resource: res}, d)
}

func (s *Service) recordDenial(ctx context.Context, p Principal, req Request, d Decision) {
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		return s.insert(ctx, tx, p.OrganizationID, uuid.NullUUID{UUID: p.UserID, Valid: true}, req, d)
	})
	if err != nil {
		s.log.ErrorContext(ctx, "recording denied permission check", "action", req.Action, "error", err)
	}
}

func (s *Service) insert(ctx context.Context, tx pgx.Tx, org uuid.UUID, actor uuid.NullUUID, req Request, d Decision) error {
	decision := "deny"
	if d.Allowed {
		decision = "allow"
	}
	detail, err := json.Marshal(map[string]string{"policy": d.Policy, "statement": d.Statement})
	if err != nil {
		return fmt.Errorf("encoding audit detail: %w", err)
	}
	err = dbgen.New(tx).InsertAuditLog(ctx, dbgen.InsertAuditLogParams{
		ID:               uuid.New(),
		OrganizationID:   org,
		ActorStaffUserID: actor,
		Action:           req.Action,
		Resource:         req.Resource.Path(),
		Decision:         decision,
		Reason:           string(d.Reason),
		Detail:           detail,
		CreatedAt:        s.clock.Now(),
	})
	if err != nil {
		return fmt.Errorf("writing audit log: %w", err)
	}
	return nil
}
