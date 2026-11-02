package httpapi

import (
	"net/http"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
)

func (s *Server) getDashboard(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	d, err := s.Billing.Dashboard(r.Context(), p, r.URL.Query().Get("period"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.Dashboard{
		Period: d.Period.Format("2006-01"), Billed: d.Billed, Collected: d.Collected, Outstanding: d.Outstanding,
		InvoiceCount: d.InvoiceCount, TenantsOutstanding: d.TenantsOutstanding,
		Overdue: d.Overdue, TenantsInArrears: d.TenantsInArrears,
		Arrears:           make([]apigen.TenantArrears, len(d.Arrears)),
		RecentCorrections: make([]apigen.DashboardCorrection, len(d.RecentCorrections)),
	}
	for i, a := range d.Arrears {
		units := make([]apigen.UnitRef, len(a.Units))
		for j, u := range a.Units {
			units[j] = apigen.UnitRef{PropertyName: u.PropertyName, UnitLabel: u.UnitLabel}
		}
		out.Arrears[i] = apigen.TenantArrears{
			TenantId: a.TenantID, TenantName: a.TenantName, Units: units, Overdue: a.Overdue,
			OldestUnpaidDueDate: date(a.OldestUnpaidDueDate), AgeDays: a.AgeDays,
		}
	}
	for i, c := range d.RecentCorrections {
		out.RecentCorrections[i] = toAPIDashboardCorrection(c)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func toAPIDashboardCorrection(c billing.DashboardCorrection) apigen.DashboardCorrection {
	base := toAPICorrection(c.CorrectionRecord)
	out := apigen.DashboardCorrection{
		Id: base.Id, TenantId: c.TenantID, TenantName: c.TenantName, InvoiceId: base.InvoiceId, PaymentId: base.PaymentId,
		AmountDelta: base.AmountDelta, Reason: base.Reason, CreatedBy: base.CreatedBy, CreatedAt: base.CreatedAt,
	}
	if c.Period != nil {
		d := date(*c.Period)
		out.Period = &d
	}
	if c.Unit != nil {
		out.Unit = &apigen.UnitRef{PropertyName: c.Unit.PropertyName, UnitLabel: c.Unit.UnitLabel}
	}
	if c.ReceivedOn != nil {
		d := date(*c.ReceivedOn)
		out.ReceivedOn = &d
	}
	return out
}

func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	lp, ok := s.readListParams(w, r, withPropertyID)
	if !ok {
		return
	}
	res, err := s.Billing.ListInvoices(r.Context(), p, billing.InvoiceQuery{
		Period: r.URL.Query().Get("period"), Status: lp.status, PropertyID: lp.propertyID,
		Search: lp.search, Sort: lp.sort, Page: lp.page,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.InvoiceList{
		Items: make([]apigen.InvoiceListItem, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
		Summary: apigen.InvoiceListSummary{
			Billed: res.Summary.Billed, Paid: res.Summary.Paid, Outstanding: res.Summary.Outstanding,
			TenantsOutstanding: res.Summary.TenantsOutstanding,
		},
	}
	if o := res.Summary.OldestOverdue; o != nil {
		out.Summary.OldestOverdue = &apigen.OldestOverdue{
			TenantId: o.TenantID, TenantName: o.TenantName, PropertyName: o.PropertyName, UnitLabel: o.UnitLabel,
			OldestUnpaidDueDate: date(o.OldestUnpaidDueDate), AgeDays: o.AgeDays,
		}
	}
	for i, inv := range res.Items {
		lines := make([]apigen.InvoiceLine, len(inv.Lines))
		for j, l := range inv.Lines {
			lines[j] = apigen.InvoiceLine{Id: l.ID, Kind: apigen.InvoiceLineKind(l.Kind), Label: l.Label, Amount: l.Amount}
		}
		out.Items[i] = apigen.InvoiceListItem{
			Id: inv.ID, TenantId: inv.TenantID, TenantName: inv.TenantName, LeaseId: inv.LeaseID,
			PropertyId: inv.PropertyID, PropertyName: inv.PropertyName, UnitLabel: inv.UnitLabel,
			Period: date(inv.Period), PeriodEnd: date(inv.PeriodEnd), DueDate: date(inv.DueDate), IssuedOn: date(inv.IssuedOn),
			Amount: inv.Amount, Billed: inv.Billed, Paid: inv.Paid, Outstanding: inv.Outstanding,
			Status: apigen.InvoiceStatus(inv.Status), OverdueDays: inv.OverdueDays, PastPlannedEnd: inv.PastPlannedEnd,
			Lines: lines,
		}
	}
	s.writeJSON(w, http.StatusOK, out)
}
