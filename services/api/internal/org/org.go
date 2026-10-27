// Package org creates organizations. Until the onboarding screens exist, the platform operator
// creates each organization and its first members with cmd/admin.
package org

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

// NewMember is one of the first members of a new organization.
type NewMember struct {
	Phone string
	Name  string
	Roles []string
}

// New is the input for creating an organization.
type New struct {
	Name        string
	DisplayName string // used in WhatsApp templates
	Members     []NewMember
}

// Service creates organizations.
type Service struct {
	store *store.Store
	authz *authz.Service
}

// NewService returns a Service.
func NewService(st *store.Store, az *authz.Service) *Service {
	return &Service{store: st, authz: az}
}

// Create makes the organization and its first members in one transaction. At least one member
// must be an Admin, so someone can add the rest of the team.
func (s *Service) Create(ctx context.Context, in New) (uuid.UUID, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.Name == "" || in.DisplayName == "" {
		return uuid.Nil, errors.New("organization name and display name are required")
	}
	members, err := normalizeMembers(in.Members)
	if err != nil {
		return uuid.Nil, err
	}

	// Staff users live outside any organization, so they are created first in the auth scope.
	userIDs := make([]uuid.UUID, len(members))
	err = s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		for i, m := range members {
			u, err := q.UpsertStaffUser(ctx, dbgen.UpsertStaffUserParams{ID: uuid.New(), Phone: m.Phone, Name: m.Name})
			if err != nil {
				return fmt.Errorf("creating staff user %s: %w", phone.Mask(m.Phone), err)
			}
			userIDs[i] = u.ID
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, err
	}

	orgID := uuid.New()
	err = s.store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.CreateOrganization(ctx, dbgen.CreateOrganizationParams{
			ID: orgID, Name: in.Name, DisplayName: in.DisplayName,
		}); err != nil {
			return fmt.Errorf("creating organization: %w", err)
		}
		for i, m := range members {
			if _, err := member.Add(ctx, q, orgID, userIDs[i], orgWide(m.Roles)); err != nil {
				return err
			}
		}
		return s.authz.RecordSystemAction(ctx, tx, orgID, "organization:create",
			authz.Resource{Type: "organization", ID: orgID.String()})
	})
	if err != nil {
		return uuid.Nil, err
	}
	return orgID, nil
}

// orgWide grants each role for the whole organization: the first members are not limited to a
// property.
func orgWide(roles []string) []member.RoleGrant {
	out := make([]member.RoleGrant, len(roles))
	for i, r := range roles {
		out[i] = member.RoleGrant{Role: r}
	}
	return out
}

func normalizeMembers(in []NewMember) ([]NewMember, error) {
	if len(in) == 0 {
		return nil, errors.New("at least one member is required")
	}
	out := make([]NewMember, len(in))
	seen := map[string]bool{}
	hasAdmin := false
	for i, m := range in {
		number, err := phone.Normalize(m.Phone)
		if err != nil {
			return nil, fmt.Errorf("member %d: %w", i+1, err)
		}
		if seen[number] {
			return nil, fmt.Errorf("member %d: phone %s is listed twice", i+1, phone.Mask(number))
		}
		seen[number] = true
		name := strings.TrimSpace(m.Name)
		if name == "" {
			return nil, fmt.Errorf("member %d: name is required", i+1)
		}
		roles, err := authz.NormalizeBuiltinRoles(m.Roles)
		if err != nil {
			return nil, fmt.Errorf("member %d: %w", i+1, err)
		}
		if len(roles) == 0 {
			return nil, fmt.Errorf("member %d: at least one role is required", i+1)
		}
		hasAdmin = hasAdmin || slices.Contains(roles, authz.RoleAdmin)
		out[i] = NewMember{Phone: number, Name: name, Roles: roles}
	}
	if !hasAdmin {
		return nil, errors.New("at least one member must have the admin role")
	}
	return out, nil
}
