//go:build integration

package s3store_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/store/dbgen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/storage/s3store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// minioImage is the MinIO the tests and infra/compose.dev.yaml run.
const minioImage = "minio/minio:RELEASE.2025-04-22T22-12-26Z"

const (
	minioUser   = "cashflow"
	minioSecret = "cashflow-test-secret"
	bucket      = "cashflow-test"
)

// startMinIO runs MinIO for the length of the test, creates the bucket, and returns its
// endpoint.
func startMinIO(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := testcontainers.Run(ctx, minioImage,
		testcontainers.WithExposedPorts("9000/tcp"),
		testcontainers.WithEnv(map[string]string{"MINIO_ROOT_USER": minioUser, "MINIO_ROOT_PASSWORD": minioSecret}),
		testcontainers.WithCmd("server", "/data"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/minio/health/ready").WithPort("9000/tcp").WithStartupTimeout(2*time.Minute)),
	)
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, err)
	endpoint, err := ctr.PortEndpoint(ctx, "9000/tcp", "http")
	require.NoError(t, err)

	admin := s3.New(s3.Options{
		Region: "us-east-1", BaseEndpoint: aws.String(endpoint), UsePathStyle: true,
		Credentials: aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return aws.Credentials{AccessKeyID: minioUser, SecretAccessKey: minioSecret}, nil
		}),
	})
	_, err = admin.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	require.NoError(t, err)
	return endpoint
}

func minioStore(t *testing.T, endpoint string) *s3store.Store {
	t.Helper()
	s, err := s3store.New(s3store.Config{
		Endpoint: endpoint, Bucket: bucket, Region: "us-east-1",
		AccessKeyID: minioUser, SecretAccessKey: minioSecret, PathStyle: true,
	})
	require.NoError(t, err)
	return s
}

// putTo sends body to a presigned PUT with headers, as the browser does, and returns the status.
func putTo(t *testing.T, put upload.PresignedPut, body []byte, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPut, put.URL, bytes.NewReader(body))
	require.NoError(t, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	_, _ = io.Copy(io.Discard, resp.Body)
	require.NoError(t, resp.Body.Close())
	return resp.StatusCode
}

// TestAgainstMinIO runs the adapter against a real S3 implementation: the signed size and type
// are enforced by the bucket, and the presigned GET downloads as an attachment.
func TestAgainstMinIO(t *testing.T) {
	endpoint := startMinIO(t)
	s := minioStore(t, endpoint)
	ctx := context.Background()
	const key = "incoming/uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/00112233445566778899aabbccddeeff"
	photo := uploadtest.Image("image/jpeg", 30_000)

	put, err := s.PresignPut(ctx, key, "image/jpeg", int64(len(photo)), time.Minute)
	require.NoError(t, err)

	t.Run("the bucket refuses another size or type than signed", func(t *testing.T) {
		require.Equal(t, http.StatusForbidden, putTo(t, put, photo[:len(photo)-1], put.Headers), "a shorter body")
		require.Equal(t, http.StatusForbidden, putTo(t, put, append(append([]byte{}, photo...), 0), put.Headers), "a longer body")
		require.Equal(t, http.StatusForbidden, putTo(t, put, photo, map[string]string{"Content-Type": "text/html"}), "another type")
		_, err := s.Head(ctx, key)
		require.ErrorIs(t, err, upload.ErrObjectNotFound)
	})

	t.Run("the signed PUT stores the file", func(t *testing.T) {
		require.Equal(t, http.StatusOK, putTo(t, put, photo, put.Headers))
		size, err := s.Head(ctx, key)
		require.NoError(t, err)
		require.Equal(t, int64(len(photo)), size)
	})

	stored := "files/uploads/7b0e0b2a-6f7c-4f43-9a52-3c6c1f0b9b11/00112233445566778899aabbccddeeff"
	t.Run("move, open, and a missing source", func(t *testing.T) {
		require.NoError(t, s.Move(ctx, key, stored))
		_, err := s.Head(ctx, key)
		require.ErrorIs(t, err, upload.ErrObjectNotFound)
		body, err := s.Open(ctx, stored)
		require.NoError(t, err)
		got, err := io.ReadAll(body)
		require.NoError(t, err)
		require.NoError(t, body.Close())
		require.Equal(t, photo, got)
		require.ErrorIs(t, s.Move(ctx, key, stored), upload.ErrObjectNotFound)
		_, err = s.Open(ctx, key)
		require.ErrorIs(t, err, upload.ErrObjectNotFound)
	})

	t.Run("the presigned GET downloads as an attachment", func(t *testing.T) {
		link, err := s.PresignGet(ctx, stored, time.Minute, "bukti-1.jpg", "image/jpeg")
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, link, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Contains(t, resp.Header.Get("Content-Disposition"), `attachment; filename="bukti-1.jpg"`)
		require.Equal(t, "image/jpeg", resp.Header.Get("Content-Type"))
		got, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, photo, got)
	})
}

// TestUploadFlowAgainstMinIO is the whole flow on a real bucket: start, PUT with net/http,
// complete, scan (the fake scanner), download; an infected file ends in quarantine; and a second
// PUT after the scan never changes what is downloaded.
func TestUploadFlowAgainstMinIO(t *testing.T) {
	endpoint := startMinIO(t)
	bucketStore := minioStore(t, endpoint)
	env := testenv.New(t)
	ctx := context.Background()
	o := env.CreateOrg(t, "Griya MinIO", "0812895")
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)
	kos, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Mawar", Kind: property.KindKos})
	require.NoError(t, err)
	unit, err := env.Properties.CreateUnit(ctx, admin, kos.ID, property.UnitInput{Label: "Kamar 3", Terms: testenv.Monthly(750_000)})
	require.NoError(t, err)
	rina, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: "Rina Wulandari", Phone: "0812-8950-5001"})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, admin, tenant.NewLease{TenantID: rina.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(env.Clock), CollectorMemberID: karyawan.MembershipID}})
	require.NoError(t, err)

	uploads := upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: bucketStore, Queue: &uploadtest.Queue{}, Log: env.Log,
	})
	checker := upload.NewChecker(upload.CheckerDeps{
		Store: env.Store, Objects: bucketStore, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
	})
	send := func(t *testing.T, p authz.Principal, body []byte) upload.Created {
		t.Helper()
		c, err := uploads.Create(ctx, p, upload.CreateInput{
			Purpose: upload.PurposeCashReceipt, TenantID: rina.ID, ContentType: "image/jpeg", Size: int64(len(body)),
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, putTo(t, c.Put, body, c.Put.Headers))
		_, err = uploads.Complete(ctx, p, c.Upload.ID)
		require.NoError(t, err)
		return c
	}
	download := func(t *testing.T, id uuid.UUID) []byte {
		t.Helper()
		got, err := uploads.Get(ctx, admin, id)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, got.Status)
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, got.DownloadURL, nil)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, fmt.Sprintf(`attachment; filename="bukti-%s.jpg"; filename*=UTF-8''bukti-%s.jpg`, id, id), resp.Header.Get("Content-Disposition"))
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return b
	}

	t.Run("a clean photo is downloaded as sent, and a later PUT changes nothing", func(t *testing.T) {
		photo := uploadtest.Image("image/jpeg", 80_000)
		c := send(t, karyawan, photo)
		got, err := uploads.Get(ctx, admin, c.Upload.ID)
		require.NoError(t, err)
		require.Empty(t, got.DownloadURL, "pending")

		res, err := checker.Scan(ctx, o.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusClean, res.Status)
		require.Equal(t, photo, download(t, c.Upload.ID))

		// The PUT URL is still valid: a second PUT lands in incoming/ and is never served.
		swapped := append(uploadtest.Image("image/jpeg", 80_000-len(uploadtest.EICAR)), uploadtest.EICAR...)
		require.Equal(t, http.StatusOK, putTo(t, c.Put, swapped, c.Put.Headers))
		require.Equal(t, photo, download(t, c.Upload.ID))
	})

	t.Run("an infected photo is quarantined", func(t *testing.T) {
		body := append(uploadtest.Image("image/jpeg", 4096), uploadtest.EICAR...)
		c := send(t, karyawan, body)
		res, err := checker.Scan(ctx, o.ID, c.Upload.ID)
		require.NoError(t, err)
		require.Equal(t, upload.StatusInfected, res.Status)
		got, err := uploads.Get(ctx, admin, c.Upload.ID)
		require.NoError(t, err)
		require.Empty(t, got.DownloadURL)

		key := objectKey(t, env, o.ID, c.Upload.ID)
		size, err := bucketStore.Head(ctx, upload.QuarantineKey(key))
		require.NoError(t, err)
		require.Equal(t, int64(len(body)), size)
		for _, k := range []string{upload.IncomingKey(key), upload.StoredKey(key)} {
			_, err := bucketStore.Head(ctx, k)
			require.ErrorIs(t, err, upload.ErrObjectNotFound, k)
		}
	})
}

// objectKey reads an upload's key from the database.
func objectKey(t *testing.T, env *testenv.Env, org, id uuid.UUID) string {
	t.Helper()
	var key string
	err := env.Store.WithOrgTx(context.Background(), org, func(tx pgx.Tx) error {
		row, err := dbgen.New(tx).GetUpload(context.Background(), dbgen.GetUploadParams{OrganizationID: org, ID: id})
		key = row.ObjectKey
		return err
	})
	require.NoError(t, err)
	return key
}
