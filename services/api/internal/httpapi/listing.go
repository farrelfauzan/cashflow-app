package httpapi

import (
	"net/http"
	"strconv"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/listing"
)

const maxSearchLength = 100

// listParams are the query parameters of a list. Each list reads the ones it has; sort and
// status are checked by the domain package, which owns their values.
type listParams struct {
	page              listing.Page
	search            listing.Search
	sort, status      string
	propertyID        uuid.UUID
	collectorDisabled bool
}

// readListParams parses the list parameters. It answers the request itself and returns false
// when one is not acceptable.
func (s *Server) readListParams(w http.ResponseWriter, r *http.Request) (listParams, bool) {
	v := r.URL.Query()
	out := listParams{page: listing.FirstPage, sort: v.Get("sort"), status: v.Get("status")}
	var ok bool
	if out.page.Number, ok = intParam(v.Get("page"), 1, 1, listing.MaxPageNumber); !ok {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Halaman harus angka dari 1 sampai 1.000.000.", "page")
		return out, false
	}
	if out.page.Size, ok = intParam(v.Get("pageSize"), listing.DefaultPageSize, 1, listing.MaxPageSize); !ok {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Jumlah per halaman harus angka dari 1 sampai 100.", "pageSize")
		return out, false
	}
	q := v.Get("q")
	if utf8.RuneCountInString(q) > maxSearchLength {
		s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Pencarian paling panjang 100 huruf.", "q")
		return out, false
	}
	out.search = listing.NewSearch(q)
	if raw := v.Get("propertyId"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Properti tidak dikenal.", "propertyId")
			return out, false
		}
		out.propertyID = id
	}
	if raw := v.Get("collectorDisabled"); raw != "" {
		b, err := strconv.ParseBool(raw)
		if err != nil || (raw != "true" && raw != "false") {
			s.writeErrorField(w, http.StatusBadRequest, "invalid_input", "Nilai collectorDisabled harus true atau false.", "collectorDisabled")
			return out, false
		}
		out.collectorDisabled = b
	}
	return out, true
}

// intParam parses a whole number from lo to hi; an empty value is def.
func intParam(raw string, def, lo, hi int) (int, bool) {
	if raw == "" {
		return def, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= lo && n <= hi
}
