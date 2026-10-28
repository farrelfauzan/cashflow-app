package tenantimport

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseFile(t *testing.T) {
	t.Parallel()

	t.Run("canonical header with commas", func(t *testing.T) {
		t.Parallel()
		recs, _, err := parseFile([]byte("property,unit,tenant_name,phone,rent,start_date\n" +
			"Kos Melati,Kamar 1,Rina,0812-1111-2222,850000,2026-10-01\n"))
		require.NoError(t, err)
		require.Len(t, recs, 1)
		require.Equal(t, 2, recs[0].Line)
		require.Equal(t, "Kamar 1", recs[0].get(colUnit))
		require.Equal(t, "2026-10-01", recs[0].get(colStartDate))
	})

	t.Run("Excel in an Indonesian locale: byte order mark, semicolons, Indonesian headers, any order", func(t *testing.T) {
		t.Parallel()
		data := "\xef\xbb\xbfNo;Nama Penyewa;No WA;Properti;Kamar;Sewa;Tanggal Mulai;Jaminan;Jenis Properti\r\n" +
			"1;Rina Wulandari;0812 1111 2222;Kos Melati;Kamar 1;Rp850.000;01/10/2026;850.000;kos\r\n" +
			";;;;;;;;\r\n" +
			"2;Budi;0812 1111 3333;Kos Melati;Kamar 2;850000;1/10/2026;;\r\n"
		recs, _, err := parseFile([]byte(data))
		require.NoError(t, err)
		require.Len(t, recs, 2, "the blank row is skipped")
		require.Equal(t, "Rina Wulandari", recs[0].get(colTenantName))
		require.Equal(t, "Rp850.000", recs[0].get(colRent))
		require.Equal(t, "kos", recs[0].get(colPropertyKind))
		require.Equal(t, 4, recs[1].Line, "line numbers count the skipped row")
	})

	t.Run("an older template with a due day column still imports, with a note when the day differs", func(t *testing.T) {
		t.Parallel()
		const differs = "Jatuh tempo kini mengikuti tanggal mulai (tanggal 1); kolom jatuh_tempo tidak dipakai."
		tests := []struct {
			name      string
			data      string
			wantNotes []string
		}{
			{"due_day header, another day", "property,unit,tenant_name,phone,rent,due_day,start_date\n" +
				"Kos Melati,Kamar 1,Rina,0812-1111-2222,850000,5,2026-10-01\n", []string{differs}},
			{"Jatuh Tempo header, another day", "Properti;Kamar;Nama Penyewa;No WA;Sewa;Jatuh Tempo;Tanggal Mulai\n" +
				"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;5;2026-10-01\n", []string{differs}},
			{"the start date's day", "Properti;Kamar;Nama Penyewa;No WA;Sewa;Jatuh Tempo;Tanggal Mulai\n" +
				"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;01;2026-10-01\n", nil},
			{"an empty due day", "Properti;Kamar;Nama Penyewa;No WA;Sewa;Jatuh Tempo;Tanggal Mulai\n" +
				"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;;2026-10-01\n", nil},
			{"a due day that is not a number", "Properti;Kamar;Nama Penyewa;No WA;Sewa;Jatuh Tempo;Tanggal Mulai\n" +
				"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;awal bulan;2026-10-01\n", []string{differs}},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				recs, _, err := parseFile([]byte(tc.data))
				require.NoError(t, err)
				require.Len(t, recs, 1)
				require.Equal(t, "Kos Melati", recs[0].get(colProperty))
				require.Equal(t, "Kamar 1", recs[0].get(colUnit))
				require.Equal(t, "850000", recs[0].get(colRent))
				require.Equal(t, "2026-10-01", recs[0].get(colStartDate))
				var row Row
				check(recs[0], &row)
				require.Empty(t, row.Errors)
				require.Equal(t, tc.wantNotes, row.Notes)
			})
		}
	})

	fileErrors := map[string]string{
		"empty file":      "",
		"header only":     "property,unit,tenant_name,phone,rent,start_date\n",
		"missing columns": "property,unit,tenant_name\nKos,K1,Rina\n",
		"repeated column": "property,properti,unit,tenant_name,phone,rent,start_date\n",
		"broken quoting":  "property,unit,tenant_name,phone,rent,start_date\n\"Kos,K1,Rina,0812,1,2026-10-01\n",
		"more than 2000":  "property,unit,tenant_name,phone,rent,start_date\n" + strings.Repeat("Kos,K1,Rina,0812,1,2026-10-01\n", MaxRows+1),
	}
	for name, data := range fileErrors {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, err := parseFile([]byte(data))
			var fileErr *FileError
			require.ErrorAs(t, err, &fileErr)
			require.NotEmpty(t, fileErr.Message)
		})
	}

	t.Run("the missing columns are named", func(t *testing.T) {
		t.Parallel()
		_, _, err := parseFile([]byte("property,unit,tenant_name\n"))
		require.ErrorContains(t, err, "phone, rent, start_date")
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
		colRent: "850rb", colStartDate: "2026-10-01",
	}}
	var row Row
	check(rec, &row)
	var cols []string
	for _, e := range row.Errors {
		cols = append(cols, e.Column)
		require.NotEmpty(t, e.Message)
	}
	require.ElementsMatch(t, []string{colProperty, colPhone, colRent}, cols)

	rec.Values[colRent] = "850.000"
	row = Row{}
	check(rec, &row)
	cols = nil
	for _, e := range row.Errors {
		cols = append(cols, e.Column)
	}
	require.ElementsMatch(t, []string{colProperty, colPhone}, cols, fmt.Sprint(row.Errors))
}

func TestTemplateReadsBackWithOnlyTheExampleError(t *testing.T) {
	t.Parallel()
	data, err := templateCSV()
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(data, []byte("\xef\xbb\xbf")), "a byte order mark, so Excel reads UTF-8")

	records, _, err := parseFile(data)
	require.NoError(t, err)
	require.Len(t, records, len(templateRows))
	for _, rec := range records {
		var row Row
		check(rec, &row)
		require.Equal(t, []RowError{{Column: colPhone, Message: "Ini baris contoh. Hapus baris ini sebelum mengimpor."}},
			row.Errors, "line %d: an example row is refused, and for nothing else", rec.Line)
		require.NotNil(t, row.Values)

		rec.Values[colPhone] = "0813-9900-" + strconv.Itoa(rec.Line*1111)
		row = Row{}
		check(rec, &row)
		require.Empty(t, row.Errors, "line %d with a real number is a valid row", rec.Line)
	}
	require.Equal(t, "Kos Melati", records[0].get(colProperty), "every Indonesian header maps to a column")
	require.Equal(t, "0", records[1].get(colDeposit))
}

func TestParseBillFromColumn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		data string
	}{
		{"canonical header", "property,unit,tenant_name,phone,rent,start_date,bill_from\n" +
			"Kos Melati,Kamar 1,Rina,0812-1111-2222,850000,2026-09-15,2026-10-15\n"},
		{"mulai_tagih", "properti;kamar;nama_penyewa;no_wa;sewa;tanggal_mulai;mulai_tagih\n" +
			"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;2026-09-15;2026-10-15\n"},
		{"Mulai Ditagih, mixed case", "Properti;Kamar;Nama Penyewa;No WA;Sewa;Tanggal Mulai;Mulai Ditagih\n" +
			"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;2026-09-15;2026-10-15\n"},
		{"Tagih Mulai", "Properti;Kamar;Nama Penyewa;No WA;Sewa;Tanggal Mulai;Tagih Mulai\n" +
			"Kos Melati;Kamar 1;Rina;0812-1111-2222;850000;2026-09-15;2026-10-15\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			recs, _, err := parseFile([]byte(tc.data))
			require.NoError(t, err)
			require.Len(t, recs, 1)
			require.Equal(t, "2026-10-15", recs[0].get(colBillFrom))
		})
	}

	t.Run("the column is optional", func(t *testing.T) {
		t.Parallel()
		recs, _, err := parseFile([]byte("property,unit,tenant_name,phone,rent,start_date\n" +
			"Kos Melati,Kamar 1,Rina,0812-1111-2222,850000,2026-09-15\n"))
		require.NoError(t, err)
		require.Empty(t, recs[0].get(colBillFrom))
	})
}

func TestHeaderWarnings(t *testing.T) {
	t.Parallel()
	const required = "property,unit,tenant_name,phone,rent,start_date"
	oldDueDay := Warning{Code: WarnOldDueDayColumn, Message: "Kolom jatuh_tempo tidak dipakai lagi: sewa jatuh tempo pada tanggal mulai."}
	noBillFrom := Warning{Code: WarnNoBillFromColumn, Message: "Tidak ada kolom mulai_tagih: setiap sewa ditagih sejak tanggal mulai."}

	tests := []struct {
		name   string
		header string
		want   []Warning
	}{
		{name: "mulai_tagih and no due day column gives none", header: required + ",mulai_tagih"},
		{name: "bill_from and no due day column gives none", header: required + ",bill_from"},
		{name: "no bill_from column", header: required, want: []Warning{noBillFrom}},
		{name: "jatuh_tempo column", header: required + ",jatuh_tempo,mulai_tagih", want: []Warning{oldDueDay}},
		{name: "tanggal_jatuh_tempo is the same column", header: required + ",tanggal_jatuh_tempo,bill_from", want: []Warning{oldDueDay}},
		{name: "due_day column", header: required + ",due_day,mulai_tagih", want: []Warning{oldDueDay}},
		{name: "both, the old column first", header: required + ",jatuh_tempo", want: []Warning{oldDueDay, noBillFrom}},
		{name: "an unknown column earns nothing", header: required + ",catatan,mulai_tagih"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, got, err := headerColumns(strings.Split(tc.header, ","))
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	t.Run("parseFile returns them with the rows", func(t *testing.T) {
		t.Parallel()
		recs, warnings, err := parseFile([]byte(required + ",jatuh_tempo\nKos,K1,Rina,0812,1,2026-10-01,5\n"))
		require.NoError(t, err)
		require.Len(t, recs, 1)
		require.Equal(t, []Warning{oldDueDay, noBillFrom}, warnings)
	})
}
