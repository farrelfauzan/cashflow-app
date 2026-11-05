package authz

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func principal(t *testing.T, keys ...string) Principal {
	t.Helper()
	p := Principal{UserID: uuid.New(), OrganizationID: uuid.New(), MembershipID: uuid.New()}
	for _, k := range keys {
		r, ok := BuiltinRole(k)
		require.True(t, ok, k)
		p.Roles = append(p.Roles, r)
	}
	return p
}

func customRole(t *testing.T, doc string) Role {
	t.Helper()
	pol, err := ParsePolicy([]byte(doc))
	require.NoError(t, err)
	return Role{Key: "custom:test", Name: "Test", Policy: pol}
}

func TestDefaultDeny(t *testing.T) {
	p := Principal{UserID: uuid.New()}
	d := Evaluate(p, Request{Action: "invoice:read", Resource: Resource{Type: "invoice"}})
	require.False(t, d.Allowed)
	require.Equal(t, ReasonImplicitDeny, d.Reason)
}

func TestUnknownActionIsDenied(t *testing.T) {
	p := principal(t, RoleAdmin)
	d := Evaluate(p, Request{Action: "invoice:delete", Resource: Resource{Type: "invoice"}})
	require.False(t, d.Allowed)
	require.Equal(t, ReasonUnknownAction, d.Reason)
}

func TestExplicitDenyBeatsAllowFromAnotherRole(t *testing.T) {
	p := principal(t, RoleAdmin)
	p.Roles = append(p.Roles, customRole(t, `{"Version":"2026-10-25","Statement":[
		{"Sid":"NoCorrections","Effect":"Deny","Action":["correction:create"]}]}`))

	d := Evaluate(p, Request{Action: "correction:create", Resource: Resource{Type: "correction"}})
	require.False(t, d.Allowed)
	require.Equal(t, ReasonExplicitDeny, d.Reason)
	require.Equal(t, "NoCorrections", d.Statement)

	d = Evaluate(p, Request{Action: "correction:read", Resource: Resource{Type: "correction"}})
	require.True(t, d.Allowed, "the deny is specific to one action")
}

func TestSupervisorCannotApproveWorkTheyChecked(t *testing.T) {
	p := principal(t, RoleSupervisor, RoleAdmin) // one person holding both roles
	other := uuid.New()
	tests := []struct {
		name        string
		actors      []uuid.UUID
		actorsKnown bool
		wantAllowed bool
		wantReason  Reason
	}{
		{"someone else checked it", []uuid.UUID{other}, true, true, ReasonAllowed},
		{"nobody acted yet", nil, true, true, ReasonAllowed},
		{"they checked it themselves", []uuid.UUID{other, p.UserID}, true, false, ReasonExplicitDeny},
		{"the caller did not say who acted", nil, false, false, ReasonExplicitDeny},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(p, Request{Action: "payroll:approve", Resource: Resource{
				Type: "payroll_run", ID: "oct-2026", Actors: tc.actors, ActorsKnown: tc.actorsKnown,
			}})
			require.Equal(t, tc.wantAllowed, d.Allowed)
			require.Equal(t, tc.wantReason, d.Reason)
		})
	}
}

func TestNobodyChangesTheirOwnAccess(t *testing.T) {
	p := principal(t, RoleAdmin)
	d := Evaluate(p, Request{Action: ActionMemberManage, Resource: Resource{Type: "member", ID: "m1", OwnerID: p.UserID}})
	require.False(t, d.Allowed)
	require.Equal(t, "NoChangingOwnAccess", d.Statement)

	d = Evaluate(p, Request{Action: ActionMemberManage, Resource: Resource{Type: "member", ID: "m2", OwnerID: uuid.New()}})
	require.True(t, d.Allowed)

	d = Evaluate(p, Request{Action: ActionMemberManage, Resource: Resource{Type: "member", ID: "m3"}})
	require.False(t, d.Allowed, "an unknown owner must fail closed")
}

func TestAdminCanAssignOperationalRolesButNotSupervisor(t *testing.T) {
	admin := principal(t, RoleAdmin)
	for role, want := range map[string]bool{RoleAdmin: true, RoleKaryawan: true, RoleSupervisor: false} {
		d := Evaluate(admin, Request{Action: ActionRoleAssign, Resource: Resource{Type: "role", ID: role}})
		require.Equal(t, want, d.Allowed, role)
	}
	supervisor := principal(t, RoleSupervisor)
	d := Evaluate(supervisor, Request{Action: ActionRoleAssign, Resource: Resource{Type: "role", ID: RoleKaryawan}})
	require.False(t, d.Allowed, "the Supervisor never writes, including role changes")
}

func TestKaryawanWorksOnlyOnTheirOwnTenants(t *testing.T) {
	p := principal(t, RoleKaryawan)
	tests := []struct {
		name  string
		owner uuid.UUID
		want  bool
	}{
		{"assigned to them", p.UserID, true},
		{"assigned to someone else", uuid.New(), false},
		{"assignment unknown", uuid.Nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(p, Request{Action: "cash:record", Resource: Resource{Type: "tenant", ID: "t1", OwnerID: tc.owner}})
			require.Equal(t, tc.want, d.Allowed)
		})
	}
}

func TestPropertyAndAmountConditions(t *testing.T) {
	assigned, elsewhere := uuid.New(), uuid.New()
	p := Principal{UserID: uuid.New(), Properties: []uuid.UUID{assigned}}
	p.Roles = append(p.Roles, customRole(t, `{"Version":"2026-10-25","Statement":[
		{"Effect":"Allow","Action":["payment:record"],"Condition":{"property":"assigned","amountAtMost":1000000}}]}`))

	amount := func(v int64) *int64 { return &v }
	tests := []struct {
		name     string
		property uuid.UUID
		amount   *int64
		want     bool
	}{
		{"assigned property, small amount", assigned, amount(950_000), true},
		{"assigned property, exactly the limit", assigned, amount(1_000_000), true},
		{"assigned property, too large", assigned, amount(1_000_001), false},
		{"other property", elsewhere, amount(10), false},
		{"property unknown", uuid.Nil, amount(10), false},
		{"amount unknown", assigned, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(p, Request{Action: "payment:record", Resource: Resource{Type: "payment", PropertyID: tc.property, Amount: tc.amount}})
			require.Equal(t, tc.want, d.Allowed)
		})
	}
}

func TestResourcePath(t *testing.T) {
	prop := uuid.MustParse("6f1c0c55-2f43-4c6b-9a0e-0d0b4b7a9f10")
	require.Equal(t, "member", Resource{Type: "member"}.Path())
	require.Equal(t, "role/admin", Resource{Type: "role", ID: "admin"}.Path())
	require.Equal(t, "property/6f1c0c55-2f43-4c6b-9a0e-0d0b4b7a9f10/tenant/t1",
		Resource{Type: "tenant", ID: "t1", PropertyID: prop}.Path())
	require.Equal(t, "property/6f1c0c55-2f43-4c6b-9a0e-0d0b4b7a9f10",
		Resource{Type: "property", ID: prop.String(), PropertyID: prop}.Path())
}

// TestBuiltinRoleMatrix checks every built-in role against every action in the catalog, so a
// new action or a policy edit cannot change access without this table changing too.
func TestBuiltinRoleMatrix(t *testing.T) {
	allow := func(actions ...string) map[string]Access {
		m := map[string]Access{}
		for _, a := range actions {
			m[a] = AccessAllow
		}
		return m
	}
	withConditional := func(m map[string]Access, actions ...string) map[string]Access {
		for _, a := range actions {
			m[a] = AccessConditional
		}
		return m
	}

	var reads []string
	for _, a := range Actions() {
		if matchWildcard("*:read", a) {
			reads = append(reads, a)
		}
	}
	expected := map[string]map[string]Access{
		RoleSupervisor: allow(append(reads, "payroll:approve", "expense:approve")...),
		RoleAdmin: withConditional(allow(
			"organization:read", "settings:read", "settings:update",
			"member:read", "member:invite", "member:manage", "role:read",
			"property:read", "property:manage", "unit:read", "unit:manage",
			"tenant:read", "tenant:manage", "lease:read", "lease:manage",
			"charge:read", "charge:record", "invoice:read", "invoice:create", "invoice:send-reminder", "invoice:share",
			"payment:read", "payment:record", "payment:check",
			"correction:read", "correction:create", "promise:read", "promise:create",
			"visit:read", "cash:read", "payroll:read", "payroll:check", "payroll:record",
			"payslip:read", "expense:read", "expense:create",
			"ledger:read", "report:read", "audit:read", "distribution:read",
		), "role:assign"),
		RoleKaryawan: withConditional(allow("organization:read"),
			"tenant:read", "invoice:read", "visit:read", "visit:record",
			"cash:record", "promise:create", "payslip:read", "cash:deposit",
			"charge:read", "charge:record", "invoice:share",
		),
	}

	for role, want := range expected {
		t.Run(role, func(t *testing.T) {
			got := map[string]Access{}
			for _, c := range Capabilities(principal(t, role)) {
				if c.Access != AccessDeny {
					got[c.Action] = c.Access
				}
			}
			require.Equal(t, want, got)
		})
	}
}

// TestMeteredChargesOnALease checks charge:read and charge:record on a lease the way the billing
// service asks: the lease in its property, owned by its collector.
func TestMeteredChargesOnALease(t *testing.T) {
	t.Parallel()
	admin, karyawan, supervisor := principal(t, RoleAdmin), principal(t, RoleKaryawan), principal(t, RoleSupervisor)
	amount := int64(123_000)
	lease := func(collector uuid.UUID) Resource {
		return Resource{Type: "lease", ID: uuid.NewString(), PropertyID: uuid.New(), OwnerID: collector, Amount: &amount}
	}
	tests := []struct {
		name   string
		p      Principal
		action string
		res    Resource
		want   bool
	}{
		{"admin records on any lease", admin, ActionChargeRecord, lease(uuid.New()), true},
		{"admin records on a lease with no collector", admin, ActionChargeRecord, lease(uuid.Nil), true},
		{"admin reads", admin, ActionChargeRead, lease(uuid.Nil), true},
		{"karyawan records for a tenant they collect from", karyawan, ActionChargeRecord, lease(karyawan.UserID), true},
		{"karyawan reads for a tenant they collect from", karyawan, ActionChargeRead, lease(karyawan.UserID), true},
		{"karyawan may not record for another collector's tenant", karyawan, ActionChargeRecord, lease(uuid.New()), false},
		{"karyawan may not record on a lease with no collector", karyawan, ActionChargeRecord, lease(uuid.Nil), false},
		{"karyawan may not read another collector's tenant", karyawan, ActionChargeRead, lease(uuid.New()), false},
		{"supervisor reads", supervisor, ActionChargeRead, lease(uuid.New()), true},
		{"supervisor never records", supervisor, ActionChargeRecord, lease(supervisor.UserID), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, Evaluate(tc.p, Request{Action: tc.action, Resource: tc.res}).Allowed)
		})
	}
	require.True(t, IsSensitive(ActionChargeRecord), "recording an amount is audited")
	require.False(t, IsSensitive(ActionChargeRead))
}

// TestSharingAnInvoice checks invoice:share (payment links) the way the paylink service asks: the
// invoice in its lease's property, owned by the lease's collector.
func TestSharingAnInvoice(t *testing.T) {
	t.Parallel()
	admin, karyawan, supervisor := principal(t, RoleAdmin), principal(t, RoleKaryawan), principal(t, RoleSupervisor)
	invoice := func(collector uuid.UUID) Resource {
		return Resource{Type: "invoice", ID: uuid.NewString(), PropertyID: uuid.New(), OwnerID: collector}
	}
	tests := []struct {
		name string
		p    Principal
		res  Resource
		want bool
	}{
		{"admin shares any invoice", admin, invoice(uuid.New()), true},
		{"admin shares an invoice with no collector", admin, invoice(uuid.Nil), true},
		{"karyawan shares an invoice of a tenant they collect from", karyawan, invoice(karyawan.UserID), true},
		{"karyawan may not share another collector's invoice", karyawan, invoice(uuid.New()), false},
		{"karyawan may not share an invoice with no collector", karyawan, invoice(uuid.Nil), false},
		{"supervisor never shares", supervisor, invoice(supervisor.UserID), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, Evaluate(tc.p, Request{Action: ActionInvoiceShare, Resource: tc.res}).Allowed)
		})
	}
	require.True(t, IsSensitive(ActionInvoiceShare), "minting and revoking links is audited")
}

func TestCapabilitiesCoverTheWholeCatalog(t *testing.T) {
	caps := Capabilities(Principal{})
	require.Len(t, caps, len(Actions()))
	for _, c := range caps {
		require.Equal(t, AccessDeny, c.Access, c.Action)
	}
}

func TestRoleLimitedToOneProperty(t *testing.T) {
	melati, mawar := uuid.New(), uuid.New()
	admin, ok := BuiltinRole(RoleAdmin)
	require.True(t, ok)
	admin.Property = melati
	p := Principal{UserID: uuid.New(), Roles: []Role{admin}, Properties: []uuid.UUID{melati}}

	tests := []struct {
		name   string
		action string
		res    Resource
		want   bool
	}{
		{"a unit in their property", "unit:manage", Resource{Type: "unit", ID: "u1", PropertyID: melati}, true},
		{"the property itself", "property:manage", Resource{Type: "property", ID: melati.String(), PropertyID: melati}, true},
		{"a unit in another property", "unit:manage", Resource{Type: "unit", ID: "u2", PropertyID: mawar}, false},
		{"a record whose property is unknown", "tenant:read", Resource{Type: "tenant", ID: "t1"}, false},
		{"creating a new property", "property:manage", Resource{Type: "property"}, false},
		{"organization-wide records", "member:read", Resource{Type: "member"}, false},
		{"settings", "settings:update", Resource{Type: "settings"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(p, Request{Action: tc.action, Resource: tc.res})
			require.Equal(t, tc.want, d.Allowed)
			if tc.want {
				require.Equal(t, "admin@property/"+melati.String(), d.Policy)
			}
		})
	}

	t.Run("an unlimited role still covers everything", func(t *testing.T) {
		karyawan, ok := BuiltinRole(RoleKaryawan)
		require.True(t, ok)
		withOrgWide := p
		withOrgWide.Roles = []Role{admin, karyawan}
		d := Evaluate(withOrgWide, Request{Action: "organization:read", Resource: Resource{Type: "organization"}})
		require.True(t, d.Allowed)
		d = Evaluate(withOrgWide, Request{Action: "unit:manage", Resource: Resource{Type: "unit", PropertyID: mawar}})
		require.False(t, d.Allowed, "karyawan does not manage units anywhere")
	})

	t.Run("its capabilities become conditional", func(t *testing.T) {
		require.Equal(t, AccessConditional, AccessTo(p, ActionPropertyRead))
		require.Equal(t, AccessConditional, AccessTo(p, ActionTenantRead))
		require.Equal(t, AccessDeny, AccessTo(p, "payroll:approve"))
		require.Equal(t, AccessDeny, AccessTo(p, "property:delete"), "unknown actions are denied")
	})
}

func TestLimitedRoleOnOrganizationWideActions(t *testing.T) {
	melati := uuid.New()
	admin, ok := BuiltinRole(RoleAdmin)
	require.True(t, ok)
	limited := admin
	limited.Property = melati

	t.Run("a limited role reports deny, as the server answers", func(t *testing.T) {
		p := Principal{UserID: uuid.New(), Roles: []Role{limited}}
		for _, action := range []string{ActionMemberRead, ActionMemberInvite, ActionMemberManage, ActionRoleAssign, "settings:update", "organization:read"} {
			require.Equal(t, AccessDeny, AccessTo(p, action), action)
		}
		d := Evaluate(p, Request{Action: ActionMemberRead, Resource: Resource{Type: "member"}})
		require.False(t, d.Allowed, "the capability matches the evaluation")
	})

	t.Run("an organization-wide role next to it still allows", func(t *testing.T) {
		p := Principal{UserID: uuid.New(), Roles: []Role{admin, limited}}
		require.Equal(t, AccessAllow, AccessTo(p, ActionMemberRead))
	})

	t.Run("a limited deny always denies organization-wide records", func(t *testing.T) {
		deny := customRole(t, `{"Version":"2026-10-25","Statement":[
			{"Sid":"NoMembers","Effect":"Deny","Action":["member:*"]}]}`)
		deny.Property = melati
		p := Principal{UserID: uuid.New(), Roles: []Role{admin, deny}}
		require.Equal(t, AccessDeny, AccessTo(p, ActionMemberRead))
		d := Evaluate(p, Request{Action: ActionMemberRead, Resource: Resource{Type: "member"}})
		require.False(t, d.Allowed, "the capability matches the evaluation")
		require.Equal(t, AccessAllow, AccessTo(p, ActionTenantRead), "other actions are not affected")
	})
}

func TestDenyInLimitedRole(t *testing.T) {
	melati, mawar := uuid.New(), uuid.New()
	admin, ok := BuiltinRole(RoleAdmin)
	require.True(t, ok)
	deny := customRole(t, `{"Version":"2026-10-25","Statement":[
		{"Sid":"NoUnitEdits","Effect":"Deny","Action":["unit:manage"]}]}`)
	deny.Property = melati
	p := Principal{UserID: uuid.New(), Roles: []Role{admin, deny}}

	tests := []struct {
		name     string
		property uuid.UUID
		want     bool
	}{
		{"inside the limited property the deny wins", melati, false},
		{"another property is not affected", mawar, true},
		{"an unknown property fails closed", uuid.Nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := Evaluate(p, Request{Action: "unit:manage", Resource: Resource{Type: "unit", PropertyID: tc.property}})
			require.Equal(t, tc.want, d.Allowed)
		})
	}
	require.Equal(t, AccessConditional, AccessTo(p, "unit:manage"),
		"a limited deny does not block the whole action, but the action is allowed only outside it")
}

// TestScopedDenyMakesAccessConditional: an organization-wide Allow with a Deny that names one
// property's records allows the action only on the others, so the capability is conditional and
// a list must check each record (AuthorizeList). A Deny with a condition (a guardrail) and a
// Deny on an organization-wide resource keep the capability as it was.
func TestScopedDenyMakesAccessConditional(t *testing.T) {
	melati, mawar := uuid.New(), uuid.New()
	role := customRole(t, `{"Version":"2026-10-25","Statement":[
		{"Effect":"Allow","Action":["tenant:read","tenant:manage","member:read","property:read"],"Resource":["*"]},
		{"Effect":"Deny","Action":["tenant:read","tenant:manage","member:read"],"Resource":["property/`+melati.String()+`/*"]},
		{"Effect":"Deny","Action":["property:read"],"Resource":["property/`+melati.String()+`"]}]}`)
	p := Principal{UserID: uuid.New(), Roles: []Role{role}}

	require.Equal(t, AccessConditional, AccessTo(p, ActionTenantRead))
	require.Equal(t, AccessConditional, AccessTo(p, ActionTenantManage))
	require.Equal(t, AccessConditional, AccessTo(p, ActionPropertyRead))
	require.Equal(t, AccessAllow, AccessTo(p, ActionMemberRead), "members have no property, so the Deny never matches")

	tenantIn := func(property uuid.UUID) Request {
		return Request{Action: ActionTenantRead, Resource: Resource{Type: "tenant", ID: "t1", PropertyID: property}}
	}
	require.False(t, Evaluate(p, tenantIn(melati)).Allowed)
	require.True(t, Evaluate(p, tenantIn(mawar)).Allowed)
	require.True(t, Evaluate(p, Request{Action: ActionTenantRead, Resource: Resource{Type: "tenant"}}).Allowed,
		"the bare tenant names no property, which is why lists must not stop at it")

	admin, ok := BuiltinRole(RoleAdmin)
	require.True(t, ok)
	require.Equal(t, AccessAllow, AccessTo(Principal{UserID: uuid.New(), Roles: []Role{admin}}, ActionMemberManage),
		"the no-self-grant guardrail is a conditional Deny and keeps member:manage allowed")
}
