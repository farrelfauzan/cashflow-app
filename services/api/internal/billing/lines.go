package billing

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/tenant"
)

// Line is one line of an invoice as generation snapshots it.
type Line struct {
	Kind         string    // LineRent or LineCharge
	ChargeTypeID uuid.UUID // the charge type of a charge line; uuid.Nil for rent
	Label        string    // Bahasa Indonesia, shown to people as is
	Amount       int64     // rupiah, more than 0
}

// Invoice line kinds, as invoice_line.kind stores them.
const (
	LineRent   = "rent"
	LineCharge = "charge"
)

// FixedCharge is a fixed monthly extra on a lease (a lease_charge row).
type FixedCharge struct {
	ChargeTypeID uuid.UUID
	Name         string
	Monthly      int64 // rupiah per month
}

// InvoiceLines returns the lines of the invoice for one period of a lease billed every
// termMonths months: the rent line (rent is the unit's price for one whole term) and one line
// per fixed extra, its monthly amount times the term's months. The invoice amount is their sum.
// It is the one place that decides what a line says, so every invoice reads the same:
//
//	Sewa 15 Okt 2026 – 14 Nov 2026
//	Wi-Fi (1 bulan)
//	Listrik (3 bulan)
//
// The rent line names the period it covers; an extra names the months it covers.
func InvoiceLines(p tenant.Period, termMonths int, rent int64, charges []FixedCharge) []Line {
	lines := make([]Line, 0, 1+len(charges))
	lines = append(lines, Line{
		Kind: LineRent, Label: "Sewa " + FormatDay(p.From) + " – " + FormatDay(p.To), Amount: rent,
	})
	for _, c := range charges {
		lines = append(lines, Line{
			Kind: LineCharge, ChargeTypeID: c.ChargeTypeID,
			Label:  fmt.Sprintf("%s (%d bulan)", c.Name, termMonths),
			Amount: c.Monthly * int64(termMonths),
		})
	}
	return lines
}

// indonesianMonths are the short month names used in labels.
var indonesianMonths = [12]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// FormatDay writes a calendar day the way invoice labels show it: "15 Okt 2026".
func FormatDay(d time.Time) string {
	return fmt.Sprintf("%d %s %d", d.Day(), indonesianMonths[d.Month()-1], d.Year())
}
