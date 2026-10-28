package authz

import (
	"slices"
	"strings"
)

// Actions the code checks today. Every action is "resource:verb".
const (
	ActionMemberRead   = "member:read"
	ActionMemberInvite = "member:invite"
	ActionMemberManage = "member:manage"
	ActionRoleAssign   = "role:assign"

	ActionPropertyRead   = "property:read"
	ActionPropertyManage = "property:manage"
	ActionUnitRead       = "unit:read"
	ActionUnitManage     = "unit:manage"

	ActionTenantRead   = "tenant:read"
	ActionTenantManage = "tenant:manage"
	ActionLeaseManage  = "lease:manage"
)

// catalog lists every action the app knows, by resource. A policy may only name these, or
// wildcards that match at least one of them, so a typo in a policy fails validation instead
// of silently granting nothing.
var catalog = map[string][]string{
	"organization": {"read"},
	"settings":     {"read", "update"},
	"member":       {"read", "invite", "manage"},
	"role":         {"read", "assign", "manage"},
	"property":     {"read", "manage"},
	"unit":         {"read", "manage"},
	"tenant":       {"read", "manage"},
	"lease":        {"read", "manage"},
	"invoice":      {"read", "create", "send-reminder"},
	"payment":      {"read", "record", "check"},
	"correction":   {"read", "create"},
	"promise":      {"read", "create"},
	"visit":        {"read", "record"},
	"cash":         {"read", "record", "deposit"},
	"payroll":      {"read", "check", "approve", "record"},
	"payslip":      {"read"},
	"expense":      {"read", "create", "approve"},
	"ledger":       {"read"},
	"distribution": {"read", "manage"},
	"report":       {"read"},
	"audit":        {"read"},
}

// orgWideResources are the resources whose records never belong to a property, so a role
// limited to one property grants nothing on them (decision 15) and Capabilities reports deny
// rather than conditional. Add a resource here when its records never carry a property.
var orgWideResources = map[string]bool{
	"organization": true,
	"settings":     true,
	"member":       true,
	"role":         true,
}

var allActions = func() []string {
	var out []string
	for resource, verbs := range catalog {
		for _, verb := range verbs {
			out = append(out, resource+":"+verb)
		}
	}
	slices.Sort(out)
	return out
}()

// Actions returns every action in the catalog, sorted.
func Actions() []string { return slices.Clone(allActions) }

// IsAction reports whether a is an exact catalog action (no wildcards).
func IsAction(a string) bool {
	_, found := slices.BinarySearch(allActions, a)
	return found
}

// sensitive actions are written to the audit log even when they are allowed.
var sensitive = map[string]bool{
	"settings:update":     true,
	"member:invite":       true,
	"member:manage":       true,
	"role:assign":         true,
	"role:manage":         true,
	"property:manage":     true,
	"unit:manage":         true,
	"tenant:manage":       true,
	"lease:manage":        true,
	"payment:record":      true,
	"payment:check":       true,
	"correction:create":   true,
	"cash:record":         true,
	"cash:deposit":        true,
	"payroll:check":       true,
	"payroll:approve":     true,
	"payroll:record":      true,
	"expense:create":      true,
	"expense:approve":     true,
	"distribution:manage": true,
	"invoice:create":      true,
}

// IsSensitive reports whether an allowed action must still be audited.
func IsSensitive(action string) bool { return sensitive[action] }

// matchesAnyAction reports whether the pattern matches at least one catalog action.
func matchesAnyAction(pattern string) bool {
	for _, a := range allActions {
		if matchWildcard(pattern, a) {
			return true
		}
	}
	return false
}

// validActionPattern checks the shape "resource:verb", where either side may use *. A lone
// "*" is allowed and means every action.
func validActionPattern(p string) bool {
	if p == "*" {
		return true
	}
	resource, verb, ok := strings.Cut(p, ":")
	return ok && validPatternPart(resource) && validPatternPart(verb)
}

func validPatternPart(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && r != '-' && r != '*' {
			return false
		}
	}
	return true
}
