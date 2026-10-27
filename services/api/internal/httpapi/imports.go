package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenantimport"
)

// importMaxBytes caps an import file. At about 100 bytes a row, 1 MiB holds the 2000-row limit
// with room to spare.
const importMaxBytes = 1 << 20

func (s *Server) importTenants(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	dryRun := true
	if v := r.URL.Query().Get("dryRun"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Nilai dryRun harus true atau false.", "dryRun")
			return
		}
		dryRun = parsed
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, importMaxBytes))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.writeError(w, http.StatusRequestEntityTooLarge, "too_large", "Berkas terlalu besar. Paling besar 1 MB.")
			return
		}
		s.fail(w, r, err)
		return
	}
	res, err := s.Imports.Run(r.Context(), p, data, dryRun)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, toAPIImportResult(res))
}

func (s *Server) tenantImportTemplate(w http.ResponseWriter, r *http.Request) {
	p, ok := principalFrom(r.Context())
	if !ok {
		s.fail(w, r, errMissingContext)
		return
	}
	data, err := s.Imports.Template(r.Context(), p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+tenantimport.TemplateFilename+`"`)
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		s.log.WarnContext(r.Context(), "writing import template", "error", err)
	}
}

func toAPIImportResult(res tenantimport.Result) apigen.ImportResult {
	out := apigen.ImportResult{
		DryRun:    res.DryRun,
		Committed: res.Committed,
		Summary: apigen.ImportSummary{
			Rows: res.Summary.Rows, RowsWithErrors: res.Summary.RowsWithErrors,
			PropertiesCreated: res.Summary.PropertiesCreated, UnitsCreated: res.Summary.UnitsCreated,
			TenantsCreated: res.Summary.TenantsCreated, LeasesCreated: res.Summary.LeasesCreated,
			LeasesMatched: res.Summary.LeasesMatched,
		},
		Rows: make([]apigen.ImportRow, len(res.Rows)),
	}
	for i, row := range res.Rows {
		r := apigen.ImportRow{
			Line: row.Line, Status: apigen.ImportRowStatusOk, Errors: []apigen.ImportRowError{}, Notes: []string{},
			Property: importAction(row.Property), Unit: importAction(row.Unit),
			Tenant: importAction(row.Tenant), Lease: importAction(row.Lease),
		}
		if !row.OK() {
			r.Status = apigen.ImportRowStatusError
		}
		for _, e := range row.Errors {
			r.Errors = append(r.Errors, apigen.ImportRowError{Column: e.Column, Message: e.Message})
		}
		r.Notes = append(r.Notes, row.Notes...)
		if v := row.Values; v != nil {
			r.Values = &apigen.ImportRowValues{
				StartDate: openapi_types.Date{Time: v.StartDate}, RentAmount: v.Rent, DepositAmount: v.Deposit,
				DueDay: int32(v.DueDay), //nolint:gosec // parseDay keeps it within int16
			}
		}
		out.Rows[i] = r
	}
	return out
}

func importAction(a tenantimport.Action) *apigen.ImportAction {
	if a == "" {
		return nil
	}
	v := apigen.ImportAction(a)
	return &v
}
