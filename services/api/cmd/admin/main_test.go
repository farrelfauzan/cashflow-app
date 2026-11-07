package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing/invoicetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging/devlog"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/config"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise/promisetask"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/reminder/remindertask"
)

// demoCommands are the commands that fill a database with demo data or run as if it were
// another day, and so run only in development, each with valid arguments.
var demoCommands = [][]string{
	{"seed-demo"},
	{"generate-invoices"},
	{"run-day", "-day", "2026-11-10"},
	{"run-day", "-day", "2026-11-10", "-org", "6f1c2a3e-0000-4000-8000-000000000001"},
}

// TestDemoCommandsRefuseOutsideDevelopment checks that seed-demo, generate-invoices and run-day
// stop before they connect to any database unless APP_ENV is explicitly development.
func TestDemoCommandsRefuseOutsideDevelopment(t *testing.T) {
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
	for _, args := range demoCommands {
		command := strings.Join(args, " ")
		for _, tc := range tests {
			t.Run(command+": "+tc.name, func(t *testing.T) {
				t.Parallel()
				connected := false
				var out bytes.Buffer
				e := env{
					lookup: func(k string) (string, bool) {
						v, ok := tc.vars[k]
						return v, ok
					},
					connect: func(context.Context, io.Writer) (conn, error) {
						connected = true
						return conn{}, errors.New("must not connect")
					},
					stdout: &out,
					stderr: &out,
				}
				err := run(context.Background(), args, e)
				require.ErrorIs(t, err, errNotDevelopment)
				require.ErrorContains(t, err, tc.wantMsg)
				require.False(t, connected, command+" opened the database outside development")
				require.Empty(t, out.String())
			})
		}
	}
}

func TestDemoCommandsInDevelopmentConnect(t *testing.T) {
	t.Parallel()
	for _, args := range demoCommands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
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
				connect: func(context.Context, io.Writer) (conn, error) {
					connected = true
					return conn{}, connectErr
				},
				stderr: io.Discard,
			}
			err := run(context.Background(), args, e)
			require.ErrorIs(t, err, connectErr)
			require.True(t, connected)
		})
	}
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
		{name: "generate-invoices with arguments", args: []string{"generate-invoices", "-force"}},
		{name: "set-autogopay-key without -org", args: []string{"set-autogopay-key"}},
		{name: "set-autogopay-key with the key as an argument", args: []string{"set-autogopay-key", "-org", "6f1c2a3e-0000-4000-8000-000000000001", "agp_live_x"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := env{
				lookup: func(string) (string, bool) { return "development", true },
				connect: func(context.Context, io.Writer) (conn, error) {
					t.Fatal("must not connect")
					return conn{}, nil
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
		{Phone: "+6281200010006", Name: "Mas Agus Setiawan", Organization: "Griya Sejahtera", Roles: []string{"Karyawan"}, Disabled: true},
	})
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	require.Len(t, lines, 4)
	require.Contains(t, lines[0], "PHONE")
	require.Contains(t, lines[0], "PROPERTY LIMIT")
	require.Contains(t, lines[0], "STATUS")
	require.Regexp(t, `^\+6281200010002\s+Pak Budi Hartono\s+Griya Sejahtera\s+Admin\s+-\s+active$`, lines[1])
	require.Regexp(t, `^\+6281200010005\s+Mas Joko Prasetyo\s+Griya Sejahtera\s+Admin, Karyawan\s+Admin: Kontrakan Mawar\s+active$`, lines[2])
	require.Regexp(t, `^\+6281200010006\s+Mas Agus Setiawan\s+Griya Sejahtera\s+Karyawan\s+-\s+disabled \(cannot sign in\)$`, lines[3])
}

// TestSetAutoGoPayKeyRefusesBeforeConnecting: a missing or bad -org, a key passed any way but
// stdin, and a missing or bad key on stdin all fail before the database is opened, and no error
// or output repeats the key.
func TestSetAutoGoPayKeyRefusesBeforeConnecting(t *testing.T) {
	t.Parallel()
	const org = "6f1c2a3e-0000-4000-8000-000000000001"
	const key = "agp_live_do_not_print_7c2e"
	tests := []struct {
		name    string
		args    []string
		stdin   io.Reader
		wantErr string
	}{
		{name: "no -org", args: nil, stdin: strings.NewReader(key + "\n"), wantErr: "needs -org"},
		{name: "-org is not a uuid", args: []string{"-org", "griya"}, stdin: strings.NewReader(key + "\n"), wantErr: "-org must be an organization id"},
		{name: "-org is the nil uuid", args: []string{"-org", "00000000-0000-0000-0000-000000000000"}, stdin: strings.NewReader(key + "\n"), wantErr: "-org must be an organization id"},
		{name: "the key as an argument", args: []string{"-org", org, key}, stdin: strings.NewReader(""), wantErr: "takes no arguments"},
		{name: "the key as a flag", args: []string{"-org", org, "-key", key}, stdin: strings.NewReader(""), wantErr: "flag provided but not defined: -key"},
		{name: "the key in -org", args: []string{"-org", key}, stdin: strings.NewReader(""), wantErr: "-org must be an organization id"},
		{name: "empty stdin", args: []string{"-org", org}, stdin: strings.NewReader(""), wantErr: "no API key on stdin"},
		{name: "no stdin", args: []string{"-org", org}, stdin: nil, wantErr: "no API key on stdin"},
		{name: "a blank line", args: []string{"-org", org}, stdin: strings.NewReader("   \n" + key + "\n"), wantErr: "no API key on stdin"},
		{name: "a control character in the key", args: []string{"-org", org}, stdin: strings.NewReader("agp_live\x00x\n"), wantErr: "without control characters"},
		{name: "an oversized line", args: []string{"-org", org}, stdin: strings.NewReader(strings.Repeat("k", 10_000)), wantErr: "longer than 512 bytes"},
		{name: "a key of 513 bytes", args: []string{"-org", org}, stdin: strings.NewReader(strings.Repeat("k", 513) + "\n"), wantErr: "1 to 512 bytes"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut bytes.Buffer
			e := env{
				lookup: func(string) (string, bool) { return "", false },
				connect: func(context.Context, io.Writer) (conn, error) {
					t.Fatal("must not connect")
					return conn{}, nil
				},
				stdin:  tc.stdin,
				stdout: &out,
				stderr: &errOut,
			}
			err := run(context.Background(), append([]string{"set-autogopay-key"}, tc.args...), e)
			require.ErrorContains(t, err, tc.wantErr)
			require.NotContains(t, err.Error(), key)
			require.Empty(t, out.String())
			require.NotContains(t, errOut.String(), key)
		})
	}
}

func TestSetAutoGoPayKeyReadsOnlyTheFirstLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		stdin string
		want  string
	}{
		{name: "a line from echo", stdin: "agp_live_a\n", want: "agp_live_a"},
		{name: "no trailing newline", stdin: "agp_live_a", want: "agp_live_a"},
		{name: "CRLF and spaces", stdin: "  agp_live_a \r\n", want: "agp_live_a"},
		{name: "later lines are not read", stdin: "agp_live_a\nsomething else\n", want: "agp_live_a"},
		{name: "512 bytes", stdin: strings.Repeat("k", 512) + "\n", want: strings.Repeat("k", 512)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := readKeyLine(strings.NewReader(tc.stdin))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestSetAutoGoPayKeyConnectsWithAValidRequest(t *testing.T) {
	t.Parallel()
	connectErr := errors.New("no database in unit tests")
	connected := false
	e := env{
		lookup: func(string) (string, bool) { return "", false },
		connect: func(context.Context, io.Writer) (conn, error) {
			connected = true
			return conn{}, connectErr
		},
		stdin: strings.NewReader("agp_live_x\n"),
	}
	err := run(context.Background(), []string{"set-autogopay-key", "-org", "6f1c2a3e-0000-4000-8000-000000000001"}, e)
	require.ErrorIs(t, err, connectErr)
	require.True(t, connected)
}

// TestSetBankAccountRefusesBeforeConnecting: a missing flag, a bad -org and a malformed value all
// fail before the database is opened.
func TestSetBankAccountRefusesBeforeConnecting(t *testing.T) {
	t.Parallel()
	const org = "6f1c2a3e-0000-4000-8000-000000000001"
	valid := map[string]string{"-org": org, "-bank": "BCA", "-number": "123 456 7890", "-holder": "Suhwendi"}
	with := func(flag, value string) []string {
		var args []string
		for _, k := range []string{"-org", "-bank", "-number", "-holder"} {
			v := valid[k]
			if k == flag {
				v = value
			}
			if v != "" {
				args = append(args, k, v)
			}
		}
		return args
	}
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no flags", args: nil, wantErr: "needs -org, -bank, -number and -holder"},
		{name: "no -org", args: with("-org", ""), wantErr: "needs -org"},
		{name: "no -bank", args: with("-bank", ""), wantErr: "needs -org, -bank"},
		{name: "no -number", args: with("-number", ""), wantErr: "-number"},
		{name: "no -holder", args: with("-holder", ""), wantErr: "-holder"},
		{name: "-org is not a uuid", args: with("-org", "griya"), wantErr: "-org must be an organization id"},
		{name: "-org is the nil uuid", args: with("-org", "00000000-0000-0000-0000-000000000000"), wantErr: "-org must be an organization id"},
		{name: "a stray argument", args: append(with("", ""), "BCA"), wantErr: "takes no arguments"},
		{name: "letters in the number", args: with("-number", "1234O6789"), wantErr: "account number"},
		{name: "a number too short", args: with("-number", "1234"), wantErr: "account number"},
		{name: "a number of 35 characters", args: with("-number", strings.Repeat("1", 35)), wantErr: "account number"},
		{name: "a blank bank name", args: with("-bank", "   "), wantErr: "bank name"},
		{name: "a bank name of 61 characters", args: with("-bank", strings.Repeat("B", 61)), wantErr: "bank name"},
		{name: "a control character in the holder", args: with("-holder", "Suh\twendi"), wantErr: "account holder"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut bytes.Buffer
			e := env{
				lookup: func(string) (string, bool) { return "", false },
				connect: func(context.Context, io.Writer) (conn, error) {
					t.Fatal("must not connect")
					return conn{}, nil
				},
				stdout: &out,
				stderr: &errOut,
			}
			err := run(context.Background(), append([]string{"set-bank-account"}, tc.args...), e)
			require.ErrorContains(t, err, tc.wantErr)
			require.Empty(t, out.String())
		})
	}
}

func TestSetBankAccountConnectsWithAValidRequest(t *testing.T) {
	t.Parallel()
	connectErr := errors.New("no database in unit tests")
	connected := false
	e := env{
		lookup: func(string) (string, bool) { return "", false },
		connect: func(context.Context, io.Writer) (conn, error) {
			connected = true
			return conn{}, connectErr
		},
	}
	err := run(context.Background(), []string{
		"set-bank-account", "-org", "6f1c2a3e-0000-4000-8000-000000000001",
		"-bank", "BCA", "-number", "123-456-7890", "-holder", "Suhwendi",
	}, e)
	require.ErrorIs(t, err, connectErr)
	require.True(t, connected)
}

// TestSetBrokenPromiseThresholdRefusesBeforeConnecting: a missing or bad -org, a threshold
// outside 1 to 10 and a stray argument all fail before the database is opened.
func TestSetBrokenPromiseThresholdRefusesBeforeConnecting(t *testing.T) {
	t.Parallel()
	const org = "6f1c2a3e-0000-4000-8000-000000000001"
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no flags", args: nil, wantErr: "needs -org and -n"},
		{name: "no -org", args: []string{"-n", "3"}, wantErr: "needs -org"},
		{name: "-org is not a uuid", args: []string{"-org", "griya", "-n", "3"}, wantErr: "-org must be an organization id"},
		{name: "-org is the nil uuid", args: []string{"-org", "00000000-0000-0000-0000-000000000000", "-n", "3"}, wantErr: "-org must be an organization id"},
		{name: "no -n", args: []string{"-org", org}, wantErr: "-n must be 1 to 10"},
		{name: "-n of 0", args: []string{"-org", org, "-n", "0"}, wantErr: "-n must be 1 to 10"},
		{name: "-n of 11", args: []string{"-org", org, "-n", "11"}, wantErr: "-n must be 1 to 10"},
		{name: "-n below zero", args: []string{"-org", org, "-n", "-2"}, wantErr: "-n must be 1 to 10"},
		{name: "-n that is not a number", args: []string{"-org", org, "-n", "dua"}, wantErr: "invalid value"},
		{name: "a stray argument", args: []string{"-org", org, "-n", "3", "4"}, wantErr: "takes no arguments"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out, errOut bytes.Buffer
			e := env{
				lookup: func(string) (string, bool) { return "", false },
				connect: func(context.Context, io.Writer) (conn, error) {
					t.Fatal("must not connect")
					return conn{}, nil
				},
				stdout: &out,
				stderr: &errOut,
			}
			err := run(context.Background(), append([]string{"set-broken-promise-threshold"}, tc.args...), e)
			require.ErrorContains(t, err, tc.wantErr)
			require.Empty(t, out.String())
		})
	}
}

func TestSetBrokenPromiseThresholdConnectsWithAValidRequest(t *testing.T) {
	t.Parallel()
	for _, n := range []string{"1", "10"} {
		t.Run(n, func(t *testing.T) {
			t.Parallel()
			connectErr := errors.New("no database in unit tests")
			connected := false
			e := env{
				lookup: func(string) (string, bool) { return "", false },
				connect: func(context.Context, io.Writer) (conn, error) {
					connected = true
					return conn{}, connectErr
				},
				stderr: io.Discard,
			}
			err := run(context.Background(), []string{
				"set-broken-promise-threshold", "-org", "6f1c2a3e-0000-4000-8000-000000000001", "-n", n,
			}, e)
			require.ErrorIs(t, err, connectErr)
			require.True(t, connected)
		})
	}
}

// TestRunDayRefusesBadFlagsBeforeConnecting: in development, a missing or malformed -day, a bad
// -org and a stray argument all fail before the database is opened.
func TestRunDayRefusesBadFlagsBeforeConnecting(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "no flags", args: nil, wantErr: "run-day needs -day YYYY-MM-DD"},
		{name: "only -org", args: []string{"-org", "6f1c2a3e-0000-4000-8000-000000000001"}, wantErr: "needs -day"},
		{name: "an empty -day", args: []string{"-day", ""}, wantErr: "needs -day"},
		{name: "a day first format", args: []string{"-day", "10-11-2026"}, wantErr: "-day must be a date as YYYY-MM-DD"},
		{name: "a day that does not exist", args: []string{"-day", "2026-02-30"}, wantErr: "-day must be a date"},
		{name: "a month that does not exist", args: []string{"-day", "2026-13-01"}, wantErr: "-day must be a date"},
		{name: "a date with a time", args: []string{"-day", "2026-11-10T08:00"}, wantErr: "-day must be a date"},
		{name: "-org is not a uuid", args: []string{"-day", "2026-11-10", "-org", "suhwendi"}, wantErr: "-org must be an organization id"},
		{name: "-org is the nil uuid", args: []string{"-day", "2026-11-10", "-org", "00000000-0000-0000-0000-000000000000"}, wantErr: "-org must be an organization id"},
		{name: "a stray argument", args: []string{"-day", "2026-11-10", "today"}, wantErr: "takes no arguments"},
		{name: "an unknown flag", args: []string{"-day", "2026-11-10", "-force"}, wantErr: "flag provided but not defined: -force"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			e := env{
				lookup: func(k string) (string, bool) { return "development", k == "APP_ENV" },
				connect: func(context.Context, io.Writer) (conn, error) {
					t.Fatal("must not connect")
					return conn{}, nil
				},
				stdout: &out,
				stderr: io.Discard,
			}
			err := run(context.Background(), append([]string{"run-day"}, tc.args...), e)
			require.ErrorContains(t, err, tc.wantErr)
			require.Empty(t, out.String())
		})
	}
}

func TestParseRunDayReadsAJakartaDay(t *testing.T) {
	t.Parallel()
	req, err := parseRunDay([]string{"-day", "2026-11-10"}, io.Discard)
	require.NoError(t, err)
	require.True(t, time.Date(2026, 11, 10, 0, 0, 0, 0, clock.Jakarta).Equal(req.day), "got %s", req.day)
	require.Equal(t, uuid.Nil, req.org, "no -org means every active organization")

	d := dayRunner{sys: clock.NewFake(time.Date(2026, 11, 7, 14, 0, 0, 0, clock.Jakarta)), day: req.day}
	require.True(t, time.Date(2026, 11, 10, 8, 0, 0, 0, clock.Jakarta).Equal(d.at(remindersAt).Now()))
}

// TestRunDayWarnsAboutAFutureDay: a day after today (by the real clock, in Asia/Jakarta) prints a
// warning to stderr before it connects; today and earlier days do not.
func TestRunDayWarnsAboutAFutureDay(t *testing.T) {
	t.Parallel()
	// 23:30 on 7 November in Jakarta is still 7 November there, though 16:30 UTC.
	now := time.Date(2026, 11, 7, 23, 30, 0, 0, clock.Jakarta)
	tests := []struct {
		name string
		day  string
		warn bool
	}{
		{name: "tomorrow", day: "2026-11-08", warn: true},
		{name: "today", day: "2026-11-07", warn: false},
		{name: "a past day", day: "2026-10-01", warn: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			connectErr := errors.New("no database in unit tests")
			var errOut bytes.Buffer
			e := env{
				lookup:  func(k string) (string, bool) { return "development", k == "APP_ENV" },
				connect: func(context.Context, io.Writer) (conn, error) { return conn{}, connectErr },
				clock:   clock.NewFake(now),
				stderr:  &errOut,
			}
			err := run(context.Background(), []string{"run-day", "-day", tc.day}, e)
			require.ErrorIs(t, err, connectErr)
			if tc.warn {
				require.Contains(t, errOut.String(), "records are dated in the future; the real daily runs will treat them as done")
			} else {
				require.Empty(t, errOut.String())
			}
		})
	}
}

// TestRunDayTimesMatchTheWorkerSchedule keeps run-day's times of day in step with the worker's
// cron entries.
func TestRunDayTimesMatchTheWorkerSchedule(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cron string
		at   time.Duration
	}{
		{name: "promises:daily", cron: promisetask.DailyCron, at: promisesAt},
		{name: "invoices:daily", cron: invoicetask.DailyCron, at: invoicesAt},
		{name: "reminders:daily", cron: remindertask.DailyCron, at: remindersAt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := fmt.Sprintf("%d %d * * *", int(tc.at.Minutes())%60, int(tc.at.Hours()))
			require.Equal(t, want, tc.cron)
		})
	}
}

func TestDaySummaryLine(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 11, 10, 0, 0, 0, 0, clock.Jakarta)
	tests := []struct {
		name string
		sum  daySummary
		want string
	}{
		{
			name: "a quiet day",
			want: "Suhwendi Corp 2026-11-10: 0 promises closed (0 kept, 0 partly kept, 0 broken), 0 invoices created, 0 reminders recorded (0 visit tasks), 0 sent",
		},
		{
			name: "one of each",
			sum: daySummary{
				promises:  promise.CloseResult{Due: 1, Broken: 1},
				invoices:  billing.Result{Leases: 1, Invoices: 1},
				reminders: reminder.Result{Tenants: 1, Reminders: 1, VisitTasks: 1},
				sent:      1,
			},
			want: "Suhwendi Corp 2026-11-10: 1 promise closed (0 kept, 0 partly kept, 1 broken), 1 invoice created, 1 reminder recorded (1 visit task), 1 sent",
		},
		{
			name: "a busy day",
			sum: daySummary{
				promises:  promise.CloseResult{Due: 2, Kept: 1, Broken: 1, VisitTasks: 1},
				invoices:  billing.Result{Leases: 9, Invoices: 3},
				reminders: reminder.Result{Tenants: 5, Reminders: 4, VisitTasks: 1},
				sent:      4,
			},
			want: "Suhwendi Corp 2026-11-10: 2 promises closed (1 kept, 0 partly kept, 1 broken), 3 invoices created, 4 reminders recorded (2 visit tasks), 4 sent",
		},
		{
			name: "missing prices and failures",
			sum: daySummary{
				promises:  promise.CloseResult{Failed: 1},
				invoices:  billing.Result{Skipped: 1},
				reminders: reminder.Result{Failed: 1},
			},
			want: "Suhwendi Corp 2026-11-10: 0 promises closed (0 kept, 0 partly kept, 0 broken), 0 invoices created, 0 reminders recorded (0 visit tasks), 0 sent, 1 lease without a price, 2 tenants failed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, tc.sum.line("Suhwendi Corp", day))
		})
	}
}

func TestCount(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		n    int
		noun string
		want string
	}{
		{name: "none", n: 0, noun: "visit task", want: "0 visit tasks"},
		{name: "one", n: 1, noun: "visit task", want: "1 visit task"},
		{name: "two", n: 2, noun: "visit task", want: "2 visit tasks"},
		{name: "many", n: 12, noun: "invoice", want: "12 invoices"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, count(tc.n, tc.noun))
		})
	}
}

// TestRunDaySender: run-day sends through the devlog only with WHATSAPP_PROVIDER=devlog in
// development; otherwise it has no provider and says the reminders are recorded, not sent.
func TestRunDaySender(t *testing.T) {
	t.Parallel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	dev := config.Common{Env: "development"}
	tests := []struct {
		name       string
		cfg        config.Admin
		wantOrigin string
		sends      bool
	}{
		{name: "unset provider", cfg: config.Admin{Common: dev}},
		{name: "cloudapi", cfg: config.Admin{Common: dev, WhatsAppProvider: "cloudapi"}},
		{name: "twilio", cfg: config.Admin{Common: dev, WhatsAppProvider: "twilio"}},
		{name: "devlog", cfg: config.Admin{Common: dev, WhatsAppProvider: "devlog"}, wantOrigin: "http://localhost:3000", sends: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var errOut bytes.Buffer
			p, origin, err := runDaySender(tc.cfg, log, &errOut)
			require.NoError(t, err)
			require.Equal(t, tc.wantOrigin, origin)
			if tc.sends {
				require.IsType(t, &devlog.Provider{}, p)
				require.Empty(t, errOut.String())
				return
			}
			require.Nil(t, p)
			require.Equal(t, "admin: reminders recorded, not sent: run-day sends only with WHATSAPP_PROVIDER=devlog\n", errOut.String())
		})
	}
}
