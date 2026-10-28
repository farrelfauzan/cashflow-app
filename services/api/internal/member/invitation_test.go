package member

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

func TestJoinIndonesian(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		{name: "none", parts: nil, want: ""},
		{name: "one", parts: []string{"Admin"}, want: "Admin"},
		{name: "two", parts: []string{"Admin (Kos Melati)", "Karyawan"}, want: "Admin (Kos Melati) dan Karyawan"},
		{name: "three", parts: []string{"Supervisor", "Admin", "Karyawan"}, want: "Supervisor, Admin dan Karyawan"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, joinIndonesian(tc.parts))
		})
	}
}

func TestNeedsOperator(t *testing.T) {
	sendErr := func(err error) error { return fmt.Errorf("%w: %w", ErrSendFailed, err) }
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "no error", err: nil, want: false},
		{name: "our own rate limit", err: ErrInviteRateLimited, want: false},
		{name: "a number without WhatsApp", err: sendErr(messaging.ErrUndeliverable), want: false},
		{name: "no provider in an offline tool", err: sendErr(errNoMessenger), want: false},
		{name: "the session is logged out", err: sendErr(messaging.ErrNotConnected), want: true},
		{name: "WhatsApp throttles the number", err: sendErr(messaging.ErrRateLimited), want: true},
		{name: "an unexpected provider error", err: sendErr(errors.New("gowa: calling server")), want: true},
		{name: "the limiter fails", err: errors.New("checking invitation limit: redis down"), want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, NeedsOperator(tc.err))
		})
	}
}
