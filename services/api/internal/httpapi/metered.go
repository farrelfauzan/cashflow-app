package httpapi

import (
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
)

// monthPattern is a month as the API writes it, YYYY-MM.
var monthPattern = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

const monthMessage = "Bulan harus berformat TTTT-BB, misalnya 2026-10."

// parseMonth reads a YYYY-MM month as its first day, midnight UTC like a date column.
func parseMonth(raw string) (time.Time, bool) {
	if !monthPattern.MatchString(raw) {
		return time.Time{}, false
	}
	m, err := time.Parse("2006-01", raw)
	return m, err == nil
}

func (s *Server) recordMeteredCharge(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	leaseID, err := uuid.Parse(chi.URLParam(r, "leaseID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	chargeTypeID, err := uuid.Parse(chi.URLParam(r, "chargeTypeID"))
	if err != nil {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Jenis biaya tidak ditemukan.", "chargeTypeId")
		return
	}
	month, ok := parseMonth(chi.URLParam(r, "usageMonth"))
	if !ok {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Bulan pemakaian harus berformat TTTT-BB, misalnya 2026-10.", "usageMonth")
		return
	}
	var body apigen.MeteredChargeInput
	if !s.decode(w, r, &body) {
		return
	}
	mc, err := s.Billing.RecordMeteredCharge(r.Context(), p, leaseID, chargeTypeID, month, billing.MeteredInput{
		Amount: body.Amount, MeterStart: body.MeterStart, MeterEnd: body.MeterEnd,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIMeteredCharge(mc))
}

func (s *Server) listLeaseMeteredCharges(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	leaseID, err := uuid.Parse(chi.URLParam(r, "leaseID"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	mq := billing.MeteredQuery{Page: lp.page}
	v := r.URL.Query()
	if raw := v.Get("chargeTypeId"); raw != "" {
		if mq.ChargeTypeID, err = uuid.Parse(raw); err != nil {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Jenis biaya tidak dikenal.", "chargeTypeId")
			return
		}
	}
	if raw := v.Get("usageMonth"); raw != "" {
		if mq.UsageMonth, ok = parseMonth(raw); !ok {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", monthMessage, "usageMonth")
			return
		}
	}
	res, err := s.Billing.ListLeaseMeteredCharges(r.Context(), p, leaseID, mq)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.MeteredChargeList{
		Items: make([]apigen.MeteredCharge, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
		NextInvoice: toAPINextInvoice(res.NextInvoice),
	}
	for i, mc := range res.Items {
		out.Items[i] = toAPIMeteredCharge(mc)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) listMissingMeterReadings(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r, withPropertyID)
	if !ok {
		return
	}
	mq := billing.MissingQuery{PropertyID: lp.propertyID, Page: lp.page}
	if raw := r.URL.Query().Get("month"); raw != "" {
		if mq.Month, ok = parseMonth(raw); !ok {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", monthMessage, "month")
			return
		}
	}
	res, err := s.Billing.ListMissingMeterReadings(r.Context(), p, mq)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.MissingMeterReadingList{
		Items: make([]apigen.MissingMeterReading, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	}
	for i, m := range res.Items {
		out.Items[i] = apigen.MissingMeterReading{
			LeaseId: m.LeaseID, TenantId: m.TenantID, TenantName: m.TenantName, PropertyName: m.PropertyName,
			UnitLabel: m.UnitLabel, Missing: toAPIMeteredTypes(m.Missing), NextInvoice: toAPINextInvoice(m.NextInvoice),
		}
	}
	s.writeJSON(w, http.StatusOK, out)
}

func toAPIMeteredCharge(mc billing.MeteredCharge) apigen.MeteredCharge {
	out := apigen.MeteredCharge{
		Id: mc.ID, LeaseId: mc.LeaseID, ChargeTypeId: mc.ChargeTypeID, ChargeTypeName: mc.ChargeTypeName,
		UsageMonth: mc.UsageMonth.Format("2006-01"), Amount: mc.Amount, MeterStart: mc.MeterStart, MeterEnd: mc.MeterEnd,
		RecordedBy: apigen.StaffRef{MemberId: mc.RecordedBy.MemberID, Name: mc.RecordedBy.Name},
		CreatedAt:  mc.CreatedAt, UpdatedAt: mc.UpdatedAt,
	}
	if mc.Billed != nil {
		out.Billed = &apigen.MeteredChargeBilled{InvoiceId: mc.Billed.InvoiceID, Period: date(mc.Billed.Period)}
	}
	return out
}

func toAPINextInvoice(n *billing.NextInvoice) *apigen.NextInvoice {
	if n == nil {
		return nil
	}
	return &apigen.NextInvoice{Period: date(n.Period), IssuedOn: date(n.IssuedOn)}
}

func toAPIInvoiceLine(l billing.AccountLine) apigen.InvoiceLine {
	out := apigen.InvoiceLine{Id: l.ID, Kind: apigen.InvoiceLineKind(l.Kind), Label: l.Label, Amount: l.Amount}
	if l.MeteredChargeID != uuid.Nil {
		id := l.MeteredChargeID
		out.MeteredChargeId = &id
	}
	return out
}
