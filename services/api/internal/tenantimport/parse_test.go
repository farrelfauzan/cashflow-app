package tenantimport

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFile(t *testing.T) {
	t.Parallel()

	t.Run("canonical header with commas", func(t *testing.T) {
		t.Parallel()
		recs, err := parseFile([]byte("property,unit,tenant_name,phone,rent,due_day,start_date\n" +
			"Kos Melati,Kamar 1,Rina,0812-1111-2222,850000,5,2026-10-01\n"))
		require.NoError(t, err)
		require.Len(t, recs, 1)
		require.Equal(t, 2, recs[0].Line)
		require.Equal(t, "Kamar 1", recs[0].get(colUnit))
	})

	t.Run("Excel in an Indonesian locale: byte order mark, semicolons, Indonesian headers, any order", func(t *testing.T) {
		t.Parallel()
		data := "\xef\xbb\xbfNo;Nama Penyewa;No WA;Properti;Kamar;Sewa;Jatuh Tempo;Tanggal Mulai;Jaminan;Jenis Properti\r\n" +
			"1;Rina Wulandari;0812 1111 2222;Kos Melati;Kamar 1;Rp850.000;5;01/10/2026;850.000;kos\r\n" +
			";;;;;;;;;\r\n" +
			"2;Budi;0812 1111 3333;Kos Melati;Kamar 2;850000;10;1/10/2026;;\r\n"
		recs, err := parseFile([]byte(data))
		require.NoError(t, err)
		require.Len(t, recs, 2, "the blank row is skipped")
		require.Equal(t, "Rina Wulandari", recs[0].get(colTenantName))
		require.Equal(t, "Rp850.000", recs[0].get(colRent))
		require.Equal(t, "kos", recs[0].get(colPropertyKind))
		require.Equal(t, 4, recs[1].Line, "line numbers count the skipped row")
	})

	fileErrors := map[string]string{
		"empty file":      "",
		"header only":     "property,unit,tenant_name,phone,rent,due_day,start_date\n",
		"missing columns": "property,unit,tenant_name\nKos,K1,Rina\n",
		"repeated column": "property,properti,unit,tenant_name,phone,rent,due_day,start_date\n",
		"broken quoting":  "property,unit,tenant_name,phone,rent,due_day,start_date\n\"Kos,K1,Rina,0812,1,1,2026-10-01\n",
		"more than 2000":  "property,unit,tenant_name,phone,rent,due_day,start_date\n" + strings.Repeat("Kos,K1,Rina,0812,1,1,2026-10-01\n", MaxRows+1),
	}
	for name, data := range fileErrors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := parseFile([]byte(data))
			var fileErr *FileError
			require.ErrorAs(t, err, &fileErr)
			require.NotEmpty(t, fileErr.Message)
		})
	}

	t.Run("the missing columns are named", func(t *testing.T) {
		t.Parallel()
		_, err := parseFile([]byte("property,unit,tenant_name\n"))
		require.ErrorContains(t, err, "phone, rent, due_day, start_date")
	})
}

func TestParseValues(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]int64{
		"850000": 850_000, "850.000": 850_000, "Rp850.000": 850_000, "rp 1.250.000": 1_250_000,
		"Rp850.000,00": 850_000, " 15000000 ": 15_000_000, "": 0,
		"850000.00": 850_000, "850000,00": 850_000, "1.250.000.00": 1_250_000, "850000.0": 850_000,
	} {
		got, ok := parseRupiah(in)
		require.True(t, ok, in)
		require.Equal(t, want, got, in)
	}
	// A dot that is not between groups of three is a decimal point: refused, never dropped.
	for _, in := range []string{
		"850rb", "850,000", "850.000,50", "-850000", "Rp", "1e6",
		"850.5", "1.25", "850000.50", "85.0000", "8.50.000", ".850", "850.",
	} {
		if in == "Rp" {
			got, ok := parseRupiah(in)
			require.True(t, ok)
			require.Zero(t, got, "an amount of just Rp is empty")
			continue
		}
		_, ok := parseRupiah(in)
		require.False(t, ok, in)
	}

	for in, want := range map[string]time.Time{
		"2026-10-01": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		"01/10/2026": time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		"1/10/2026":  time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		"31-12-2025": time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
	} {
		got, ok := parseDate(in)
		require.True(t, ok, in)
		require.Equal(t, want, got, in)
	}
	for _, in := range []string{"10/31/2026", "1 Oktober 2026", "2026/10/01", "01/10/26"} {
		_, ok := parseDate(in)
		require.False(t, ok, in)
	}

	require.Equal(t, "other", parseKind(""))
	require.Equal(t, "other", parseKind("Lainnya"))
	require.Equal(t, "kontrakan", parseKind(" Kontrakan "))
	require.Equal(t, "apartemen", parseKind("apartemen"), "unknown kinds are left for the property rules")
}

func TestCheckReportsEveryProblemInARow(t *testing.T) {
	t.Parallel()
	rec := record{Line: 7, Values: map[string]string{
		colProperty: "", colUnit: "Kamar 1", colTenantName: "Rina", colPhone: "12345",
		colRent: "850rb", colDueDay: "31", colStartDate: "2026-10-01",
	}}
	var row Row
	check(rec, &row)
	var cols []string
	for _, e := range row.Errors {
		cols = append(cols, e.Column)
		require.NotEmpty(t, e.Message)
	}
	require.ElementsMatch(t, []string{colProperty, colPhone, colRent}, cols,
		"due_day is checked with the other lease terms once the amounts parse")

	rec.Values[colRent] = "850.000"
	row = Row{}
	check(rec, &row)
	cols = nil
	for _, e := range row.Errors {
		cols = append(cols, e.Column)
	}
	require.ElementsMatch(t, []string{colProperty, colPhone, colDueDay}, cols, fmt.Sprint(row.Errors))
}

func TestTemplateReadsBackWithoutErrors(t *testing.T) {
	t.Parallel()
	data, err := templateCSV()
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("\xef\xbb\xbf")), "a byte order mark, so Excel reads UTF-8")

	records, err := parseFile(data)
	require.NoError(t, err)
	require.Len(t, records, len(templateRows))
	for _, rec := range records {
		var row Row
		check(rec, &row)
		require.Empty(t, row.Errors, "line %d", rec.Line)
		require.NotNil(t, row.Values)
	}
	require.Equal(t, "Kos Melati", records[0].get(colProperty), "every Indonesian header maps to a column")
	require.Equal(t, "0", records[1].get(colDeposit))
}
