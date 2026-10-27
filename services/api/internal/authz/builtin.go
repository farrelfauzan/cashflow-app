package authz

import (
	"cmp"
	"fmt"
	"slices"
)

// Keys of the built-in roles. They match the CHECK constraint on role_assignment.builtin_role.
const (
	RoleSupervisor = "supervisor"
	RoleAdmin      = "admin"
	RoleKaryawan   = "karyawan"
)

func ptr[T any](v T) *T { return &v }

// Guardrails apply to every principal before any role and cannot be overridden.
var Guardrails = Policy{
	Version: PolicyVersion,
	Statement: []Statement{
		{
			// Whoever checked a payroll run or created an expense cannot approve it.
			Sid:       "NoApprovingOwnWork",
			Effect:    Deny,
			Action:    []string{"payroll:approve", "expense:approve"},
			Condition: &Condition{Actor: ptr(true)},
		},
		{
			// Nobody changes their own roles or status, so access cannot be self-granted.
			Sid:       "NoChangingOwnAccess",
			Effect:    Deny,
			Action:    []string{"member:manage"},
			Condition: &Condition{Self: ptr(true)},
		},
	},
}

var builtinRoles = map[string]Role{
	RoleSupervisor: {
		Key: RoleSupervisor, Name: "Supervisor", Builtin: true,
		Policy: Policy{
			Version: PolicyVersion,
			Statement: []Statement{
				// The Supervisor sees everything and approves, but never types data.
				{Sid: "ReadEverything", Effect: Allow, Action: []string{"*:read"}},
				{Sid: "Approve", Effect: Allow, Action: []string{"payroll:approve", "expense:approve"}},
			},
		},
	},
	RoleAdmin: {
		Key: RoleAdmin, Name: "Admin", Builtin: true,
		Policy: Policy{
			Version: PolicyVersion,
			Statement: []Statement{
				{
					Sid:    "RunOperations",
					Effect: Allow,
					Action: []string{
						"organization:read", "settings:*", "member:*", "role:read",
						"property:*", "unit:*", "tenant:*", "lease:*", "invoice:*",
						"payment:read", "payment:record", "payment:check",
						"correction:*", "promise:*", "visit:read", "cash:read",
						"payroll:read", "payroll:check", "payroll:record", "payslip:read",
						"expense:read", "expense:create",
						"ledger:read", "report:read", "audit:read", "distribution:read",
					},
				},
				{
					// Admins may hand out the Admin and Karyawan roles, never Supervisor.
					Sid:      "AssignOperationalRoles",
					Effect:   Allow,
					Action:   []string{"role:assign"},
					Resource: []string{"role/" + RoleAdmin, "role/" + RoleKaryawan},
				},
			},
		},
	},
	RoleKaryawan: {
		Key: RoleKaryawan, Name: "Karyawan", Builtin: true,
		Policy: Policy{
			Version: PolicyVersion,
			Statement: []Statement{
				{Sid: "SeeOrganization", Effect: Allow, Action: []string{"organization:read"}},
				{
					// Field work on the tenants assigned to this karyawan only.
					Sid:    "OwnFieldWork",
					Effect: Allow,
					Action: []string{
						"tenant:read", "invoice:read", "visit:read", "visit:record",
						"cash:record", "promise:create",
					},
					Condition: &Condition{Self: ptr(true)},
				},
				{
					Sid:       "OwnPay",
					Effect:    Allow,
					Action:    []string{"payslip:read", "cash:deposit"},
					Condition: &Condition{Self: ptr(true)},
				},
			},
		},
	},
}

// BuiltinRoleKeys returns the built-in role keys in display order.
func BuiltinRoleKeys() []string {
	return []string{RoleSupervisor, RoleAdmin, RoleKaryawan}
}

// BuiltinRole returns the built-in role with the given key.
func BuiltinRole(key string) (Role, bool) {
	r, ok := builtinRoles[key]
	return r, ok
}

// SortRoles orders roles for display: built-in roles in their fixed order, then custom roles
// by name.
func SortRoles[T any](roles []T, key func(T) string, name func(T) string) {
	rank := func(k string) int {
		if i := slices.Index(BuiltinRoleKeys(), k); i >= 0 {
			return i
		}
		return len(builtinRoles)
	}
	slices.SortStableFunc(roles, func(a, b T) int {
		if c := cmp.Compare(rank(key(a)), rank(key(b))); c != 0 {
			return c
		}
		return cmp.Compare(name(a), name(b))
	})
}

// NormalizeBuiltinRoles checks that every key is a built-in role and returns them without
// duplicates, in display order.
func NormalizeBuiltinRoles(keys []string) ([]string, error) {
	seen := map[string]bool{}
	for _, k := range keys {
		if _, ok := builtinRoles[k]; !ok {
			return nil, fmt.Errorf("unknown role %q", k)
		}
		seen[k] = true
	}
	var out []string
	for _, k := range BuiltinRoleKeys() {
		if seen[k] {
			out = append(out, k)
		}
	}
	return out, nil
}
