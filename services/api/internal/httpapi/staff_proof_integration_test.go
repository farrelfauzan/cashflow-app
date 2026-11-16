//go:build integration

package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/checkqueue"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/paylink"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

// TestStaffProofOverHTTP covers a transfer proof a member sends for a tenant (decision 49):
// createUpload with purpose transfer_proof, completeUpload, and its item in listCheckQueue with
// sentBy and no invoiceId, next to a link proof that keeps its invoiceId and has no sentBy.
func TestStaffProofOverHTTP(t *testing.T) {
	ctx := context.Background()
	env := testenv.New(t)
	env.Clock.Set(time.Date(2026, 11, 14, 9, 0, 0, 0, clock.Jakarta))
	a := newLinkOrg(t, env, "Griya Bukti Staf", "0812568")
	dina, err := env.Members.Invite(ctx, a.admin, member.Invite{Phone: "0812568099", Name: "Dina Kasir",
		Roles: []member.RoleGrant{{Role: authz.RoleKaryawan}}})
	require.NoError(t, err)

	links := paylink.NewService(env.Store, env.Authz, env.Clock, linkOrigin)
	storage, scanQueue := uploadtest.NewStorage(), &uploadtest.Queue{}
	uploads := upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: storage, Queue: scanQueue, Log: env.Log,
	})
	proofs := upload.NewProofs(upload.ProofsDeps{
		Store: env.Store, Links: links, Objects: storage, Queue: scanQueue, Clock: env.Clock, Log: env.Log,
	})
	srv := httptest.NewServer(httpapi.New(httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		Properties: env.Properties, Tenants: env.Tenants, Billing: env.Billing, PayLinks: links,
		Uploads:      uploads,
		CheckQueue:   checkqueue.NewService(env.Store, env.Authz, env.Billing, env.Tenants, env.Clock),
		CookieSecure: false, AllowedOrigins: []string{origin},
	}).Handler())
	t.Cleanup(srv.Close)

	rinaID, rinaInvoice := a.rent(t, "Rina Wulandari", a.karyawan.MembershipID)
	budiID, _ := a.rent(t, "Budi Santoso", dina.ID)
	_, rinaToken := a.mintLink(t, rinaInvoice)
	photo := uploadtest.Image("image/jpeg", 3_000)
	linkProof, err := proofs.Send(ctx, rinaToken, uuid.Nil, func() (io.Reader, error) { return bytes.NewReader(photo), nil })
	require.NoError(t, err)
	env.Clock.Advance(time.Minute)

	kar, adm := newClient(t, srv.URL), newClient(t, srv.URL)
	kar.signIn(env, a.o.Karyawan.Phone)
	adm.signIn(env, a.o.Admin.Phone)
	orgPath := "/api/orgs/" + a.o.ID.String()
	proofInput := func(tenantID uuid.UUID) apigen.UploadInput {
		return apigen.UploadInput{Purpose: apigen.UploadPurposeTransferProof, TenantId: &tenantID, ContentType: "image/jpeg", Size: int64(len(photo))}
	}

	t.Run("createUpload refuses a tenant the karyawan does not collect from, and a proof without a tenant", func(t *testing.T) {
		tests := []struct {
			name       string
			body       apigen.UploadInput
			wantStatus int
			wantCode   string
			wantField  string
		}{
			{"another karyawan's tenant", proofInput(budiID), http.StatusForbidden, "forbidden", ""},
			{"no tenant", apigen.UploadInput{Purpose: apigen.UploadPurposeTransferProof, ContentType: "image/jpeg", Size: 10}, http.StatusBadRequest, "invalid_input", "tenantId"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := kar.do(http.MethodPost, orgPath+"/uploads", tc.body)
				require.Equal(t, tc.wantStatus, resp.StatusCode, string(body))
				e := decode[apigen.Error](t, body)
				require.Equal(t, tc.wantCode, e.Error.Code)
				field := ""
				if e.Error.Field != nil {
					field = *e.Error.Field
				}
				require.Equal(t, tc.wantField, field)
			})
		}
	})

	var staffUpload uuid.UUID
	t.Run("a karyawan starts, PUTs and completes a proof for their tenant", func(t *testing.T) {
		resp, body := kar.do(http.MethodPost, orgPath+"/uploads", proofInput(rinaID))
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		ticket := decode[apigen.UploadTicket](t, body)
		require.Equal(t, apigen.UploadPurposeTransferProof, ticket.Upload.Purpose)
		require.Equal(t, &rinaID, ticket.Upload.TenantId)
		require.Equal(t, apigen.UploadStatus("pending"), ticket.Upload.Status)
		staffUpload = ticket.Upload.Id

		u, err := url.Parse(ticket.PutUrl)
		require.NoError(t, err)
		storage.Put(strings.TrimPrefix(u.Path, "/"), photo)

		path := orgPath + "/uploads/" + staffUpload.String() + "/complete"
		resp, body = adm.do(http.MethodPost, path, nil)
		require.Equal(t, http.StatusForbidden, resp.StatusCode, "only the sender completes it: %s", body)
		resp, body = kar.do(http.MethodPost, path, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.NotNil(t, decode[apigen.Upload](t, body).CompletedAt)
		require.Contains(t, scanQueue.Enqueued(), staffUpload)
		resp, body = kar.do(http.MethodPost, path, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "completing again is harmless: %s", body)

		resp, body = kar.do(http.MethodGet, orgPath+"/uploads/"+staffUpload.String(), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, "the sender reads it: %s", body)
	})

	t.Run("listCheckQueue shows the staff proof with sentBy and no invoiceId, the link proof with invoiceId and no sentBy", func(t *testing.T) {
		require.NotEqual(t, uuid.Nil, staffUpload)
		resp, body := adm.do(http.MethodGet, orgPath+"/check-queue?kind=transfer_proof", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		var raw struct {
			Items []json.RawMessage `json:"items"`
		}
		require.NoError(t, json.Unmarshal(body, &raw))
		require.Len(t, raw.Items, 2, string(body))

		var staff, link *apigen.CheckQueueItem
		var staffRaw, linkRaw string
		for _, r := range raw.Items {
			it := decode[apigen.CheckQueueItem](t, r)
			require.Equal(t, apigen.CheckQueueKindTransferProof, it.Kind)
			require.NotNil(t, it.TransferProof)
			switch it.TransferProof.UploadId {
			case staffUpload:
				staff, staffRaw = &it, string(r)
			case linkProof.UploadID:
				link, linkRaw = &it, string(r)
			}
		}
		require.NotNil(t, staff, "the staff proof is listed")
		require.NotNil(t, link, "the link proof is listed")

		require.Equal(t, &apigen.CheckStaff{StaffUserId: a.o.Karyawan.UserID, Name: a.o.Karyawan.Name}, staff.TransferProof.SentBy)
		require.Nil(t, staff.TransferProof.InvoiceId)
		require.NotContains(t, staffRaw, `"invoiceId"`)
		require.Equal(t, rinaID, staff.TransferProof.Tenant.Id)

		require.Equal(t, linkProof.ID, link.Id)
		require.Equal(t, &rinaInvoice, link.TransferProof.InvoiceId)
		require.Nil(t, link.TransferProof.SentBy)
		require.NotContains(t, linkRaw, `"sentBy"`)
	})
}
