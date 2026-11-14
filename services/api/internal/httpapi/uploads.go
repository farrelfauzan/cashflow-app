package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/upload"
)

// uploadsAvailable answers 503 uploads_unavailable when this server has no file storage
// (development without STORAGE_*), and reports whether the handler may go on.
func (s *Server) uploadsAvailable(w http.ResponseWriter) bool {
	if s.Uploads != nil {
		return true
	}
	s.writeError(w, http.StatusServiceUnavailable, "uploads_unavailable", "Unggah foto belum tersedia di server ini.")
	return false
}

func (s *Server) createUpload(w http.ResponseWriter, r *http.Request) {
	if !s.uploadsAvailable(w) {
		return
	}
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.UploadInput
	if !s.decode(w, r, &body) {
		return
	}
	in := upload.CreateInput{Purpose: string(body.Purpose), ContentType: body.ContentType, Size: body.Size}
	if body.TenantId != nil {
		in.TenantID = *body.TenantId
	}
	res, err := s.Uploads.Create(r.Context(), p, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, apigen.UploadTicket{
		Upload: toAPIUpload(upload.Read{Upload: res.Upload}), PutUrl: res.Put.URL, PutHeaders: res.Put.Headers,
		PutExpiresAt: res.ExpiresAt,
	})
}

func (s *Server) completeUpload(w http.ResponseWriter, r *http.Request) {
	if !s.uploadsAvailable(w) {
		return
	}
	p, id, ok := s.uploadRequest(w, r)
	if !ok {
		return
	}
	u, err := s.Uploads.Complete(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIUpload(upload.Read{Upload: u}))
}

func (s *Server) getUpload(w http.ResponseWriter, r *http.Request) {
	if !s.uploadsAvailable(w) {
		return
	}
	p, id, ok := s.uploadRequest(w, r)
	if !ok {
		return
	}
	u, err := s.Uploads.Get(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIUpload(u))
}

// uploadRequest reads the principal and the upload id of an upload route. A malformed id is
// the same 404 as an upload that does not exist.
func (s *Server) uploadRequest(w http.ResponseWriter, r *http.Request) (p authz.Principal, id uuid.UUID, ok bool) {
	p, ok = principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return p, uuid.Nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "uploadID"))
	if err != nil {
		s.fail(w, r, upload.ErrNotFound)
		return p, uuid.Nil, false
	}
	return p, id, true
}

func toAPIUpload(u upload.Read) apigen.Upload {
	out := apigen.Upload{
		Id: u.ID, Purpose: apigen.UploadPurpose(u.Purpose), Status: apigen.UploadStatus(u.Status), Size: u.Size,
		CreatedAt: u.CreatedAt, CompletedAt: u.CompletedAt, ScannedAt: u.ScannedAt,
	}
	if u.TenantID != uuid.Nil {
		id := u.TenantID
		out.TenantId = &id
	}
	if u.ContentType != "" {
		ct := u.ContentType
		out.ContentType = &ct
	}
	if u.DownloadURL != "" {
		url, exp := u.DownloadURL, u.DownloadExpiresAt
		out.DownloadUrl, out.DownloadExpiresAt = &url, &exp
	}
	return out
}

// failUpload answers the upload package's errors and reports whether err was one of them.
func (s *Server) failUpload(w http.ResponseWriter, err error) bool {
	var input *upload.InputError
	switch {
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, input.Field)
	case errors.Is(err, upload.ErrPurposeUnknown):
		s.writeErrorField(w, http.StatusBadRequest, "upload_purpose_unknown", "Jenis unggahan tidak dikenal.", "purpose")
	case errors.Is(err, upload.ErrTypeNotAllowed):
		s.writeErrorField(w, http.StatusBadRequest, "upload_type_not_allowed",
			"Hanya foto JPEG, PNG atau WebP yang bisa diunggah.", "contentType")
	case errors.Is(err, upload.ErrTooLarge):
		s.writeErrorField(w, http.StatusBadRequest, "upload_too_large", "Ukuran foto paling besar 10 MB.", "size")
	case errors.Is(err, upload.ErrTenantNotFound):
		s.writeErrorField(w, http.StatusNotFound, "not_found", "Tidak ditemukan.", "tenantId")
	case errors.Is(err, upload.ErrNotFound):
		s.writeError(w, http.StatusNotFound, "upload_not_found", "Unggahan tidak ditemukan.")
	default:
		return false
	}
	return true
}
