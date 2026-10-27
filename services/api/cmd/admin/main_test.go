package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store"
)

// TestSeedDemoRefusesOutsideDevelopment checks that seed-demo stops before it connects to any
// database unless APP_ENV is explicitly development.
func TestSeedDemoRefusesOutsideDevelopment(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		vars    map[string]string
		wantMsg string
	}{
		{name: "unset, even though config defaults to development", vars: map[string]string{}, wantMsg: "APP_ENV is not set"},
		{name: "empty", vars: map[string]string{"APP_ENV": ""}, wantMsg: `APP_ENV is ""`},
		{name: "staging", vars: map[string]string{"APP_ENV": "staging"}, wantMsg: `APP_ENV is "staging"`},
		{name: "production", vars: map[string]string{"APP_ENV": "production"}, wantMsg: `APP_ENV is "production"`},
		{name: "not exactly development", vars: map[string]string{"APP_ENV": "Development"}, wantMsg: `APP_ENV is "Development"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			connected := false
			var out bytes.Buffer
			e := env{
				lookup: func(k string) (string, bool) {
					v, ok := tc.vars[k]
					return v, ok
				},
				connect: func(context.Context) (*store.Store, *slog.Logger, error) {
					connected = true
					return nil, nil, errors.New("must not connect")
				},
				stdout: &out,
				stderr: &out,
			}
			err := run(context.Background(), []string{"seed-demo"}, e)
			require.ErrorIs(t, err, errNotDevelopment)
			require.ErrorContains(t, err, tc.wantMsg)
			require.False(t, connected, "seed-demo opened the database outside development")
			require.Empty(t, out.String())
		})
	}
}

func TestSeedDemoInDevelopmentConnects(t *testing.T) {
	t.Parallel()
	connectErr := errors.New("no database in unit tests")
	connected := false
	e := env{
		lookup: func(k string) (string, bool) {
			if k == "APP_ENV" {
				return "development", true
			}
			return "", false
		},
		connect: func(context.Context) (*store.Store, *slog.Logger, error) {
			connected = true
			return nil, nil, connectErr
		},
	}
	err := run(context.Background(), []string{"seed-demo"}, e)
	require.ErrorIs(t, err, connectErr)
	require.True(t, connected)
}

func TestRunRejectsBadCommands(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{name: "no command", args: nil},
		{name: "unknown command", args: []string{"drop-everything"}},
		{name: "seed-demo with arguments", args: []string{"seed-demo", "-force"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := env{
				lookup: func(string) (string, bool) { return "development", true },
				connect: func(context.Context) (*store.Store, *slog.Logger, error) {
					t.Fatal("must not connect")
					return nil, nil, nil
				},
			}
			err := run(context.Background(), tc.args, e)
			require.ErrorContains(t, err, "usage:")
		})
	}
}

func TestWriteLogins(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	err := writeLogins(&out, []demoseed.Login{
		{Phone: "+6281200010002", Name: "Pak Budi Hartono", Organization: "Griya Sejahtera", Roles: []string{"Admin"}},
		{
			Phone: "+6281200010005", Name: "Mas Joko Prasetyo", Organization: "Griya Sejahtera",
			Roles: []string{"Admin", "Karyawan"}, PropertyLimits: []string{"Admin: Kontrakan Mawar"},
		},
	})
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 3)
	require.Contains(t, lines[0], "PHONE")
	require.Contains(t, lines[0], "PROPERTY LIMIT")
	require.Regexp(t, `^\+6281200010002\s+Pak Budi Hartono\s+Griya Sejahtera\s+Admin\s+-$`, lines[1])
	require.Regexp(t, `^\+6281200010005\s+Mas Joko Prasetyo\s+Griya Sejahtera\s+Admin, Karyawan\s+Admin: Kontrakan Mawar$`, lines[2])
}
