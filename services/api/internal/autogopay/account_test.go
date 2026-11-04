package autogopay

import (
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestNormalizeAPIKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "a plain key", in: "agp_live_3f9a1c0b7e", want: "agp_live_3f9a1c0b7e"},
		{name: "a trailing newline from a pipe", in: "agp_live_3f9a\n", want: "agp_live_3f9a"},
		{name: "surrounding spaces and CRLF", in: "  agp_live_3f9a \r\n", want: "agp_live_3f9a"},
		{name: "exactly 512 bytes", in: strings.Repeat("a", 512), want: strings.Repeat("a", 512)},
		{name: "empty", in: "", wantErr: true},
		{name: "only whitespace", in: " \t\n", wantErr: true},
		{name: "513 bytes", in: strings.Repeat("a", 513), wantErr: true},
		{name: "a newline inside", in: "agp_live\n3f9a", wantErr: true},
		{name: "a tab inside", in: "agp_live\t3f9a", wantErr: true},
		{name: "a NUL byte", in: "agp\x00live", wantErr: true},
		{name: "DEL", in: "agp\x7flive", wantErr: true},
		{name: "a C1 control character", in: "agp\u0085live", wantErr: true},
		{name: "invalid UTF-8", in: "agp\xfflive", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeAPIKey(tc.in)
			if tc.wantErr {
				require.ErrorIs(t, err, ErrInvalidAPIKey)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestNewWebhookID(t *testing.T) {
	t.Parallel()
	shape := regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`) // the CHECK in the migration
	seen := map[string]bool{}
	for range 100 {
		id, err := newWebhookID()
		require.NoError(t, err)
		require.Regexp(t, shape, id)
		require.False(t, seen[id], "webhook ids repeat")
		seen[id] = true
	}
}

func TestAdditionalDataNamesTheOrganization(t *testing.T) {
	t.Parallel()
	a, b := uuid.New(), uuid.New()
	require.Equal(t, "autogopay_api_key:"+a.String(), string(additionalData(a)))
	require.NotEqual(t, additionalData(a), additionalData(b))
	require.Equal(t, "/webhooks/autogopay/abc", WebhookPath("abc"))
}
