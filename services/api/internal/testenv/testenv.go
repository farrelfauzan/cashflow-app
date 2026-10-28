//go:build integration

// Package testenv builds a migrated database and the domain services on top of it, with fake
// clock, messaging and rate limiting, for integration tests across packages.
package testenv

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/messagingtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/pgtest"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenantimport"
)

// LoginURL is the web login page linked from member invitations in tests.
const LoginURL = "https://app.test/login"

// Secret is the login-code key used in tests.
var Secret = []byte("test-secret-test-secret-test-secret")

// Env is a migrated database with every service wired to fakes.
type Env struct {
	URLs       pgtest.URLs
	Store      *store.Store
	Clock      *clock.Fake
	Messenger  *messagingtest.Fake
	Limiter    *ratelimit.Memory
	Log        *slog.Logger
	Auth       *auth.Service
	Authz      *authz.Service
	Members    *member.Service
	Orgs       *org.Service
	Properties *property.Service
	Tenants    *tenant.Service
	Imports    *tenantimport.Service
}

// New starts PostgreSQL, applies the migrations and wires the services.
func New(t *testing.T) *Env {
	t.Helper()
	ctx := context.Background()
	urls := pgtest.Start(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, store.Migrate(ctx, urls.Owner, log))

	st, err := store.Open(ctx, urls.App, log)
	require.NoError(t, err)
	t.Cleanup(st.Close)

	e := &Env{
		URLs:      urls,
		Store:     st,
		Clock:     clock.NewFake(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta)),
		Messenger: &messagingtest.Fake{},
		Log:       log,
	}
	e.Limiter = ratelimit.NewMemory(e.Clock)
	e.Auth, err = auth.NewService(st, e.Messenger, e.Limiter, e.Clock, log, Secret)
	require.NoError(t, err)
	e.Authz = authz.NewService(st, e.Clock, log)
	e.Members = member.NewService(st, e.Authz, member.Invitations{
		Messenger: e.Messenger, Limiter: e.Limiter, Clock: e.Clock, Log: log, LoginURL: LoginURL,
	})
	e.Orgs = org.NewService(st, e.Authz)
	e.Properties = property.NewService(st, e.Authz)
	e.Tenants = tenant.NewService(st, e.Authz, e.Clock)
	e.Imports = tenantimport.NewService(st, e.Authz, e.Properties, e.Tenants)
	return e
}

// Person is a staff member created by CreateOrg.
type Person struct {
	Phone  string
	Name   string
	UserID uuid.UUID
}

// Org is an organization created by CreateOrg, with one person per built-in role.
type Org struct {
	ID         uuid.UUID
	Supervisor Person
	Admin      Person
	Karyawan   Person
}

// CreateOrg creates an organization whose Supervisor, Admin and Karyawan have numbers that start
// with the given prefix, for example "0812111" gives 0812111001, 0812111002 and 0812111003.
func (e *Env) CreateOrg(t *testing.T, name, prefix string) Org {
	t.Helper()
	ctx := context.Background()
	o := Org{
		Supervisor: Person{Phone: prefix + "001", Name: "Bu Ani " + name},
		Admin:      Person{Phone: prefix + "002", Name: "Pak Budi " + name},
		Karyawan:   Person{Phone: prefix + "003", Name: "Mas Dimas " + name},
	}
	id, err := e.Orgs.Create(ctx, org.New{
		Name: name, DisplayName: name,
		Members: []org.NewMember{
			{Phone: o.Supervisor.Phone, Name: o.Supervisor.Name, Roles: []string{authz.RoleSupervisor}},
			{Phone: o.Admin.Phone, Name: o.Admin.Name, Roles: []string{authz.RoleAdmin}},
			{Phone: o.Karyawan.Phone, Name: o.Karyawan.Name, Roles: []string{authz.RoleKaryawan}},
		},
	})
	require.NoError(t, err)
	o.ID = id
	for _, p := range []*Person{&o.Supervisor, &o.Admin, &o.Karyawan} {
		p.UserID = e.UserID(t, p.Phone)
	}
	return o
}

// UserID returns the staff user id for a phone number in any common format.
func (e *Env) UserID(t *testing.T, rawPhone string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := e.Store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		u, err := dbgen.New(tx).GetStaffUserByPhone(ctx, normalize(t, rawPhone))
		id = u.ID
		return err
	})
	require.NoError(t, err)
	return id
}

// Principal loads a staff user's principal in an organization.
func (e *Env) Principal(t *testing.T, orgID, userID uuid.UUID) authz.Principal {
	t.Helper()
	p, err := e.Authz.LoadPrincipal(context.Background(), orgID, userID)
	require.NoError(t, err)
	return p
}

// LastCode returns the login code in the most recent message the fake sent.
func (e *Env) LastCode(t *testing.T) string {
	t.Helper()
	sent := e.Messenger.Sent()
	require.NotEmpty(t, sent, "no message was sent")
	last := sent[len(sent)-1]
	require.Equal(t, messaging.TemplateLoginCode, last.Template)
	return last.Params["code"]
}

// AuditLog returns the organization's audit rows, newest first.
func (e *Env) AuditLog(t *testing.T, orgID uuid.UUID) []dbgen.AuditLog {
	t.Helper()
	ctx := context.Background()
	var rows []dbgen.AuditLog
	err := e.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		var err error
		rows, err = dbgen.New(tx).ListAuditLog(ctx, dbgen.ListAuditLogParams{OrganizationID: orgID, Limit: 100})
		return err
	})
	require.NoError(t, err)
	return rows
}

// GrantCustomRole stores a custom role with the given policy document and gives it to a
// membership, as the role editor will once it exists.
func (e *Env) GrantCustomRole(t *testing.T, orgID, membershipID uuid.UUID, name, policy string) {
	t.Helper()
	ctx := context.Background()
	err := e.Store.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		role, err := q.CreateRole(ctx, dbgen.CreateRoleParams{ID: uuid.New(), OrganizationID: orgID, Name: name, Policy: []byte(policy)})
		if err != nil {
			return err
		}
		return q.CreateCustomRoleAssignment(ctx, dbgen.CreateCustomRoleAssignmentParams{
			ID: uuid.New(), OrganizationID: orgID, MembershipID: membershipID, RoleID: uuid.NullUUID{UUID: role.ID, Valid: true},
		})
	})
	require.NoError(t, err)
}

// WaitUntilBlockedOn polls pg_locks from the holding transaction until another session waits on
// that transaction's id, which is how Postgres queues a second locker of a row it locked or
// changed. Tests use it to make a race deterministic.
func WaitUntilBlockedOn(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var blocked bool
		err := tx.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_locks
			WHERE locktype = 'transactionid' AND NOT granted AND transactionid = xid(pg_current_xact_id()))`).Scan(&blocked)
		require.NoError(t, err)
		if blocked {
			return
		}
		require.True(t, time.Now().Before(deadline), "the second change never waited on the locked row")
		time.Sleep(10 * time.Millisecond)
	}
}
