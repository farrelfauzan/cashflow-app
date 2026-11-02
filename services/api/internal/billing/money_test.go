package billing_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
)

func TestValidPayment(t *testing.T) {
	t.Parallel()
	today := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	valid := billing.PaymentInput{
		RequestID: uuid.New(), TenantID: uuid.New(), Amount: 700_000, Method: billing.MethodCash,
		ReceivedOn: today, Note: "  BCA atas nama suami ",
	}
	tests := []struct {
		name   string
		change func(in *billing.PaymentInput)
		field  string // "" when valid
	}{
		{name: "a cash payment received today is valid", change: func(*billing.PaymentInput) {}},
		{name: "a transfer received last month is valid", change: func(in *billing.PaymentInput) {
			in.Method, in.ReceivedOn = billing.MethodTransfer, today.AddDate(0, -1, 0)
		}},
		{name: "the largest amount is valid", change: func(in *billing.PaymentInput) { in.Amount = billing.MaxAmount }},
		{name: "a note of 500 letters is valid", change: func(in *billing.PaymentInput) { in.Note = strings.Repeat("é", 500) }},
		{name: "no requestId", change: func(in *billing.PaymentInput) { in.RequestID = uuid.Nil }, field: "requestId"},
		{name: "a zero amount", change: func(in *billing.PaymentInput) { in.Amount = 0 }, field: "amount"},
		{name: "a negative amount", change: func(in *billing.PaymentInput) { in.Amount = -700_000 }, field: "amount"},
		{name: "an amount over the limit", change: func(in *billing.PaymentInput) { in.Amount = billing.MaxAmount + 1 }, field: "amount"},
		{name: "qris cannot be recorded by hand", change: func(in *billing.PaymentInput) { in.Method = billing.MethodQRIS }, field: "method"},
		{name: "an unknown method", change: func(in *billing.PaymentInput) { in.Method = "giro" }, field: "method"},
		{name: "no received date", change: func(in *billing.PaymentInput) { in.ReceivedOn = time.Time{} }, field: "receivedOn"},
		{name: "tomorrow", change: func(in *billing.PaymentInput) { in.ReceivedOn = today.AddDate(0, 0, 1) }, field: "receivedOn"},
		{name: "a note of 501 letters", change: func(in *billing.PaymentInput) { in.Note = strings.Repeat("a", 501) }, field: "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := valid
			tc.change(&in)
			out, err := billing.ValidPayment(in, today)
			if tc.field == "" {
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(in.Note), out.Note)
				return
			}
			var ie *billing.InputError
			require.True(t, errors.As(err, &ie), "got %v", err)
			require.Equal(t, tc.field, ie.Field)
			require.NotEmpty(t, ie.Message)
		})
	}
}

func TestValidCorrection(t *testing.T) {
	t.Parallel()
	valid := billing.CorrectionInput{RequestID: uuid.New(), InvoiceID: uuid.New(), AmountDelta: i64(-200_000), Reason: " AC rusak sebulan "}
	tests := []struct {
		name   string
		change func(in *billing.CorrectionInput)
		field  string
	}{
		{name: "an invoice corrected down is valid", change: func(*billing.CorrectionInput) {}},
		{name: "a payment corrected up is valid", change: func(in *billing.CorrectionInput) {
			in.InvoiceID, in.PaymentID, in.AmountDelta = uuid.Nil, uuid.New(), i64(50_000)
		}},
		{name: "the largest change down is valid", change: func(in *billing.CorrectionInput) { in.AmountDelta = i64(-billing.MaxAmount) }},
		{name: "no requestId", change: func(in *billing.CorrectionInput) { in.RequestID = uuid.Nil }, field: "requestId"},
		{name: "both an invoice and a payment", change: func(in *billing.CorrectionInput) { in.PaymentID = uuid.New() }, field: "invoiceId"},
		{name: "neither an invoice nor a payment", change: func(in *billing.CorrectionInput) { in.InvoiceID = uuid.Nil }, field: "invoiceId"},
		{name: "a zero change", change: func(in *billing.CorrectionInput) { in.AmountDelta = i64(0) }, field: "amountDelta"},
		{name: "a change over the limit", change: func(in *billing.CorrectionInput) { in.AmountDelta = i64(billing.MaxAmount + 1) }, field: "amountDelta"},
		{name: "a change under the limit", change: func(in *billing.CorrectionInput) { in.AmountDelta = i64(-billing.MaxAmount - 1) }, field: "amountDelta"},
		{name: "a new amount instead of a difference is valid", change: func(in *billing.CorrectionInput) {
			in.AmountDelta, in.NewAmount = nil, i64(1_400_000)
		}},
		{name: "a new amount of zero cancels, and is valid", change: func(in *billing.CorrectionInput) { in.AmountDelta, in.NewAmount = nil, i64(0) }},
		{name: "both a new amount and a difference", change: func(in *billing.CorrectionInput) { in.NewAmount = i64(1_400_000) }, field: "newAmount"},
		{name: "neither a new amount nor a difference", change: func(in *billing.CorrectionInput) { in.AmountDelta = nil }, field: "newAmount"},
		{name: "a negative new amount", change: func(in *billing.CorrectionInput) { in.AmountDelta, in.NewAmount = nil, i64(-1) }, field: "newAmount"},
		{name: "a new amount over the limit", change: func(in *billing.CorrectionInput) {
			in.AmountDelta, in.NewAmount = nil, i64(billing.MaxAmount+1)
		}, field: "newAmount"},
		{name: "a blank reason", change: func(in *billing.CorrectionInput) { in.Reason = "   " }, field: "reason"},
		{name: "a reason of 501 letters", change: func(in *billing.CorrectionInput) { in.Reason = strings.Repeat("a", 501) }, field: "reason"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			in := valid
			tc.change(&in)
			out, err := billing.ValidCorrection(in)
			if tc.field == "" {
				require.NoError(t, err)
				require.Equal(t, strings.TrimSpace(in.Reason), out.Reason)
				return
			}
			var ie *billing.InputError
			require.True(t, errors.As(err, &ie), "got %v", err)
			require.Equal(t, tc.field, ie.Field)
		})
	}
}

func i64(v int64) *int64 { return &v }
