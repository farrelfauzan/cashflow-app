package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	tenants, err := s.Tenants.List(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := apigen.TenantList{Tenants: make([]apigen.Tenant, len(tenants))}
	for i, t := range tenants {
		resp.Tenants[i] = toAPITenant(t)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.TenantInput
	if !s.decode(w, r, &body) {
		return
	}
	t, err := s.Tenants.Create(r.Context(), p, tenantInput(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPITenant(t))
}

func (s *Server) getTenant(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	t, err := s.Tenants.Get(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPITenant(t))
}

func (s *Server) updateTenant(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "tenantID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.TenantInput
	if !s.decode(w, r, &body) {
		return
	}
	t, err := s.Tenants.Update(r.Context(), p, id, tenantInput(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPITenant(t))
}

func (s *Server) createLease(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.LeaseInput
	if !s.decode(w, r, &body) {
		return
	}
	l, err := s.Tenants.CreateLease(r.Context(), p, tenant.NewLease{
		TenantID: body.TenantId,
		UnitID:   body.UnitId,
		Terms: leaseTerms(apigen.LeaseTerms{
			StartDate: body.StartDate, RentAmount: body.RentAmount,
			DepositAmount: body.DepositAmount, CollectorMemberId: body.CollectorMemberId,
		}),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPILease(l))
}

func (s *Server) updateLease(w http.ResponseWriter, r *http.Request) {
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
	var body apigen.LeaseTerms
	if !s.decode(w, r, &body) {
		return
	}
	l, err := s.Tenants.UpdateLease(r.Context(), p, id, leaseTerms(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPILease(l))
}

func (s *Server) moveOut(w http.ResponseWriter, r *http.Request) {
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
	var body apigen.MoveOut
	if !s.decode(w, r, &body) {
		return
	}
	l, err := s.Tenants.MoveOut(r.Context(), p, id, body.EndDate.Time)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPILease(l))
}

func tenantInput(body apigen.TenantInput) tenant.Input {
	in := tenant.Input{Name: body.Name, Phone: body.Phone}
	if body.Note != nil {
		in.Note = *body.Note
	}
	return in
}

func leaseTerms(body apigen.LeaseTerms) tenant.Terms {
	t := tenant.Terms{StartDate: body.StartDate.Time, RentAmount: body.RentAmount}
	if body.DepositAmount != nil {
		t.DepositAmount = *body.DepositAmount
	}
	if body.CollectorMemberId != nil {
		t.CollectorMemberID = *body.CollectorMemberId
	}
	return t
}

func toAPITenant(t tenant.Tenant) apigen.Tenant {
	out := apigen.Tenant{
		Id: t.ID, Name: t.Name, Phone: t.Phone, Note: t.Note,
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt,
		Leases: make([]apigen.Lease, len(t.Leases)),
	}
	for i, l := range t.Leases {
		out.Leases[i] = toAPILease(l)
	}
	return out
}

func toAPILease(l tenant.Lease) apigen.Lease {
	out := apigen.Lease{
		Id: l.ID, TenantId: l.TenantID, UnitId: l.UnitID, UnitLabel: l.UnitLabel,
		PropertyId: l.PropertyID, PropertyName: l.PropertyName,
		StartDate:     openapi_types.Date{Time: l.StartDate},
		RentAmount:    l.RentAmount,
		DepositAmount: l.DepositAmount, Status: apigen.LeaseStatus(l.Status),
		Charges:   make([]apigen.LeaseCharge, len(l.Charges)),
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
	for i, c := range l.Charges {
		out.Charges[i] = apigen.LeaseCharge{ChargeTypeId: c.ChargeTypeID, Name: c.Name, Amount: c.Amount}
	}
	if l.EndDate != nil {
		out.EndDate = &openapi_types.Date{Time: *l.EndDate}
	}
	if l.CollectorMemberID != uuid.Nil {
		out.Collector = &apigen.Collector{MemberId: l.CollectorMemberID, Name: l.CollectorName}
	}
	return out
}
