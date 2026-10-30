package property

import (
	"strings"
	"testing"
	"time"

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
			got, err := ValidProperty(tc.in)
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
		wantMsg   string
		wantTerms []int // the months after validation, when it passes
	}{
		{"label and a priced term", UnitInput{Label: " Kamar 11 ", Terms: []TermInput{{Months: 1, Price: rent(1_250_000)}}}, "", "", []int{1}},
		{"terms are optional", UnitInput{Label: "Kamar 1"}, "", "", nil},
		{"a term without a price", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 12}}}, "", "", []int{12}},
		{"terms come back sorted", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 12}, {Months: 1}, {Months: 6}}}, "", "", []int{1, 6, 12}},
		{"label is required", UnitInput{Label: " "}, "label", "", nil},
		{"label has a limit", UnitInput{Label: strings.Repeat("a", 51)}, "label", "", nil},
		{"an empty list of terms", UnitInput{Label: "Kamar 1", Terms: []TermInput{}}, "terms", "Pilih paling sedikit satu jangka sewa.", nil},
		{"a zero term", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 0}}}, "terms", "Jangka sewa harus 1 sampai 60 bulan.", nil},
		{"a custom term over 60 months", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 1}, {Months: 61}}}, "terms", "Jangka sewa harus 1 sampai 60 bulan.", nil},
		{"a custom term alone", UnitInput{Label: "Ruko 1", Terms: []TermInput{{Months: 24}}}, "", "", []int{24}},
		{"a custom term beside the standard ones, sorted", UnitInput{Label: "Ruko 1", Terms: []TermInput{{Months: 12}, {Months: 60}, {Months: 1}, {Months: 6}}}, "", "", []int{1, 6, 12, 60}},
		{"two custom terms", UnitInput{Label: "Ruko 1", Terms: []TermInput{{Months: 2}, {Months: 12}, {Months: 24}}}, "terms", "Selain 1, 3, 6 dan 12 bulan, unit hanya bisa menawarkan satu jangka sewa lain.", nil},
		{"a repeated custom term", UnitInput{Label: "Ruko 1", Terms: []TermInput{{Months: 24}, {Months: 24}}}, "terms", "Jangka sewa 24 bulan tercantum dua kali.", nil},
		{"a custom term typed as a standard one is that term repeated", UnitInput{Label: "Ruko 1", Terms: []TermInput{{Months: 12}, {Months: 12}}}, "terms", "Jangka sewa 12 bulan tercantum dua kali.", nil},
		{"a repeated term", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 3}, {Months: 3}}}, "terms", "Jangka sewa 3 bulan tercantum dua kali.", nil},
		{"a zero price", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 1, Price: rent(0)}}}, "terms", "Harga sewa harus lebih dari Rp0.", nil},
		{"a negative price", UnitInput{Label: "Kamar 1", Terms: []TermInput{{Months: 1, Price: rent(-5)}}}, "terms", "Harga sewa harus lebih dari Rp0.", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidUnit(tc.in)
			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(tc.in.Label), got.Label)
				var months []int
				for _, term := range got.Terms {
					months = append(months, term.Months)
				}
				require.Equal(t, tc.wantTerms, months)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
			if tc.wantMsg != "" {
				require.Equal(t, tc.wantMsg, inputErr.Message)
			}
		})
	}
}

func TestValidPrice(t *testing.T) {
	t.Parallel()
	from := time.Date(2027, 1, 1, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		in        PriceInput
		wantField string
	}{
		{"a valid price", PriceInput{Months: 12, Amount: 16_000_000, EffectiveFrom: from}, ""},
		{"a term over 60 months", PriceInput{Months: 61, Amount: 1, EffectiveFrom: from}, "months"},
		{"a custom term", PriceInput{Months: 24, Amount: 30_000_000, EffectiveFrom: from}, ""},
		{"no term", PriceInput{Amount: 1, EffectiveFrom: from}, "months"},
		{"a zero amount", PriceInput{Months: 1, EffectiveFrom: from}, "amount"},
		{"no date", PriceInput{Months: 1, Amount: 1}, "effectiveFrom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ValidPrice(tc.in)
			if tc.wantField == "" {
				require.NoError(t, err)
				require.Equal(t, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), got.EffectiveFrom)
				return
			}
			var inputErr *InputError
			require.ErrorAs(t, err, &inputErr)
			require.Equal(t, tc.wantField, inputErr.Field)
		})
	}
}

func TestDefaultTerms(t *testing.T) {
	t.Parallel()
	for kind, want := range map[string][]int{KindRuko: {12}, KindKos: {1}, KindKontrakan: {1}, KindOther: {1}} {
		require.Equal(t, want, DefaultTerms(kind), kind)
	}
}
