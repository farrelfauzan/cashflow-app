package messaging

import (
	"strconv"
	"strings"
	"time"
)

// monthNames are the Indonesian month names, January first.
var monthNames = [12]string{
	"Januari", "Februari", "Maret", "April", "Mei", "Juni",
	"Juli", "Agustus", "September", "Oktober", "November", "Desember",
}

// Rupiah formats an amount as message text shows it: Rp1.250.000, with a dot every three digits
// and no decimals. A negative amount gets a leading minus (-Rp50.000).
func Rupiah(v int64) string {
	digits := strconv.FormatInt(v, 10)
	sign := ""
	if v < 0 {
		sign, digits = "-", digits[1:]
	}
	var b strings.Builder
	b.WriteString(sign)
	b.WriteString("Rp")
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// Date formats a calendar day in Indonesian: "20 November 2026". day is a date column's value
// (midnight UTC) or any time already in the zone whose calendar day is meant; its year, month and
// day are used as they are, never converted to another zone.
func Date(day time.Time) string {
	return strconv.Itoa(day.Day()) + " " + monthNames[day.Month()-1] + " " + strconv.Itoa(day.Year())
}

// Month formats the month of a calendar day in Indonesian: "November 2026". Like Date, it reads
// day's fields as they are.
func Month(day time.Time) string {
	return monthNames[day.Month()-1] + " " + strconv.Itoa(day.Year())
}

// AllOutstanding is how a promise without an amount names what it promises: everything the
// tenant still owes.
const AllOutstanding = "seluruh sisa tagihan"
