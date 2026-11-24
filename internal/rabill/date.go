package rabill

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Date is a calendar day with no time and no zone. Bills, measurements and
// defect-liability periods are all reckoned in whole days on site.
type Date struct {
	Y int
	M time.Month
	D int
}

// dateLayouts are the ways a date may be written: the ISO form the interface
// sends, and the day-first forms used on Indian and Philippine paperwork.
var dateLayouts = []string{"2006-01-02", "02/01/2006", "02-01-2006", "02.01.2006", "2/1/2006"}

// NewDate builds a date, normalising a day that overflows its month the way
// time.Date does.
func NewDate(y int, m time.Month, d int) Date {
	t := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	return Date{Y: t.Year(), M: t.Month(), D: t.Day()}
}

// Today is the current date in the machine's own time zone, which is the date a
// bill written today carries.
func Today() Date {
	n := time.Now()
	return Date{Y: n.Year(), M: n.Month(), D: n.Day()}
}

// ParseDate reads a date written in any of the accepted forms. An empty string
// gives the zero date, which reads as "not given".
func ParseDate(s string) (Date, error) {
	text := strings.TrimSpace(s)
	if text == "" {
		return Date{}, nil
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, text); err == nil {
			return Date{Y: t.Year(), M: t.Month(), D: t.Day()}, nil
		}
	}
	return Date{}, fmt.Errorf("date %q is not a date — write it as 2026-04-30 or 30/04/2026", s)
}

// IsZero reports whether the date was never set.
func (d Date) IsZero() bool { return d.Y == 0 && d.M == 0 && d.D == 0 }

// Time returns the date as midnight UTC, for arithmetic and comparison.
func (d Date) Time() time.Time {
	return time.Date(d.Y, d.M, d.D, 0, 0, 0, 0, time.UTC)
}

// String renders the date in ISO form, or an empty string when it was never set.
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return d.Time().Format("2006-01-02")
}

// Long renders the date the way a bill prints it: 30 April 2026.
func (d Date) Long() string {
	if d.IsZero() {
		return ""
	}
	return d.Time().Format("2 January 2006")
}

// MarshalJSON writes the date as an ISO string, or null when it was never set.
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(d.String())
}

// UnmarshalJSON reads a date from an ISO string, null or an empty string.
func (d *Date) UnmarshalJSON(b []byte) error {
	text := strings.TrimSpace(string(b))
	if text == "null" || text == `""` {
		*d = Date{}
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("date %s did not read as a date", text)
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// Before reports whether the date falls earlier than another.
func (d Date) Before(o Date) bool { return d.Time().Before(o.Time()) }

// After reports whether the date falls later than another.
func (d Date) After(o Date) bool { return d.Time().After(o.Time()) }

// Equal reports whether two dates are the same day.
func (d Date) Equal(o Date) bool { return d == o }

// AddDays moves the date forward by a number of days, backward for a negative
// number. A contract's payment period is counted in days, so this is plain
// calendar addition with no month-end subtlety.
func (d Date) AddDays(n int) Date {
	if d.IsZero() {
		return d
	}
	t := d.Time().AddDate(0, 0, n)
	return Date{Y: t.Year(), M: t.Month(), D: t.Day()}
}

// AddMonths moves the date forward by a number of whole months, holding the day
// of the month where it can and falling back to the last day of the target
// month where it cannot.
//
// This matters for a defect-liability period, which runs in months from
// completion. Twelve months from 29 February 2024 is 28 February 2025, and one
// month from 31 January is the last day of February — the 28th, or the 29th in
// a leap year. Go's own AddDate would roll both of those forward into the next
// month, which would quietly extend the period the contractor's money is held
// for, so this does the clamping itself.
func (d Date) AddMonths(n int) Date {
	if d.IsZero() {
		return d
	}
	y := d.Y
	month := int(d.M) - 1 + n
	y += month / 12
	month %= 12
	if month < 0 {
		month += 12
		y--
	}
	target := time.Month(month + 1)

	day := d.D
	if last := daysIn(y, target); day > last {
		day = last
	}
	return Date{Y: y, M: target, D: day}
}

// daysIn returns the number of days in a month, leap years included.
func daysIn(y int, m time.Month) int {
	return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// DaysBetween returns the number of whole days from the date to another,
// negative when the other falls earlier.
func DaysBetween(from, to Date) int {
	return int(to.Time().Sub(from.Time()).Hours() / 24)
}
