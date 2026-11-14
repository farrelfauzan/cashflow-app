//go:build integration

package httpapi_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/testenv"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload/uploadtest"
)

func TestUploadsOverHTTP(t *testing.T) {
	env := testenv.New(t)
	o := env.CreateOrg(t, "Griya Foto", "0812891")
	ctx := context.Background()
	admin := env.Principal(t, o.ID, o.Admin.UserID)
	karyawan := env.Principal(t, o.ID, o.Karyawan.UserID)

	kos, err := env.Properties.Create(ctx, admin, property.PropertyInput{Name: "Kos Foto", Kind: property.KindKos})
	require.NoError(t, err)
	unit, err := env.Properties.CreateUnit(ctx, admin, kos.ID, property.UnitInput{Label: "Kamar 1", Terms: testenv.Monthly(700_000)})
	require.NoError(t, err)
	rina, err := env.Tenants.Create(ctx, admin, tenant.Input{Name: "Rina Wulandari", Phone: "0812-8910-5001"})
	require.NoError(t, err)
	_, err = env.Tenants.CreateLease(ctx, admin, tenant.NewLease{TenantID: rina.ID, UnitID: unit.ID, TermMonths: 1,
		Terms: tenant.Terms{StartDate: tenant.Today(env.Clock), CollectorMemberID: karyawan.MembershipID}})
	require.NoError(t, err)

	storage, queue := uploadtest.NewStorage(), &uploadtest.Queue{}
	uploads := upload.NewService(upload.ServiceDeps{
		Store: env.Store, Authz: env.Authz, Clock: env.Clock, Presigner: storage, Queue: queue, Log: env.Log,
	})
	checker := upload.NewChecker(upload.CheckerDeps{
		Store: env.Store, Objects: storage, Scanner: &uploadtest.Scanner{}, Audit: env.Authz, Clock: env.Clock, Log: env.Log,
	})
	deps := httpapi.Deps{
		Log: env.Log, DB: env.Store, Auth: env.Auth, Authz: env.Authz, Members: env.Members,
		CookieSecure: false, AllowedOrigins: []string{origin},
	}
	off := httptest.NewServer(httpapi.New(deps).Handler())
	t.Cleanup(off.Close)
	deps.Uploads = uploads
	srv := httptest.NewServer(httpapi.New(deps).Handler())
	t.Cleanup(srv.Close)

	orgPath := "/api/orgs/" + o.ID.String()
	field := func(e apigen.Error) string {
		if e.Error.Field == nil {
			return ""
		}
		return *e.Error.Field
	}

	t.Run("without file storage the routes answer 503 uploads_unavailable", func(t *testing.T) {
		c := newClient(t, off.URL)
		c.signIn(env, o.Karyawan.Phone)
		for _, req := range []struct{ method, path string }{
			{http.MethodPost, orgPath + "/uploads"},
			{http.MethodPost, orgPath + "/uploads/" + uuid.NewString() + "/complete"},
			{http.MethodGet, orgPath + "/uploads/" + uuid.NewString()},
		} {
			resp, body := c.do(req.method, req.path, apigen.UploadInput{Purpose: apigen.UploadPurpose("cash_deposit"), ContentType: "image/jpeg", Size: 10})
			require.Equal(t, http.StatusServiceUnavailable, resp.StatusCode, req.path)
			require.Equal(t, "uploads_unavailable", decode[apigen.Error](t, body).Error.Code)
		}
	})

	kar := newClient(t, srv.URL)
	kar.signIn(env, o.Karyawan.Phone)
	adm := newClient(t, srv.URL)
	adm.signIn(env, o.Admin.Phone)

	t.Run("bad input names its code and field", func(t *testing.T) {
		tenantID := rina.ID
		missing := uuid.New()
		tests := []struct {
			name       string
			body       any
			wantStatus int
			wantCode   string
			wantField  string
		}{
			{"unknown purpose", map[string]any{"purpose": "selfie", "contentType": "image/jpeg", "size": 10}, http.StatusBadRequest, "upload_purpose_unknown", "purpose"},
			{"not an image type", apigen.UploadInput{Purpose: "cash_receipt", TenantId: &tenantID, ContentType: "application/pdf", Size: 10}, http.StatusBadRequest, "upload_type_not_allowed", "contentType"},
			{"too large", apigen.UploadInput{Purpose: "cash_receipt", TenantId: &tenantID, ContentType: "image/jpeg", Size: upload.MaxSize + 1}, http.StatusBadRequest, "upload_too_large", "size"},
			{"no size", apigen.UploadInput{Purpose: "cash_receipt", TenantId: &tenantID, ContentType: "image/jpeg"}, http.StatusBadRequest, "invalid_input", "size"},
			{"a receipt without a tenant", apigen.UploadInput{Purpose: "cash_receipt", ContentType: "image/jpeg", Size: 10}, http.StatusBadRequest, "invalid_input", "tenantId"},
			{"a tenant that does not exist", apigen.UploadInput{Purpose: "cash_receipt", TenantId: &missing, ContentType: "image/jpeg", Size: 10}, http.StatusNotFound, "not_found", "tenantId"},
			{"an unknown field", map[string]any{"purpose": "cash_deposit", "contentType": "image/jpeg", "size": 10, "key": "uploads/x"}, http.StatusBadRequest, "invalid_json", ""},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				resp, body := kar.do(http.MethodPost, orgPath+"/uploads", tc.body)
				require.Equal(t, tc.wantStatus, resp.StatusCode, string(body))
				e := decode[apigen.Error](t, body)
				require.Equal(t, tc.wantCode, e.Error.Code)
				require.Equal(t, tc.wantField, field(e))
			})
		}
	})

	t.Run("an admin cannot start a receipt upload", func(t *testing.T) {
		tenantID := rina.ID
		resp, body := adm.do(http.MethodPost, orgPath+"/uploads", apigen.UploadInput{Purpose: "cash_receipt", TenantId: &tenantID, ContentType: "image/jpeg", Size: 10})
		require.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
		require.Equal(t, "forbidden", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("an unknown upload is upload_not_found", func(t *testing.T) {
		for _, path := range []string{orgPath + "/uploads/" + uuid.NewString(), orgPath + "/uploads/not-a-uuid"} {
			resp, body := kar.do(http.MethodGet, path, nil)
			require.Equal(t, http.StatusNotFound, resp.StatusCode)
			require.Equal(t, "upload_not_found", decode[apigen.Error](t, body).Error.Code)
		}
		resp, body := kar.do(http.MethodPost, orgPath+"/uploads/"+uuid.NewString()+"/complete", nil)
		require.Equal(t, http.StatusNotFound, resp.StatusCode)
		require.Equal(t, "upload_not_found", decode[apigen.Error](t, body).Error.Code)
	})

	t.Run("start, PUT, complete, scan, download", func(t *testing.T) {
		tenantID := rina.ID
		photo := uploadtest.Image("image/webp", 12_345)
		resp, body := kar.do(http.MethodPost, orgPath+"/uploads", apigen.UploadInput{
			Purpose: "cash_receipt", TenantId: &tenantID, ContentType: "image/webp", Size: int64(len(photo)),
		})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		ticket := decode[apigen.UploadTicket](t, body)
		require.Equal(t, apigen.UploadStatus("pending"), ticket.Upload.Status)
		require.Equal(t, &tenantID, ticket.Upload.TenantId)
		require.Equal(t, map[string]string{"Content-Type": "image/webp"}, ticket.PutHeaders)
		require.Nil(t, ticket.Upload.DownloadUrl)
		require.Nil(t, ticket.Upload.ContentType)
		require.Contains(t, string(body), `"putExpiresAt"`)

		u, err := url.Parse(ticket.PutUrl)
		require.NoError(t, err)
		storage.Put(strings.TrimPrefix(u.Path, "/"), photo)

		uploadPath := orgPath + "/uploads/" + ticket.Upload.Id.String()
		resp, body = kar.do(http.MethodPost, uploadPath+"/complete", nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.NotNil(t, decode[apigen.Upload](t, body).CompletedAt)
		require.Contains(t, queue.Enqueued(), ticket.Upload.Id)

		resp, body = adm.do(http.MethodGet, uploadPath, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Nil(t, decode[apigen.Upload](t, body).DownloadUrl, "pending: no link")

		_, err = checker.Scan(ctx, o.ID, ticket.Upload.Id)
		require.NoError(t, err)

		resp, body = adm.do(http.MethodGet, uploadPath, nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		got := decode[apigen.Upload](t, body)
		require.Equal(t, apigen.UploadStatus("clean"), got.Status)
		require.Equal(t, "image/webp", *got.ContentType)
		require.NotNil(t, got.DownloadUrl)
		require.Contains(t, *got.DownloadUrl, fmt.Sprintf("filename=bukti-%s.webp", ticket.Upload.Id))
		require.NotNil(t, got.DownloadExpiresAt)
		require.Equal(t, "no-store", resp.Header.Get("Cache-Control"))
	})
}
