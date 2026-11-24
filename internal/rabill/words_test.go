package rabill

import "testing"

func TestAmountInWordsIndianGrouping(t *testing.T) {
	cases := []struct {
		in   Money
		want string
	}{
		{0, "Rupees zero only"},
		{1, "Rupees zero and paise one only"},
		{50, "Rupees zero and paise fifty only"},
		{100, "Rupees one only"},
		{1500, "Rupees fifteen only"},
		{1900, "Rupees nineteen only"},
		{2000, "Rupees twenty only"},
		{2100, "Rupees twenty-one only"},
		{10000, "Rupees one hundred only"},
		{10100, "Rupees one hundred one only"},
		{100000, "Rupees one thousand only"},
		{1234500, "Rupees twelve thousand three hundred forty-five only"},
		{10000000, "Rupees one lakh only"},
		{12345678, "Rupees one lakh twenty-three thousand four hundred fifty-six and paise seventy-eight only"},
		{100000000, "Rupees ten lakh only"},
		{1000000000, "Rupees one crore only"},
		{12345678912, "Rupees twelve crore thirty-four lakh fifty-six thousand seven hundred eighty-nine and paise twelve only"},
		{-1234500, "Minus rupees twelve thousand three hundred forty-five only"},
		{-1, "Minus rupees zero and paise one only"},
	}
	for _, c := range cases {
		if got := AmountInWords(c.in, MarketIN); got != c.want {
			t.Errorf("AmountInWords(%s, IN) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAmountInWordsWesternGrouping(t *testing.T) {
	cases := []struct {
		in   Money
		want string
	}{
		{0, "Pesos zero only"},
		{100, "Pesos one only"},
		{10000, "Pesos one hundred only"},
		{100000, "Pesos one thousand only"},
		{123456789, "Pesos one million two hundred thirty-four thousand five hundred sixty-seven and centavos eighty-nine only"},
		{100000000000, "Pesos one billion only"},
		{1234500, "Pesos twelve thousand three hundred forty-five only"},
		{-50000, "Minus pesos five hundred only"},
	}
	for _, c := range cases {
		if got := AmountInWords(c.in, MarketPH); got != c.want {
			t.Errorf("AmountInWords(%s, PH) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestAmountInWordsGroupsTheTwoMarketsDifferently(t *testing.T) {
	// The same figure, grouped the two ways, must read differently — that is
	// the whole reason the market is passed in.
	const tenLakh = Money(100000000) // 10,00,000.00
	in := AmountInWords(tenLakh, MarketIN)
	ph := AmountInWords(tenLakh, MarketPH)
	if in == ph {
		t.Fatalf("both markets read %s as %q", tenLakh, in)
	}
	if in != "Rupees ten lakh only" {
		t.Errorf("India reads it as %q", in)
	}
	if ph != "Pesos one million only" {
		t.Errorf("the Philippines reads it as %q", ph)
	}
}

func TestNumberWordsBoundaries(t *testing.T) {
	cases := []struct {
		n      int64
		market Market
		want   string
	}{
		{0, MarketIN, "zero"},
		{9, MarketIN, "nine"},
		{10, MarketIN, "ten"},
		{11, MarketIN, "eleven"},
		{19, MarketIN, "nineteen"},
		{20, MarketIN, "twenty"},
		{99, MarketIN, "ninety-nine"},
		{100, MarketIN, "one hundred"},
		{101, MarketIN, "one hundred one"},
		{110, MarketIN, "one hundred ten"},
		{999, MarketIN, "nine hundred ninety-nine"},
		{1000, MarketIN, "one thousand"},
		{99999, MarketIN, "ninety-nine thousand nine hundred ninety-nine"},
		{100000, MarketIN, "one lakh"},
		{9999999, MarketIN, "ninety-nine lakh ninety-nine thousand nine hundred ninety-nine"},
		{10000000, MarketIN, "one crore"},
		{1000000, MarketPH, "one million"},
		{1000000000, MarketPH, "one billion"},
		{1000000000000, MarketPH, "one trillion"},
	}
	for _, c := range cases {
		if got := numberWords(c.n, c.market); got != c.want {
			t.Errorf("numberWords(%d, %s) = %q, want %q", c.n, c.market, got, c.want)
		}
	}
}
