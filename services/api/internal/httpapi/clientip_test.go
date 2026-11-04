package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClientIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		remoteAddr string
		xff        []string // one entry per header line
		want       string
	}{
		{name: "no X-Forwarded-For: the TCP peer", remoteAddr: "203.0.113.7:51234", want: "203.0.113.7"},
		{name: "no X-Forwarded-For, IPv6 peer", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
		{name: "one entry, from Caddy", remoteAddr: "172.18.0.5:40000", xff: []string{"203.0.113.7"}, want: "203.0.113.7"},
		{name: "several entries: the rightmost, which Caddy appended", remoteAddr: "172.18.0.5:40000",
			xff: []string{"10.0.0.1, 198.51.100.2, 203.0.113.7"}, want: "203.0.113.7"},
		{name: "a forged leftmost entry is ignored", remoteAddr: "172.18.0.5:40000",
			xff: []string{"1.2.3.4,203.0.113.7"}, want: "203.0.113.7"},
		{name: "several header lines: the rightmost of the last", remoteAddr: "172.18.0.5:40000",
			xff: []string{"1.2.3.4", "198.51.100.2, 203.0.113.7"}, want: "203.0.113.7"},
		{name: "IPv6 entry", remoteAddr: "172.18.0.5:40000", xff: []string{"2001:db8::1"}, want: "2001:db8::1"},
		{name: "IPv4 mapped into IPv6 shares the IPv4 bucket", remoteAddr: "172.18.0.5:40000",
			xff: []string{"::ffff:203.0.113.7"}, want: "203.0.113.7"},
		{name: "an entry with a port", remoteAddr: "172.18.0.5:40000", xff: []string{"203.0.113.7:8080"}, want: "203.0.113.7"},
		{name: "garbage: falls back to the TCP peer", remoteAddr: "172.18.0.5:40000", xff: []string{"not-an-ip"}, want: "172.18.0.5"},
		{name: "garbage on the right of a real address", remoteAddr: "172.18.0.5:40000",
			xff: []string{"203.0.113.7, <script>"}, want: "172.18.0.5"},
		{name: "an empty header", remoteAddr: "172.18.0.5:40000", xff: []string{""}, want: "172.18.0.5"},
		{name: "a trailing comma", remoteAddr: "172.18.0.5:40000", xff: []string{"203.0.113.7,"}, want: "172.18.0.5"},
		{name: "a RemoteAddr without a port", remoteAddr: "203.0.113.7", want: "203.0.113.7"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/t/x", nil)
			r.RemoteAddr = tc.remoteAddr
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			require.Equal(t, tc.want, clientIP(r))
		})
	}
}
