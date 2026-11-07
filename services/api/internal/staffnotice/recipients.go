package staffnotice

import (
	"slices"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
)

// Lease is one of a tenant's leases as Recipients reads it.
type Lease struct {
	PropertyID uuid.UUID
	// CollectorUserID is the staff user who collects from the tenant on this lease; uuid.Nil for
	// none.
	CollectorUserID uuid.UUID
	Active          bool
}

// Recipients returns the staff users a notice about tenantID goes to, each once, in a fixed order:
// first the collectors of the tenant's active leases, then the members whom authz allows
// payment:record on the tenant through one of those leases (the request in the lease's property,
// owned by its collector, as billing authorizes a payment), in members' order. members are the
// organization's active members (authz.Service.ActivePrincipalsTx), so a disabled collector or
// admin is never a recipient. A Supervisor, who may not record payments, is one only as a
// collector.
//
// A tenant with no active lease (moved out, still owing) is looked after through the leases they
// had; with none at all, only an organization-wide grant allows. The decision is authz.Evaluate,
// which records nothing: this is the platform deciding whom to tell, not a member's request.
func Recipients(tenantID uuid.UUID, leases []Lease, members []authz.Principal) []uuid.UUID {
	var active []Lease
	for _, l := range leases {
		if l.Active {
			active = append(active, l)
		}
	}
	scope := active
	if len(scope) == 0 {
		scope = leases
	}
	isMember := make(map[uuid.UUID]bool, len(members))
	for _, m := range members {
		isMember[m.UserID] = true
	}

	var out []uuid.UUID
	add := func(user uuid.UUID) {
		if user != uuid.Nil && !slices.Contains(out, user) {
			out = append(out, user)
		}
	}
	for _, l := range active {
		if isMember[l.CollectorUserID] {
			add(l.CollectorUserID)
		}
	}
	for _, m := range members {
		if mayRecordPayment(m, tenantID, scope) {
			add(m.UserID)
		}
	}
	return out
}

// mayRecordPayment reports whether p may record a payment of the tenant through one of leases, or
// on the bare tenant when there is none.
func mayRecordPayment(p authz.Principal, tenantID uuid.UUID, leases []Lease) bool {
	req := authz.Request{Action: authz.ActionPaymentRecord, Resource: authz.Resource{Type: "tenant", ID: tenantID.String()}}
	if len(leases) == 0 {
		return authz.Evaluate(p, req).Allowed
	}
	for _, l := range leases {
		via := req
		via.Resource.PropertyID = l.PropertyID
		via.Resource.OwnerID = l.CollectorUserID
		if authz.Evaluate(p, via).Allowed {
			return true
		}
	}
	return false
}
