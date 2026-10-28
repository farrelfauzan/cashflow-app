package member

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
)

// Invitation states. They match the CHECK constraint on membership.invitation_state.
const (
	InvitationSent    = "sent"
	InvitationNotSent = "not_sent"
)

// Invitation sends are limited because every organization shares one WhatsApp number, which
// WhatsApp can restrict when it sends too much (CLAUDE.md, "Messaging"). Every send counts,
// the first one on invite included, whether or not WhatsApp accepts it:
//   - 20 per organization per hour, so one organization cannot burn the shared number;
//   - 3 per WhatsApp number per 24 hours, across organizations, so nobody is flooded with
//     invitations.
//
// The Redis keys are ratelimit:invite:org:<organization id> and ratelimit:invite:phone:<E.164>.
const (
	InvitesPerOrganization = 20
	InviteOrganizationSpan = time.Hour
	InvitesPerPhone        = 3
	InvitePhoneSpan        = 24 * time.Hour
)

// recordTimeout bounds recording a send's outcome once the request may be gone.
const recordTimeout = 5 * time.Second

var (
	// ErrMemberDisabled means the member is disabled, so there is no invitation to resend.
	ErrMemberDisabled = errors.New("member: disabled")
	// ErrNotInvited means the member was never invited (an organization founder).
	ErrNotInvited = errors.New("member: never invited")
	// ErrInviteRateLimited means the organization or the number has had too many invitation
	// sends; nothing was sent.
	ErrInviteRateLimited = errors.New("member: too many invitation sends")
	// ErrSendFailed means WhatsApp did not take the invitation. It wraps the provider's error.
	ErrSendFailed = errors.New("member: invitation not sent")

	errNoMessenger = errors.New("no WhatsApp provider configured")
)

// Invitation is the WhatsApp invitation of a member who was invited.
type Invitation struct {
	State string
	// LastSentAt is the last send that reached WhatsApp, or nil if none did.
	LastSentAt *time.Time
}

// Invitations is what sending invitations needs. LoginURL is the web login page linked from
// the message; it holds no token. A nil Messenger sends nothing and leaves every invitation
// not_sent; only offline tools such as the demo seed use that.
type Invitations struct {
	Messenger messaging.Provider
	Limiter   ratelimit.Limiter
	Clock     clock.Clock
	Log       *slog.Logger
	LoginURL  string
}

// ResendInvitation sends a member's invitation again. It needs member:invite (audited). A
// disabled member gives ErrMemberDisabled and a founder ErrNotInvited; nothing is sent then.
// The rate limits give ErrInviteRateLimited and leave the state as it was; a failed send gives
// ErrSendFailed and leaves the state not_sent.
func (s *Service) ResendInvitation(ctx context.Context, p authz.Principal, membershipID uuid.UUID) (Member, error) {
	var (
		m      Member
		params map[string]string
	)
	err := s.store.WithOrgTx(ctx, p.OrganizationID, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		current, err := load(ctx, q, p.OrganizationID, membershipID)
		if err != nil {
			return err
		}
		req := authz.Request{Action: authz.ActionMemberInvite, Resource: authz.Resource{
			Type: "member", ID: membershipID.String(), OwnerID: current.UserID,
		}}
		if err := s.authz.Authorize(ctx, tx, p, req); err != nil {
			return err
		}
		if current.Status == StatusDisabled {
			return ErrMemberDisabled
		}
		if current.Invitation == nil {
			return ErrNotInvited
		}
		params, err = invitationParams(ctx, q, p, current)
		m = current
		return err
	})
	if err != nil {
		return Member{}, err
	}
	return s.deliver(ctx, p.OrganizationID, m, params)
}

// deliver sends the invitation and records the outcome on the membership. It runs after the
// transaction that authorized it has committed, never inside it, so a slow or failing
// WhatsApp never holds a database transaction open or rolls back an invite.
//
// The send is synchronous, like the login code: there is no queue task for invitations yet,
// and the project forbids goroutines in request handlers. The caller learns the outcome at once
// and the members screen offers a resend when it is not_sent.
func (s *Service) deliver(ctx context.Context, org uuid.UUID, m Member, params map[string]string) (Member, error) {
	if s.inv.Messenger == nil {
		s.logSend(ctx, org, m, "not_sent_no_provider")
		return m, fmt.Errorf("%w: %w", ErrSendFailed, errNoMessenger)
	}
	if err := s.allowSend(ctx, org, m.Phone); err != nil {
		outcome := "not_sent_rate_limited"
		if !errors.Is(err, ErrInviteRateLimited) {
			outcome = "not_sent_limiter_error"
		}
		s.logSend(ctx, org, m, outcome)
		return m, err
	}

	_, sendErr := s.inv.Messenger.SendTemplate(ctx, messaging.Message{
		OrganizationID: org,
		To:             m.Phone,
		Template:       messaging.TemplateMemberInvite,
		Params:         params,
		Buttons:        []messaging.Button{{Title: "Masuk", Payload: messaging.TemplateMemberInvite, URL: s.inv.LoginURL}},
	})
	state, outcome, sentAt := InvitationSent, "sent", (*time.Time)(nil)
	if sendErr != nil {
		state, outcome = InvitationNotSent, "not_sent_"+sendFailure(sendErr)
	} else {
		now := s.inv.Clock.Now()
		sentAt = &now
	}
	s.logSend(ctx, org, m, outcome, sendErrAttrs(sendErr)...)

	updated, err := s.recordInvitation(ctx, org, m.ID, state, sentAt)
	if err != nil {
		// The message went out (or not) either way; the response must not claim otherwise.
		s.inv.Log.ErrorContext(ctx, "recording member invitation outcome", "organization_id", org,
			"membership_id", m.ID, "outcome", outcome, "error", err)
		next := Invitation{State: state, LastSentAt: sentAt}
		if sentAt == nil && m.Invitation != nil {
			next.LastSentAt = m.Invitation.LastSentAt
		}
		m.Invitation = &next
		updated = m
	}
	if sendErr != nil {
		return updated, fmt.Errorf("%w: %w", ErrSendFailed, sendErr)
	}
	return updated, nil
}

// allowSend counts one send against the organization, then against the number. A send the
// organization's limit refuses does not count against the number.
func (s *Service) allowSend(ctx context.Context, org uuid.UUID, number string) error {
	limits := []struct {
		key   string
		limit int
		span  time.Duration
	}{
		{"invite:org:" + org.String(), InvitesPerOrganization, InviteOrganizationSpan},
		{"invite:phone:" + number, InvitesPerPhone, InvitePhoneSpan},
	}
	for _, l := range limits {
		ok, err := s.inv.Limiter.Allow(ctx, l.key, l.limit, l.span)
		if err != nil {
			return fmt.Errorf("checking invitation limit: %w", err)
		}
		if !ok {
			return ErrInviteRateLimited
		}
	}
	return nil
}

// recordInvitation stores a send's outcome in its own small organization transaction. It
// survives a client that hung up after the send, so the state matches what happened.
func (s *Service) recordInvitation(ctx context.Context, org, membershipID uuid.UUID, state string, sentAt *time.Time) (Member, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recordTimeout)
	defer cancel()
	var m Member
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := setInvitation(ctx, q, org, membershipID, state, sentAt); err != nil {
			return err
		}
		var err error
		m, err = load(ctx, q, org, membershipID)
		return err
	})
	return m, err
}

func setInvitation(ctx context.Context, q *dbgen.Queries, org, membershipID uuid.UUID, state string, sentAt *time.Time) error {
	n, err := q.SetMembershipInvitation(ctx, dbgen.SetMembershipInvitationParams{
		OrganizationID: org, ID: membershipID, State: state, LastSentAt: sentAt,
	})
	if err != nil {
		return fmt.Errorf("recording invitation: %w", err)
	}
	if n != 1 {
		return ErrNotFound
	}
	return nil
}

// logSend writes one line per invitation send. message_log is slice 5; until then this is the
// record. The number is masked, and the provider's error text is left out because it can
// quote the number.
func (s *Service) logSend(ctx context.Context, org uuid.UUID, m Member, outcome string, extra ...any) {
	attrs := append([]any{
		"organization_id", org, "membership_id", m.ID, "template", messaging.TemplateMemberInvite,
		"outcome", outcome, "phone", phone.Mask(m.Phone),
	}, extra...)
	s.inv.Log.InfoContext(ctx, "member invitation", attrs...)
}

// sendFailure names a provider error for the log.
func sendFailure(err error) string {
	switch {
	case errors.Is(err, messaging.ErrNotConnected):
		return "not_connected"
	case errors.Is(err, messaging.ErrRateLimited):
		return "whatsapp_rate_limited"
	case errors.Is(err, messaging.ErrUndeliverable):
		return "undeliverable"
	default:
		return "provider_error"
	}
}

// sendErrAttrs keeps the error text of failures that are not one of the provider's known
// answers (a timeout, a bad response), which do not carry the number.
func sendErrAttrs(err error) []any {
	if err == nil || sendFailure(err) != "provider_error" {
		return nil
	}
	return []any{"error", err}
}

// invitationParams fills the member_invite template: the invitee's name, their roles in
// Indonesian with property limits in brackets, the organization's display name and the
// inviter's name.
func invitationParams(ctx context.Context, q *dbgen.Queries, p authz.Principal, m Member) (map[string]string, error) {
	org, err := q.GetOrganization(ctx, p.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("loading organization: %w", err)
	}
	inviter, err := q.GetMembership(ctx, dbgen.GetMembershipParams{OrganizationID: p.OrganizationID, ID: p.MembershipID})
	if err != nil {
		return nil, fmt.Errorf("loading inviter: %w", err)
	}
	roles, err := roleText(ctx, q, p.OrganizationID, m.Roles)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"name":         m.Name,
		"role":         roles,
		"organization": org.DisplayName,
		"inviter":      inviter.Name,
	}, nil
}

// roleText describes roles for the invitation, for example "Admin (Kos Melati, Kos Mawar)
// dan Karyawan". A role held for the whole organization shows no properties.
func roleText(ctx context.Context, q *dbgen.Queries, org uuid.UUID, roles []RoleRef) (string, error) {
	type entry struct {
		name       string
		orgWide    bool
		properties []string
	}
	var entries []*entry
	byKey := map[string]*entry{}
	for _, r := range roles {
		e, ok := byKey[r.Key]
		if !ok {
			e = &entry{name: r.Name}
			byKey[r.Key] = e
			entries = append(entries, e)
		}
		if r.PropertyID == uuid.Nil {
			e.orgWide = true
			continue
		}
		prop, err := q.GetProperty(ctx, dbgen.GetPropertyParams{OrganizationID: org, ID: r.PropertyID})
		if err != nil {
			return "", fmt.Errorf("loading property name: %w", err)
		}
		if !slices.Contains(e.properties, prop.Name) {
			e.properties = append(e.properties, prop.Name)
		}
	}
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = e.name
		if !e.orgWide && len(e.properties) > 0 {
			parts[i] += " (" + strings.Join(e.properties, ", ") + ")"
		}
	}
	return joinIndonesian(parts), nil
}

// joinIndonesian joins "a", "b" and "c" as "a, b dan c".
func joinIndonesian(parts []string) string {
	if len(parts) <= 1 {
		return strings.Join(parts, "")
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " dan " + parts[len(parts)-1]
}

func invitationFrom(state *string, lastSentAt *time.Time) *Invitation {
	if state == nil {
		return nil
	}
	return &Invitation{State: *state, LastSentAt: lastSentAt}
}

// NeedsOperator reports whether a send error means an operator should look: the WhatsApp
// session is logged out, WhatsApp is throttling the shared number, the provider failed in an
// unexpected way, or the rate limiter itself failed. A number that cannot receive WhatsApp or
// one of our own limits is not an operator problem.
func NeedsOperator(err error) bool {
	if err == nil || errors.Is(err, ErrInviteRateLimited) || errors.Is(err, messaging.ErrUndeliverable) ||
		errors.Is(err, errNoMessenger) {
		return false
	}
	return true
}

// failureReason names a delivery error for the log without its text, which can quote the
// number.
func failureReason(err error) string {
	if errors.Is(err, ErrSendFailed) {
		return sendFailure(err)
	}
	return "limiter_error"
}
