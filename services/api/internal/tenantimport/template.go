package tenantimport

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/authz"
)

// TemplateFilename is the name the template downloads under.
const TemplateFilename = "template-impor-penyewa.csv"

// templateColumns are the template's header, in the order an owner fills a row in.
var templateColumns = []string{
	idProperty, idPropertyKind, idUnit, idTenantName, idPhone, idRent, idDueDay, idStartDate, idDeposit,
}

// templateRows are the example rows. Phones keep their spaces and dashes so Excel treats them as
// text instead of turning +62... into a number; amounts are plain digits so no locale reads a
// thousands dot as a decimal point; dates are day first, as Indonesians write them.
var templateRows = [][]string{
	{"Kos Melati", "kos", "Kamar 1", "Rina Wulandari", "+62 812-8100-5001", "850000", "5", "01/10/2026", "850000"},
	{"Kontrakan Mawar", "kontrakan", "Rumah A", "Budi Santoso", "+62 812-8100-5002", "15000000", "1", "01/07/2026", "0"},
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
