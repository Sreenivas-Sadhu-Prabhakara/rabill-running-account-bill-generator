package rabill

import "strings"

// A bill is certified for an amount written in words as well as in figures,
// because the figures are the part that gets altered. The two markets group
// digits differently: India reads 12,34,567 as twelve lakh thirty-four thousand
// five hundred sixty-seven, and the Philippines reads 1,234,567 as one million
// two hundred thirty-four thousand five hundred sixty-seven.

var onesWords = [...]string{
	"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine",
	"ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen",
	"seventeen", "eighteen", "nineteen",
}

var tensWords = [...]string{
	"", "", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety",
}

// indianScales and westernScales are the groups each numbering system names,
// largest first, with the size of the group below them.
var indianScales = []struct {
	name string
	size int64
}{
	{"crore", 10000000},
	{"lakh", 100000},
	{"thousand", 1000},
}

var westernScales = []struct {
	name string
	size int64
}{
	{"trillion", 1000000000000},
	{"billion", 1000000000},
	{"million", 1000000},
	{"thousand", 1000},
}

// currencyWords names the major and minor unit of a currency.
var currencyWords = map[string][2]string{
	"INR": {"rupees", "paise"},
	"PHP": {"pesos", "centavos"},
}

// AmountInWords writes an amount out the way a certified bill writes it,
// grouped the way the market groups digits, and ending in "only".
func AmountInWords(m Money, market Market) string {
	units, known := currencyWords[market.Currency()]
	if !known {
		units = [2]string{"units", "hundredths"}
	}

	negative := m < 0
	m = m.Abs()
	major := int64(m) / moneyScale
	minor := int64(m) % moneyScale

	var parts []string
	if negative {
		parts = append(parts, "minus")
	}
	parts = append(parts, units[0], numberWords(major, market))
	if minor > 0 {
		parts = append(parts, "and", units[1], numberWords(minor, market))
	}
	parts = append(parts, "only")

	out := strings.Join(parts, " ")
	return strings.ToUpper(out[:1]) + out[1:]
}

// numberWords writes a whole number out in the market's grouping.
func numberWords(n int64, market Market) string {
	if n == 0 {
		return onesWords[0]
	}

	scales := westernScales
	if market != MarketPH {
		scales = indianScales
	}

	var parts []string
	for _, scale := range scales {
		if n >= scale.size {
			group := n / scale.size
			n %= scale.size
			parts = append(parts, numberWords(group, market), scale.name)
		}
	}
	if n >= 100 {
		parts = append(parts, onesWords[n/100], "hundred")
		n %= 100
	}
	switch {
	case n == 0:
	case n < 20:
		parts = append(parts, onesWords[n])
	default:
		word := tensWords[n/10]
		if n%10 != 0 {
			word += "-" + onesWords[n%10]
		}
		parts = append(parts, word)
	}
	return strings.Join(parts, " ")
}
