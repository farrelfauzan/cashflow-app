package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/autogopay"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/demoseed"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/org"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/storage/s3store"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// putTimeout bounds one PUT of a demo photo to storage.
const putTimeout = 30 * time.Second

// seedCases seeds the case organization after the other demo organizations are billed, and
// prints what it did. Without STORAGE_* it seeds the cases that need no photo and says which
// were skipped.
func seedCases(ctx context.Context, cn conn, c clock.Clock, stdout, stderr io.Writer) error {
	az := authz.NewService(cn.store, c, cn.log)
	d := demoseed.CaseDeps{
		Store: cn.store, Authz: az, Orgs: org.NewService(cn.store, az),
		AutoGoPay: autogopay.NewService(cn.store, cn.keys, az, c),
		Log:       cn.log,
	}
	bucket, ok, err := cn.admin.CaseStorage()
	if err != nil {
		return err
	}
	if ok {
		st, err := s3store.New(s3store.Config{
			Endpoint: bucket.Endpoint, PresignEndpoint: bucket.PresignEndpoint, Bucket: bucket.Bucket, Region: bucket.Region,
			AccessKeyID: bucket.AccessKeyID, SecretAccessKey: bucket.SecretAccessKey, PathStyle: bucket.PathStyle,
		})
		if err != nil {
			return err
		}
		d.Files = &demoseed.CaseFiles{Storage: st, Put: putFile(&http.Client{Timeout: putTimeout})}
	}
	sum, err := demoseed.SeedCases(ctx, d, c)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s: %d invoices created, %d payments recorded, %d promises, %d visit tasks, %d visit outcomes, %d photos, %d deposits\n",
		sum.Organization, sum.Invoices, sum.Payments, sum.Promises, sum.VisitTasks, sum.Outcomes, sum.Uploads, sum.Deposits)
	if sum.FilesSkipped {
		fmt.Fprintf(stderr, "admin: note: %s: STORAGE_* is not set, so the cases that need a photo were skipped "+
			"(cash, deposits, transfer proofs, the infected file); set it, start the storage (make dev-up-uploads) and run generate-invoices again\n",
			sum.Organization)
	}
	return nil
}

// putFile PUTs a demo photo to its presigned URL with the signed headers, as the browser does.
func putFile(hc *http.Client) func(ctx context.Context, put upload.PresignedPut, body []byte) error {
	return func(ctx context.Context, put upload.PresignedPut, body []byte) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, put.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("building the PUT: %w", err)
		}
		for k, v := range put.Headers {
			req.Header.Set(k, v)
		}
		res, err := hc.Do(req)
		if err != nil {
			// The URL is presigned: it is not part of the error.
			return fmt.Errorf("putting the photo in storage: %w", stripURL(err))
		}
		defer func() { _ = res.Body.Close() }() // read only
		if res.StatusCode/100 != 2 {
			return fmt.Errorf("storage answered the PUT with %s", res.Status)
		}
		return nil
	}
}

// stripURL drops the URL from an HTTP client error: a presigned URL carries its signature.
func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}
