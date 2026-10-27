// Package authz decides what a staff member may do inside one organization.
//
// It follows AWS IAM: permissions are "resource:verb" actions, roles carry policy documents of
// Allow and Deny statements, everything is denied by default, an explicit Deny beats any Allow,
// and otherwise any matching Allow grants. Guardrails are Deny statements that apply to every
// principal and cannot be overridden by any role.
//
// Row-level security keeps organizations apart. This package decides what members of the same
// organization may do. Both must pass.
package authz

import (
	"slices"
	"strings"

	"github.com/google/uuid"
)

// Principal is a staff member acting inside one organization.
type Principal struct {
	UserID         uuid.UUID
	OrganizationID uuid.UUID
	MembershipID   uuid.UUID
	Roles          []Role
	// Properties the member is assigned to, for the "property": "assigned" condition: every
	// property a role assignment is limited to.
	Properties []uuid.UUID
}

// RoleKeys returns the keys of the principal's roles.
func (p Principal) RoleKeys() []string {
	keys := make([]string, len(p.Roles))
	for i, r := range p.Roles {
		keys[i] = r.Key
	}
	return keys
}

// Role is a named policy. Built-in roles come from code, custom roles from the database.
type Role struct {
	Key     string
	Name    string
	Builtin bool
	Policy  Policy
	// Property, when set, limits the role to records of that one property. Its statements
	// then act as if each carried an extra condition: an Allow grants only on records of the
	// property, and a Deny applies there and wherever the property is unknown.
	Property uuid.UUID
}

// Resource is what an action targets. Fill in what the record has; leave the rest empty and
// any condition that needs it fails closed.
type Resource struct {
	Type       string
	ID         string
	PropertyID uuid.UUID
	// OwnerID is the staff user the record belongs to or is assigned to.
	OwnerID uuid.UUID
	// Actors are staff users who already acted on the record. Set ActorsKnown when the list
	// is complete, even if it is empty.
	Actors      []uuid.UUID
	ActorsKnown bool
	// Amount in rupiah, when the action involves money.
	Amount *int64
}

// Path is the resource name policies match against: "type/id", prefixed with
// "property/<id>/" when the record belongs to a property. A property is its own property, and
// its path is just "property/<id>".
func (r Resource) Path() string {
	var b strings.Builder
	if r.PropertyID != uuid.Nil && (r.Type != "property" || r.ID != r.PropertyID.String()) {
		b.WriteString("property/")
		b.WriteString(r.PropertyID.String())
		b.WriteString("/")
	}
	b.WriteString(r.Type)
	if r.ID != "" {
		b.WriteString("/")
		b.WriteString(r.ID)
	}
	return b.String()
}

// Request is one permission check.
type Request struct {
	Action   string
	Resource Resource
}

// Reason says why a decision came out the way it did.
type Reason string

// The possible reasons.
const (
	ReasonAllowed       Reason = "allowed"
	ReasonExplicitDeny  Reason = "explicit_deny"
	ReasonImplicitDeny  Reason = "implicit_deny"
	ReasonUnknownAction Reason = "unknown_action"
)

// Decision is the outcome of Evaluate. Policy and Statement name the deciding statement, if
// one decided.
type Decision struct {
	Allowed   bool
	Reason    Reason
	Policy    string
	Statement string
}

// Evaluate applies the IAM rules to one request. It is a pure function.
func Evaluate(p Principal, req Request) Decision {
	if !IsAction(req.Action) {
		return Decision{Reason: ReasonUnknownAction}
	}
	path := req.Resource.Path()

	var allow *Decision
	for _, np := range principalPolicies(p) {
		for _, st := range np.policy.Statement {
			if !st.matches(req.Action, path) {
				continue
			}
			cond := both(st.Condition.evaluate(p, req.Resource), inScope(np.property, req.Resource))
			switch st.Effect {
			case Deny:
				if cond != no {
					return Decision{Reason: ReasonExplicitDeny, Policy: np.name, Statement: st.Sid}
				}
			case Allow:
				if cond == yes && allow == nil {
					allow = &Decision{Allowed: true, Reason: ReasonAllowed, Policy: np.name, Statement: st.Sid}
				}
			}
		}
	}
	if allow != nil {
		return *allow
	}
	return Decision{Reason: ReasonImplicitDeny}
}

type namedPolicy struct {
	name   string
	policy Policy
	// property limits the policy to one property's records; uuid.Nil means no limit.
	property uuid.UUID
}

// principalPolicies returns the guardrails first, then every role's policy.
func principalPolicies(p Principal) []namedPolicy {
	out := make([]namedPolicy, 0, len(p.Roles)+1)
	out = append(out, namedPolicy{name: "guardrails", policy: Guardrails})
	for _, r := range p.Roles {
		name := r.Key
		if r.Property != uuid.Nil {
			name += "@property/" + r.Property.String()
		}
		out = append(out, namedPolicy{name: name, policy: r.Policy, property: r.Property})
	}
	return out
}

func (st Statement) matches(action, path string) bool {
	return st.matchesAction(action) && st.matchesResource(path)
}

func (st Statement) matchesAction(action string) bool {
	return slices.ContainsFunc(st.Action, func(p string) bool { return matchWildcard(p, action) })
}

func (st Statement) matchesResource(path string) bool {
	if len(st.Resource) == 0 {
		return true
	}
	return slices.ContainsFunc(st.Resource, func(p string) bool { return matchWildcard(p, path) })
}

// everyResource reports whether the statement applies to all resources.
func (st Statement) everyResource() bool {
	return len(st.Resource) == 0 || slices.Contains(st.Resource, "*")
}

// tri is a three-valued truth: a condition can hold, fail, or lack the data to tell.
type tri int8

const (
	no tri = iota
	yes
	unknown
)

func (c *Condition) evaluate(p Principal, r Resource) tri {
	if c == nil {
		return yes
	}
	result := yes
	and := func(t tri) {
		switch {
		case t == no:
			result = no
		case t == unknown && result == yes:
			result = unknown
		}
	}
	if c.Self != nil {
		and(want(*c.Self, isSelf(p, r)))
	}
	if c.Actor != nil {
		and(want(*c.Actor, isActor(p, r)))
	}
	if c.Property == "assigned" {
		and(isAssignedProperty(p, r))
	}
	if c.AmountAtMost != nil {
		and(amountAtMost(r, *c.AmountAtMost))
	}
	return result
}

// both is the three-valued AND.
func both(a, b tri) tri {
	switch {
	case a == no || b == no:
		return no
	case a == unknown || b == unknown:
		return unknown
	default:
		return yes
	}
}

// inScope reports whether the resource belongs to the property a role is limited to. A role
// without a limit covers every resource; a resource without a property is unknown, so a
// limited role grants nothing on organization-wide records such as members or settings.
func inScope(property uuid.UUID, r Resource) tri {
	if property == uuid.Nil {
		return yes
	}
	if r.PropertyID == uuid.Nil {
		return unknown
	}
	return fromBool(r.PropertyID == property)
}

// want compares an expected boolean with a three-valued fact.
func want(expected bool, actual tri) tri {
	if actual == unknown {
		return unknown
	}
	if (actual == yes) == expected {
		return yes
	}
	return no
}

func fromBool(b bool) tri {
	if b {
		return yes
	}
	return no
}

func isSelf(p Principal, r Resource) tri {
	if r.OwnerID == uuid.Nil {
		return unknown
	}
	return fromBool(r.OwnerID == p.UserID)
}

func isActor(p Principal, r Resource) tri {
	if !r.ActorsKnown {
		return unknown
	}
	return fromBool(slices.Contains(r.Actors, p.UserID))
}

func isAssignedProperty(p Principal, r Resource) tri {
	if r.PropertyID == uuid.Nil {
		return unknown
	}
	return fromBool(slices.Contains(p.Properties, r.PropertyID))
}

func amountAtMost(r Resource, limit int64) tri {
	if r.Amount == nil {
		return unknown
	}
	return fromBool(*r.Amount <= limit)
}

// Access is how much of an action a principal has, for showing or hiding controls in the UI.
// The server still checks every request.
type Access string

// The three levels of access.
const (
	AccessAllow       Access = "allow"
	AccessConditional Access = "conditional" // allowed on some records, such as their own
	AccessDeny        Access = "deny"
)

// Capability is one action and the principal's access to it.
type Capability struct {
	Action string
	Access Access
}

// Capabilities summarizes, for every catalog action, whether the principal can do it always,
// only on some records, or never.
func Capabilities(p Principal) []Capability {
	policies := principalPolicies(p)
	out := make([]Capability, 0, len(allActions))
	for _, action := range allActions {
		out = append(out, Capability{Action: action, Access: accessTo(policies, action)})
	}
	return out
}

// AccessTo is Capabilities for one action.
func AccessTo(p Principal, action string) Access {
	if !IsAction(action) {
		return AccessDeny
	}
	return accessTo(principalPolicies(p), action)
}

func accessTo(policies []namedPolicy, action string) Access {
	access := AccessDeny
	blocked := false
	for _, np := range policies {
		for _, st := range np.policy.Statement {
			if !st.matchesAction(action) {
				continue
			}
			unconditional := st.Condition == nil && st.everyResource() && np.property == uuid.Nil
			switch {
			case st.Effect == Deny && unconditional:
				blocked = true
			case st.Effect == Allow && unconditional:
				access = AccessAllow
			case st.Effect == Allow && access == AccessDeny:
				access = AccessConditional
			}
		}
	}
	if blocked {
		return AccessDeny
	}
	return access
}
