// Package auth signs staff in with a one-time code sent over WhatsApp and keeps them signed in
// with a server-side session.
//
// Login codes are 6 digits, valid for 5 minutes and for 5 tries, and only their HMAC is
// stored. Asking for a new code cancels the previous one. Session tokens are 32 random bytes;
// only their SHA-256 is stored, so a leaked database does not leak working sessions.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/ratelimit"
)

const (
	codeTTL          = 5 * time.Minute
	codeMaxAttempts  = 5
	sessionTTL       = 30 * 24 * time.Hour
	touchEvery       = 5 * time.Minute
	limitWindow      = 15 * time.Minute
	requestsPerPhone = 3
	verifiesPerPhone = 10
	minSecretBytes   = 32
)

var (
	// ErrInvalidPhone means the phone number could not be understood.
	ErrInvalidPhone = errors.New("auth: invalid phone number")
	// ErrInvalidCode covers a wrong, expired, used or unknown code. The caller is not told
	// which, so the response reveals nothing about the account.
	ErrInvalidCode = errors.New("auth: invalid or expired code")
	// ErrRateLimited means too many attempts for this phone number.
	ErrRateLimited = errors.New("auth: too many attempts")
	// ErrUnauthenticated means there is no valid session.
	ErrUnauthenticated = errors.New("auth: not signed in")
)

// Identity is the signed-in staff user.
type Identity struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Name      string
	Phone     string
}

// Session is a new session. Token goes into the cookie and is never stored.
type Session struct {
	Token     string
	ExpiresAt time.Time
}

// Organization is one organization the staff user belongs to.
type Organization struct {
	ID          uuid.UUID
	Name        string
	DisplayName string
}

// Service handles login codes and sessions.
type Service struct {
	store   *store.Store
	msg     messaging.Provider
	limiter ratelimit.Limiter
	clock   clock.Clock
	log     *slog.Logger
	secret  []byte
}

// NewService returns a Service. secret keys the HMAC of login codes and must be at least 32
// bytes.
func NewService(st *store.Store, msg messaging.Provider, limiter ratelimit.Limiter, c clock.Clock, log *slog.Logger, secret []byte) (*Service, error) {
	if len(secret) < minSecretBytes {
		return nil, fmt.Errorf("auth: secret must be at least %d bytes", minSecretBytes)
	}
	return &Service{store: st, msg: msg, limiter: limiter, clock: c, log: log, secret: secret}, nil
}

// RequestCode sends a login code to the number if it belongs to a staff user with at least one
// active membership. It returns nil whether or not a code was sent, so the response does not
// reveal which numbers are registered.
func (s *Service) RequestCode(ctx context.Context, rawPhone string) error {
	number, err := phone.Normalize(rawPhone)
	if err != nil {
		return ErrInvalidPhone
	}
	if err := s.limit(ctx, "otp:request:"+number, requestsPerPhone); err != nil {
		return err
	}

	user, ok, err := s.activeStaffUser(ctx, number)
	if err != nil {
		return err
	}
	if !ok {
		s.log.InfoContext(ctx, "login code not sent: no active staff user", "phone", phone.Mask(number))
		return nil
	}

	code, err := newCode()
	if err != nil {
		return err
	}
	now := s.clock.Now()
	challengeID := uuid.New()
	err = s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if err := q.SupersedeOpenChallenges(ctx, dbgen.SupersedeOpenChallengesParams{Phone: number, ConsumedAt: &now}); err != nil {
			return fmt.Errorf("cancelling earlier codes: %w", err)
		}
		return q.CreateChallenge(ctx, dbgen.CreateChallengeParams{
			ID:        challengeID,
			Phone:     number,
			CodeHash:  s.codeHash(challengeID, code),
			ExpiresAt: now.Add(codeTTL),
			CreatedAt: now,
		})
	})
	if err != nil {
		return fmt.Errorf("storing login code: %w", err)
	}

	_, err = s.msg.SendTemplate(ctx, messaging.Message{
		To:       number,
		Template: messaging.TemplateLoginCode,
		Params:   map[string]string{"code": code},
	})
	if err != nil {
		return fmt.Errorf("sending login code to %s: %w", phone.Mask(number), err)
	}
	s.log.InfoContext(ctx, "login code sent", "staff_user_id", user.ID)
	return nil
}

// activeStaffUser finds the staff user for the number and reports whether they belong to at
// least one organization.
func (s *Service) activeStaffUser(ctx context.Context, number string) (dbgen.StaffUser, bool, error) {
	var user dbgen.StaffUser
	err := s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		var err error
		user, err = dbgen.New(tx).GetStaffUserByPhone(ctx, number)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return dbgen.StaffUser{}, false, nil
	}
	if err != nil {
		return dbgen.StaffUser{}, false, fmt.Errorf("finding staff user: %w", err)
	}
	orgs, err := s.Organizations(ctx, user.ID)
	if err != nil {
		return dbgen.StaffUser{}, false, err
	}
	return user, len(orgs) > 0, nil
}

// VerifyCode checks the code and, when it is right, consumes it and opens a session.
func (s *Service) VerifyCode(ctx context.Context, rawPhone, code string) (Session, Identity, error) {
	number, err := phone.Normalize(rawPhone)
	if err != nil {
		return Session{}, Identity{}, ErrInvalidCode
	}
	if err := s.limit(ctx, "otp:verify:"+number, verifiesPerPhone); err != nil {
		return Session{}, Identity{}, err
	}
	if !validCodeFormat(code) {
		return Session{}, Identity{}, ErrInvalidCode
	}

	now := s.clock.Now()
	var (
		sess     Session
		identity Identity
		accepted bool
	)
	// Every outcome commits, so a wrong guess still counts against the code.
	err = s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		ch, err := q.GetOpenChallengeForUpdate(ctx, number)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("loading login code: %w", err)
		}
		if !now.Before(ch.ExpiresAt) || ch.Attempts >= codeMaxAttempts {
			return q.ConsumeChallenge(ctx, dbgen.ConsumeChallengeParams{ID: ch.ID, ConsumedAt: &now})
		}
		if !hmac.Equal(ch.CodeHash, s.codeHash(ch.ID, code)) {
			return q.IncrementChallengeAttempts(ctx, ch.ID)
		}
		if err := q.ConsumeChallenge(ctx, dbgen.ConsumeChallengeParams{ID: ch.ID, ConsumedAt: &now}); err != nil {
			return fmt.Errorf("consuming login code: %w", err)
		}

		user, err := q.GetStaffUserByPhone(ctx, number)
		if err != nil {
			return fmt.Errorf("loading staff user: %w", err)
		}
		// Kept only the first time, so the members screen can tell an invitee who never signed in.
		if err := q.SetFirstSignIn(ctx, dbgen.SetFirstSignInParams{ID: user.ID, FirstSignInAt: &now}); err != nil {
			return fmt.Errorf("recording first sign-in: %w", err)
		}
		token, tokenHash, err := newToken()
		if err != nil {
			return err
		}
		sessionID := uuid.New()
		expires := now.Add(sessionTTL)
		err = q.CreateSession(ctx, dbgen.CreateSessionParams{
			ID: sessionID, TokenHash: tokenHash, StaffUserID: user.ID, CreatedAt: now, ExpiresAt: expires,
		})
		if err != nil {
			return fmt.Errorf("creating session: %w", err)
		}
		sess = Session{Token: token, ExpiresAt: expires}
		identity = Identity{UserID: user.ID, SessionID: sessionID, Name: user.Name, Phone: user.Phone}
		accepted = true
		return nil
	})
	if err != nil {
		return Session{}, Identity{}, err
	}
	if !accepted {
		return Session{}, Identity{}, ErrInvalidCode
	}
	return sess, identity, nil
}

// Authenticate returns the identity behind a session token, or ErrUnauthenticated.
func (s *Service) Authenticate(ctx context.Context, token string) (Identity, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return Identity{}, ErrUnauthenticated
	}
	hash := sha256.Sum256(raw)
	now := s.clock.Now()

	var identity Identity
	err = s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		row, err := q.GetActiveSession(ctx, dbgen.GetActiveSessionParams{TokenHash: hash[:], ExpiresAt: now})
		if err != nil {
			return err
		}
		identity = Identity{UserID: row.StaffUserID, SessionID: row.ID, Name: row.Name, Phone: row.Phone}
		if now.Sub(row.LastSeenAt) >= touchEvery {
			return q.TouchSession(ctx, dbgen.TouchSessionParams{ID: row.ID, LastSeenAt: now})
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, ErrUnauthenticated
	}
	if err != nil {
		return Identity{}, fmt.Errorf("loading session: %w", err)
	}
	return identity, nil
}

// Logout revokes the session.
func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	now := s.clock.Now()
	err := s.store.WithAuthTx(ctx, uuid.Nil, func(tx pgx.Tx) error {
		return dbgen.New(tx).RevokeSession(ctx, dbgen.RevokeSessionParams{ID: sessionID, RevokedAt: &now})
	})
	if err != nil {
		return fmt.Errorf("revoking session: %w", err)
	}
	return nil
}

// Organizations lists the organizations where the staff user has an active membership.
func (s *Service) Organizations(ctx context.Context, userID uuid.UUID) ([]Organization, error) {
	var out []Organization
	err := s.store.WithAuthTx(ctx, userID, func(tx pgx.Tx) error {
		rows, err := dbgen.New(tx).ListMyOrganizations(ctx, userID)
		if err != nil {
			return err
		}
		out = make([]Organization, len(rows))
		for i, r := range rows {
			out[i] = Organization{ID: r.ID, Name: r.Name, DisplayName: r.DisplayName}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("listing organizations: %w", err)
	}
	return out, nil
}

func (s *Service) limit(ctx context.Context, key string, limit int) error {
	ok, err := s.limiter.Allow(ctx, key, limit, limitWindow)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRateLimited
	}
	return nil
}

// codeHash binds the code to its challenge, so a hash cannot be replayed against another
// challenge, and keys it with the server secret, so a leaked table cannot be brute-forced
// offline.
func (s *Service) codeHash(challengeID uuid.UUID, code string) []byte {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(challengeID[:])
	mac.Write([]byte(code))
	return mac.Sum(nil)
}

func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", fmt.Errorf("generating login code: %w", err)
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func validCodeFormat(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// newToken returns a session token for the cookie and the hash to store.
func newToken() (token string, hash []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generating session token: %w", err)
	}
	sum := sha256.Sum256(raw)
	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}
