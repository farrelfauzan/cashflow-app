package paylink

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
)

var (
	// ErrInvalidBankAccount means a bank account field is empty, too long or malformed. The error
	// returned wraps it and says which field.
	ErrInvalidBankAccount = errors.New("paylink: invalid bank account")
	// ErrOrganizationNotFound means no organization has that id.
	ErrOrganizationNotFound = errors.New("paylink: organization not found")
)

// BankAccount is where a tenant transfers by hand (decision 38). The page shows it and asks the
// tenant to write their name in the transfer note; the admin records the transfer as a manual
// payment.
type BankAccount struct {
	BankName      string // "BCA", "Bank Mandiri"
	AccountNumber string // as printed on the bank book: digits, spaces or '-'
	AccountHolder string
}

// accountNumberShape mirrors the organization_bank_account_number check: 5 to 34 characters of
// digits, spaces and '-', starting and ending with a digit.
var accountNumberShape = regexp.MustCompile(`^[0-9][0-9 -]{3,32}[0-9]$`)

// bankAccountAuditAction is how setting the bank account appears in the audit log, the same
// catalog action a settings screen will check.
const bankAccountAuditAction = "settings:update"

// NormalizeBankAccount trims each field and checks it as the database will: a bank name of 1 to
// 60 characters, an account number as accountNumberShape, a holder of 1 to 100 characters, and
// no control characters anywhere.
func NormalizeBankAccount(in BankAccount) (BankAccount, error) {
	out := BankAccount{
		BankName:      strings.TrimSpace(in.BankName),
		AccountNumber: strings.TrimSpace(in.AccountNumber),
		AccountHolder: strings.TrimSpace(in.AccountHolder),
	}
	if !validText(out.BankName, 60) {
		return BankAccount{}, fmt.Errorf("%w: the bank name must be 1 to 60 characters of text", ErrInvalidBankAccount)
	}
	if !accountNumberShape.MatchString(out.AccountNumber) {
		return BankAccount{}, fmt.Errorf("%w: the account number must be 5 to 34 digits, spaces or '-', starting and ending with a digit",
			ErrInvalidBankAccount)
	}
	if !validText(out.AccountHolder, 100) {
		return BankAccount{}, fmt.Errorf("%w: the account holder must be 1 to 100 characters of text", ErrInvalidBankAccount)
	}
	return out, nil
}

func validText(s string, maxRunes int) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= 1 && n <= maxRunes && strings.IndexFunc(s, unicode.IsControl) < 0
}

// SetBankAccount stores org's bank account, replacing any earlier one, and returns what was
// stored. It is the platform operator's action (cmd/admin) until the settings screen exists, so
// it checks no member and is recorded in the organization's audit log with no actor.
func (s *Service) SetBankAccount(ctx context.Context, org uuid.UUID, in BankAccount) (BankAccount, error) {
	in, err := NormalizeBankAccount(in)
	if err != nil {
		return BankAccount{}, err
	}
	if org == uuid.Nil {
		return BankAccount{}, ErrOrganizationNotFound
	}
	var out BankAccount
	err = s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		q := dbgen.New(tx)
		if _, err := q.GetOrganization(ctx, org); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrganizationNotFound
			}
			return fmt.Errorf("paylink: reading the organization: %w", err)
		}
		row, err := q.UpsertBankAccount(ctx, dbgen.UpsertBankAccountParams{
			OrganizationID: org, BankName: in.BankName, AccountNumber: in.AccountNumber, AccountHolder: in.AccountHolder,
			Now: s.clock.Now(),
		})
		if err != nil {
			return fmt.Errorf("paylink: storing the bank account: %w", err)
		}
		out = BankAccount{BankName: row.BankName, AccountNumber: row.AccountNumber, AccountHolder: row.AccountHolder}
		return s.authz.RecordSystemAction(ctx, tx, org, bankAccountAuditAction,
			authz.Resource{Type: "organization_bank_account", ID: org.String()})
	})
	if err != nil {
		return BankAccount{}, err
	}
	return out, nil
}

// BankAccount returns org's bank account, or nil when none is set. It checks no member: like
// SetBankAccount it serves the platform operator's tools (the demo seed).
func (s *Service) BankAccount(ctx context.Context, org uuid.UUID) (*BankAccount, error) {
	var out *BankAccount
	err := s.store.WithOrgTx(ctx, org, func(tx pgx.Tx) error {
		var err error
		out, err = loadBankAccount(ctx, dbgen.New(tx), org)
		return err
	})
	return out, err
}

// loadBankAccount reads org's bank account in q's scope; nil when none is set.
func loadBankAccount(ctx context.Context, q *dbgen.Queries, org uuid.UUID) (*BankAccount, error) {
	row, err := q.GetBankAccount(ctx, org)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // no bank account yet is a normal state, shown as null
	}
	if err != nil {
		return nil, fmt.Errorf("paylink: loading the bank account: %w", err)
	}
	return &BankAccount{BankName: row.BankName, AccountNumber: row.AccountNumber, AccountHolder: row.AccountHolder}, nil
}
