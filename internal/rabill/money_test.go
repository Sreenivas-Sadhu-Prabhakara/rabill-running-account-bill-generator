package rabill

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
)

func TestAmount(t *testing.T) {
	cases := []struct {
		name string
		qty  Qty
		rate Money
		want Money
	}{
		{"zero quantity", 0, 450000, 0},
		{"zero rate", 12345, 0, 0},
		{"whole units", QtyFromUnits(10), MoneyFromMajor(550), MoneyFromMajor(5500)},
		// 12.345 cum at 6,450.00 => 79,625.25 exactly.
		{"three decimal quantity", 12345, 645000, 7962525},
		// 1.005 cum at 100.00 => 100.50 exactly.
		{"half paise lands clean", 1005, 10000, 10050},
		// 0.001 cum at 1,234.56 => 1.23456 => rounds up to 1.23.
		{"rounds down", 1, 123456, 123},
		// 0.001 cum at 1,235.00 => 1.235 => a half, rounds away from zero.
		{"half rounds away from zero", 1, 123500, 124},
		// A minus quantity, as a downward correction produces.
		{"negative quantity", -1, 123500, -124},
		{"negative rate is allowed", 1, -123500, -124},
		{"both negative", -QtyFromUnits(2), MoneyFromMajor(-100), MoneyFromMajor(200)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Amount(c.qty, c.rate)
			if err != nil {
				t.Fatalf("Amount(%s, %s): %v", c.qty, c.rate, err)
			}
			if got != c.want {
				t.Errorf("Amount(%s, %s) = %s, want %s", c.qty, c.rate, got, c.want)
			}
		})
	}
}

func TestAmountRefusesFiguresItCannotHoldExactly(t *testing.T) {
	if _, err := Amount(maxQty+1, 100); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("quantity beyond the limit: err = %v, want ErrOutOfRange", err)
	}
	if _, err := Amount(-maxQty-1, 100); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("negative quantity beyond the limit: err = %v, want ErrOutOfRange", err)
	}
	if _, err := Amount(100, maxMoney+1); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("rate beyond the limit: err = %v, want ErrOutOfRange", err)
	}

	// A quantity and a rate that are each within bounds can still multiply out
	// to an amount no 64-bit figure holds. That is reported, never truncated.
	if _, err := Amount(maxQty, maxMoney); !errors.Is(err, ErrOutOfRange) {
		t.Errorf("the largest quantity at the largest rate: err = %v, want ErrOutOfRange", err)
	}

	// The figures a real contract carries must work: 420,000 kg of reinforcement
	// at 81.20 a kilogram is 34,104,000.00, and both are well inside the bounds.
	got, err := Amount(QtyFromUnits(420_000), rupees(81, 20))
	if err != nil {
		t.Fatalf("a real bar-bending schedule must work: %v", err)
	}
	if want := rupees(34_104_000, 0); got != want {
		t.Errorf("420,000 kg at 81.20 = %s, want %s", got, want)
	}

	// And a figure large enough to need the 128-bit intermediate: the product
	// of these two is about 8.1 x 10^19, past what a 64-bit product holds, and
	// the answer still comes out exactly.
	got, err = Amount(QtyFromUnits(1_000_000_000), rupees(81, 20))
	if err != nil {
		t.Fatalf("a product past the 64-bit range must still divide down: %v", err)
	}
	if want := rupees(81_200_000_000, 0); got != want {
		t.Errorf("1,000,000,000 units at 81.20 = %s, want %s", got, want)
	}
}

func TestMoneyOf(t *testing.T) {
	cases := []struct {
		name   string
		amount Money
		rate   Pct
		want   Money
	}{
		{"nothing of nothing", 0, 500, 0},
		{"zero rate", MoneyFromMajor(100000), 0, 0},
		{"five percent", MoneyFromMajor(100000), PctFromPercent(5), MoneyFromMajor(5000)},
		{"ten percent", MoneyFromMajor(250000), PctFromPercent(10), MoneyFromMajor(25000)},
		{"two percent", MoneyFromMajor(100000), PctFromPercent(2), MoneyFromMajor(2000)},
		// 2.5% of 1,234.56 = 30.864 => 30.86.
		{"fractional rate rounds", 123456, 250, 3086},
		// 50% of 0.01 = 0.005 => exactly half a paise, rounds away from zero.
		{"half paise rounds away from zero", 1, PctFromPercent(50), 1},
		{"half paise rounds away from zero downwards", -1, PctFromPercent(50), -1},
		// 1% of 0.04 = 0.0004 => under half a paise, rounds to nothing.
		{"under half a paise rounds to nothing", 4, PctFromPercent(1), 0},
		{"of a negative amount", MoneyFromMajor(-100000), PctFromPercent(5), MoneyFromMajor(-5000)},
		{"a hundred percent is the whole", 123456, PctFromPercent(100), 123456},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.amount.Of(c.rate); got != c.want {
				t.Errorf("%s.Of(%s%%) = %s, want %s", c.amount, c.rate, got, c.want)
			}
		})
	}
}

func TestQtyFromFloat(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want Qty
		bad  bool
	}{
		{name: "zero", in: 0, want: 0},
		{name: "whole", in: 12, want: 12000},
		{name: "three places", in: 12.345, want: 12345},
		{name: "rounds down below half", in: 12.3454, want: 12345},
		{name: "rounds up above half", in: 12.3456, want: 12346},
		{name: "binary noise from multiplying lengths", in: 2 * 0.15, want: 300},
		{name: "negative", in: -1.25, want: -1250},
		{name: "not a number", in: math.NaN(), bad: true},
		{name: "infinite", in: math.Inf(1), bad: true},
		{name: "a quantity a site could really measure", in: 12000, want: 12000000},
		{name: "beyond the limit", in: 2e9, bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := QtyFromFloat(c.in)
			if c.bad {
				if err == nil {
					t.Fatalf("QtyFromFloat(%g) = %s, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("QtyFromFloat(%g): %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("QtyFromFloat(%g) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

func TestDecimalRendering(t *testing.T) {
	moneyCases := []struct {
		in   Money
		want string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{50, "0.50"},
		{100, "1.00"},
		{123456, "1234.56"},
		{-123456, "-1234.56"},
		{-5, "-0.05"},
	}
	for _, c := range moneyCases {
		if got := c.in.Decimal(); got != c.want {
			t.Errorf("Money(%d).Decimal() = %q, want %q", int64(c.in), got, c.want)
		}
	}

	qtyCases := []struct {
		in   Qty
		want string
	}{
		{0, "0"},
		{1, "0.001"},
		{12000, "12"},
		{12500, "12.5"},
		{12345, "12.345"},
		{-12345, "-12.345"},
	}
	for _, c := range qtyCases {
		if got := c.in.Decimal(); got != c.want {
			t.Errorf("Qty(%d).Decimal() = %q, want %q", int64(c.in), got, c.want)
		}
	}

	pctCases := []struct {
		in   Pct
		want string
	}{
		{0, "0"},
		{100, "1"},
		{250, "2.5"},
		{500, "5"},
		{2000, "20"},
		{125, "1.25"},
	}
	for _, c := range pctCases {
		if got := c.in.Decimal(); got != c.want {
			t.Errorf("Pct(%d).Decimal() = %q, want %q", int64(c.in), got, c.want)
		}
	}
}

func TestParseMoney(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want Money
		bad  bool
	}{
		{name: "empty is zero", in: "", want: 0},
		{name: "blank is zero", in: "   ", want: 0},
		{name: "plain", in: "1234.56", want: 123456},
		{name: "no decimals", in: "1234", want: 123400},
		{name: "lakh grouping", in: "12,34,567.89", want: 123456789},
		{name: "thousand grouping", in: "1,234,567.89", want: 123456789},
		{name: "rupee sign", in: "₹1,23,456.00", want: 12345600},
		{name: "peso sign", in: "₱12,345.50", want: 1234550},
		{name: "spaces as grouping", in: "12 345.50", want: 1234550},
		{name: "leading minus", in: "-500.25", want: -50025},
		{name: "brackets mean negative", in: "(500.25)", want: -50025},
		{name: "third decimal rounds up", in: "10.005", want: 1001},
		{name: "third decimal rounds down", in: "10.004", want: 1000},
		{name: "bare decimal point", in: ".5", want: 50},
		{name: "letters are refused", in: "12abc", bad: true},
		{name: "two decimal points are refused", in: "1.2.3", bad: true},
		{name: "no digits is refused", in: "-", bad: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseMoney(c.in)
			if c.bad {
				if err == nil {
					t.Fatalf("ParseMoney(%q) = %s, want an error", c.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMoney(%q): %v", c.in, err)
			}
			if got != c.want {
				t.Errorf("ParseMoney(%q) = %s, want %s", c.in, got, c.want)
			}
		})
	}
}

func TestParseQtyAndPct(t *testing.T) {
	qtyCases := []struct {
		in   string
		want Qty
	}{
		{"", 0},
		{"12", 12000},
		{"12.345", 12345},
		{"12.3456", 12346},
		{"1,234.5", 1234500},
		{"-0.5", -500},
	}
	for _, c := range qtyCases {
		got, err := ParseQty(c.in)
		if err != nil {
			t.Fatalf("ParseQty(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseQty(%q) = %d, want %d", c.in, int64(got), int64(c.want))
		}
	}

	pctCases := []struct {
		in   string
		want Pct
	}{
		{"", 0},
		{"5", 500},
		{"5%", 500},
		{" 2.5 % ", 250},
		{"0.75", 75},
		{"18", 1800},
	}
	for _, c := range pctCases {
		got, err := ParsePct(c.in)
		if err != nil {
			t.Fatalf("ParsePct(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParsePct(%q) = %d, want %d", c.in, int64(got), int64(c.want))
		}
	}

	if _, err := ParsePct("five"); err == nil {
		t.Error("ParsePct(\"five\") should be refused")
	}
}

func TestJSONRoundTrip(t *testing.T) {
	type holder struct {
		Amount Money `json:"amount"`
		Qty    Qty   `json:"qty"`
		Rate   Pct   `json:"rate"`
	}

	in := holder{Amount: -123456, Qty: 12345, Rate: 250}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	const want = `{"amount":-1234.56,"qty":12.345,"rate":2.5}`
	if string(body) != want {
		t.Errorf("marshalled as %s, want %s", body, want)
	}

	var out holder
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if out != in {
		t.Errorf("round trip gave %+v, want %+v", out, in)
	}
}

func TestJSONAcceptsStringsAndNull(t *testing.T) {
	type holder struct {
		Amount Money `json:"amount"`
		Qty    Qty   `json:"qty"`
		Rate   Pct   `json:"rate"`
	}
	var out holder
	raw := []byte(`{"amount":"₹1,23,456.78","qty":"12.5","rate":null}`)
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if out.Amount != 12345678 {
		t.Errorf("amount = %s, want 123456.78", out.Amount)
	}
	if out.Qty != 12500 {
		t.Errorf("qty = %s, want 12.5", out.Qty)
	}
	if out.Rate != 0 {
		t.Errorf("rate = %s, want 0", out.Rate)
	}

	if err := json.Unmarshal([]byte(`{"amount":"twelve"}`), &out); err == nil {
		t.Error("an amount that is not a number should be refused")
	}
}

func TestMoneyAbs(t *testing.T) {
	cases := []struct{ in, want Money }{{0, 0}, {5, 5}, {-5, 5}, {-123456, 123456}}
	for _, c := range cases {
		if got := c.in.Abs(); got != c.want {
			t.Errorf("Money(%d).Abs() = %s, want %s", int64(c.in), got, c.want)
		}
	}
}

func TestMulDivUsesItsFullWidth(t *testing.T) {
	cases := []struct {
		name    string
		a, b, d int64
		want    int64
		ok      bool
	}{
		{name: "plain", a: 10, b: 20, d: 4, want: 50, ok: true},
		{name: "rounds half away from zero", a: 5, b: 1, d: 2, want: 3, ok: true},
		{name: "rounds half away from zero downwards", a: -5, b: 1, d: 2, want: -3, ok: true},
		{name: "rounds down below half", a: 4, b: 1, d: 3, want: 1, ok: true},
		{name: "zero", a: 0, b: math.MaxInt64, d: 7, want: 0, ok: true},
		{
			// The product is about 8.5 x 10^36, far beyond 64 bits, and the
			// quotient is exactly the other factor.
			name: "a product no 64-bit integer holds", a: math.MaxInt64, b: 1_000_000_000, d: 1_000_000_000,
			want: math.MaxInt64, ok: true,
		},
		{
			name: "a quotient no 64-bit integer holds", a: math.MaxInt64, b: 1_000_000_000, d: 1,
			ok: false,
		},
		{name: "the most negative integer", a: math.MinInt64, b: 1, d: 1, want: math.MinInt64, ok: true},
		{name: "a negative product past the range", a: math.MinInt64, b: 1_000_000_000, d: 1, ok: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := mulDiv(c.a, c.b, c.d)
			if ok != c.ok {
				t.Fatalf("mulDiv(%d, %d, %d) ok = %v, want %v", c.a, c.b, c.d, ok, c.ok)
			}
			if ok && got != c.want {
				t.Errorf("mulDiv(%d, %d, %d) = %d, want %d", c.a, c.b, c.d, got, c.want)
			}
		})
	}
}

func TestMulDivRefusesADivisorThatIsNotPositive(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("dividing by zero should not be allowed to pass quietly")
		}
	}()
	_, _ = mulDiv(1, 1, 0)
}

func TestPercentOf(t *testing.T) {
	cases := []struct {
		name        string
		part, whole int64
		want        Pct
	}{
		{name: "nothing of nothing", part: 0, whole: 0, want: 0},
		{name: "a part of nothing", part: 500, whole: 0, want: 0},
		{name: "half", part: 50, whole: 100, want: PctFromPercent(50)},
		{name: "the whole", part: 100, whole: 100, want: PctFromPercent(100)},
		{name: "more than the whole", part: 125, whole: 100, want: PctFromPercent(125)},
		{name: "a third, rounded", part: 1, whole: 3, want: 3333},
		{name: "two thirds, rounded", part: 2, whole: 3, want: 6667},
		{name: "a small share", part: 4, whole: 120, want: 333},
		{name: "negative", part: -50, whole: 100, want: -PctFromPercent(50)},
		{name: "a large pair", part: 263923500, whole: 265550000, want: 9939},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := percentOf(c.part, c.whole); got != c.want {
				t.Errorf("percentOf(%d, %d) = %s%%, want %s%%", c.part, c.whole, got, c.want)
			}
		})
	}
}

func TestOfIsExactAcrossEveryValidatedRate(t *testing.T) {
	// The 128-bit path must give the same answer as the plain one wherever the
	// plain one is safe, across every whole percentage — which is the range
	// Terms.Validate allows a contract rate to be in.
	plain := func(amount Money, p Pct) Money {
		n := int64(amount) * int64(p)
		if n >= 0 {
			return Money((n + pctScale/2) / pctScale)
		}
		return Money(-((-n + pctScale/2) / pctScale))
	}

	for _, amount := range []Money{0, 1, 99, 100, 123456, -123456, 45678901, 265550000, 92233720368} {
		for p := int64(0); p <= 100; p++ {
			rate := PctFromPercent(p)
			got := amount.Of(rate)
			if want := plain(amount, rate); got != want {
				t.Fatalf("%s%% of %s = %s, want %s", rate, amount, got, want)
			}
			// A share of an amount is never more than the amount itself, which
			// is what lets a deduction be taken without overdrawing the bill.
			if amount >= 0 && (got < 0 || got > amount) {
				t.Fatalf("%s%% of %s came out as %s", rate, amount, got)
			}
		}
	}

	// And beyond the plain path's safe range it still works: 100% of the
	// largest amount is that amount, where amount x 10000 would have
	// overflowed a 64-bit product.
	const huge = Money(math.MaxInt64)
	if got := huge.Of(PctFromPercent(100)); got != huge {
		t.Errorf("100%% of the largest amount = %s, want %s", got, huge)
	}
	if got := huge.Of(PctFromPercent(50)); got != huge/2+1 {
		t.Errorf("50%% of the largest amount = %s, want %s", got, huge/2+1)
	}
}
