package httpapi

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
)

// makePaymentPromise is "Simpan janji" on the payment page. Public like getPaymentPage, with the
// same rate limits counted before the token is looked up; the token grants it, so there is no
// principal to authorize. The token is never logged.
func (s *Server) makePaymentPromise(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !s.allowPaymentPage(w, r, token) {
		return
	}
	var body apigen.MakePaymentPromiseRequest
	if !s.decode(w, r, &body) {
		return
	}
	in := promiseInput(body.Kind, body.PromisedDate, body.PromisedAmount, body.PayingNow)
	p, err := s.Promises.MakeFromLink(r.Context(), token, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIPaymentPromise(p.Status, p.Latest()))
}

func (s *Server) listTenantPromises(w http.ResponseWriter, r *http.Request) {
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
	lp, ok := s.readListParams(w, r)
	if !ok {
		return
	}
	res, err := s.Promises.List(r.Context(), p, id, lp.page)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := apigen.TenantPromiseList{
		Items: make([]apigen.TenantPromise, len(res.Items)), Page: res.Page.Number, PageSize: res.Page.Size, Total: int(res.Total),
	}
	for i, item := range res.Items {
		out.Items[i] = toAPITenantPromise(item)
	}
	s.writeJSON(w, http.StatusOK, out)
}

func (s *Server) makeTenantPromise(w http.ResponseWriter, r *http.Request) {
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
	var body apigen.MakeTenantPromiseRequest
	if !s.decode(w, r, &body) {
		return
	}
	in := promiseInput(body.Kind, body.PromisedDate, body.PromisedAmount, body.PayingNow)
	if body.Note != nil {
		in.Note = *body.Note
	}
	made, err := s.Promises.MakeForTenant(r.Context(), p, id, in)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPITenantPromise(made))
}

func promiseInput(kind apigen.PromiseKind, date *openapi_types.Date, amount, payingNow *int64) promise.Input {
	in := promise.Input{Kind: string(kind), PromisedAmount: amount}
	if date != nil {
		d := date.Time
		in.PromisedDate = &d
	}
	if payingNow != nil {
		in.PayingNow = *payingNow
	}
	return in
}

// failPromise answers the promise service's own errors; it returns false for any other error.
func (s *Server) failPromise(w http.ResponseWriter, err error) bool {
	var input *promise.InputError
	if errors.As(err, &input) {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", input.Message, input.Field)
		return true
	}
	field := ""
	var fe *promise.FieldError
	if errors.As(err, &fe) {
		field = fe.Field
	}
	switch {
	case errors.Is(err, promise.ErrNothingOwed):
		s.writeError(w, http.StatusConflict, "promise_nothing_owed", "Tidak ada sisa tagihan yang perlu dijanjikan.")
	case errors.Is(err, promise.ErrDateOutOfRange):
		s.writeErrorField(w, http.StatusBadRequest, "promise_date_out_of_range",
			"Tanggal janji paling cepat besok dan paling lambat 60 hari lagi.", field)
	case errors.Is(err, promise.ErrNoNextInvoice):
		s.writeErrorField(w, http.StatusBadRequest, "promise_no_next_invoice",
			"Tidak ada tagihan berikutnya. Pilih tanggal sendiri.", field)
	case errors.Is(err, promise.ErrAmountAboveOutstanding):
		s.writeErrorField(w, http.StatusBadRequest, "promise_amount_above_outstanding", "Jumlah melebihi sisa tagihan.", field)
	default:
		return false
	}
	return true
}

func toAPIPaymentPromise(status string, v promise.Version) apigen.PaymentPromise {
	return apigen.PaymentPromise{
		Status: apigen.PromiseStatus(status), Version: v.Number, Kind: apigen.PromiseKind(v.Kind),
		PromisedDate: date(v.PromisedDate), PromisedAmount: v.PromisedAmount, PayingNow: v.PayingNow,
		CreatedAt: v.CreatedAt,
	}
}

func toAPITenantPromise(p promise.Promise) apigen.TenantPromise {
	out := apigen.TenantPromise{
		Id: p.ID, TenantId: p.TenantID, Status: apigen.PromiseStatus(p.Status), CreatedAt: p.CreatedAt, ClosedAt: p.ClosedAt,
		Versions: make([]apigen.TenantPromiseVersion, len(p.Versions)),
	}
	for i, v := range p.Versions {
		av := apigen.TenantPromiseVersion{
			Version: v.Number, Kind: apigen.PromiseKind(v.Kind), PromisedDate: date(v.PromisedDate),
			PromisedAmount: v.PromisedAmount, PayingNow: v.PayingNow, OutstandingAt: v.OutstandingAt,
			Source: apigen.PromiseSource(v.Source), CreatedAt: v.CreatedAt,
			CoveredInvoiceIds: make([]openapi_types.UUID, len(v.CoveredInvoiceIDs)),
		}
		copy(av.CoveredInvoiceIds, v.CoveredInvoiceIDs)
		if v.Note != "" {
			note := v.Note
			av.Note = &note
		}
		if v.CreatedBy != nil {
			av.CreatedBy = &apigen.StaffRef{MemberId: v.CreatedBy.MemberID, Name: v.CreatedBy.Name}
		}
		out.Versions[i] = av
	}
	return out
}

// promisePageFields fills the payment page's promise fields from the page's promise state.
func promisePageFields(out *apigen.PaymentPage, st promise.PageState) {
	if v := st.Active; v != nil {
		ap := toAPIPaymentPromise(promise.StatusActive, *v)
		out.Promise = &ap
	}
	if win := st.Window; win != nil && !win.Empty() {
		out.PromiseDates = &apigen.PromiseDates{Earliest: date(win.Earliest), Latest: date(win.Latest)}
		if due := win.NextDueInRange(); due != nil {
			d := date(*due)
			out.NextInvoiceDueDate = &d
		}
	}
}
