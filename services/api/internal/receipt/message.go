package receipt

import (
	"strings"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/billing"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/messaging"
)

// Params are the payment_receipt parameters for a payment as it stands now (res, from
// billing.LoadPaymentResultTx): organization is the display name the template names, name the
// tenant's. The amount is the payment's net (what it counts after corrections) and the date the
// Asia/Jakarta day it was received. Each parameter is one line, as the WhatsApp Cloud API needs.
func Params(organization, name string, res billing.PaymentResult) map[string]string {
	return map[string]string{
		"organization": organization,
		"name":         name,
		"amount":       messaging.Rupiah(res.Payment.Net),
		"date":         messaging.Date(res.Payment.ReceivedOn),
		"paid":         paidLine(res),
		"balance":      balanceLine(res.Totals),
	}
}

// paidLine says what the payment paid, oldest invoice first, then any part of it left as credit:
// "Tagihan Oktober 2026 Rp700.000 (lunas); Tagihan November 2026 Rp300.000, sisa Rp400.000; saldo
// Rp50.000".
func paidLine(res billing.PaymentResult) string {
	parts := make([]string, 0, len(res.Paid)+1)
	var used int64
	for _, inv := range res.Paid {
		used += inv.Amount
		part := invoiceName(inv) + " " + messaging.Rupiah(inv.Amount)
		if inv.Outstanding == 0 {
			part += " (lunas)"
		} else {
			part += ", sisa " + messaging.Rupiah(inv.Outstanding)
		}
		parts = append(parts, part)
	}
	if rest := res.Payment.Net - used; rest > 0 {
		if len(parts) == 0 {
			return "disimpan sebagai saldo " + messaging.Rupiah(rest) + " untuk tagihan berikutnya"
		}
		parts = append(parts, "saldo "+messaging.Rupiah(rest))
	}
	return strings.Join(parts, "; ")
}

// invoiceName names an invoice by the month of its period, or as the final invoice of a move-out.
func invoiceName(inv billing.PaidInvoice) string {
	if inv.Kind == billing.InvoiceFinal {
		return "Tagihan akhir"
	}
	return "Tagihan " + messaging.Month(inv.Period)
}

// balanceLine says in one sentence what the tenant still owes, or the credit they have.
func balanceLine(t billing.Totals) string {
	switch {
	case t.Outstanding > 0:
		return "Sisa tagihan Anda sekarang " + messaging.Rupiah(t.Outstanding) + "."
	case t.Credit > 0:
		return "Semua tagihan sudah lunas. Saldo Anda " + messaging.Rupiah(t.Credit) + " dipakai untuk tagihan berikutnya."
	default:
		return "Semua tagihan Anda sudah lunas."
	}
}
