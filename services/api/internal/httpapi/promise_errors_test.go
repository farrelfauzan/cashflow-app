package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/httpapi/apigen"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/promise"
)

// Each promise error answers its own code with the field the page puts it under, in Bahasa
// Indonesia, and never the internal text.
func TestPromiseErrors(t *testing.T) {
	t.Parallel()
	s, _ := newTestServer(t, fakePinger{})
	wrapped := func(err error) error { return fmt.Errorf("promise: deciding for tenant 1234: %w", err) }
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{
			name:       "an input wrong whatever is owed",
			err:        &promise.InputError{Field: "promisedDate", Message: "Pilih tanggal janji."},
			wantStatus: http.StatusBadRequest, wantCode: "invalid_input", wantField: "promisedDate",
		},
		{
			name:       "a date out of range",
			err:        wrapped(&promise.FieldError{Err: promise.ErrDateOutOfRange, Field: "promisedDate"}),
			wantStatus: http.StatusBadRequest, wantCode: "promise_date_out_of_range", wantField: "promisedDate",
		},
		{
			name:       "no next invoice",
			err:        wrapped(&promise.FieldError{Err: promise.ErrNoNextInvoice, Field: "kind"}),
			wantStatus: http.StatusBadRequest, wantCode: "promise_no_next_invoice", wantField: "kind",
		},
		{
			name:       "paying more than owed now",
			err:        wrapped(&promise.FieldError{Err: promise.ErrAmountAboveOutstanding, Field: "payingNow"}),
			wantStatus: http.StatusBadRequest, wantCode: "promise_amount_above_outstanding", wantField: "payingNow",
		},
		{
			name:       "promising more than is left",
			err:        wrapped(&promise.FieldError{Err: promise.ErrAmountAboveOutstanding, Field: "promisedAmount"}),
			wantStatus: http.StatusBadRequest, wantCode: "promise_amount_above_outstanding", wantField: "promisedAmount",
		},
		{
			name:       "nothing owed",
			err:        wrapped(promise.ErrNothingOwed),
			wantStatus: http.StatusConflict, wantCode: "promise_nothing_owed",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			w := httptest.NewRecorder()
			s.fail(w, httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/t/x/promise", nil), tc.err)
			require.Equal(t, tc.wantStatus, w.Code)
			var body apigen.Error
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			require.Equal(t, tc.wantCode, body.Error.Code)
			require.NotEmpty(t, body.Error.Message)
			require.NotContains(t, body.Error.Message, "promise:", "no internal text")
			require.NotContains(t, w.Body.String(), "1234", "no internal text")
			if tc.wantField == "" {
				require.Nil(t, body.Error.Field)
			} else {
				require.NotNil(t, body.Error.Field)
				require.Equal(t, tc.wantField, *body.Error.Field)
			}
		})
	}
}
