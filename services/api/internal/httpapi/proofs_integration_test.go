//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// proofForm is a multipart/form-data body with one file part named field.
func proofForm(t *testing.T, field string, file []byte) (contentType string, body []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, "bukti.jpg")
	require.NoError(t, err)
	_, err = part.Write(file)
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return mw.FormDataContentType(), buf.Bytes()
}

// postProof posts body as contentType to token's proof route with no session, from a fresh
// address, with origin when it is not empty.
func postProof(t *testing.T, c *qrisCaller, token, origin, contentType string, body []byte) (result, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, c.base+"/api/t/"+token+"/proof", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-Forwarded-For", c.freshIP())
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return result{StatusCode: resp.StatusCode, Header: resp.Header}, data
}

// sendProofFile posts file as the "file" part, from the app's origin as the payment page does.
func sendProofFile(t *testing.T, c *qrisCaller, token string, file []byte) (result, []byte) {
	t.Helper()
	ct, body := proofForm(t, "file", file)
	return postProof(t, c, token, origin, ct, body)
}

// TestPaymentProofsOverHTTP covers sendPaymentProof end to end, and the proofs the payment page
// then lists.
func TestPaymentProofsOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 2, 9, 0, 0, 0, clock.Jakarta))
	a := newLinkOrg(t, env, "Griya Bukti", "0812631")

	storage, queue := uploadtest.NewStorage(), &uploadtest.Queue{}
	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	deps := httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing, PayLinks: links,
		PublicLimiter: env.Limiter, CookieSecure: false, AllowedOrigins: []string{origin},
	}
	off := httptest.NewServer(httpapi.New(deps).Handler())
	t.Cleanup(off.Close)
	deps.Proofs = upload.NewProofs(upload.ProofsDeps{
		Store: env.Store, Links: links, Objects: storage, Queue: queue, Clock: env.Clock, Log: env.Log,
	})
	srv := httptest.NewServer(httpapi.New(deps).Handler())
	t.Cleanup(srv.Close)
	caller := &qrisCaller{base: srv.URL}
	photo := uploadtest.Image("image/jpeg", 20_000)

	t.Run("a photo is received, and the page lists it as waiting", func(t *testing.T) {
		rina := a.rentWithLink(t, "Rina Wulandari")
		resp, body := getPage(t, srv.URL, rina.token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Contains(t, string(body), `"proofsWaiting":[]`, "an empty list, not null")

		resp, body = sendProofFile(t, caller, rina.token, photo)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		requirePublicHeaders(t, resp)
		proof := decode[apigen.PaymentProof](t, body)
		require.NotEqual(t, uuid.Nil, proof.Id)
		require.True(t, proof.CreatedAt.Equal(env.Clock.Now()), "%s", proof.CreatedAt)
		require.Len(t, queue.Enqueued(), 1)
		keys := storage.Keys()
		require.Len(t, keys, 1)
		require.True(t, strings.HasPrefix(keys[0], "incoming/uploads/"+a.o.ID.String()+"/"), keys[0])
		stored, ok := storage.Object(keys[0])
		require.True(t, ok)
		require.Equal(t, photo, stored)

		resp, body = getPage(t, srv.URL, rina.token, caller.freshIP())
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		page := decode[apigen.PaymentPage](t, body)
		require.Len(t, page.ProofsWaiting, 1)
		require.Equal(t, proof.Id, page.ProofsWaiting[0].Id)
		require.True(t, page.ProofsWaiting[0].CreatedAt.Equal(proof.CreatedAt))
	})

	t.Run("a photo above the 1 MiB body limit of other routes is received", func(t *testing.T) {
		r := a.rentWithLink(t, "Budi Santoso")
		resp, body := sendProofFile(t, caller, r.token, uploadtest.Image("image/png", 2<<20))
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	})

	t.Run("a bad request names its code and the file field", func(t *testing.T) {
		textCT, textBody := proofForm(t, "file", uploadtest.Image("text/plain", 500))
		photoCT, photoBody := proofForm(t, "photo", photo)
		bigCT, bigBody := proofForm(t, "file", uploadtest.Image("image/jpeg", int(upload.MaxSize)+1))
		emptyCT, emptyBody := proofForm(t, "file", nil)
		tests := []struct {
			name        string
			contentType string
			body        []byte
			code        string
		}{
			{"not multipart", "application/json", []byte(`{"file":"x"}`), "invalid_input"},
			{"multipart with no part", "multipart/form-data; boundary=xyz", []byte("--xyz--\r\n"), "invalid_input"},
			{"the first part is not the file", photoCT, photoBody, "invalid_input"},
			{"an empty file", emptyCT, emptyBody, "invalid_input"},
			{"a text file", textCT, textBody, "upload_type_not_allowed"},
			{"over 10 MB", bigCT, bigBody, "upload_too_large"},
		}
		r := a.rentWithLink(t, "Dewi Lestari")
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				before := len(storage.Keys())
				resp, body := postProof(t, caller, r.token, origin, tc.contentType, tc.body)
				requirePublicHeaders(t, resp)
				requireAPIError(t, resp, body, http.StatusBadRequest, tc.code, "file")
				require.Len(t, storage.Keys(), before, "nothing stored")
			})
		}
	})

	t.Run("an unknown or revoked link is 404 payment_link_not_found", func(t *testing.T) {
		r := a.rentWithLink(t, "Eko Prasetyo")
		_, err := links.RevokeAll(ctx, a.admin, r.invoiceID)
		require.NoError(t, err)
		for name, token := range map[string]string{"unknown": strings.Repeat("A", 43), "revoked": r.token} {
			t.Run(name, func(t *testing.T) {
				resp, body := sendProofFile(t, caller, token, photo)
				requirePublicHeaders(t, resp)
				requireAPIError(t, resp, body, http.StatusNotFound, "payment_link_not_found", "")
			})
		}
	})

	t.Run("an eleventh proof on one link within the hour is 429 rate_limited", func(t *testing.T) {
		r := a.rentWithLink(t, "Sari Puspita")
		small := uploadtest.Image("image/webp", 1000)
		for i := range 10 {
			resp, body := sendProofFile(t, caller, r.token, small)
			require.Equal(t, http.StatusCreated, resp.StatusCode, "proof %d: %s", i+1, body)
		}
		before := len(storage.Keys())
		resp, body := sendProofFile(t, caller, r.token, small)
		requirePublicHeaders(t, resp)
		requireAPIError(t, resp, body, http.StatusTooManyRequests, "rate_limited", "")
		require.Len(t, storage.Keys(), before)

		env.Clock.Advance(time.Hour)
		resp, body = sendProofFile(t, caller, r.token, small)
		require.Equal(t, http.StatusCreated, resp.StatusCode, "a new hour: %s", body)
	})

	t.Run("a request from another site is 403 bad_origin", func(t *testing.T) {
		r := a.rentWithLink(t, "Tono Wijaya")
		ct, body := proofForm(t, "file", photo)
		resp, data := postProof(t, caller, r.token, "https://evil.example.com", ct, body)
		require.Equal(t, "no-referrer", resp.Header.Get("Referrer-Policy"))
		requireAPIError(t, resp, data, http.StatusForbidden, "bad_origin", "")
	})

	t.Run("without file storage it is 503 uploads_unavailable", func(t *testing.T) {
		r := a.rentWithLink(t, "Wati Susanti")
		resp, body := sendProofFile(t, &qrisCaller{base: off.URL, sent: 5000}, r.token, photo)
		requirePublicHeaders(t, resp)
		requireAPIError(t, resp, body, http.StatusServiceUnavailable, "uploads_unavailable", "")
	})

	// Last: it moves the clock 30 days ahead.
	t.Run("an expired link is 410 payment_link_expired", func(t *testing.T) {
		r := a.rentWithLink(t, "Yanti Kusuma")
		zero := int64(0)
		_, err := env.Billing.CreateCorrection(ctx, a.admin, billing.CorrectionInput{
			RequestID: uuid.New(), InvoiceID: r.invoiceID, NewAmount: &zero, Reason: "sewa tidak diperpanjang",
		})
		require.NoError(t, err)
		env.Clock.Set(time.Now().Add(paylink.LinkLifetimeAfterPaid + time.Hour).In(clock.Jakarta))
		resp, body := sendProofFile(t, caller, r.token, photo)
		requirePublicHeaders(t, resp)
		requireAPIError(t, resp, body, http.StatusGone, "payment_link_expired", "")
	})
}
