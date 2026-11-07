//go:build integration

package staffnotice_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/router"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/staffnotice"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
)

const webOrigin = "https://app.test"

// noticeQueue is a fake staffnotice.Queue: it records the ids it is asked to enqueue.
type noticeQueue struct {
	mu  sync.Mutex
	ids []uuid.UUID
}

func (q *noticeQueue) EnqueueNotice(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, id)
	return nil
}

func (q *noticeQueue) enqueued() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]uuid.UUID(nil), q.ids...)
}

// telegramSent is one message the fake bot sent.
type telegramSent struct {
	chat int64
	msg  messaging.Message
}

// fakeTelegram is the bot behind the router: it records sends.
type fakeTelegram struct {
	mu   sync.Mutex
	sent []telegramSent
}

func (f *fakeTelegram) Send(_ context.Context, chat int64, m messaging.Message) (messaging.Receipt, error) {
	if _, err := messaging.RenderPlain(m); err != nil {
		return messaging.Receipt{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, telegramSent{chat: chat, msg: m})
	return messaging.Receipt{ProviderMessageID: strconv.Itoa(len(f.sent))}, nil
}

func (f *fakeTelegram) messages() []telegramSent {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]telegramSent(nil), f.sent...)
}

// noTenantLinks: no tenant has a Telegram chat.
type noTenantLinks struct{}

func (noTenantLinks) TenantChat(context.Context, uuid.UUID, uuid.UUID) (int64, bool, error) {
	return 0, false, nil
}

func (noTenantLinks) UnlinkUndeliverable(context.Context, uuid.UUID, uuid.UUID, int64) error {
	return nil
}

// staffLinks are the staff users' live links.
type staffLinks map[uuid.UUID]int64

func (s staffLinks) StaffChat(_ context.Context, user uuid.UUID) (int64, bool, error) {
	c, ok := s[user]
	return c, ok, nil
}

func (s staffLinks) UnlinkUndeliverable(context.Context, uuid.UUID, int64) error { return nil }

// logBuffer is a JSON log the test reads back.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

// world is one organization with a kos and a ruko, the members a notice may or may not go to, and
// the services wired as cmd/api and cmd/worker wire them with TELEGRAM_MESSAGES=linked: staff
// notices on, sent through the router with env.Messenger as WhatsApp.
type world struct {
	env   *testenv.Env
	org   testenv.Org
	admin authz.Principal
	kos   property.Property
	links *paylink.Service

	// kosAdmin is an Admin limited to the kos (not linked to Telegram); rukoAdmin one limited to
	// the ruko; disabledAdmin an organization-wide Admin who was disabled.
	kosAdmin, rukoAdmin, disabledAdmin member.Member
	// collector is the Karyawan who collects from every tenant here.
	collectorMember uuid.UUID
	chats           staffLinks

	queue    *noticeQueue
	telegram *fakeTelegram
	logs     *logBuffer
	promises *promise.Service
	closer   *promise.Closer
	decider  *reminder.Decider
	sender   *staffnotice.Sender
	prefix   string
	n        int
}

// Chats of the linked staff users.
const (
	adminChat      int64 = 7770000001
	collectorChat  int64 = 7770000002
	supervisorChat int64 = 7770000003
	rukoAdminChat  int64 = 7770000004
	disabledChat   int64 = 7770000005
)

func newWorld(t *testing.T, env *testenv.Env, name, prefix string) *world {
	t.Helper()
	ctx := context.Background()
	w := &world{
		env: env, org: env.CreateOrg(t, name, prefix), prefix: prefix,
		links: paylink.NewService(env.Store, env.Authz, env.Clock, webOrigin),
		queue: &noticeQueue{}, telegram: &fakeTelegram{}, logs: &logBuffer{},
	}
	w.admin = env.Principal(t, w.org.ID, w.org.Admin.UserID)
	var err error
	w.kos, err = env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Kos Melati", Kind: property.KindKos})
	require.NoError(t, err)
	ruko, err := env.Properties.Create(ctx, w.admin, property.PropertyInput{Name: "Ruko Sentosa", Kind: property.KindRuko})
	require.NoError(t, err)
	invite := func(phone, name string, grant member.RoleGrant) member.Member {
		m, err := env.Members.Invite(ctx, w.admin, member.Invite{Phone: phone, Name: name, Roles: []member.RoleGrant{grant}})
		require.NoError(t, err)
		return m
	}
	w.kosAdmin = invite(prefix+"011", "Mbak Sari", member.RoleGrant{Role: authz.RoleAdmin, PropertyID: w.kos.ID})
	w.rukoAdmin = invite(prefix+"012", "Mas Joko", member.RoleGrant{Role: authz.RoleAdmin, PropertyID: ruko.ID})
	w.disabledAdmin = invite(prefix+"013", "Pak Herman", member.RoleGrant{Role: authz.RoleAdmin})
	_, err = env.Members.SetStatus(ctx, w.admin, w.disabledAdmin.ID, member.StatusDisabled)
	require.NoError(t, err)
	w.collectorMember = env.Principal(t, w.org.ID, w.org.Karyawan.UserID).MembershipID
	w.chats = staffLinks{
		w.org.Admin.UserID: adminChat, w.org.Karyawan.UserID: collectorChat, w.org.Supervisor.UserID: supervisorChat,
		w.rukoAdmin.UserID: rukoAdminChat, w.disabledAdmin.UserID: disabledChat,
	}

	log := slog.New(slog.NewJSONHandler(w.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	notices := staffnotice.NewNotices(env.Clock, w.queue, log)
	w.promises = promise.NewService(promise.Deps{Store: env.Store, Links: w.links, Tenants: env.Billing, Clock: env.Clock, StaffNotices: notices, Log: log})
	w.closer = promise.NewCloser(env.Store, env.Clock, log, promise.NotifyStaff(notices))
	w.decider = reminder.NewDecider(env.Store, env.Clock, log, nil, reminder.NotifyStaff(notices))
	rtr := router.New(env.Messenger, w.telegram, noTenantLinks{}, w.chats, log)
	w.sender = staffnotice.NewSender(staffnotice.SenderDeps{Store: env.Store, Principals: env.Authz, Provider: rtr, Clock: env.Clock, Log: log})
	return w
}

// renter is a tenant with one monthly lease of Rp700.000 in the kos from today, collected by the
// Karyawan, its first invoice and a link to it.
type renter struct {
	tenantID uuid.UUID
	unit     string
	token    string
}

func (w *world) rent(t *testing.T, name string) renter {
	t.Helper()
	ctx := context.Background()
	w.n++
	label := fmt.Sprintf("Kamar %d", w.n)
	unit, err := w.env.Properties.CreateUnit(ctx, w.admin, w.kos.ID, property.UnitInput{Label: label, Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	tn, err := w.env.Tenants.Create(ctx, w.admin, tenant.Input{Name: name, Phone: fmt.Sprintf("%s%04d", w.prefix, 5000+w.n)})
	require.NoError(t, err)
	_, err = w.env.Tenants.CreateLease(ctx, w.admin, tenant.NewLease{
		TenantID: tn.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(w.env.Clock), CollectorMemberID: w.collectorMember},
	})
	require.NoError(t, err)
	_, err = w.env.Billing.GenerateOrg(ctx, w.org.ID)
	require.NoError(t, err)
	acct, err := w.env.Billing.Account(ctx, w.admin, tn.ID)
	require.NoError(t, err)
	require.Len(t, acct.Invoices, 1)
	link, err := w.links.Mint(ctx, w.admin, acct.Invoices[0].ID)
	require.NoError(t, err)
	return renter{tenantID: tn.ID, unit: label + ", Kos Melati", token: strings.TrimPrefix(link.URL, webOrigin+"/t/")}
}

// fromPage makes or edits r's promise on the payment page.
func (w *world) fromPage(t *testing.T, r renter, d time.Time) promise.Promise {
	t.Helper()
	p, err := w.promises.MakeFromLink(context.Background(), r.token, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &d})
	require.NoError(t, err)
	return p
}

// byAdmin makes r's promise as the organization's admin.
func (w *world) byAdmin(t *testing.T, r renter, d time.Time) promise.Promise {
	t.Helper()
	p, err := w.promises.MakeForTenant(context.Background(), w.admin, r.tenantID, promise.Input{Kind: promise.KindSpecificDate, PromisedDate: &d})
	require.NoError(t, err)
	return p
}

func (w *world) closeDue(t *testing.T, at time.Time) promise.CloseResult {
	t.Helper()
	w.env.Clock.Set(at)
	res, err := w.closer.CloseDue(context.Background(), w.org.ID)
	require.NoError(t, err)
	return res
}

func (w *world) send(t *testing.T, id uuid.UUID) staffnotice.Outcome {
	t.Helper()
	out, err := w.sender.Send(context.Background(), w.org.ID, id)
	require.NoError(t, err)
	return out
}

func (w *world) inTx(fn func(ctx context.Context, tx pgx.Tx) error) error {
	ctx := context.Background()
	return w.env.Store.WithOrgTx(ctx, w.org.ID, func(tx pgx.Tx) error { return fn(ctx, tx) })
}

// notice is one staff_notice row.
type notice struct {
	ID     uuid.UUID
	Kind   string
	Source uuid.UUID
	Status string
	SentAt *time.Time
}

// notices are the tenant's notices in the order they were recorded.
func (w *world) notices(t *testing.T, tenantID uuid.UUID) []notice {
	t.Helper()
	var out []notice
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, kind, coalesce(promise_version_id, promise_id, visit_task_id), status, sent_at
			FROM staff_notice WHERE tenant_id = $1 ORDER BY created_at, kind, id`, tenantID)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (notice, error) {
			var n notice
			err := row.Scan(&n.ID, &n.Kind, &n.Source, &n.Status, &n.SentAt)
			return n, err
		})
		return err
	}))
	return out
}

func (w *world) notice(t *testing.T, id uuid.UUID) notice {
	t.Helper()
	var n notice
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id, kind, coalesce(promise_version_id, promise_id, visit_task_id), status, sent_at
			FROM staff_notice WHERE id = $1`, id).Scan(&n.ID, &n.Kind, &n.Source, &n.Status, &n.SentAt)
	}))
	return n
}

// countNotices counts the organization's notices.
func (w *world) countNotices(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, "SELECT count(*) FROM staff_notice").Scan(&n)
	}))
	return n
}

// requireDelivered checks that the fake bot sent, since before, one message of template with
// params to the collector's chat and one to the organization-wide admin's, and nothing else: the
// kos admin is a recipient but has no link, and the others are not recipients.
func (w *world) requireDelivered(t *testing.T, before int, template string, params map[string]string) {
	t.Helper()
	sent := w.telegram.messages()[before:]
	chats := make([]int64, len(sent))
	for i, s := range sent {
		chats[i] = s.chat
		require.Equal(t, template, s.msg.Template)
		require.Equal(t, params, s.msg.Params)
		require.True(t, s.msg.TelegramOnly)
		require.Empty(t, s.msg.To, "a notice carries no phone")
		require.Empty(t, s.msg.Buttons)
		require.Equal(t, w.org.ID, s.msg.OrganizationID)
	}
	require.ElementsMatch(t, []int64{collectorChat, adminChat}, chats)
}

func wib(m time.Month, d, hour, minute int) time.Time {
	return time.Date(2026, m, d, hour, minute, 0, 0, clock.Jakarta)
}

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }

// TestPromiseMadeNotice: a promise made on the payment page records one notice, which goes on
// Telegram to the linked recipients (collector and admins of the tenant's property) and never on
// WhatsApp; a promise a member makes records none; a second delivery sends nothing; an edit makes
// the earlier version's notice stale.
func TestPromiseMadeNotice(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812851")
	rina := w.rent(t, "Rina Wulandari")
	budi := w.rent(t, "Budi Santoso")
	whatsapp := len(env.Messenger.Sent()) // the invitations

	made := w.fromPage(t, rina, day(time.November, 10))
	got := w.notices(t, rina.tenantID)
	require.Len(t, got, 1)
	require.Equal(t, notice{ID: got[0].ID, Kind: staffnotice.KindPromiseMade, Source: made.Latest().ID, Status: staffnotice.StatusPending}, got[0])
	require.Equal(t, []uuid.UUID{got[0].ID}, w.queue.enqueued(), "enqueued once the version committed")

	t.Run("a promise a member makes notifies nobody", func(t *testing.T) {
		w.byAdmin(t, budi, day(time.November, 10))
		require.Empty(t, w.notices(t, budi.tenantID))
		require.Len(t, w.queue.enqueued(), 1)
	})

	t.Run("it goes to the linked recipients on Telegram only", func(t *testing.T) {
		require.Equal(t, staffnotice.OutcomeSent, w.send(t, got[0].ID))
		w.requireDelivered(t, 0, messaging.TemplateStaffPromiseMade, map[string]string{
			"organization": "Griya Sejahtera", "tenant": "Rina Wulandari", "unit": rina.unit,
			"action": messaging.StaffPromiseMade, "amount": messaging.AllOutstanding, "date": "10 November 2026",
		})
		require.Len(t, env.Messenger.Sent(), whatsapp, "nothing on WhatsApp, not even for the unlinked kos admin")
		n := w.notice(t, got[0].ID)
		require.Equal(t, staffnotice.StatusSent, n.Status)
		require.NotNil(t, n.SentAt)
		require.True(t, env.Clock.Now().Equal(*n.SentAt))
	})

	t.Run("a second delivery sends nothing", func(t *testing.T) {
		before := len(w.telegram.messages())
		require.Equal(t, staffnotice.OutcomeSettled, w.send(t, got[0].ID))
		require.Len(t, w.telegram.messages(), before)
	})

	t.Run("an edit makes the earlier notice stale and says it changed", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 3, 9, 0))
		v2 := w.fromPage(t, rina, day(time.November, 12))
		env.Clock.Set(wib(time.November, 3, 9, 1))
		v3 := w.fromPage(t, rina, day(time.November, 14))
		got := w.notices(t, rina.tenantID)
		require.Len(t, got, 3)
		require.Equal(t, v2.Latest().ID, got[1].Source)
		require.Equal(t, v3.Latest().ID, got[2].Source)

		before := len(w.telegram.messages())
		require.Equal(t, staffnotice.OutcomeSkipped, w.send(t, got[1].ID), "edited again before it went out")
		require.Equal(t, staffnotice.StatusSkipped, w.notice(t, got[1].ID).Status)
		require.Len(t, w.telegram.messages(), before)

		require.Equal(t, staffnotice.OutcomeSent, w.send(t, got[2].ID))
		w.requireDelivered(t, before, messaging.TemplateStaffPromiseMade, map[string]string{
			"organization": "Griya Sejahtera", "tenant": "Rina Wulandari", "unit": rina.unit,
			"action": messaging.StaffPromiseEdited, "amount": messaging.AllOutstanding, "date": "14 November 2026",
		})
	})

	t.Run("the log names no chat, phone or text", func(t *testing.T) {
		logs := w.logs.String()
		require.Contains(t, logs, `"msg":"staff notice sent"`)
		require.Contains(t, logs, `"recipients":3`)
		require.Contains(t, logs, `"notLinked":1`)
		require.Contains(t, logs, "staff notice recipient not linked to Telegram")
		for _, chat := range []int64{adminChat, collectorChat, supervisorChat, rukoAdminChat, disabledChat} {
			require.NotContains(t, logs, strconv.FormatInt(chat, 10))
		}
		require.NotContains(t, logs, "Rina Wulandari")
		require.NotContains(t, logs, "janji bayar")
		require.NotContains(t, logs, w.org.Admin.Phone)
	})
}

// TestNoticesOff: with no notices given (TELEGRAM_MESSAGES=off), making, breaking and opening
// record nothing.
func TestNoticesOff(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812852")
	rina := w.rent(t, "Rina Wulandari")
	w.promises = promise.NewService(promise.Deps{Store: env.Store, Links: w.links, Tenants: env.Billing, Clock: env.Clock})
	w.closer = env.Promises
	w.decider = reminder.NewDecider(env.Store, env.Clock, env.Log, nil)

	w.fromPage(t, rina, day(time.November, 3))
	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1}, w.closeDue(t, wib(time.November, 4, 0, 30)))
	env.Clock.Set(wib(time.November, 4, 9, 0))
	w.fromPage(t, rina, day(time.November, 5))
	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1, VisitTasks: 1}, w.closeDue(t, wib(time.November, 6, 0, 30)))
	env.Clock.Set(wib(time.November, 9, 8, 0))
	res, err := w.decider.DecideOrg(context.Background(), w.org.ID)
	require.NoError(t, err)
	require.Equal(t, 1, res.VisitTasks)

	require.Zero(t, w.countNotices(t))
	require.Empty(t, w.queue.enqueued())
}

// TestBrokenAndVisitNotices: a broken promise and a visit task opened by the closer or by the
// decider each record one notice in the same transaction, and a rerun records nothing more; a
// notice whose task runs at night waits for the morning; one whose visit task was resolved
// before it went out is skipped.
func TestBrokenAndVisitNotices(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	w := newWorld(t, env, "Griya Sejahtera", "0812853")
	rina := w.rent(t, "Rina Wulandari")
	budi := w.rent(t, "Budi Santoso")
	whatsapp := len(env.Messenger.Sent())

	first := w.byAdmin(t, rina, day(time.November, 3))
	require.Equal(t, promise.CloseResult{Due: 1, Broken: 1}, w.closeDue(t, wib(time.November, 4, 0, 30)))
	got := w.notices(t, rina.tenantID)
	require.Len(t, got, 1)
	require.Equal(t, staffnotice.KindPromiseBroken, got[0].Kind)
	require.Equal(t, first.ID, got[0].Source)
	require.Equal(t, promise.CloseResult{}, w.closeDue(t, wib(time.November, 4, 0, 45)))
	require.Len(t, w.notices(t, rina.tenantID), 1, "a rerun records nothing more")

	t.Run("at night it waits for the morning sweep", func(t *testing.T) {
		require.Equal(t, staffnotice.OutcomeDeferred, w.send(t, got[0].ID))
		require.Equal(t, staffnotice.StatusPending, w.notice(t, got[0].ID).Status)
		require.Empty(t, w.telegram.messages())

		env.Clock.Set(wib(time.November, 4, 8, 0))
		pending, err := w.sender.Pending(context.Background(), w.org.ID)
		require.NoError(t, err)
		require.Equal(t, []uuid.UUID{got[0].ID}, pending)
		require.Equal(t, staffnotice.OutcomeSent, w.send(t, got[0].ID))
		w.requireDelivered(t, 0, messaging.TemplateStaffPromiseBroken, map[string]string{
			"organization": "Griya Sejahtera", "tenant": "Rina Wulandari", "unit": rina.unit, "date": "3 November 2026",
		})
		pending, err = w.sender.Pending(context.Background(), w.org.ID)
		require.NoError(t, err)
		require.Empty(t, pending)
	})

	t.Run("the second broken promise also opens a visit task", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 4, 9, 0))
		second := w.byAdmin(t, rina, day(time.November, 5))
		require.Equal(t, promise.CloseResult{Due: 1, Broken: 1, VisitTasks: 1}, w.closeDue(t, wib(time.November, 6, 0, 30)))
		require.Equal(t, promise.CloseResult{}, w.closeDue(t, wib(time.November, 6, 0, 45)))
		got := w.notices(t, rina.tenantID)
		require.Len(t, got, 3)
		require.Equal(t, staffnotice.KindPromiseBroken, got[1].Kind)
		require.Equal(t, second.ID, got[1].Source)
		require.Equal(t, staffnotice.KindVisitTask, got[2].Kind)
		require.Equal(t, w.openVisit(t, rina.tenantID), got[2].Source, "the notice names the task it opened")

		env.Clock.Set(wib(time.November, 6, 8, 0))
		before := len(w.telegram.messages())
		require.Equal(t, staffnotice.OutcomeSent, w.send(t, got[2].ID))
		w.requireDelivered(t, before, messaging.TemplateStaffVisitTask, map[string]string{
			"organization": "Griya Sejahtera", "tenant": "Rina Wulandari", "unit": rina.unit,
			"reason": "janji bayar tidak ditepati 2 kali",
		})
	})

	t.Run("the decider's h7 task records one notice, once", func(t *testing.T) {
		env.Clock.Set(wib(time.November, 9, 8, 0))
		res, err := w.decider.DecideOrg(context.Background(), w.org.ID)
		require.NoError(t, err)
		require.Equal(t, 2, res.VisitTasks, "Rina and Budi are both at H+7")
		res, err = w.decider.DecideOrg(context.Background(), w.org.ID)
		require.NoError(t, err)
		require.Zero(t, res.VisitTasks)

		got := w.notices(t, budi.tenantID)
		require.Len(t, got, 1)
		require.Equal(t, staffnotice.KindVisitTask, got[0].Kind)
		before := len(w.telegram.messages())
		require.Equal(t, staffnotice.OutcomeSent, w.send(t, got[0].ID))
		w.requireDelivered(t, before, messaging.TemplateStaffVisitTask, map[string]string{
			"organization": "Griya Sejahtera", "tenant": "Budi Santoso", "unit": budi.unit, "reason": "tunggakan lewat 7 hari",
		})
	})

	t.Run("a visit task resolved before its notice went out is skipped", func(t *testing.T) {
		got := w.notices(t, rina.tenantID)
		h7 := got[len(got)-1]
		require.Equal(t, staffnotice.KindVisitTask, h7.Kind)
		require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "UPDATE visit_task SET resolved_at = $1 WHERE id = $2", env.Clock.Now(), h7.Source)
			return err
		}))
		before := len(w.telegram.messages())
		require.Equal(t, staffnotice.OutcomeSkipped, w.send(t, h7.ID))
		require.Equal(t, staffnotice.StatusSkipped, w.notice(t, h7.ID).Status)
		require.Len(t, w.telegram.messages(), before)
	})

	require.Len(t, env.Messenger.Sent(), whatsapp, "no notice ever reached WhatsApp")
}

// openVisit is the id of the tenant's one open visit task.
func (w *world) openVisit(t *testing.T, tenantID uuid.UUID) uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	require.NoError(t, w.inTx(func(ctx context.Context, tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListOpenVisitTasks(ctx, w.org.ID)
		for _, r := range rows {
			if r.TenantID == tenantID {
				ids = append(ids, r.ID)
			}
		}
		return err
	}))
	require.Len(t, ids, 1)
	return ids[0]
}

func requireCode(t *testing.T, err error, code string) *pgconn.PgError {
	t.Helper()
	var pgErr *pgconn.PgError
	require.True(t, errors.As(err, &pgErr), "expected a Postgres error, got %v", err)
	require.Equal(t, code, pgErr.Code, pgErr.Message)
	return pgErr
}

// TestStaffNoticeSchema: notices are separated by organization, the app role can change only a
// notice's status and sent_at and delete none, and a notice names exactly the source its kind
// says, of its own tenant.
func TestStaffNoticeSchema(t *testing.T) {
	env := testenv.New(t)
	env.Clock.Set(wib(time.November, 2, 9, 0))
	a := newWorld(t, env, "Griya Sejahtera", "0812854")
	b := newWorld(t, env, "Kos Bu Tini", "0812855")
	rina := a.rent(t, "Rina Wulandari")
	budi := a.rent(t, "Budi Santoso")
	siti := b.rent(t, "Siti Rahayu")
	made := a.fromPage(t, rina, day(time.November, 10))
	budiMade := a.byAdmin(t, budi, day(time.November, 10)) // noticed by nobody, so its ids are free
	b.fromPage(t, siti, day(time.November, 10))
	rinaNotice := a.notices(t, rina.tenantID)[0]

	t.Run("each organization sees only its own notices", func(t *testing.T) {
		require.Equal(t, 1, a.countNotices(t))
		require.Equal(t, 1, b.countNotices(t))
		_, err := b.sender.Send(context.Background(), b.org.ID, rinaNotice.ID)
		require.ErrorIs(t, err, staffnotice.ErrNoticeNotFound)
	})

	t.Run("a notice cannot be written into another organization", func(t *testing.T) {
		err := b.inTx(func(ctx context.Context, tx pgx.Tx) error {
			_, err := dbgen.New(tx).InsertStaffNotice(ctx, dbgen.InsertStaffNoticeParams{
				ID: uuid.New(), OrganizationID: a.org.ID, TenantID: rina.tenantID, Kind: staffnotice.KindPromiseMade,
				PromiseVersionID: uuid.NullUUID{UUID: made.Latest().ID, Valid: true}, CreatedAt: env.Clock.Now(),
			})
			return err
		})
		requireCode(t, err, "42501") // the row-level security policy's WITH CHECK
	})

	t.Run("the app role changes only status and sent_at, and deletes nothing", func(t *testing.T) {
		for name, stmt := range map[string]string{
			"delete":           "DELETE FROM staff_notice WHERE id = $1",
			"change the kind":  "UPDATE staff_notice SET kind = 'visit_task' WHERE id = $1",
			"change the time":  "UPDATE staff_notice SET created_at = now() WHERE id = $1",
			"move the tenant":  "UPDATE staff_notice SET tenant_id = tenant_id WHERE id = $1",
			"truncate (no id)": "TRUNCATE staff_notice",
		} {
			t.Run(name, func(t *testing.T) {
				err := a.inTx(func(ctx context.Context, tx pgx.Tx) error {
					if !strings.Contains(stmt, "$1") {
						_, err := tx.Exec(ctx, stmt)
						return err
					}
					_, err := tx.Exec(ctx, stmt, rinaNotice.ID)
					return err
				})
				requireCode(t, err, "42501")
			})
		}
	})

	insert := func(mod func(p *dbgen.InsertStaffNoticeParams)) func(ctx context.Context, q *dbgen.Queries) error {
		return func(ctx context.Context, q *dbgen.Queries) error {
			p := dbgen.InsertStaffNoticeParams{
				ID: uuid.New(), OrganizationID: a.org.ID, TenantID: rina.tenantID, Kind: staffnotice.KindPromiseBroken,
				PromiseID: uuid.NullUUID{UUID: made.ID, Valid: true}, CreatedAt: env.Clock.Now(),
			}
			mod(&p)
			_, err := q.InsertStaffNotice(ctx, p)
			return err
		}
	}
	tests := []struct {
		name       string
		write      func(ctx context.Context, q *dbgen.Queries) error
		code       string
		constraint string
	}{
		{
			name:  "a kind naming another source",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) { p.Kind = staffnotice.KindVisitTask }),
			code:  "23514", constraint: "staff_notice_source",
		},
		{
			name: "two sources",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) {
				p.PromiseVersionID = uuid.NullUUID{UUID: made.Latest().ID, Valid: true}
			}),
			code: "23514", constraint: "staff_notice_source",
		},
		{
			name:  "no source",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) { p.PromiseID = uuid.NullUUID{} }),
			code:  "23514", constraint: "staff_notice_source",
		},
		{
			name:  "an unknown kind",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) { p.Kind = "payment" }),
			code:  "23514", constraint: "staff_notice_kind",
		},
		{
			name: "another tenant's promise version",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) {
				p.Kind, p.PromiseID = staffnotice.KindPromiseMade, uuid.NullUUID{}
				p.PromiseVersionID = uuid.NullUUID{UUID: budiMade.Latest().ID, Valid: true}
			}),
			code: "23503", constraint: "staff_notice_version_same_tenant",
		},
		{
			name:  "another tenant's promise",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) { p.PromiseID.UUID = budiMade.ID }),
			code:  "23503", constraint: "staff_notice_promise_same_tenant",
		},
		{
			name: "a visit task that does not exist",
			write: insert(func(p *dbgen.InsertStaffNoticeParams) {
				p.Kind, p.PromiseID, p.VisitTaskID = staffnotice.KindVisitTask, uuid.NullUUID{}, uuid.NullUUID{UUID: uuid.New(), Valid: true}
			}),
			code: "23503", constraint: "staff_notice_visit_task_same_tenant",
		},
		{
			name: "sent without a time",
			write: func(ctx context.Context, q *dbgen.Queries) error {
				_, err := q.MarkStaffNotice(ctx, dbgen.MarkStaffNoticeParams{OrganizationID: a.org.ID, ID: rinaNotice.ID, Status: staffnotice.StatusSent})
				return err
			},
			code: "23514", constraint: "staff_notice_sent_at",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := a.inTx(func(ctx context.Context, tx pgx.Tx) error { return tc.write(ctx, dbgen.New(tx)) })
			pgErr := requireCode(t, err, tc.code)
			require.Equal(t, tc.constraint, pgErr.ConstraintName)
		})
	}

	t.Run("a source already noticed records nothing more", func(t *testing.T) {
		require.NoError(t, a.inTx(func(ctx context.Context, tx pgx.Tx) error {
			n, err := dbgen.New(tx).InsertStaffNotice(ctx, dbgen.InsertStaffNoticeParams{
				ID: uuid.New(), OrganizationID: a.org.ID, TenantID: rina.tenantID, Kind: staffnotice.KindPromiseMade,
				PromiseVersionID: uuid.NullUUID{UUID: made.Latest().ID, Valid: true}, CreatedAt: env.Clock.Now(),
			})
			require.Zero(t, n)
			return err
		}))
		require.Equal(t, 1, a.countNotices(t))
	})
}
