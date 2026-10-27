package tenant

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidChargeType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		in        ChargeTypeInput
		want      ChargeTypeInput
		wantField string
	}{
		{"trims the name", ChargeTypeInput{Name: "  Wi-Fi "}, ChargeTypeInput{Name: "Wi-Fi"}, ""},
		{"keeps metered", ChargeTypeInput{Name: "Listrik", Metered: true}, ChargeTypeInput{Name: "Listrik", Metered: true}, ""},
		{"fifty letters", ChargeTypeInput{Name: strings.Repeat("é", 50)}, ChargeTypeInput{Name: strings.Repeat("é", 50)}, ""},
		{"empty name", ChargeTypeInput{Name: ""}, ChargeTypeInput{}, "name"},
		{"only spaces", ChargeTypeInput{Name: "   "}, ChargeTypeInput{}, "name"},
		{"fifty-one letters", ChargeTypeInput{Name: strings.Repeat("a", 51)}, ChargeTypeInput{}, "name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidChargeType(tc.in)
			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, tc.want, got)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
		})
	}
}

func TestValidCharges(t *testing.T) {
	t.Parallel()
	wifi, parkir := uuid.New(), uuid.New()
	many := make([]ChargeInput, maxLeaseCharges+1)
	for i := range many {
		many[i] = ChargeInput{ChargeTypeID: uuid.New(), Amount: 1}
	}
	tests := []struct {
		name      string
		in        []ChargeInput
		wantField string
	}{
		{"no charges", nil, ""},
		{"empty list", []ChargeInput{}, ""},
		{"two charges", []ChargeInput{{wifi, 50_000}, {parkir, 100_000}}, ""},
		{"the most charges", many[:maxLeaseCharges], ""},
		{"too many charges", many, "charges"},
		{"no charge type", []ChargeInput{{uuid.Nil, 50_000}}, "charges[0].chargeTypeId"},
		{"the same type twice", []ChargeInput{{wifi, 50_000}, {parkir, 1}, {wifi, 60_000}}, "charges[2].chargeTypeId"},
		{"a free charge", []ChargeInput{{wifi, 50_000}, {parkir, 0}}, "charges[1].amount"},
		{"a negative charge", []ChargeInput{{wifi, -1}}, "charges[0].amount"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidCharges(tc.in)
			if tc.wantField == "" {
				require.NoError(t, err)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
		})
	}
}
