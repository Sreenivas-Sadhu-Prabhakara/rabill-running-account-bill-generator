// Package rabill works out running-account bills for a construction contract.
//
// A running-account bill is cumulative. Every figure on it is a figure "up to
// date" — the whole of the work done since the contract started — and the
// amount the bill asks for is that figure less everything already certified in
// the previous bill. Get that subtraction wrong and you either pay for the same
// concrete twice or lose an item out of the account altogether.
//
// Because of that, nothing in this package holds money or quantity as a
// floating-point number. Money is whole minor units (paise, centavos),
// quantity is whole thousandths of a unit, and a percentage is whole hundredths
// of a percent. Every up-to-date figure is rounded once, to the unit it is paid
// in, and every "this bill" figure is the difference of two already-rounded
// up-to-date figures. The sum of the bills therefore equals the last bill's
// up-to-date figure exactly, with no rounding drift to argue about on site.
package rabill

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"strconv"
	"strings"
)

// Money is an amount in whole minor units of the contract currency: paise for
// rupees, centavos for pesos. It may be negative — a running-account bill often
// carries a minus figure when an earlier measurement is corrected downwards.
type Money int64

// Qty is a measured quantity in whole thousandths of the item's unit. Three
// decimal places is what a measurement book is written to; it holds 12.345 cum
// and 1.500 kg exactly.
type Qty int64

// Pct is a percentage in whole hundredths of a percent, so 5% is 500 and 2.5%
// is 250. Deduction rates are written in contracts to at most two decimals.
type Pct int64

const (
	// moneyScale is the number of minor units in one major unit.
	moneyScale = 100
	// qtyScale is the number of thousandths in one unit of measure.
	qtyScale = 1000
	// pctScale is the number of Pct units in 100%.
	pctScale = 10000

	// maxMoney and maxQty are sanity bounds on what a person can type into a
	// rate or a quantity: a rate of up to 1,000,000,000.00 per unit and a
	// quantity of up to 1,000,000,000.000 units. They are not there to keep the
	// arithmetic from overflowing — the multiplications below are done on a
	// 128-bit intermediate and cannot — but to catch the misplaced digit that
	// turns 120 cum into 120,000,000 cum before it reaches a bill.
	maxMoney = Money(100_000_000_000) // 1,000,000,000.00
	maxQty   = Qty(1_000_000_000_000) // 1,000,000,000.000
)

// ErrOutOfRange is returned when a rate or a quantity is too large for the
// exact arithmetic this package promises, rather than silently losing precision.
var ErrOutOfRange = errors.New("figure is too large for this tool to hold exactly")

// MoneyFromMajor builds an amount from a whole number of major units.
func MoneyFromMajor(n int64) Money { return Money(n * moneyScale) }

// QtyFromUnits builds a quantity from a whole number of units.
func QtyFromUnits(n int64) Qty { return Qty(n * qtyScale) }

// PctFromPercent builds a rate from a whole number of percent.
func PctFromPercent(n int64) Pct { return Pct(n * moneyScale) }

// magnitude returns the size of a signed integer, without the trouble that
// negating the most negative int64 would cause.
func magnitude(v int64) uint64 {
	if v < 0 {
		return uint64(-(v + 1)) + 1
	}
	return uint64(v)
}

// mulDiv returns a × b ÷ d, rounded a half away from zero, worked out on a
// 128-bit intermediate so that the product cannot overflow whatever a rate and
// a quantity are. It reports false only when the answer itself will not fit in
// a 64-bit integer.
//
// Rounding half away from zero — 2.5 to 3 and -2.5 to -3 — is what a bill clerk
// does by hand and what a contract that troubles to say so says.
func mulDiv(a, b, d int64) (int64, bool) {
	if d <= 0 {
		panic("rabill: mulDiv needs a positive divisor")
	}
	negative := (a < 0) != (b < 0)
	hi, lo := bits.Mul64(magnitude(a), magnitude(b))

	// Add half the divisor before dividing, which turns truncation into
	// rounding half away from zero.
	lo, carry := bits.Add64(lo, uint64(d)/2, 0)
	hi += carry

	if hi >= uint64(d) {
		return 0, false // the quotient would not fit in 64 bits at all
	}
	q, _ := bits.Div64(hi, lo, uint64(d))

	if negative {
		if q > 1<<63 {
			return 0, false
		}
		return -int64(q), true
	}
	if q > math.MaxInt64 {
		return 0, false
	}
	return int64(q), true
}

// divRound divides by a positive divisor, rounding a half away from zero.
func divRound(n, d int64) int64 {
	q, ok := mulDiv(n, 1, d)
	if !ok {
		panic("rabill: divRound overflowed")
	}
	return q
}

// Amount returns quantity × rate, rounded to the minor unit. It is the only
// place a quantity meets a rate, so it is the only place that rounding happens
// on the value of work done.
func Amount(q Qty, rate Money) (Money, error) {
	if q > maxQty || q < -maxQty {
		return 0, fmt.Errorf("quantity %s: %w", q, ErrOutOfRange)
	}
	if rate > maxMoney || rate < -maxMoney {
		return 0, fmt.Errorf("rate %s: %w", rate, ErrOutOfRange)
	}
	amount, ok := mulDiv(int64(q), int64(rate), qtyScale)
	if !ok {
		return 0, fmt.Errorf("%s at %s: %w", q, rate, ErrOutOfRange)
	}
	return Money(amount), nil
}

// Of returns the given percentage of an amount, rounded to the minor unit.
//
// Terms.Validate holds every rate in a contract to 100% or less, and a
// percentage of an amount is never larger than the amount, so for any rate a
// bill is worked out on this cannot overflow. A rate beyond 100% that has not
// been through Validate is reported as nothing rather than as a wrong figure.
func (m Money) Of(p Pct) Money {
	amount, ok := mulDiv(int64(m), int64(p), pctScale)
	if !ok {
		return 0
	}
	return Money(amount)
}

// percentOf returns a part as a percentage of a whole, or zero when there is no
// whole to be a part of. Both figures must be on the same scale — two amounts,
// or two quantities — since a ratio of like things has no scale of its own.
func percentOf(part, whole int64) Pct {
	if whole == 0 {
		return 0
	}
	p, ok := mulDiv(part, pctScale, whole)
	if !ok {
		return 0
	}
	return Pct(p)
}

// Abs returns the amount without its sign.
func (m Money) Abs() Money {
	if m < 0 {
		return -m
	}
	return m
}

// Decimal renders the amount as a plain decimal string with two places and no
// grouping or currency symbol: the form a spreadsheet and a JSON number both
// read without ambiguity.
func (m Money) Decimal() string { return scaledDecimal(int64(m), moneyScale, 2) }

// String renders the amount the same way Decimal does.
func (m Money) String() string { return m.Decimal() }

// MarshalJSON writes the amount as a JSON number with two decimal places.
func (m Money) MarshalJSON() ([]byte, error) { return []byte(m.Decimal()), nil }

// UnmarshalJSON reads an amount from a JSON number or from a string, so a
// hand-edited data file carrying "1234.56" still loads.
func (m *Money) UnmarshalJSON(b []byte) error {
	v, err := parseScaled(b, moneyScale, "amount")
	if err != nil {
		return err
	}
	*m = Money(v)
	return nil
}

// ParseMoney reads an amount written the way people write it: with or without a
// currency symbol, with lakh-style or thousand-style grouping, with a leading
// minus or in brackets. An empty string is zero.
func ParseMoney(s string) (Money, error) {
	v, err := parseHuman(s, moneyScale, 2, "amount")
	if err != nil {
		return 0, err
	}
	return Money(v), nil
}

// Abs returns the quantity without its sign.
func (q Qty) Abs() Qty {
	if q < 0 {
		return -q
	}
	return q
}

// Decimal renders the quantity as a plain decimal string with up to three
// places and no trailing zeros, the way a measurement book is written.
func (q Qty) Decimal() string { return trimZeros(scaledDecimal(int64(q), qtyScale, 3)) }

// String renders the quantity the same way Decimal does.
func (q Qty) String() string { return q.Decimal() }

// MarshalJSON writes the quantity as a JSON number.
func (q Qty) MarshalJSON() ([]byte, error) { return []byte(q.Decimal()), nil }

// UnmarshalJSON reads a quantity from a JSON number or string.
func (q *Qty) UnmarshalJSON(b []byte) error {
	v, err := parseScaled(b, qtyScale, "quantity")
	if err != nil {
		return err
	}
	*q = Qty(v)
	return nil
}

// ParseQty reads a quantity written the way people write it.
func ParseQty(s string) (Qty, error) {
	v, err := parseHuman(s, qtyScale, 3, "quantity")
	if err != nil {
		return 0, err
	}
	return Qty(v), nil
}

// QtyFromFloat rounds a quantity worked out from measured dimensions to the
// thousandth. Ties round away from zero. Dimensions are the one place a
// floating-point number is unavoidable — a measurement book multiplies lengths
// — so it is also the one place rounding to the book's three places happens.
func QtyFromFloat(f float64) (Qty, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, errors.New("quantity is not a number")
	}
	scaled := math.Round(f * qtyScale)
	if math.Abs(scaled) > float64(maxQty) {
		return 0, fmt.Errorf("quantity %g: %w", f, ErrOutOfRange)
	}
	return Qty(int64(scaled)), nil
}

// Float returns the quantity as a floating-point number, for display only.
func (q Qty) Float() float64 { return float64(q) / qtyScale }

// Decimal renders the rate as a plain percentage figure, such as "5" or "2.5".
func (p Pct) Decimal() string { return trimZeros(scaledDecimal(int64(p), moneyScale, 2)) }

// String renders the rate the same way Decimal does.
func (p Pct) String() string { return p.Decimal() }

// MarshalJSON writes the rate as a JSON number of percent.
func (p Pct) MarshalJSON() ([]byte, error) { return []byte(p.Decimal()), nil }

// UnmarshalJSON reads a rate from a JSON number or string of percent.
func (p *Pct) UnmarshalJSON(b []byte) error {
	v, err := parseScaled(b, moneyScale, "percentage")
	if err != nil {
		return err
	}
	*p = Pct(v)
	return nil
}

// ParsePct reads a percentage, with or without a trailing percent sign.
func ParsePct(s string) (Pct, error) {
	v, err := parseHuman(strings.TrimSuffix(strings.TrimSpace(s), "%"), moneyScale, 2, "percentage")
	if err != nil {
		return 0, err
	}
	return Pct(v), nil
}

// scaledDecimal renders a scaled integer as a decimal string with a fixed
// number of places.
func scaledDecimal(v, scale int64, places int) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	whole := v / scale
	frac := v % scale
	if places == 0 {
		return sign + strconv.FormatInt(whole, 10)
	}
	return sign + strconv.FormatInt(whole, 10) + "." +
		strings.Repeat("0", places-len(strconv.FormatInt(frac, 10))) + strconv.FormatInt(frac, 10)
}

// trimZeros drops a decimal point and the zeros after it when they carry
// nothing, so 12.500 reads as 12.5 and 12.000 as 12.
func trimZeros(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// parseScaled reads a JSON number or quoted string into a scaled integer.
func parseScaled(b []byte, scale int64, what string) (int64, error) {
	text := strings.TrimSpace(string(b))
	if text == "null" {
		return 0, nil
	}
	if len(text) >= 2 && text[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return 0, fmt.Errorf("%s %s did not read as a number", what, text)
		}
		text = s
	}
	places := 2
	if scale == qtyScale {
		places = 3
	}
	return parseHuman(text, scale, places, what)
}

// parseHuman reads a decimal figure into a scaled integer, accepting the
// punctuation people actually type: currency symbols, grouping commas or
// spaces, a leading or trailing minus, or brackets around a negative figure.
// Anything beyond the places the scale holds is rounded, not truncated.
func parseHuman(s string, scale int64, places int, what string) (int64, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, nil
	}

	negative := false
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") {
		negative = true
		raw = strings.TrimSuffix(strings.TrimPrefix(raw, "("), ")")
	}

	var digits strings.Builder
	seenDot := false
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '.':
			if seenDot {
				return 0, fmt.Errorf("%s %q has more than one decimal point", what, s)
			}
			seenDot = true
			digits.WriteRune(r)
		case r == '-':
			negative = !negative
		case r == ',' || r == ' ' || r == ' ' || r == '\'' || r == '_':
			// grouping, ignored
		case r == '₹' || r == '₱' || r == '$' || r == '%':
			// a currency or percent sign the typist left in
		case r == '+':
			// an explicit plus
		default:
			return 0, fmt.Errorf("%s %q contains %q, which is not part of a number", what, s, string(r))
		}
	}

	text := digits.String()
	if text == "" || text == "." {
		return 0, fmt.Errorf("%s %q has no digits in it", what, s)
	}

	whole, frac, _ := strings.Cut(text, ".")
	if whole == "" {
		whole = "0"
	}
	wholeVal, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q is too large to read", what, s)
	}

	// Carry one extra place so the value can be rounded rather than cut short.
	if len(frac) > places+1 {
		frac = frac[:places+1]
	}
	for len(frac) < places+1 {
		frac += "0"
	}
	fracVal, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s %q is not a number", what, s)
	}

	if wholeVal > math.MaxInt64/(scale*10) {
		return 0, fmt.Errorf("%s %q is too large to read", what, s)
	}
	scaled := divRound(wholeVal*scale*10+fracVal, 10)
	if negative {
		scaled = -scaled
	}
	return scaled, nil
}
