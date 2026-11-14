package clamd_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/scan/clamd"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// fakeClamd is a tiny clamd on loopback: it reads one null-terminated command per connection,
// reassembles an INSTREAM's chunks, and answers what reply says, as clamd does, null-terminated.
type fakeClamd struct {
	ln    net.Listener
	reply func(command string, content []byte) string
	// stall, when set, makes it read the command and then answer nothing until the client goes.
	stall bool

	mu       sync.Mutex
	commands []string
	contents [][]byte
	chunks   []int
}

func startFake(t *testing.T, reply func(command string, content []byte) string) *fakeClamd {
	t.Helper()
	return start(t, &fakeClamd{reply: reply})
}

// start serves f, set up before any connection arrives.
func start(t *testing.T, f *fakeClamd) *fakeClamd {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f.ln = ln
	t.Cleanup(func() { _ = ln.Close() })
	go f.serve()
	return f
}

func (f *fakeClamd) addr() string { return f.ln.Addr().String() }

func (f *fakeClamd) serve() {
	for {
		conn, err := f.ln.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeClamd) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	cmd, err := r.ReadString(0)
	if err != nil {
		return
	}
	cmd = strings.TrimSuffix(cmd, "\x00")
	var content []byte
	var chunks []int
	if cmd == "zINSTREAM" {
		for {
			var size uint32
			if err := binary.Read(r, binary.BigEndian, &size); err != nil {
				return
			}
			if size == 0 {
				break
			}
			chunks = append(chunks, int(size))
			buf := make([]byte, size)
			if _, err := io.ReadFull(r, buf); err != nil {
				return
			}
			content = append(content, buf...)
		}
	}
	f.mu.Lock()
	f.commands = append(f.commands, cmd)
	f.contents = append(f.contents, content)
	f.chunks = append(f.chunks, chunks...)
	f.mu.Unlock()
	if f.stall {
		_, _ = io.Copy(io.Discard, r)
		return
	}
	_, _ = io.WriteString(conn, f.reply(cmd, content)+"\x00")
}

// clamdReply answers like clamd with the EICAR signature.
func clamdReply(cmd string, content []byte) string {
	switch cmd {
	case "zPING":
		return "PONG"
	case "zINSTREAM":
		if bytes.Contains(content, []byte(uploadtest.EICAR)) {
			return "stream: Eicar-Test-Signature FOUND"
		}
		return "stream: OK"
	}
	return "UNKNOWN COMMAND"
}

func TestScan(t *testing.T) {
	t.Parallel()
	big := bytes.Repeat([]byte("abcdefgh"), 40_000) // 320 kB: several 64 kB chunks
	tests := []struct {
		name    string
		reply   func(string, []byte) string
		content []byte
		want    upload.Verdict
		wantErr error
	}{
		{name: "a clean file", reply: clamdReply, content: uploadtest.Image("image/jpeg", 2048), want: upload.Verdict{Clean: true}},
		{name: "EICAR is found", reply: clamdReply, content: []byte(uploadtest.EICAR), want: upload.Verdict{Signature: "Eicar-Test-Signature"}},
		{name: "a file of several chunks", reply: clamdReply, content: big, want: upload.Verdict{Clean: true}},
		{name: "an empty file", reply: clamdReply, content: nil, want: upload.Verdict{Clean: true}},
		{
			name:    "clamd's error is no verdict",
			reply:   func(string, []byte) string { return "INSTREAM size limit exceeded. ERROR" },
			content: []byte("x"), wantErr: clamd.ErrScanFailed,
		},
		{
			name:    "an answer it does not know is never clean",
			reply:   func(string, []byte) string { return "stream: MAYBE" },
			content: []byte("x"), wantErr: clamd.ErrScanFailed,
		},
		{
			name:    "a signature with spaces",
			reply:   func(string, []byte) string { return "stream: Win.Test.Some Thing-1 FOUND" },
			content: []byte("x"), want: upload.Verdict{Signature: "Win.Test.Some Thing-1"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := startFake(t, tc.reply)
			c, err := clamd.New(f.addr())
			require.NoError(t, err)
			got, err := c.Scan(context.Background(), bytes.NewReader(tc.content))
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Equal(t, []string{"zINSTREAM"}, f.commands)
			require.Equal(t, len(tc.content), len(f.contents[0]))
			require.True(t, bytes.Equal(tc.content, f.contents[0]), "clamd received the file as sent")
			for _, n := range f.chunks {
				require.LessOrEqual(t, n, 64<<10)
			}
		})
	}
}

func TestPing(t *testing.T) {
	t.Parallel()
	t.Run("PONG", func(t *testing.T) {
		t.Parallel()
		c, err := clamd.New(startFake(t, clamdReply).addr())
		require.NoError(t, err)
		require.NoError(t, c.Ping(context.Background()))
	})
	t.Run("another answer", func(t *testing.T) {
		t.Parallel()
		c, err := clamd.New(startFake(t, func(string, []byte) string { return "LOADING" }).addr())
		require.NoError(t, err)
		require.ErrorIs(t, c.Ping(context.Background()), clamd.ErrScanFailed)
	})
	t.Run("nobody listening", func(t *testing.T) {
		t.Parallel()
		ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := ln.Addr().String()
		require.NoError(t, ln.Close())
		c, err := clamd.New(addr)
		require.NoError(t, err)
		require.Error(t, c.Ping(context.Background()))
	})
}

func TestScanStopsWithTheContext(t *testing.T) {
	t.Parallel()
	f := start(t, &fakeClamd{reply: clamdReply, stall: true})
	c, err := clamd.New(f.addr())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = c.Scan(ctx, strings.NewReader("x"))
	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded) || isTimeout(err), "got %v", err)
	require.Less(t, time.Since(start), 5*time.Second)
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func TestNewChecksTheAddress(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"", "clamav", "clamav:3310:1"} {
		_, err := clamd.New(addr)
		require.Error(t, err, addr)
	}
	_, err := clamd.New("clamav:3310")
	require.NoError(t, err)
}
