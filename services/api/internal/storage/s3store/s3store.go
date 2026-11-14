// Package s3store keeps uploads in an S3-compatible bucket (S6-1): Cloudflare R2 in production,
// MinIO in development and tests, through the AWS SDK for Go v2. It implements the upload
// package's Presigner (the API) and Objects (the worker).
//
// R2 has no presigned POST, so the size limit of an upload is its Content-Length, signed into
// the presigned PUT together with its Content-Type: a body of any other size or type is refused
// by the bucket. Downloads are presigned GETs that override the response's Content-Disposition
// to attachment and its Content-Type to the type the scan found, so a file is never shown inline.
//
// Two endpoints: Endpoint is where the API and the worker reach the bucket, PresignEndpoint the
// host the browser reaches (they differ when the bucket sits on an internal network, as MinIO in
// a compose network does). A presigned URL is signed for its host, so it is made with the
// browser's endpoint.
package s3store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// Config is where the bucket is and how to sign for it. SecretAccessKey is a secret: never log
// it or a Config.
type Config struct {
	// Endpoint is the bucket's S3 endpoint for the API and the worker, such as
	// https://<account>.r2.cloudflarestorage.com or http://localhost:9000.
	Endpoint string
	// PresignEndpoint is the endpoint in the URLs the browser uses; empty means Endpoint.
	PresignEndpoint string
	Bucket          string
	// Region is "auto" for R2; MinIO accepts any, conventionally us-east-1.
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	// PathStyle puts the bucket in the path (http://host/bucket/key), as MinIO needs; R2 takes
	// either.
	PathStyle bool
	// HTTPClient is optional; the default has a timeout of RequestTimeout.
	HTTPClient *http.Client
}

// RequestTimeout bounds one request to the bucket from the default client.
const RequestTimeout = 2 * time.Minute

// String leaves the secret out, so printing a Config never shows it.
func (c Config) String() string {
	return fmt.Sprintf("s3store.Config{Endpoint:%s PresignEndpoint:%s Bucket:%s Region:%s PathStyle:%t}",
		c.Endpoint, c.PresignEndpoint, c.Bucket, c.Region, c.PathStyle)
}

// safeKey is the shape of every key the upload package makes: prefixes, the organization id
// and hex. Nothing else is sent, so no key needs escaping.
var safeKey = regexp.MustCompile(`^[a-z]+/[a-z0-9/-]+$`)

// Store is the bucket.
type Store struct {
	bucket  string
	client  *s3.Client
	presign *s3.PresignClient
}

// New returns a Store. It makes no request.
func New(cfg Config) (*Store, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, errors.New("s3store: endpoint, bucket, access key id and secret access key are required")
	}
	if cfg.Region == "" {
		cfg.Region = "auto"
	}
	if cfg.PresignEndpoint == "" {
		cfg.PresignEndpoint = cfg.Endpoint
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: RequestTimeout}
	}
	creds := aws.Credentials{AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey, Source: "s3store"}
	options := func(endpoint string) s3.Options {
		return s3.Options{
			Region:       cfg.Region,
			BaseEndpoint: aws.String(endpoint),
			UsePathStyle: cfg.PathStyle,
			Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
				return creds, nil
			}),
			HTTPClient: hc,
			// No checksum headers the bucket did not ask for: R2 and MinIO need none, and a
			// presigned PUT must carry only what the browser can send.
			RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
			ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
		}
	}
	return &Store{
		bucket:  cfg.Bucket,
		client:  s3.New(options(cfg.Endpoint)),
		presign: s3.NewPresignClient(s3.New(options(cfg.PresignEndpoint))),
	}, nil
}

// PresignPut implements upload.Presigner. The returned headers are the signed ones the browser
// must send, without Host and Content-Length, which the browser sets itself.
func (s *Store) PresignPut(ctx context.Context, key, contentType string, size int64, ttl time.Duration) (upload.PresignedPut, error) {
	if err := checkKey(key); err != nil {
		return upload.PresignedPut{}, err
	}
	req, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		ContentType: aws.String(contentType), ContentLength: aws.Int64(size),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return upload.PresignedPut{}, fmt.Errorf("presigning PUT: %w", err)
	}
	headers := map[string]string{}
	for name, values := range req.SignedHeader {
		switch http.CanonicalHeaderKey(name) {
		case "Host", "Content-Length":
			continue
		}
		headers[http.CanonicalHeaderKey(name)] = strings.Join(values, ",")
	}
	return upload.PresignedPut{URL: req.URL, Headers: headers}, nil
}

// PresignGet implements upload.Presigner.
func (s *Store) PresignGet(ctx context.Context, key string, ttl time.Duration, filename, contentType string) (string, error) {
	if err := checkKey(key); err != nil {
		return "", err
	}
	req, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		ResponseContentDisposition: aws.String(attachment(filename)),
		ResponseContentType:        aws.String(contentType),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("presigning GET: %w", err)
	}
	return req.URL, nil
}

// Head implements upload.Objects.
func (s *Store) Head(ctx context.Context, key string) (int64, error) {
	if err := checkKey(key); err != nil {
		return 0, err
	}
	out, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return 0, wrap("HEAD", err)
	}
	if out.ContentLength == nil {
		return 0, errors.New("s3store: HEAD answered no size")
	}
	return *out.ContentLength, nil
}

// Open implements upload.Objects.
func (s *Store) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, wrap("GET", err)
	}
	return out.Body, nil
}

// Move implements upload.Objects: a server-side copy, then a delete of the source. A copy that
// succeeds before a failed delete leaves both; running Move again finishes it.
func (s *Store) Move(ctx context.Context, src, dst string) error {
	if err := checkKey(src); err != nil {
		return err
	}
	if err := checkKey(dst); err != nil {
		return err
	}
	// The source is "<bucket>/<key>"; checkKey keeps the key to characters that need no escaping.
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(dst), CopySource: aws.String(s.bucket + "/" + src),
	})
	if err != nil {
		return wrap("COPY", err)
	}
	if _, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(src)}); err != nil {
		return wrap("DELETE", err)
	}
	return nil
}

func checkKey(key string) error {
	if !safeKey.MatchString(key) || strings.Contains(key, "//") || strings.Contains(key, "..") {
		return fmt.Errorf("s3store: refusing key %q", key)
	}
	return nil
}

// wrap names the operation and turns a 404 (NoSuchKey, NotFound) into upload.ErrObjectNotFound.
func wrap(op string, err error) error {
	var re *awshttp.ResponseError
	if errors.As(err, &re) && re.HTTPStatusCode() == http.StatusNotFound {
		return fmt.Errorf("s3store %s: %w", op, upload.ErrObjectNotFound)
	}
	return fmt.Errorf("s3store %s: %w", op, err)
}

// attachment is a Content-Disposition that downloads the file under name, which is ASCII (the
// upload package makes it from the upload id).
func attachment(name string) string {
	return `attachment; filename="` + strings.ReplaceAll(name, `"`, "") + `"; filename*=UTF-8''` + url.PathEscape(name)
}
