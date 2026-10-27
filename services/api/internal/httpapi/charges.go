package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

func (s *Server) listChargeTypes(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	types, err := s.Tenants.ListChargeTypes(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := apigen.ChargeTypeList{ChargeTypes: make([]apigen.ChargeType, len(types))}
	for i, ct := range types {
		resp.ChargeTypes[i] = toAPIChargeType(ct)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createChargeType(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.ChargeTypeInput
	if !s.decode(w, r, &body) {
		return
	}
	in := tenant.ChargeTypeInput{Name: body.Name}
	if body.Metered != nil {
		in.Metered = *body.Metered
	}
	ct, err := s.Tenants.CreateChargeType(r.Context(), p, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPIChargeType(ct))
}

func (s *Server) setLeaseCharges(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "leaseID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.LeaseChargesInput
	if !s.decode(w, r, &body) {
		return
	}
	// A missing or null list would read as "remove every extra"; only an explicit [] does that.
	if body.Charges == nil {
		s.fail(w, r, &tenant.InputError{Field: "charges", Message: "Daftar biaya wajib diisi. Kirim daftar kosong untuk menghapus semua biaya."})
		return
	}
	in := make([]tenant.ChargeInput, len(body.Charges))
	for i, c := range body.Charges {
		in[i] = tenant.ChargeInput{ChargeTypeID: c.ChargeTypeId, Amount: c.Amount}
	}
	l, err := s.Tenants.SetLeaseCharges(r.Context(), p, id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPILease(l))
}

func toAPIChargeType(ct tenant.ChargeType) apigen.ChargeType {
	return apigen.ChargeType{Id: ct.ID, Name: ct.Name, Metered: ct.Metered, CreatedAt: ct.CreatedAt, UpdatedAt: ct.UpdatedAt}
}
