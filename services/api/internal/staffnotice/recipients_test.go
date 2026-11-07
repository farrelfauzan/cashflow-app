package staffnotice_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice"
)

// principal is a principal holding the given built-in role, limited to property when it is set.
func principal(t *testing.T, user uuid.UUID, role string, property uuid.UUID) authz.Principal {
	t.Helper()
	r, ok := authz.BuiltinRole(role)
	require.True(t, ok)
	p := authz.Principal{UserID: user, MembershipID: uuid.New()}
	if property != uuid.Nil {
		r.Property = property
		p.Properties = []uuid.UUID{property}
	}
	p.Roles = []authz.Role{r}
	return p
}

func TestRecipients(t *testing.T) {
	t.Parallel()
	tenantID := uuid.New()
	kos, ruko := uuid.New(), uuid.New()
	admin, kosAdmin, rukoAdmin, supervisor, collector, otherKaryawan := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	disabledCollector := uuid.New()
	members := []authz.Principal{
		principal(t, supervisor, authz.RoleSupervisor, uuid.Nil),
		principal(t, admin, authz.RoleAdmin, uuid.Nil),
		principal(t, kosAdmin, authz.RoleAdmin, kos),
		principal(t, rukoAdmin, authz.RoleAdmin, ruko),
		principal(t, collector, authz.RoleKaryawan, uuid.Nil),
		principal(t, otherKaryawan, authz.RoleKaryawan, uuid.Nil),
	}
	tests := []struct {
		name    string
		leases  []staffnotice.Lease
		members []authz.Principal
		want    []uuid.UUID
	}{
		{
			name:    "the collector first, then the admins of the lease's property",
			leases:  []staffnotice.Lease{{PropertyID: kos, CollectorUserID: collector, Active: true}},
			members: members,
			want:    []uuid.UUID{collector, admin, kosAdmin},
		},
		{
			name:    "a supervisor who collects is told as the collector",
			leases:  []staffnotice.Lease{{PropertyID: kos, CollectorUserID: supervisor, Active: true}},
			members: members,
			want:    []uuid.UUID{supervisor, admin, kosAdmin},
		},
		{
			name:    "no collector",
			leases:  []staffnotice.Lease{{PropertyID: ruko, Active: true}},
			members: members,
			want:    []uuid.UUID{admin, rukoAdmin},
		},
		{
			name: "two active leases on two properties, each told once",
			leases: []staffnotice.Lease{
				{PropertyID: kos, CollectorUserID: collector, Active: true},
				{PropertyID: ruko, CollectorUserID: collector, Active: true},
			},
			members: members,
			want:    []uuid.UUID{collector, admin, kosAdmin, rukoAdmin},
		},
		{
			name: "an ended lease's property and collector do not count while one is active",
			leases: []staffnotice.Lease{
				{PropertyID: kos, CollectorUserID: collector, Active: true},
				{PropertyID: ruko, CollectorUserID: otherKaryawan},
			},
			members: members,
			want:    []uuid.UUID{collector, admin, kosAdmin},
		},
		{
			name:    "a tenant who moved out is looked after through the leases they had, without a collector",
			leases:  []staffnotice.Lease{{PropertyID: ruko, CollectorUserID: collector}},
			members: members,
			want:    []uuid.UUID{admin, rukoAdmin},
		},
		{
			name:    "a tenant with no lease: only organization-wide admins",
			members: members,
			want:    []uuid.UUID{admin},
		},
		{
			name:    "a collector who is not an active member is not told",
			leases:  []staffnotice.Lease{{PropertyID: kos, CollectorUserID: disabledCollector, Active: true}},
			members: members,
			want:    []uuid.UUID{admin, kosAdmin},
		},
		{
			name:   "nobody looks after the tenant",
			leases: []staffnotice.Lease{{PropertyID: kos, Active: true}},
			members: []authz.Principal{
				principal(t, supervisor, authz.RoleSupervisor, uuid.Nil),
				principal(t, rukoAdmin, authz.RoleAdmin, ruko),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, staffnotice.Recipients(tenantID, tc.leases, tc.members))
		})
	}
}

func TestInSendingHours(t *testing.T) {
	t.Parallel()
	at := func(hour, minute int) time.Time { return time.Date(2026, 11, 2, hour, minute, 0, 0, clock.Jakarta) }
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{name: "half past midnight, when promises close", at: at(0, 30)},
		{name: "a minute before eight", at: at(7, 59)},
		{name: "eight, when reminders go out", at: at(8, 0), want: true},
		{name: "a minute before ten at night", at: at(21, 59), want: true},
		{name: "ten at night", at: at(22, 0)},
		{name: "eight in Jakarta is one in UTC", at: time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, staffnotice.InSendingHours(tc.at))
		})
	}
}
