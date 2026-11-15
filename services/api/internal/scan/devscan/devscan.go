// Package devscan is a stand-in for clamd in development only (UPLOAD_SCANNER=fake): the web
// app's end-to-end tests create clean and infected uploads without running ClamAV, which needs
// about 1 GB of memory and minutes to load its signatures.
//
// It knows exactly one signature, the EICAR antivirus test file: content that holds the EICAR
// string anywhere is infected, as clamd reports it, and everything else is clean. Nothing else a
// user sends, and nothing about a file's name or declared type, changes its answer. It is no
// malware scan at all, which is why config.LoadWorker refuses it outside development.
package devscan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// EICAR is the standard antivirus test file: harmless, and every scanner reports it.
const EICAR = `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`

// Signature is what clamd, and so Scanner, reports for EICAR.
const Signature = "Eicar-Test-Signature"

// chunkSize is how much content Scan reads at a time.
const chunkSize = 64 << 10

// Scanner implements upload.Scanner for development. The zero value is ready.
type Scanner struct{}

// Scan reads r to its end and reports EICAR when the content holds it. It holds at most a chunk
// plus the length of EICAR in memory, whatever the size of the content.
func (Scanner) Scan(ctx context.Context, r io.Reader) (upload.Verdict, error) {
	needle := []byte(EICAR)
	buf := make([]byte, 0, chunkSize+len(needle))
	chunk := make([]byte, chunkSize)
	for {
		if err := ctx.Err(); err != nil {
			return upload.Verdict{}, err
		}
		n, err := r.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if bytes.Contains(buf, needle) {
			return upload.Verdict{Signature: Signature}, nil
		}
		// Keep only the tail a match could still start in.
		if keep := len(needle) - 1; len(buf) > keep {
			buf = append(buf[:0], buf[len(buf)-keep:]...)
		}
		if errors.Is(err, io.EOF) {
			return upload.Verdict{Clean: true}, nil
		}
		if err != nil {
			return upload.Verdict{}, fmt.Errorf("devscan: reading the content: %w", err)
		}
	}
}
