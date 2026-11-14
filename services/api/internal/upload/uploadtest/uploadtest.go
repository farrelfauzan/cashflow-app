// Package uploadtest has hand-written fakes of the upload package's ports: object storage
// (Storage, both the API's Presigner and the worker's Objects), the malware scanner (Scanner)
// and the scan queue (Queue). Tests only.
package uploadtest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// EICAR is the standard antivirus test file: harmless, and every scanner reports it.
const EICAR = `X5O!P%@AP[4\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*`

// EICARSignature is what Scanner, like clamd, reports for EICAR.
const EICARSignature = "Eicar-Test-Signature"

// Storage is an in-memory bucket. Its presigned URLs are fake (https://storage.test/...) and
// work only through Put.
type Storage struct {
	mu      sync.Mutex
	objects map[string][]byte
	calls   []string
	// Err, when set, fails every call that touches the bucket.
	Err error
}

// NewStorage returns an empty bucket.
func NewStorage() *Storage { return &Storage{objects: map[string][]byte{}} }

// Put stores body at key, as a client's PUT would.
func (s *Storage) Put(key string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = slices.Clone(body)
}

// Object returns the object at key and whether there is one.
func (s *Storage) Object(key string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.objects[key]
	return slices.Clone(b), ok
}

// Keys returns every key in the bucket, sorted.
func (s *Storage) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.objects))
	for k := range s.objects {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Calls returns the bucket operations made so far ("head <key>", "open <key>",
// "move <src> <dst>"), oldest first.
func (s *Storage) Calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// PresignPut implements upload.Presigner.
func (s *Storage) PresignPut(_ context.Context, key, contentType string, size int64, ttl time.Duration) (upload.PresignedPut, error) {
	q := url.Values{"size": {strconv.FormatInt(size, 10)}, "ttl": {ttl.String()}}
	return upload.PresignedPut{
		URL:     "https://storage.test/" + key + "?" + q.Encode(),
		Headers: map[string]string{"Content-Type": contentType},
	}, nil
}

// PresignGet implements upload.Presigner.
func (s *Storage) PresignGet(_ context.Context, key string, ttl time.Duration, filename, contentType string) (string, error) {
	q := url.Values{"filename": {filename}, "type": {contentType}, "ttl": {ttl.String()}}
	return "https://storage.test/" + key + "?" + q.Encode(), nil
}

// Head implements upload.Objects.
func (s *Storage) Head(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "head "+key)
	if s.Err != nil {
		return 0, s.Err
	}
	b, ok := s.objects[key]
	if !ok {
		return 0, upload.ErrObjectNotFound
	}
	return int64(len(b)), nil
}

// Open implements upload.Objects.
func (s *Storage) Open(_ context.Context, key string) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "open "+key)
	if s.Err != nil {
		return nil, s.Err
	}
	b, ok := s.objects[key]
	if !ok {
		return nil, upload.ErrObjectNotFound
	}
	return io.NopCloser(bytes.NewReader(slices.Clone(b))), nil
}

// Move implements upload.Objects.
func (s *Storage) Move(_ context.Context, src, dst string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, "move "+src+" "+dst)
	if s.Err != nil {
		return s.Err
	}
	b, ok := s.objects[src]
	if !ok {
		return upload.ErrObjectNotFound
	}
	s.objects[dst] = b
	delete(s.objects, src)
	return nil
}

// Scanner is a fake malware scanner: content holding the EICAR string is infected.
type Scanner struct {
	mu      sync.Mutex
	scanned int
	// Err, when set, fails every scan.
	Err error
}

// Scan implements upload.Scanner.
func (s *Scanner) Scan(_ context.Context, r io.Reader) (upload.Verdict, error) {
	s.mu.Lock()
	s.scanned++
	err := s.Err
	s.mu.Unlock()
	if err != nil {
		return upload.Verdict{}, err
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return upload.Verdict{}, fmt.Errorf("reading the stream: %w", err)
	}
	if bytes.Contains(b, []byte(EICAR)) {
		return upload.Verdict{Signature: EICARSignature}, nil
	}
	return upload.Verdict{Clean: true}, nil
}

// Scanned returns how many scans ran.
func (s *Scanner) Scanned() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scanned
}

// Queue records the scans enqueued; nothing runs them.
type Queue struct {
	mu  sync.Mutex
	ids []uuid.UUID
	// Err, when set, fails every enqueue.
	Err error
}

// EnqueueScan implements upload.ScanQueue.
func (q *Queue) EnqueueScan(_ context.Context, _, id uuid.UUID) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.Err != nil {
		return q.Err
	}
	q.ids = append(q.ids, id)
	return nil
}

// Enqueued returns the upload ids enqueued so far, oldest first.
func (q *Queue) Enqueued() []uuid.UUID {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.ids)
}

// ErrDown is a storage or scanner outage for tests.
var ErrDown = errors.New("uploadtest: down")

// Image returns size bytes that sniff as contentType (image/jpeg, image/png or image/webp), or
// as text for any other type, padded with zeros.
func Image(contentType string, size int) []byte {
	var head []byte
	switch contentType {
	case "image/jpeg":
		head = []byte("\xFF\xD8\xFF\xE0\x00\x10JFIF\x00")
	case "image/png":
		head = []byte("\x89PNG\r\n\x1a\n")
	case "image/webp":
		head = []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")
	default:
		head = []byte("Ini bukan gambar.\n")
	}
	if size < len(head) {
		size = len(head)
	}
	out := make([]byte, size)
	copy(out, head)
	return out
}
