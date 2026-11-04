package qrimage_test

import (
	"bytes"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/stretchr/testify/require"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/qrimage"
)

var update = flag.Bool("update", false, "rewrite the golden PNGs in testdata")

// sampleQRIS looks like what AutoGoPay returns: an EMV QRIS payload (the checksum is not real).
const sampleQRIS = "00020101021226670016COM.NOBUBANK.WWW01189360050300000879140214123456789012340303UMI51440014ID.CO.QRIS.WWW0215ID20243456789120303UMI5204581253033605802ID5913KOS MELATI JKT6007JAKARTA61051234062070703A0163041D3F"

func normalInput() qrimage.Input {
	return qrimage.Input{
		QRString:     sampleQRIS,
		Amount:       1_250_000,
		UnitLabel:    "Kamar 3",
		PropertyName: "Kos Melati",
		ExpiresAt:    time.Date(2026, 11, 4, 7, 35, 0, 0, time.UTC),
	}
}

func longQRString() string {
	// A longer payload gives a bigger QR version, so the module scale shrinks.
	return sampleQRIS + strings.Repeat("0123456789ABCDEF", 24)
}

func TestRender_GoldenFiles(t *testing.T) {
	t.Parallel()
	long := normalInput()
	long.UnitLabel = "Kamar Paviliun Belakang Lantai Dua Sebelah Kiri Tangga Utama Nomor 12A"
	long.PropertyName = "Kos Eksklusif Melati Residence Pondok Indah Jakarta Selatan Blok Barat"
	maxAmount := normalInput()
	maxAmount.QRString = longQRString()
	maxAmount.Amount = 10_000_000
	maxAmount.UnitLabel = "Ruko A-1"
	maxAmount.PropertyName = "Ruko Sudirman"
	tests := []struct {
		name  string
		file  string
		input qrimage.Input
	}{
		{name: "normal", file: "normal.golden.png", input: normalInput()},
		{name: "long names", file: "long_names.golden.png", input: long},
		{name: "max amount and bigger QR", file: "max_amount.golden.png", input: maxAmount},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := qrimage.Render(tc.input)
			require.NoError(t, err)
			path := filepath.Join("testdata", tc.file)
			if *update {
				require.NoError(t, os.WriteFile(path, got, 0o600))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "run `go test ./internal/qrimage -update` to create the golden file")
			require.True(t, bytes.Equal(want, got), "output differs from %s; if the change is intended run with -update", path)
		})
	}
}

func TestRender_RoundTrip(t *testing.T) {
	t.Parallel()
	long := normalInput()
	long.QRString = longQRString()
	longNames := normalInput()
	longNames.UnitLabel = strings.Repeat("Kamar ", 30)
	tests := []struct {
		name  string
		input qrimage.Input
	}{
		{name: "normal", input: normalInput()},
		{name: "bigger QR version", input: long},
		{name: "long names", input: longNames},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := qrimage.Render(tc.input)
			require.NoError(t, err)
			img, err := png.Decode(bytes.NewReader(data))
			require.NoError(t, err)
			require.Equal(t, tc.input.QRString, decodeQR(t, img))
			require.LessOrEqual(t, img.Bounds().Dx(), 720)
			require.GreaterOrEqual(t, img.Bounds().Dx(), 600)
		})
	}
}

func decodeQR(t *testing.T, img image.Image) string {
	t.Helper()
	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	require.NoError(t, err)
	res, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	require.NoError(t, err)
	return res.GetText()
}

func TestRender_Validation(t *testing.T) {
	t.Parallel()
	with := func(f func(in *qrimage.Input)) qrimage.Input {
		in := normalInput()
		f(&in)
		return in
	}
	tests := []struct {
		name    string
		input   qrimage.Input
		wantErr error
	}{
		{name: "valid", input: normalInput()},
		{name: "empty QR string", input: with(func(in *qrimage.Input) { in.QRString = "" }), wantErr: qrimage.ErrEmptyQRString},
		{name: "QR string at the limit", input: with(func(in *qrimage.Input) { in.QRString = strings.Repeat("A", 1024) })},
		{name: "QR string over the limit", input: with(func(in *qrimage.Input) { in.QRString = strings.Repeat("A", 1025) }), wantErr: qrimage.ErrQRStringTooLong},
		{name: "newline in QR string", input: with(func(in *qrimage.Input) { in.QRString = "0002\n0101" }), wantErr: qrimage.ErrQRStringCharacter},
		{name: "NUL in QR string", input: with(func(in *qrimage.Input) { in.QRString = "0002\x000101" }), wantErr: qrimage.ErrQRStringCharacter},
		{name: "non-ASCII in QR string", input: with(func(in *qrimage.Input) { in.QRString = "0002é0101" }), wantErr: qrimage.ErrQRStringCharacter},
		{name: "amount zero", input: with(func(in *qrimage.Input) { in.Amount = 0 }), wantErr: qrimage.ErrAmountOutOfRange},
		{name: "amount negative", input: with(func(in *qrimage.Input) { in.Amount = -5 }), wantErr: qrimage.ErrAmountOutOfRange},
		{name: "amount one rupiah", input: with(func(in *qrimage.Input) { in.Amount = 1 })},
		{name: "amount at the maximum", input: with(func(in *qrimage.Input) { in.Amount = 10_000_000 })},
		{name: "amount over the maximum", input: with(func(in *qrimage.Input) { in.Amount = 10_000_001 }), wantErr: qrimage.ErrAmountOutOfRange},
		{name: "zero expiry", input: with(func(in *qrimage.Input) { in.ExpiresAt = time.Time{} }), wantErr: qrimage.ErrNoExpiry},
		{name: "no unit or property", input: with(func(in *qrimage.Input) { in.UnitLabel, in.PropertyName = "", "" })},
		{name: "control characters in names", input: with(func(in *qrimage.Input) { in.UnitLabel = "Kamar\n3\t" })},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := qrimage.Render(tc.input)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, data)
				return
			}
			require.NoError(t, err)
			_, err = png.Decode(bytes.NewReader(data))
			require.NoError(t, err)
		})
	}
}

func TestFormatRupiah(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   int64
		want string
	}{
		{name: "one", in: 1, want: "Rp1"},
		{name: "hundreds", in: 999, want: "Rp999"},
		{name: "thousand", in: 1000, want: "Rp1.000"},
		{name: "rent", in: 1_250_000, want: "Rp1.250.000"},
		{name: "QRIS maximum", in: 10_000_000, want: "Rp10.000.000"},
		{name: "zero is formatted but never rendered", in: 0, want: "Rp0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, qrimage.FormatRupiah(tc.in))
		})
	}
}

func TestFormatExpiry(t *testing.T) {
	t.Parallel()
	wib := time.FixedZone("WIB", 7*60*60)
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "UTC input", in: time.Date(2026, 11, 4, 7, 35, 0, 0, time.UTC), want: "Berlaku sampai 14.35 WIB"},
		{name: "Jakarta input", in: time.Date(2026, 11, 4, 14, 35, 0, 0, wib), want: "Berlaku sampai 14.35 WIB"},
		{name: "far-west input", in: time.Date(2026, 11, 4, 0, 35, 0, 0, time.FixedZone("UTC-7", -7*60*60)), want: "Berlaku sampai 14.35 WIB"},
		{name: "crosses midnight in Jakarta", in: time.Date(2026, 11, 4, 17, 5, 0, 0, time.UTC), want: "Berlaku sampai 00.05 WIB"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, qrimage.FormatExpiry(tc.in))
		})
	}
}

func TestRender_Deterministic(t *testing.T) {
	t.Parallel()
	first, err := qrimage.Render(normalInput())
	require.NoError(t, err)
	second, err := qrimage.Render(normalInput())
	require.NoError(t, err)
	require.True(t, bytes.Equal(first, second))
}

func TestRender_ExpiryZoneDoesNotChangeImage(t *testing.T) {
	t.Parallel()
	utc := normalInput()
	jakarta := normalInput()
	jakarta.ExpiresAt = utc.ExpiresAt.In(time.FixedZone("WIB", 7*60*60))
	a, err := qrimage.Render(utc)
	require.NoError(t, err)
	b, err := qrimage.Render(jakarta)
	require.NoError(t, err)
	require.True(t, bytes.Equal(a, b))
}
