// Package qrimage renders the QRIS payment image the tenant page shows: the QR code with the
// amount, unit and expiry printed under it, drawn by us so the page never loads a third-party
// image. The encoder is rsc.io/qr: pure Go with no dependencies, it lets us pick error
// correction level M and read the raw modules, so the quiet zone and the integer module scale
// are ours to control; the other candidates pull in more code for features we do not use.
//
// Render is pure: the same input always gives the same bytes, and it has no I/O.
package qrimage

import (
	"bytes"
	_ "embed" // the fonts are embedded
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"rsc.io/qr"

	"github.com/farrelfauzan/cashflow-app/services/api/internal/platform/clock"
)

// Plus Jakarta Sans (SIL OFL 1.1, see fonts/OFL.txt) is the app's font.
//
//go:embed fonts/PlusJakartaSans-Regular.ttf
var fontRegular []byte

//go:embed fonts/PlusJakartaSans-Bold.ttf
var fontBold []byte

const (
	// MaxQRStringBytes bounds the payload; a QRIS string is a few hundred characters.
	MaxQRStringBytes = 1024
	// MaxAmount is the largest amount AutoGoPay accepts for one QRIS code, in rupiah.
	MaxAmount int64 = 10_000_000

	imageWidth   = 660 // pixels; shown at phone width (390px) it stays sharp
	sideMargin   = 40
	topMargin    = 40
	quietModules = 4 // the QR specification's quiet zone
	textGap      = 36

	amountPx = 52.0
	labelPx  = 28.0
	expiryPx = 26.0
)

// Errors Render returns for input it refuses.
var (
	ErrEmptyQRString     = errors.New("qrimage: QR string is empty")
	ErrQRStringTooLong   = errors.New("qrimage: QR string is too long")
	ErrQRStringCharacter = errors.New("qrimage: QR string has a non-printable character")
	ErrAmountOutOfRange  = errors.New("qrimage: amount is outside the QRIS range")
	ErrNoExpiry          = errors.New("qrimage: expiry time is missing")
)

var ink = color.RGBA{R: 0x1E, G: 0x2A, B: 0x2B, A: 0xFF}

// Input is everything printed on the image. Amount is whole rupiah.
type Input struct {
	QRString     string
	Amount       int64
	UnitLabel    string
	PropertyName string
	ExpiresAt    time.Time
}

func (in Input) validate() error {
	switch {
	case in.QRString == "":
		return ErrEmptyQRString
	case len(in.QRString) > MaxQRStringBytes:
		return ErrQRStringTooLong
	case in.Amount < 1 || in.Amount > MaxAmount:
		return ErrAmountOutOfRange
	case in.ExpiresAt.IsZero():
		return ErrNoExpiry
	}
	for i := 0; i < len(in.QRString); i++ {
		if c := in.QRString[i]; c < 0x20 || c > 0x7E {
			return ErrQRStringCharacter
		}
	}
	return nil
}

// Render draws the payment image for in and returns it as PNG bytes.
func Render(in Input) ([]byte, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	code, err := qr.Encode(in.QRString, qr.M)
	if err != nil {
		return nil, fmt.Errorf("encoding QR code: %w", err)
	}

	regular, err := opentype.Parse(fontRegular)
	if err != nil {
		return nil, fmt.Errorf("parsing regular font: %w", err)
	}
	bold, err := opentype.Parse(fontBold)
	if err != nil {
		return nil, fmt.Errorf("parsing bold font: %w", err)
	}
	amountFace, err := newFace(bold, amountPx)
	if err != nil {
		return nil, err
	}
	defer amountFace.Close()
	labelFace, err := newFace(regular, labelPx)
	if err != nil {
		return nil, err
	}
	defer labelFace.Close()
	expiryFace, err := newFace(regular, expiryPx)
	if err != nil {
		return nil, err
	}
	defer expiryFace.Close()

	maxTextWidth := fixed.I(imageWidth - 2*sideMargin)
	lines := []textLine{{text: formatRupiah(in.Amount), face: amountFace}}
	for _, l := range unitLines(in.UnitLabel, in.PropertyName, labelFace, maxTextWidth) {
		lines = append(lines, textLine{text: l, face: labelFace})
	}
	lines = append(lines, textLine{text: formatExpiry(in.ExpiresAt), face: expiryFace})

	// Whole pixels per module keep the squares crisp; the quiet zone is part of the square.
	modules := code.Size + 2*quietModules
	scale := (imageWidth - 2*sideMargin) / modules
	if scale < 1 {
		scale = 1
	}
	qrSide := modules * scale

	height := topMargin + qrSide + textGap
	for _, l := range lines {
		height += lineHeight(l.face)
	}
	height += topMargin - textGap/2

	img := image.NewRGBA(image.Rect(0, 0, imageWidth, height))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	drawModules(img, code, scale, (imageWidth-qrSide)/2+quietModules*scale, topMargin+quietModules*scale)

	y := topMargin + qrSide + textGap
	for _, l := range lines {
		d := &font.Drawer{Dst: img, Src: image.NewUniform(ink), Face: l.face}
		text := fitText(d, l.text, maxTextWidth)
		x := (fixed.I(imageWidth) - d.MeasureString(text)) / 2
		d.Dot = fixed.Point26_6{X: x, Y: fixed.I(y) + l.face.Metrics().Ascent}
		d.DrawString(text)
		y += lineHeight(l.face)
	}

	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding PNG: %w", err)
	}
	return buf.Bytes(), nil
}

type textLine struct {
	text string
	face font.Face
}

func newFace(f *opentype.Font, px float64) (font.Face, error) {
	// DPI 72 makes the size in points equal to pixels.
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return nil, fmt.Errorf("creating font face: %w", err)
	}
	return face, nil
}

// lineHeight is the font's line height plus a little air between lines, in whole pixels.
func lineHeight(face font.Face) int {
	return face.Metrics().Height.Ceil() + 10
}

// drawModules paints the dark modules; ox and oy are where module (0, 0) starts.
func drawModules(img *image.RGBA, code *qr.Code, scale, ox, oy int) {
	dark := image.NewUniform(ink)
	for my := 0; my < code.Size; my++ {
		for mx := 0; mx < code.Size; mx++ {
			if !code.Black(mx, my) {
				continue
			}
			x, y := ox+mx*scale, oy+my*scale
			draw.Draw(img, image.Rect(x, y, x+scale, y+scale), dark, image.Point{}, draw.Src)
		}
	}
}

// unitLines returns the line or lines naming the unit and property. They share one line as
// "Kamar 3 · Kos Melati" when they fit, otherwise each gets its own line (and is truncated).
func unitLines(unit, property string, face font.Face, maxWidth fixed.Int26_6) []string {
	unit, property = cleanText(unit), cleanText(property)
	d := &font.Drawer{Face: face}
	switch {
	case unit == "" && property == "":
		return nil
	case unit == "":
		return []string{property}
	case property == "":
		return []string{unit}
	}
	if joined := unit + " · " + property; d.MeasureString(joined) <= maxWidth {
		return []string{joined}
	}
	return []string{unit, property}
}

// cleanText drops control characters and collapses runs of spaces, so a name cannot break the layout.
func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

// fitText shortens s with an ellipsis until it is no wider than maxWidth.
func fitText(d *font.Drawer, s string, maxWidth fixed.Int26_6) string {
	if d.MeasureString(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		candidate := strings.TrimRight(string(runes), " ") + "…"
		if d.MeasureString(candidate) <= maxWidth {
			return candidate
		}
	}
	return "…"
}

// formatRupiah writes a whole rupiah amount as Rp1.250.000: dot thousands separator, no decimals.
func formatRupiah(v int64) string {
	digits := strconv.FormatInt(v, 10)
	var b strings.Builder
	b.WriteString("Rp")
	start := 0
	if v < 0 {
		b.WriteByte('-')
		start = 1
	}
	for i := start; i < len(digits); i++ {
		if i > start && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteByte(digits[i])
	}
	return b.String()
}

// formatExpiry writes the expiry line, always in Asia/Jakarta: "Berlaku sampai 14.35 WIB".
func formatExpiry(t time.Time) string {
	return "Berlaku sampai " + t.In(clock.Jakarta).Format("15.04") + " WIB"
}
