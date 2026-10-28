package tenantimport

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Canonical column names. Error messages name columns by these.
const (
	colProperty     = "property"
	colPropertyKind = "property_kind"
	colUnit         = "unit"
	colTenantName   = "tenant_name"
	colPhone        = "phone"
	colRent         = "rent"
	colStartDate    = "start_date"
	colDeposit      = "deposit"
	colBillFrom     = "bill_from"
	// colOldDueDay is the due day of older templates. Rent is now due on the start date's day,
	// so it is read only to warn when an older sheet says otherwise.
	colOldDueDay = "due_day"
)

// MaxRows is the most data rows one file may hold.
const MaxRows = 2000

var requiredColumns = []string{colProperty, colUnit, colTenantName, colPhone, colRent, colStartDate}

var knownColumns = append(slices.Clone(requiredColumns), colPropertyKind, colDeposit, colBillFrom, colOldDueDay)

// The Indonesian column names. The template writes them, and the parser reads them as aliases.
const (
	idProperty     = "properti"
	idPropertyKind = "jenis_properti"
	idUnit         = "kamar"
	idTenantName   = "nama_penyewa"
	idPhone        = "no_wa"
	idRent         = "sewa"
	idStartDate    = "tanggal_mulai"
	idDeposit      = "jaminan"
	idBillFrom     = "mulai_tagih"
)

// aliases maps the Indonesian headers people are likely to type to the canonical names.
var aliases = map[string]string{
	idProperty: colProperty, "nama_properti": colProperty,
	idPropertyKind: colPropertyKind, "jenis": colPropertyKind,
	idUnit: colUnit, "nama_unit": colUnit,
	idTenantName: colTenantName, "penyewa": colTenantName, "nama": colTenantName,
	idPhone: colPhone, "nomor_wa": colPhone, "whatsapp": colPhone, "telepon": colPhone, "no_hp": colPhone,
	idRent: colRent, "harga_sewa": colRent,
	idStartDate: colStartDate, "mulai_sewa": colStartDate,
	idDeposit: colDeposit, "uang_jaminan": colDeposit,
	idBillFrom: colBillFrom, "mulai_ditagih": colBillFrom, "tagih_mulai": colBillFrom,
	"jatuh_tempo": colOldDueDay, "tanggal_jatuh_tempo": colOldDueDay,
}

// kindAliases maps property kinds as people write them to the stored kinds.
var kindAliases = map[string]string{"": "other", "lainnya": "other", "lain": "other"}

// record is one data row as text, with the line it came from.
type record struct {
	Line   int
	Values map[string]string
}

func (r record) get(col string) string { return strings.TrimSpace(r.Values[col]) }

// FileError means the file as a whole cannot be read. Message is safe to show to the user.
type FileError struct{ Message string }

func (e *FileError) Error() string { return "tenantimport: " + e.Message }

// The codes of the warnings about a file as a whole.
const (
	WarnOldDueDayColumn  = "old_due_day_column"
	WarnNoBillFromColumn = "no_bill_from_column"
)

// Warning is something about the whole file worth a look that is not an error. Message is
// Bahasa Indonesia and safe to show to the user.
type Warning struct {
	Code    string
	Message string
}

// parseFile reads the CSV: a header line, then data rows. The separator is a comma or, as
// Excel writes in an Indonesian locale, a semicolon. Blank rows are skipped. The warnings say
// what the header means for every row, such as an ignored older column.
func parseFile(data []byte) ([]record, []Warning, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // Excel's UTF-8 byte order mark
	firstLine, _, _ := bytes.Cut(data, []byte("\n"))
	r := csv.NewReader(bytes.NewReader(data))
	if bytes.Count(firstLine, []byte(";")) > bytes.Count(firstLine, []byte(",")) {
		r.Comma = ';'
	}
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true

	header, err := r.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil, &FileError{Message: "Berkas kosong."}
	}
	if err != nil {
		return nil, nil, &FileError{Message: "Berkas bukan CSV yang bisa dibaca."}
	}
	columns, warnings, err := headerColumns(header)
	if err != nil {
		return nil, nil, err
	}

	var out []record
	for {
		fields, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		var parseErr *csv.ParseError
		if errors.As(err, &parseErr) {
			return nil, nil, &FileError{Message: fmt.Sprintf("Baris %d tidak bisa dibaca sebagai CSV.", parseErr.StartLine)}
		}
		if err != nil {
			return nil, nil, &FileError{Message: "Berkas bukan CSV yang bisa dibaca."}
		}
		if blank(fields) {
			continue
		}
		line, _ := r.FieldPos(0)
		rec := record{Line: line, Values: map[string]string{}}
		for i, col := range columns {
			if col != "" && i < len(fields) {
				rec.Values[col] = fields[i]
			}
		}
		out = append(out, rec)
		if len(out) > MaxRows {
			return nil, nil, &FileError{Message: fmt.Sprintf("Paling banyak %d baris per berkas. Bagi berkas menjadi beberapa bagian.", MaxRows)}
		}
	}
	if len(out) == 0 {
		return nil, nil, &FileError{Message: "Berkas tidak berisi baris data."}
	}
	return out, warnings, nil
}

// headerColumns returns the canonical name of each column, or "" for a column the import does
// not use, and the warnings the header earns.
func headerColumns(header []string) ([]string, []Warning, error) {
	columns := make([]string, len(header))
	seen := map[string]bool{}
	for i, h := range header {
		name := strings.NewReplacer(" ", "_", "-", "_").Replace(strings.ToLower(strings.TrimSpace(h)))
		if alias, ok := aliases[name]; ok {
			name = alias
		}
		if !slices.Contains(knownColumns, name) {
			continue
		}
		if seen[name] {
			return nil, nil, &FileError{Message: fmt.Sprintf("Kolom %s muncul dua kali.", name)}
		}
		seen[name] = true
		columns[i] = name
	}
	var missing []string
	for _, col := range requiredColumns {
		if !seen[col] {
			missing = append(missing, col)
		}
	}
	if len(missing) > 0 {
		return nil, nil, &FileError{Message: "Kolom wajib tidak ada: " + strings.Join(missing, ", ") + "."}
	}
	var warnings []Warning
	if seen[colOldDueDay] {
		warnings = append(warnings, Warning{Code: WarnOldDueDayColumn, Message: "Kolom jatuh_tempo tidak dipakai lagi: sewa jatuh tempo pada tanggal mulai."})
	}
	if !seen[colBillFrom] {
		warnings = append(warnings, Warning{Code: WarnNoBillFromColumn, Message: "Tidak ada kolom mulai_tagih: setiap sewa ditagih sejak tanggal mulai."})
	}
	return columns, warnings, nil
}

func blank(fields []string) bool {
	return !slices.ContainsFunc(fields, func(f string) bool { return strings.TrimSpace(f) != "" })
}

var (
	// zeroCents is an explicit "no cents" tail, as Excel writes it in either locale.
	zeroCents = regexp.MustCompile(`[.,]00?$`)
	// wholeRupiah is plain digits, or digits with a dot between every group of three.
	wholeRupiah = regexp.MustCompile(`^(\d+|\d{1,3}(\.\d{3})+)$`)
)

// parseRupiah reads a whole rupiah amount written as "850000", "850.000", "Rp850.000",
// "Rp 850.000,00" or "850000.00". A dot is only a thousands separator; any other fraction, such
// as "850.5", is refused rather than guessed. An empty cell is 0; the services decide whether 0
// is allowed.
func parseRupiah(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && strings.EqualFold(s[:2], "rp") {
		s = s[2:]
	}
	s = strings.ReplaceAll(s, " ", "")
	if s == "" {
		return 0, true
	}
	s = zeroCents.ReplaceAllString(s, "")
	if !wholeRupiah.MatchString(s) {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.ReplaceAll(s, ".", ""), 10, 64)
	return v, err == nil
}

// dateLayouts are the accepted date forms: ISO, and day first as Indonesians write dates.
var dateLayouts = []string{"2006-01-02", "02/01/2006", "2/1/2006", "02-01-2006", "2-1-2006"}

// parseDate reads a calendar day. An empty cell is the zero time, which the lease rules reject.
func parseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, true
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// parseKind turns a property kind as typed into a stored kind. Unknown values pass through for
// the property rules to reject.
func parseKind(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if k, ok := kindAliases[s]; ok {
		return k
	}
	return s
}
