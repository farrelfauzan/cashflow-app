package phone

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"081234567890", "+6281234567890"},
		{"0812-3456-7890", "+6281234567890"},
		{"0812 3456 7890", "+6281234567890"},
		{"(0812) 3456.7890", "+6281234567890"},
		{"81234567890", "+6281234567890"},
		{"6281234567890", "+6281234567890"},
		{"+62 812 3456 7890", "+6281234567890"},
		{"  +6281234567890  ", "+6281234567890"},
		{"0812345678", "+62812345678"},       // shortest mobile: 9 national digits
		{"0812345678901", "+62812345678901"}, // longest mobile: 12 national digits
		{"+60123456789", "+60123456789"},     // Malaysia, given with +
		{"+6591234567", "+6591234567"},       // Singapore
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := Normalize(tc.in)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNormalizeRejects(t *testing.T) {
	for _, in := range []string{
		"",
		"abc",
		"021-5551234",        // Jakarta landline, not a WhatsApp mobile
		"+62 0812 3456 7890", // national 0 kept after the country code
		"+0812345678",        // country code cannot start with 0
		"08123",              // too short
		"08123456789012345",  // too long
		"0812-3456-789x",     // letters
		"60123456789",        // foreign number without +
		"+12",                // too short internationally
		"+1234567890123456",  // too long internationally
	} {
		t.Run(in, func(t *testing.T) {
			_, err := Normalize(in)
			require.ErrorIs(t, err, ErrInvalid)
		})
	}
}

func TestMask(t *testing.T) {
	require.Equal(t, "+62812*****890", Mask("+6281234567890"))
	require.Equal(t, "****", Mask("+621"))
}
