package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientIP is the address a request came from, for per-client rate limits.
//
// It assumes the deployment: only Caddy reaches the API (no port but Caddy's is published), and
// Caddy appends the address of the peer it accepted to X-Forwarded-For. So the rightmost entry is
// the one hop we trust, and anything to its left was sent by the client and can be forged. When
// the header is missing (local development, tests) it is the TCP peer, RemoteAddr. A rightmost
// entry that is not an IP address means a proxy we do not expect, so it falls back to RemoteAddr
// rather than let a forged value pick the bucket. If the API is ever exposed without Caddy in
// front, a client could choose its own bucket here; revisit this then.
func clientIP(r *http.Request) string {
	if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
		last := values[len(values)-1]
		if i := strings.LastIndexByte(last, ','); i >= 0 {
			last = last[i+1:]
		}
		if addr, ok := parseAddr(strings.TrimSpace(last)); ok {
			return addr
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if addr, ok := parseAddr(host); ok {
		return addr
	}
	return host
}

// parseAddr normalizes an IP address, with or without a port, to its canonical text; an IPv4
// address mapped into IPv6 becomes plain IPv4, so both spellings share a bucket.
func parseAddr(s string) (string, bool) {
	if addr, err := netip.ParseAddr(s); err == nil {
		return addr.Unmap().String(), true
	}
	if ap, err := netip.ParseAddrPort(s); err == nil {
		return ap.Addr().Unmap().String(), true
	}
	return "", false
}
