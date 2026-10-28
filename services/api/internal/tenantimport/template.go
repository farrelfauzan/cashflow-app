package tenantimport

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"slices"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
	"github.com/farrelfauzan/cashflow-app/services/api/internal/phone"
)

// TemplateFilename is the name the template downloads under.
const TemplateFilename = "template-impor-penyewa.csv"

// templateColumns are the template's header, in the order an owner fills a row in.
var templateColumns = []string{
	idProperty, idPropertyKind, idUnit, idTenantName, idPhone, idRent, idStartDate, idDeposit,
}

// templateRows are the example rows. The import refuses them by their phone numbers (see
// isExamplePhone), so they only show the format. Phones are in the local form with dashes: the leading zero
// and the dashes keep Excel from reading them as a number, and a leading + would make Excel read
// the cell as a formula. Amounts are plain digits so no locale reads a thousands dot as a decimal
// point; dates are day first, as Indonesians write them.
var templateRows = [][]string{
	{"Kos Melati", "kos", "Kamar 1", "Rina Wulandari", "0812-0000-0001", "850000", "15/10/2026", "850000"},
	{"Kontrakan Mawar", "kontrakan", "Rumah A", "Budi Santoso", "0812-0000-0002", "15000000", "01/07/2026", "0"},
}

// isExamplePhone reports whether a normalized phone is one of the example rows' numbers. The
// import refuses those rows, so an example left in a filled-in template never becomes a tenant
// who gets reminders from the shared WhatsApp number.
func isExamplePhone(e164 string) bool {
	col := slices.Index(templateColumns, idPhone)
	for _, row := range templateRows {
		if p, err := phone.Normalize(row[col]); err == nil && p == e164 {
			return true
		}
	}
	return false
}

// Template returns the CSV an owner or admin fills in for Run. It needs the same permissions as
// the import itself; a denial is audited.
func (s *Service) Template(ctx context.Context, p authz.Principal) ([]byte, error) {
	for _, req := range importRequests() {
		if err := s.authz.Check(ctx, p, req); err != nil {
			return nil, err
		}
	}
	return templateCSV()
}

// templateCSV writes the template: a UTF-8 byte order mark so Excel reads the encoding, then
// semicolon-separated rows, as Excel writes CSV in an Indonesian locale.
func templateCSV() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	w.UseCRLF = true
	if err := w.Write(templateColumns); err != nil {
		return nil, fmt.Errorf("writing template header: %w", err)
	}
	if err := w.WriteAll(templateRows); err != nil {
		return nil, fmt.Errorf("writing template rows: %w", err)
	}
	return buf.Bytes(), nil
}
