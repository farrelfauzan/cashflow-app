package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/auth"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/member"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenantimport"
)

// fail turns a domain error into a response. Messages are in Bahasa Indonesia and never carry
// internal detail; anything unexpected is logged and answered with a generic 500.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var input *member.InputError
	var propertyInput *property.InputError
	var tenantInput *tenant.InputError
	var fileErr *tenantimport.FileError
	switch {
	case errors.As(err, &input):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, input.Field)
	case errors.As(err, &propertyInput):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", propertyInput.Message, propertyInput.Field)
	case errors.As(err, &tenantInput):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", tenantInput.Message, tenantInput.Field)
	case errors.As(err, &fileErr):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", fileErr.Message, "file")
	case errors.Is(err, auth.ErrInvalidPhone):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_phone", "Nomor WhatsApp tidak valid.", "phone")
	case errors.Is(err, auth.ErrInvalidCode):
		s.writeErrorField(w, http.StatusBadRequest, "invalid_code", "Kode salah atau sudah kedaluwarsa.", "code")
	case errors.Is(err, auth.ErrRateLimited):
		s.writeError(w, http.StatusTooManyRequests, "rate_limited", "Terlalu banyak percobaan. Coba lagi dalam 15 menit.")
	case errors.Is(err, auth.ErrUnauthenticated):
		s.writeError(w, http.StatusUnauthorized, "unauthenticated", "Silakan masuk terlebih dahulu.")
	case errors.Is(err, authz.ErrForbidden):
		s.log.InfoContext(r.Context(), "permission denied", "error", err)
		s.writeError(w, http.StatusForbidden, "forbidden", "Anda tidak punya akses untuk tindakan ini.")
	case errors.Is(err, authz.ErrNotMember), errors.Is(err, member.ErrNotFound), errors.Is(err, property.ErrNotFound),
		errors.Is(err, tenant.ErrNotFound):
		s.notFound(w, r)
	case errors.Is(err, property.ErrNameTaken):
		s.writeErrorField(w, http.StatusConflict, "name_taken", "Nama properti ini sudah dipakai.", "name")
	case errors.Is(err, property.ErrLabelTaken):
		s.writeErrorField(w, http.StatusConflict, "label_taken", "Nama unit ini sudah dipakai di properti ini.", "label")
	case errors.Is(err, tenant.ErrPhoneTaken):
		s.writeErrorField(w, http.StatusConflict, "phone_taken", "Nomor ini sudah dipakai penyewa lain.", "phone")
	case errors.Is(err, tenant.ErrChargeTypeTaken):
		s.writeErrorField(w, http.StatusConflict, "charge_type_taken", "Nama biaya ini sudah dipakai.", "name")
	case errors.Is(err, tenant.ErrUnitOccupied):
		s.writeErrorField(w, http.StatusConflict, "unit_occupied", "Unit ini masih disewa. Akhiri sewa yang lama dulu.", "unitId")
	case errors.Is(err, tenant.ErrLeaseEnded):
		s.writeError(w, http.StatusConflict, "lease_ended", "Sewa ini sudah berakhir dan tidak bisa diubah.")
	case errors.Is(err, member.ErrLastAdmin):
		s.writeError(w, http.StatusConflict, "last_admin", "Anggota ini admin terakhir yang aktif. Tambahkan admin lain dulu.")
	case errors.Is(err, member.ErrAlreadyMember):
		s.writeErrorField(w, http.StatusConflict, "already_member", "Nomor ini sudah terdaftar di usaha ini.", "phone")
	case errors.Is(err, messaging.ErrNotConnected), errors.Is(err, messaging.ErrRateLimited),
		errors.Is(err, messaging.ErrUndeliverable):
		// The platform number is logged out, throttled by WhatsApp, or cannot reach this number.
		// An operator may need to act, so log it loudly.
		s.log.ErrorContext(r.Context(), "whatsapp unavailable", "error", err)
		s.writeError(w, http.StatusServiceUnavailable, "whatsapp_unavailable",
			"WhatsApp sedang tidak bisa mengirim pesan. Coba lagi beberapa menit lagi.")
	default:
		s.log.ErrorContext(r.Context(), "request failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal", "Terjadi kesalahan. Coba lagi.")
	}
}

// decode reads a single JSON object into dst and rejects unknown fields. It answers the request
// itself and returns false when the body is not acceptable.
func (s *Server) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil || dec.More() {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Permintaan terlalu besar.")
			return false
		}
		s.writeError(w, http.StatusBadRequest, "invalid_json", "Format permintaan tidak valid.")
		return false
	}
	return true
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, message string) {
	s.writeErrorField(w, status, code, message, "")
}

func (s *Server) writeErrorField(w http.ResponseWriter, status int, code, message, field string) {
	var body apigen.Error
	body.Error.Code = code
	body.Error.Message = message
	if field != "" {
		body.Error.Field = &field
	}
	s.writeJSON(w, status, body)
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Error("writing response", "error", err)
	}
}
