package authz

import "testing"

func TestMatchWildcard(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"invoice:read", "invoice:read", true},
		{"invoice:read", "invoice:create", false},
		{"invoice:*", "invoice:send-reminder", true},
		{"*:read", "payroll:read", true},
		{"*:read", "payroll:approve", false},
		{"*", "anything:at-all", true},
		{"*", "", true},
		{"", "", true},
		{"", "a", false},
		{"role/admin", "role/admin", true},
		{"role/admin", "role/administrator", false},
		{"property/*", "property/abc/tenant/123", true},
		{"property/abc/*", "property/abd/tenant/1", false},
		{"*/tenant/*", "property/abc/tenant/1", true},
		{"a*b*c", "aXXbYYc", true},
		{"a*b*c", "aXXbYY", false},
		{"**", "x", true},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+" vs "+tc.s, func(t *testing.T) {
			if got := matchWildcard(tc.pattern, tc.s); got != tc.want {
				t.Fatalf("matchWildcard(%q, %q) = %v, want %v", tc.pattern, tc.s, got, tc.want)
			}
		})
	}
}
