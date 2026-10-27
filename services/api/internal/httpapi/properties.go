package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
)

func (s *Server) listProperties(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	props, err := s.Properties.List(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := apigen.PropertyList{Properties: make([]apigen.Property, len(props))}
	for i, prop := range props {
		resp.Properties[i] = toAPIProperty(prop)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createProperty(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	var body apigen.PropertyInput
	if !s.decode(w, r, &body) {
		return
	}
	prop, err := s.Properties.Create(r.Context(), p, propertyInput(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPIProperty(prop))
}

func (s *Server) updateProperty(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "propertyID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.PropertyInput
	if !s.decode(w, r, &body) {
		return
	}
	prop, err := s.Properties.Update(r.Context(), p, id, propertyInput(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIProperty(prop))
}

func (s *Server) listUnits(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	propertyID, err := uuid.Parse(chi.URLParam(r, "propertyID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	units, err := s.Properties.ListUnits(r.Context(), p, propertyID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	resp := apigen.UnitList{Units: make([]apigen.Unit, len(units))}
	for i, u := range units {
		resp.Units[i] = toAPIUnit(u)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createUnit(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	propertyID, err := uuid.Parse(chi.URLParam(r, "propertyID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.UnitInput
	if !s.decode(w, r, &body) {
		return
	}
	u, err := s.Properties.CreateUnit(r.Context(), p, propertyID, property.UnitInput{Label: body.Label, DefaultRent: body.DefaultRent})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPIUnit(u))
}

func (s *Server) updateUnit(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "unitID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	var body apigen.UnitInput
	if !s.decode(w, r, &body) {
		return
	}
	u, err := s.Properties.UpdateUnit(r.Context(), p, id, property.UnitInput{Label: body.Label, DefaultRent: body.DefaultRent})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIUnit(u))
}

func propertyInput(body apigen.PropertyInput) property.PropertyInput {
	in := property.PropertyInput{Name: body.Name, Kind: string(body.Kind)}
	if body.Address != nil {
		in.Address = *body.Address
	}
	return in
}

func toAPIProperty(p property.Property) apigen.Property {
	return apigen.Property{
		Id: p.ID, Name: p.Name, Kind: apigen.PropertyKind(p.Kind), Address: p.Address,
		UnitCount: p.UnitCount, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toAPIUnit(u property.Unit) apigen.Unit {
	return apigen.Unit{
		Id: u.ID, PropertyId: u.PropertyID, Label: u.Label, DefaultRent: u.DefaultRent,
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
}
