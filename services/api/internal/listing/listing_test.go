package listing_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
)

func ptr(s string) *string { return &s }

func TestNewSearch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		q    string
		want listing.Search
	}{
		{name: "empty is no search", q: "", want: listing.Search{}},
		{name: "blank is no search", q: "   ", want: listing.Search{}},
		{name: "a name matches text only", q: " Rina ", want: listing.Search{Text: ptr("%Rina%")}},
		{name: "LIKE characters are escaped", q: `50%_a\b`, want: listing.Search{Text: ptr(`%50\%\_a\\b%`)}},
		{
			name: "a national number", q: "0812-3456",
			want: listing.Search{Text: ptr("%0812-3456%"), PhoneNational: ptr("%08123456%"), PhoneDigits: ptr("%08123456%")},
		},
		{
			name: "a number with the country code", q: "+62 812 34",
			want: listing.Search{Text: ptr("%+62 812 34%"), PhoneNational: ptr("%081234%"), PhoneDigits: ptr("%6281234%")},
		},
		{
			name: "62 without a plus may be part of the number", q: "6281",
			want: listing.Search{Text: ptr("%6281%"), PhoneNational: ptr("%081%"), PhoneDigits: ptr("%6281%")},
		},
		{name: "fewer than three digits is not a phone search", q: "08", want: listing.Search{Text: ptr("%08%")}},
		{name: "letters and digits is not a phone search", q: "Kamar 12", want: listing.Search{Text: ptr("%Kamar 12%")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, listing.NewSearch(tc.q))
		})
	}
}

func TestPageLimitAndOffset(t *testing.T) {
	t.Parallel()
	p := listing.Page{Number: 3, Size: 20}
	require.Equal(t, int32(20), p.Limit())
	require.Equal(t, int32(40), p.Offset())
	require.Equal(t, int32(0), listing.FirstPage.Offset())
}
