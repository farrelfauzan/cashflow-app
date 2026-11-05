package paylink

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeBankAccount(t *testing.T) {
	t.Parallel()
	valid := BankAccount{BankName: "BCA", AccountNumber: "123 456 7890", AccountHolder: "Suhwendi"}
	with := func(f func(*BankAccount)) BankAccount {
		b := valid
		f(&b)
		return b
	}
	tests := []struct {
		name    string
		in      BankAccount
		want    BankAccount
		wantErr string
	}{
		{name: "valid", in: valid, want: valid},
		{name: "surrounding spaces are trimmed",
			in:   BankAccount{BankName: " Bank Mandiri ", AccountNumber: " 137-00-1234567-8\n", AccountHolder: "\tPT Griya Sejahtera "},
			want: BankAccount{BankName: "Bank Mandiri", AccountNumber: "137-00-1234567-8", AccountHolder: "PT Griya Sejahtera"}},
		{name: "five digits", in: with(func(b *BankAccount) { b.AccountNumber = "12345" }), want: with(func(b *BankAccount) { b.AccountNumber = "12345" })},
		{name: "34 characters", in: with(func(b *BankAccount) { b.AccountNumber = strings.Repeat("1", 34) }),
			want: with(func(b *BankAccount) { b.AccountNumber = strings.Repeat("1", 34) })},
		{name: "a holder of 100 letters", in: with(func(b *BankAccount) { b.AccountHolder = strings.Repeat("é", 100) }),
			want: with(func(b *BankAccount) { b.AccountHolder = strings.Repeat("é", 100) })},
		{name: "four digits", in: with(func(b *BankAccount) { b.AccountNumber = "1234" }), wantErr: "account number"},
		{name: "35 characters", in: with(func(b *BankAccount) { b.AccountNumber = strings.Repeat("1", 35) }), wantErr: "account number"},
		{name: "a letter in the number", in: with(func(b *BankAccount) { b.AccountNumber = "12345O789" }), wantErr: "account number"},
		{name: "a number ending in a dash", in: with(func(b *BankAccount) { b.AccountNumber = "123456-" }), wantErr: "account number"},
		{name: "a dot in the number", in: with(func(b *BankAccount) { b.AccountNumber = "123.456.789" }), wantErr: "account number"},
		{name: "no bank name", in: with(func(b *BankAccount) { b.BankName = "  " }), wantErr: "bank name"},
		{name: "a bank name of 61 characters", in: with(func(b *BankAccount) { b.BankName = strings.Repeat("B", 61) }), wantErr: "bank name"},
		{name: "no holder", in: with(func(b *BankAccount) { b.AccountHolder = "" }), wantErr: "account holder"},
		{name: "a holder of 101 characters", in: with(func(b *BankAccount) { b.AccountHolder = strings.Repeat("a", 101) }), wantErr: "account holder"},
		{name: "a newline inside the holder", in: with(func(b *BankAccount) { b.AccountHolder = "Suh\nwendi" }), wantErr: "account holder"},
		{name: "invalid UTF-8", in: with(func(b *BankAccount) { b.BankName = "B\xffCA" }), wantErr: "bank name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeBankAccount(tc.in)
			if tc.wantErr != "" {
				require.ErrorIs(t, err, ErrInvalidBankAccount)
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
