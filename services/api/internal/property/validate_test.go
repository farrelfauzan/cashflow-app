package property

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidProperty(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		in        PropertyInput
		wantField string
		want      PropertyInput
	}{
		{"trims spaces", PropertyInput{Name: "  Kos Melati ", Kind: KindKos, Address: " Jl. Melati 1 "}, "",
			PropertyInput{Name: "Kos Melati", Kind: KindKos, Address: "Jl. Melati 1"}},
		{"address is optional", PropertyInput{Name: "Ruko Godean", Kind: KindRuko}, "",
			PropertyInput{Name: "Ruko Godean", Kind: KindRuko}},
		{"name is required", PropertyInput{Name: "   ", Kind: KindKos}, "name", PropertyInput{}},
		{"name has a limit", PropertyInput{Name: strings.Repeat("a", 101), Kind: KindKos}, "name", PropertyInput{}},
		{"a 100-letter name fits", PropertyInput{Name: strings.Repeat("é", 100), Kind: KindOther}, "",
			PropertyInput{Name: strings.Repeat("é", 100), Kind: KindOther}},
		{"kind must be known", PropertyInput{Name: "Kos", Kind: "Kos"}, "kind", PropertyInput{}},
		{"address has a limit", PropertyInput{Name: "Kos", Kind: KindKos, Address: strings.Repeat("a", 301)}, "address", PropertyInput{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := validProperty(tc.in)
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

func TestValidUnit(t *testing.T) {
	t.Parallel()
	rent := func(v int64) *int64 { return &v }
	tests := []struct {
		name      string
		in        UnitInput
		wantField string
	}{
		{"label and rent", UnitInput{Label: " Kamar 11 ", DefaultRent: rent(1_250_000)}, ""},
		{"rent is optional", UnitInput{Label: "Kamar 1"}, ""},
		{"label is required", UnitInput{Label: " "}, "label"},
		{"label has a limit", UnitInput{Label: strings.Repeat("a", 51)}, "label"},
		{"rent cannot be zero", UnitInput{Label: "Kamar 1", DefaultRent: rent(0)}, "defaultRent"},
		{"rent cannot be negative", UnitInput{Label: "Kamar 1", DefaultRent: rent(-5)}, "defaultRent"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := validUnit(tc.in)
			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(tc.in.Label), got.Label)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
		})
	}
}
