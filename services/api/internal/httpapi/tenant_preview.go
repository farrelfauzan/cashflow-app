package httpapi

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
)

// previewTenantPayment tells a member what recording a payment of an amount for a tenant would
// pay (S6-6, "Cocokkan ke tagihan"). It records nothing.
func (s *Server) previewTenantPayment(w http.ResponseWriter, r *http.Request) {
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
	amount, err := strconv.ParseInt(r.URL.Query().Get("amount"), 10, 64)
	if err != nil || amount < 1 {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", previewAmountMessage, "amount")
		return
	}
	pv, err := s.Billing.PreviewTenantPayment(r.Context(), p, id, amount)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPITenantPaymentPreview(pv))
}

func toAPITenantPaymentPreview(pv billing.TenantPreview) apigen.TenantPaymentPreview {
	out := apigen.TenantPaymentPreview{
		Amount: pv.Amount, OutstandingBefore: pv.OutstandingBefore, OutstandingAfter: pv.OutstandingAfter,
		CreditAdded: pv.CreditAdded, Items: make([]apigen.PaymentPreviewItem, len(pv.Items)),
	}
	for i, it := range pv.Items {
		inv := it.Invoice
		out.Items[i] = apigen.PaymentPreviewItem{
			InvoiceId: inv.ID, Kind: apigen.InvoiceKind(inv.Kind), UnitLabel: inv.UnitLabel, PropertyName: inv.PropertyName,
			Period: date(inv.Period), PeriodEnd: date(inv.PeriodEnd), DueDate: date(inv.DueDate),
			Balance: it.Balance, Applied: it.Applied, BalanceAfter: it.BalanceAfter, PaidInFull: it.PaidInFull,
		}
	}
	return out
}
