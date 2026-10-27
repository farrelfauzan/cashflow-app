// Package phone turns the ways people type WhatsApp numbers into one E.164 form.
package phone

import (
	"errors"
	"strings"
)

// ErrInvalid means the input is not a usable WhatsApp number.
var ErrInvalid = errors.New("phone: not a valid WhatsApp number")

// Normalize returns the number in E.164 form, for example "+6281234567890".
//
// Indonesian numbers may be typed as 0812..., 812..., 62812... or +62 812..., with spaces,
// dashes, dots or brackets. They must be mobile numbers (8 after the country code) with 9 to
// 12 national digits. Other countries must be given with a leading +.
func Normalize(raw string) (string, error) {
	s := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '.', '(', ')':
			return -1
		}
		return r
	}, strings.TrimSpace(raw))

	international := false
	switch {
	case strings.HasPrefix(s, "+"):
		s = s[1:]
		international = true
	case strings.HasPrefix(s, "62"):
	case strings.HasPrefix(s, "0"):
		s = "62" + s[1:]
	case strings.HasPrefix(s, "8"):
		s = "62" + s
	default:
		return "", ErrInvalid
	}

	if s == "" || s[0] == '0' || !allDigits(s) {
		return "", ErrInvalid
	}
	if national, ok := strings.CutPrefix(s, "62"); ok {
		if len(national) < 9 || len(national) > 12 || national[0] != '8' {
			return "", ErrInvalid
		}
	} else if !international || len(s) < 8 || len(s) > 15 {
		return "", ErrInvalid
	}
	return "+" + s, nil
}

// Mask hides the middle of a number for logs: "+6281234567890" becomes "+62812*****890".
func Mask(e164 string) string {
	if len(e164) <= 8 {
		return strings.Repeat("*", len(e164))
	}
	return e164[:6] + strings.Repeat("*", len(e164)-9) + e164[len(e164)-3:]
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
