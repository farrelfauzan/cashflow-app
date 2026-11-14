// Package clamd scans files with ClamAV's daemon over TCP (S6-1, decision 18). clamd runs as
// its own container on the internal network, never routed by Caddy.
//
// It speaks two commands of clamd's protocol, each on its own connection and each
// null-terminated (the "z" prefix): zINSTREAM, which streams the content as chunks of a 4-byte
// big-endian length followed by that many bytes, ended by a zero length, and answers
// "stream: OK", "stream: <signature> FOUND" or "<message> ERROR"; and zPING, which answers
// "PONG". clamd refuses a stream above its StreamMaxLength (100 MB in the official image), far
// above upload.MaxSize.
package clamd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

const (
	// chunkSize is how much content goes in one INSTREAM chunk.
	chunkSize = 64 << 10
	// maxReply bounds a reply; clamd's are a line.
	maxReply = 4 << 10
	// DefaultTimeout bounds a whole command when the context has no earlier deadline. A 10 MB
	// image scans in well under a second; the slack covers a busy daemon.
	DefaultTimeout = 2 * time.Minute
	// dialTimeout bounds the connection.
	dialTimeout = 5 * time.Second
)

// ErrScanFailed is clamd answering an error, or something it should never answer.
var ErrScanFailed = errors.New("clamd: scan failed")

// Client talks to one clamd. It is safe for concurrent use: each call opens its own connection.
type Client struct {
	addr    string
	timeout time.Duration
}

// New returns a Client for clamd at addr (host:port).
func New(addr string) (*Client, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return nil, fmt.Errorf("clamd address must be host:port: %w", err)
	}
	return &Client{addr: addr, timeout: DefaultTimeout}, nil
}

// Scan implements upload.Scanner: it streams r to clamd and reads the verdict. An error means no
// verdict; the caller retries.
func (c *Client) Scan(ctx context.Context, r io.Reader) (upload.Verdict, error) {
	var reply string
	err := c.do(ctx, "zINSTREAM\x00", func(conn net.Conn) error {
		return stream(conn, r)
	}, &reply)
	if err != nil {
		return upload.Verdict{}, err
	}
	return parseScanReply(reply)
}

// Ping checks that clamd answers, for a startup or health check.
func (c *Client) Ping(ctx context.Context) error {
	var reply string
	if err := c.do(ctx, "zPING\x00", nil, &reply); err != nil {
		return err
	}
	if reply != "PONG" {
		return fmt.Errorf("%w: ping answered %q", ErrScanFailed, truncate(reply))
	}
	return nil
}

// do sends command on a new connection, then runs body (when set), then reads the
// null-terminated reply. The connection's deadline is the context's, or DefaultTimeout; a
// cancelled context closes the connection at once.
func (c *Client) do(ctx context.Context, command string, body func(net.Conn) error, reply *string) error {
	d := net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("connecting to clamd: %w", err)
	}
	defer func() { _ = conn.Close() }() // the reply is already read, or the call has failed
	deadline := time.Now().Add(c.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("setting the clamd deadline: %w", err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Unix(1, 0)) })
	defer stop()

	if _, err := io.WriteString(conn, command); err != nil {
		return c.failed(ctx, "sending the command", err)
	}
	if body != nil {
		if err := body(conn); err != nil {
			// clamd may have closed the stream with a reason (a size limit); read it if it did.
			if r, rerr := readReply(conn); rerr == nil {
				return fmt.Errorf("%w: %s (%w)", ErrScanFailed, truncate(r), err)
			}
			return c.failed(ctx, "streaming the content", err)
		}
	}
	r, err := readReply(conn)
	if err != nil {
		return c.failed(ctx, "reading the answer", err)
	}
	*reply = r
	return nil
}

// failed wraps err, preferring the context's error when the context ended it.
func (c *Client) failed(ctx context.Context, what string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("clamd %s: %w", what, ctxErr)
	}
	return fmt.Errorf("clamd %s: %w", what, err)
}

// stream writes r as INSTREAM chunks and the zero-length end.
func stream(w io.Writer, r io.Reader) error {
	buf := make([]byte, 4+chunkSize)
	for {
		n, err := io.ReadFull(r, buf[4:])
		if n > 0 {
			binary.BigEndian.PutUint32(buf[:4], uint32(n)) //nolint:gosec // n <= chunkSize
			if _, werr := w.Write(buf[:4+n]); werr != nil {
				return werr
			}
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading the content: %w", err)
		}
	}
	_, err := w.Write([]byte{0, 0, 0, 0})
	return err
}

// readReply reads up to the terminating null, or the end of the connection, and trims it. An
// empty answer is an error.
func readReply(r io.Reader) (string, error) {
	b, err := bufio.NewReader(io.LimitReader(r, maxReply)).ReadBytes(0)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	reply := strings.TrimSpace(string(bytes.TrimSuffix(b, []byte{0})))
	if reply == "" {
		return "", io.ErrUnexpectedEOF
	}
	return reply, nil
}

// parseScanReply reads an INSTREAM answer.
func parseScanReply(reply string) (upload.Verdict, error) {
	switch {
	case reply == "stream: OK":
		return upload.Verdict{Clean: true}, nil
	case strings.HasPrefix(reply, "stream: ") && strings.HasSuffix(reply, " FOUND"):
		sig := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(reply, "stream: "), " FOUND"))
		if sig == "" {
			sig = "unknown"
		}
		return upload.Verdict{Signature: sig}, nil
	default:
		// "... ERROR", or an answer we do not know: never take it as clean.
		return upload.Verdict{}, fmt.Errorf("%w: %s", ErrScanFailed, truncate(reply))
	}
}

func truncate(s string) string {
	const n = 200
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
