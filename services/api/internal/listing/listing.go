// Package listing holds what every paginated list shares: the page asked for, one page of
// rows with the total, and the search text turned into SQL LIKE patterns.
package listing

import (
	"math"
	"strings"
	"unicode"
)

// Page sizes. A bigger size than MaxPageSize is refused, not capped, so a client never gets
// fewer rows than it asked for without knowing.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
	// MaxPageNumber keeps the offset far inside int32.
	MaxPageNumber = 1_000_000
)

// Page is the page asked for: Number from 1 to MaxPageNumber, Size from 1 to MaxPageSize. The
// HTTP layer checks the ranges.
type Page struct {
	Number, Size int
}

// FirstPage is page 1 at the default size.
var FirstPage = Page{Number: 1, Size: DefaultPageSize}

// Limit and Offset are the page as SQL LIMIT and OFFSET.
func (p Page) Limit() int32  { return toInt32(p.Size) }
func (p Page) Offset() int32 { return toInt32((p.Number - 1) * p.Size) }

// toInt32 keeps n in int32's range; a checked Page never needs it.
func toInt32(n int) int32 {
	switch {
	case n < 0:
		return 0
	case n > math.MaxInt32:
		return math.MaxInt32
	}
	return int32(n)
}

// Result is one page of rows. Total counts every matching row the caller may see, on all
// pages, so a page past the last has no items and the real total.
type Result[T any] struct {
	Items []T
	Page  Page
	Total int64
}

// Search is the q of a list as LIKE patterns, each nil when it does not apply. Text matches
// names and labels anywhere, ignoring case. Phone is set when q is made of digits and phone
// punctuation with at least PhoneMinDigits digits: PhoneNational matches the number in
// national form (0812...) and PhoneDigits the digits as typed, because digits that start
// with 62 may be the country code or just part of the number.
type Search struct {
	Text, PhoneNational, PhoneDigits *string
}

// PhoneMinDigits is the fewest digits a phone search needs; fewer say nothing about a number.
const PhoneMinDigits = 3

// NewSearch turns q into patterns. An empty or blank q is no search.
func NewSearch(q string) Search {
	q = strings.TrimSpace(q)
	if q == "" {
		return Search{}
	}
	s := Search{Text: contains(q)}
	digits, ok := phoneDigits(q)
	if ok && len(digits) >= PhoneMinDigits {
		national := digits
		if strings.HasPrefix(digits, "62") {
			national = "0" + digits[2:]
		}
		s.PhoneNational, s.PhoneDigits = contains(national), contains(digits)
	}
	return s
}

// phoneDigits returns the digits of q when q has only digits, spaces, +, -, ( and ).
func phoneDigits(q string) (string, bool) {
	var b strings.Builder
	for _, r := range q {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case unicode.IsSpace(r), r == '+', r == '-', r == '(', r == ')':
		default:
			return "", false
		}
	}
	return b.String(), true
}

// contains is a LIKE pattern matching s anywhere, with LIKE's special characters escaped
// (backslash is PostgreSQL's default LIKE escape).
func contains(s string) *string {
	p := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(s) + "%"
	return &p
}

// All collects every page of a list, for the few callers that need every row, such as the
// demo seed.
func All[T any](fetch func(Page) (Result[T], error)) ([]T, error) {
	var out []T
	for page := (Page{Number: 1, Size: MaxPageSize}); ; page.Number++ {
		res, err := fetch(page)
		if err != nil {
			return nil, err
		}
		out = append(out, res.Items...)
		if len(res.Items) < page.Size || int64(len(out)) >= res.Total {
			return out, nil
		}
	}
}
