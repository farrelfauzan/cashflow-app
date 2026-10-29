package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/property"
)

func (s *Server) listProperties(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Properties.List(r.Context(), p, property.ListQuery{Page: lp.page, Search: lp.search, Sort: lp.sort})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]apigen.Property, len(res.Items))
	for i, prop := range res.Items {
		items[i] = toAPIProperty(prop)
	}
	s.writeJSON(w, http.StatusOK, apigen.PropertyList{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	})
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

func (s *Server) getProperty(w http.ResponseWriter, r *http.Request) {
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
	prop, err := s.Properties.Get(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIProperty(prop))
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
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Properties.ListUnits(r.Context(), p, propertyID, property.UnitQuery{Page: lp.page, Search: lp.search, Sort: lp.sort})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items := make([]apigen.Unit, len(res.Items))
	for i, u := range res.Items {
		items[i] = toAPIUnit(u)
	}
	s.writeJSON(w, http.StatusOK, apigen.UnitList{
		Items: items, Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	})
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
	u, err := s.Properties.CreateUnit(r.Context(), p, propertyID, unitInput(body))
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
	u, err := s.Properties.UpdateUnit(r.Context(), p, id, unitInput(body))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIUnit(u))
}

func (s *Server) listUnitPrices(w http.ResponseWriter, r *http.Request) {
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
	prices, err := s.Properties.ListUnitPrices(r.Context(), p, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make(apigen.UnitPriceHistory, len(prices))
	for i, up := range prices {
		out[i] = toAPIUnitPrice(up)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUnitPrice(w http.ResponseWriter, r *http.Request) {
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
	var body apigen.UnitPriceInput
	if !s.decode(w, r, &body) {
		return
	}
	up, err := s.Properties.CreateUnitPrice(r.Context(), p, id, property.PriceInput{
		Months: int(body.Months), Amount: body.Amount, EffectiveFrom: body.EffectiveFrom.Time,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusCreated, toAPIUnitPrice(up))
}

func unitInput(body apigen.UnitInput) property.UnitInput {
	in := property.UnitInput{Label: body.Label}
	if body.Terms != nil {
		in.Terms = make([]property.TermInput, len(*body.Terms))
		for i, t := range *body.Terms {
			in.Terms[i] = property.TermInput{Months: int(t.Months), Price: t.Price}
		}
	}
	return in
}

func toAPIUnitPrice(up property.UnitPrice) apigen.UnitPrice {
	return apigen.UnitPrice{
		Id: up.ID, Months: apigen.TermMonths(up.Months), Amount: up.Amount,
		EffectiveFrom: openapi_types.Date{Time: up.EffectiveFrom}, RecordedAt: up.RecordedAt,
		RecordedByName: up.RecordedByName,
	}
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
		UnitCount: p.UnitCount, OccupiedUnitCount: p.OccupiedUnitCount, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func toAPIUnit(u property.Unit) apigen.Unit {
	out := apigen.Unit{
		Id: u.ID, PropertyId: u.PropertyID, Label: u.Label, Occupied: u.Occupied,
		Terms:     make([]apigen.UnitTerm, len(u.Terms)),
		CreatedAt: u.CreatedAt, UpdatedAt: u.UpdatedAt,
	}
	for i, t := range u.Terms {
		out.Terms[i] = apigen.UnitTerm{Months: apigen.TermMonths(t.Months), Price: t.Price}
		if t.Next != nil {
			out.Terms[i].NextPrice = &apigen.ScheduledPrice{
				Amount: t.Next.Amount, EffectiveFrom: openapi_types.Date{Time: t.Next.EffectiveFrom},
			}
		}
	}
	if u.CurrentTenant != nil {
		out.CurrentTenant = &apigen.UnitTenant{Id: u.CurrentTenant.ID, Name: u.CurrentTenant.Name}
	}
	return out
}
