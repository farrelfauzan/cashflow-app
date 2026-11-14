package s3store_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/storage/s3store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

const key = "incoming/uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/00112233445566778899aabbccddeeff"

func newStore(t *testing.T, endpoint, presign string) *s3store.Store {
	t.Helper()
	s, err := s3store.New(s3store.Config{
		Endpoint: endpoint, PresignEndpoint: presign, Bucket: "cashflow-test", Region: "us-east-1",
		AccessKeyID: "test-access", SecretAccessKey: "test-secret-key", PathStyle: true,
	})
	require.NoError(t, err)
	return s
}

func TestPresignPut(t *testing.T) {
	t.Parallel()
	s := newStore(t, "http://minio:9000", "http://localhost:9100")
	put, err := s.PresignPut(context.Background(), key, "image/jpeg", 2_483_120, 10*time.Minute)
	require.NoError(t, err)

	u, err := url.Parse(put.URL)
	require.NoError(t, err)
	require.Equal(t, "localhost:9100", u.Host, "the browser's endpoint")
	require.Equal(t, "/cashflow-test/"+key, u.Path, "path style")
	q := u.Query()
	require.Equal(t, "600", q.Get("X-Amz-Expires"))
	signed := strings.Split(q.Get("X-Amz-SignedHeaders"), ";")
	require.Contains(t, signed, "content-length", "the size limit is signed")
	require.Contains(t, signed, "content-type")
	require.Contains(t, signed, "host")
	for name := range q {
		require.False(t, strings.HasPrefix(strings.ToLower(name), "x-amz-checksum"), "no checksum the browser cannot send: %s", name)
	}
	require.Equal(t, map[string]string{"Content-Type": "image/jpeg"}, put.Headers)
}

func TestPresignGet(t *testing.T) {
	t.Parallel()
	s := newStore(t, "http://minio:9000", "")
	raw, err := s.PresignGet(context.Background(), "files/uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/00112233445566778899aabbccddeeff",
		5*time.Minute, "bukti-7b0e0b2a.jpg", "image/jpeg")
	require.NoError(t, err)
	u, err := url.Parse(raw)
	require.NoError(t, err)
	require.Equal(t, "minio:9000", u.Host, "the internal endpoint when no presign endpoint is set")
	q := u.Query()
	require.Equal(t, "300", q.Get("X-Amz-Expires"))
	require.True(t, strings.HasPrefix(q.Get("response-content-disposition"), `attachment; filename="bukti-7b0e0b2a.jpg"`), q.Get("response-content-disposition"))
	require.Equal(t, "image/jpeg", q.Get("response-content-type"))
}

func TestKeysAreChecked(t *testing.T) {
	t.Parallel()
	s := newStore(t, "http://minio:9000", "")
	ctx := context.Background()
	for _, bad := range []string{"", "../etc/passwd", "files//x", "files/../x", "Files/X", "files/a?b", "/files/a"} {
		_, err := s.PresignPut(ctx, bad, "image/jpeg", 1, time.Minute)
		require.Error(t, err, bad)
		_, err = s.Head(ctx, bad)
		require.Error(t, err, bad)
		require.Error(t, s.Move(ctx, bad, "files/a"), bad)
	}
}

func TestNewNeedsTheBucket(t *testing.T) {
	t.Parallel()
	_, err := s3store.New(s3store.Config{Endpoint: "http://minio:9000"})
	require.Error(t, err)
	require.NotContains(t, s3store.Config{SecretAccessKey: "very-secret"}.String(), "very-secret")
}

// TestNotFound: a 404 from the bucket, on HEAD (no body), GET (NoSuchKey) or COPY, is
// upload.ErrObjectNotFound; any other failure is not.
func TestNotFound(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/broken") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`))
		}
	}))
	t.Cleanup(srv.Close)
	s := newStore(t, srv.URL, "")
	ctx := context.Background()

	_, err := s.Head(ctx, "files/missing")
	require.ErrorIs(t, err, upload.ErrObjectNotFound)
	_, err = s.Open(ctx, "files/missing")
	require.ErrorIs(t, err, upload.ErrObjectNotFound)
	require.ErrorIs(t, s.Move(ctx, "incoming/missing", "files/missing"), upload.ErrObjectNotFound)

	_, err = s.Head(ctx, "files/broken")
	require.Error(t, err)
	require.NotErrorIs(t, err, upload.ErrObjectNotFound)
	require.NotContains(t, err.Error(), "test-secret-key")

	mu.Lock()
	defer mu.Unlock()
	require.Contains(t, seen, "HEAD /cashflow-test/files/missing")
	require.Contains(t, seen, "PUT /cashflow-test/files/missing", "a copy is a PUT to the destination")
}
