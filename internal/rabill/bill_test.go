package rabill

import (
	"strings"
	"testing"
	"time"
)

// rupees is a shorthand for an amount written in whole rupees and paise.
func rupees(major int64, paise int64) Money { return Money(major*moneyScale + paise) }

// cum is a shorthand for a quantity in whole units.
func units(n int64) Qty { return QtyFromUnits(n) }

// contractBOQ is a small reinforced-concrete building contract.
//
//	 400 cum excavation            @   285.00 =   114,000.00
//	  60 cum PCC 1:4:8             @ 5,400.00 =   324,000.00
//	 120 cum RCC M25               @ 7,250.00 =   870,000.00
//	 800 sqm brickwork 230 thick   @ 1,150.00 =   920,000.00
//	1500 sqm cement plaster 12 mm  @   285.00 =   427,500.00
//	                                 contract = 2,655,500.00
func contractBOQ() BOQ {
	return BOQ{
		{Code: "1.1", Part: "Earthwork", Description: "Excavation in foundation in ordinary soil", Unit: "cum", ContractQty: units(400), Rate: rupees(285, 0)},
		{Code: "2.1", Part: "Concrete", Description: "Plain cement concrete 1:4:8 in foundation", Unit: "cum", ContractQty: units(60), Rate: rupees(5400, 0)},
		{Code: "2.2", Part: "Concrete", Description: "Reinforced cement concrete M25 in footings and columns", Unit: "cum", ContractQty: units(120), Rate: rupees(7250, 0)},
		{Code: "3.1", Part: "Masonry", Description: "Brickwork in cement mortar 1:6, 230 mm thick", Unit: "sqm", ContractQty: units(800), Rate: rupees(1150, 0)},
		{Code: "4.1", Part: "Finishes", Description: "Cement plaster 12 mm thick in cement mortar 1:4", Unit: "sqm", ContractQty: units(1500), Rate: rupees(285, 0)},
	}
}

const contractTotal = Money(265550000) // 2,655,500.00

// contractTerms are the indicative defaults for India with a 10% mobilisation
// advance paid against the contract value.
func contractTerms() Terms {
	t := DefaultTerms(MarketIN)
	t.AdvancePaid = contractTotal.Of(PctFromPercent(10)) // 265,550.00
	return t
}

func hasFlag(b Bill, code string) bool {
	for _, f := range b.Flags {
		if f.Code == code {
			return true
		}
	}
	return false
}

func flagsWithCode(b Bill, code string) []Flag {
	var out []Flag
	for _, f := range b.Flags {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func line(t *testing.T, b Bill, code string) ItemLine {
	t.Helper()
	for _, l := range b.Items {
		if NormaliseCode(l.Code) == NormaliseCode(code) {
			return l
		}
	}
	t.Fatalf("RA bill %d has no line for item %s", b.No, code)
	return ItemLine{}
}

func mustCompute(t *testing.T, in Input) Bill {
	t.Helper()
	b, err := Compute(in)
	if err != nil {
		t.Fatalf("Compute(bill %d): %v", in.No, err)
	}
	return b
}

// runningAccount prepares the four bills of the worked example used by several
// tests below: a first bill, a second bill, a third carrying a quantity revised
// downwards, and a final bill with an extra item.
func runningAccount(t *testing.T) []Bill {
	t.Helper()

	boq := contractBOQ()
	terms := contractTerms()

	first := mustCompute(t, Input{
		No: 1, Date: NewDate(2026, time.April, 30),
		PeriodFrom: NewDate(2026, time.April, 1), PeriodTo: NewDate(2026, time.April, 30),
		BOQ: boq, Terms: terms,
		UpToDate: map[string]Qty{"1.1": units(180), "2.1": units(28), "2.2": units(35)},
	})

	second := mustCompute(t, Input{
		No: 2, Date: NewDate(2026, time.May, 31),
		PeriodFrom: NewDate(2026, time.May, 1), PeriodTo: NewDate(2026, time.May, 31),
		BOQ: boq, Terms: terms, Previous: &first,
		UpToDate: map[string]Qty{"1.1": units(360), "2.1": units(55), "2.2": units(78), "3.1": units(240)},
	})

	// Bill 3 corrects item 2.1 downwards from 55 cum to 52 cum: three cubic
	// metres of blinding concrete were over-measured and are taken back.
	third := mustCompute(t, Input{
		No: 3, Date: NewDate(2026, time.June, 30),
		PeriodFrom: NewDate(2026, time.June, 1), PeriodTo: NewDate(2026, time.June, 30),
		BOQ: boq, Terms: terms, Previous: &second,
		UpToDate: map[string]Qty{"1.1": units(360), "2.1": units(52), "2.2": units(110), "3.1": units(520), "4.1": units(300)},
	})

	// The final bill adds an extra item the contract never priced, closes the
	// account, releases half the retention and recovers site charges.
	finalBOQ := append(contractBOQ(), Item{
		Code: "9.1", Part: "Extra items", Description: "Excavation in hard rock requiring chiselling",
		Unit: "cum", Rate: rupees(920, 0), Extra: true,
	})
	fourth := mustCompute(t, Input{
		No: 4, Date: NewDate(2026, time.July, 31), Final: true,
		PeriodFrom: NewDate(2026, time.July, 1), PeriodTo: NewDate(2026, time.July, 31),
		BOQ: finalBOQ, Terms: terms, Previous: &third,
		UpToDate: map[string]Qty{
			"1.1": units(395), "2.1": units(52), "2.2": units(124),
			"3.1": units(790), "4.1": units(1480), "9.1": units(18),
		},
		RetentionReleased: rupees(65980, 88),
		OtherRecoveries:   rupees(25000, 0),
	})

	return []Bill{first, second, third, fourth}
}

func TestFirstBillIsEntirelyItsOwnClaim(t *testing.T) {
	bills := runningAccount(t)
	first := bills[0]

	// 180 × 285 + 28 × 5,400 + 35 × 7,250 = 51,300 + 151,200 + 253,750.
	wantValue := rupees(456250, 0)
	if first.Cumulative.ValueOfWork != wantValue {
		t.Errorf("value of work up to date = %s, want %s", first.Cumulative.ValueOfWork, wantValue)
	}

	// With no bill before it, what the bill claims is the whole of the account.
	if first.Previous != (Ledger{}) {
		t.Errorf("the first bill has a previous column of %+v, want all zeros", first.Previous)
	}
	if first.ThisBill != first.Cumulative {
		t.Errorf("the first bill claims %s but stands at %s", first.ThisBill.NetPayable, first.Cumulative.NetPayable)
	}

	cases := []struct {
		name       string
		got, want  Money
		workedFrom string
	}{
		{"tax on the supply", first.Cumulative.SupplyTax, rupees(82125, 0), "18% of 456,250.00"},
		{"retention withheld", first.Cumulative.RetentionWithheld, rupees(22812, 50), "5% of 456,250.00"},
		{"advance recovery", first.Cumulative.AdvanceRecovery, rupees(68437, 50), "15% of 456,250.00"},
		{"income-tax deducted", first.Cumulative.IncomeTaxTDS, rupees(9125, 0), "2% of 456,250.00"},
		{"net payable", first.ThisBill.NetPayable, rupees(438000, 0), "456,250 + 82,125 - 22,812.50 - 68,437.50 - 9,125"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s (%s)", c.name, c.got, c.want, c.workedFrom)
		}
	}

	// A private client makes no deduction out of the GST charged to it.
	if first.Cumulative.SupplyTaxTDS != 0 {
		t.Errorf("deduction out of the tax on the supply = %s, want nothing until it is switched on", first.Cumulative.SupplyTaxTDS)
	}

	if got := line(t, first, "2.2"); got.PresentQty != units(35) || got.PresentAmount != rupees(253750, 0) {
		t.Errorf("item 2.2 claims %s cum / %s, want 35 cum / 253,750.00", got.PresentQty, got.PresentAmount)
	}

	// Items never measured are not in the first bill at all.
	for _, code := range []string{"3.1", "4.1"} {
		for _, l := range first.Items {
			if l.Code == code {
				t.Errorf("item %s has not been measured and should not be in the first bill", code)
			}
		}
	}

	if want := NewDate(2026, time.May, 30); first.PaymentDueBy != want {
		t.Errorf("payment due by %s, want %s (30 days from 30 April)", first.PaymentDueBy, want)
	}
	if !first.RetentionReleaseDue.IsZero() {
		t.Errorf("retention falls due only on the final bill, got %s", first.RetentionReleaseDue)
	}
}

func TestSecondBillClaimsOnlyWhatTheFirstDidNot(t *testing.T) {
	bills := runningAccount(t)
	first, second := bills[0], bills[1]

	// 360 × 285 + 55 × 5,400 + 78 × 7,250 + 240 × 1,150.
	wantCumulative := rupees(1241100, 0)
	if second.Cumulative.ValueOfWork != wantCumulative {
		t.Errorf("value of work up to date = %s, want %s", second.Cumulative.ValueOfWork, wantCumulative)
	}
	if want := wantCumulative - first.Cumulative.ValueOfWork; second.ThisBill.ValueOfWork != want {
		t.Errorf("value claimed in this bill = %s, want %s", second.ThisBill.ValueOfWork, want)
	}
	if second.Previous != first.Cumulative {
		t.Error("the second bill's previous column must be the first bill's cumulative column exactly")
	}

	if want := rupees(1191456, 0); second.Cumulative.NetPayable != want {
		t.Errorf("net payable up to date = %s, want %s", second.Cumulative.NetPayable, want)
	}
	if want := rupees(753456, 0); second.ThisBill.NetPayable != want {
		t.Errorf("net claimed in this bill = %s, want %s (1,191,456 less 438,000)", second.ThisBill.NetPayable, want)
	}

	// Every item that has already been paid for is in this bill too, with the
	// previous figures stated, whatever happened on site this month.
	for _, code := range []string{"1.1", "2.1", "2.2"} {
		l := line(t, second, code)
		if l.PreviousQty == 0 {
			t.Errorf("item %s was billed before, so its previous quantity cannot be nothing", code)
		}
		if l.UpToDateQty != l.PreviousQty+l.PresentQty {
			t.Errorf("item %s: %s up to date does not equal %s before plus %s now", code, l.UpToDateQty, l.PreviousQty, l.PresentQty)
		}
		if l.UpToDateAmount != l.PreviousAmount+l.PresentAmount {
			t.Errorf("item %s: %s up to date does not equal %s before plus %s now", code, l.UpToDateAmount, l.PreviousAmount, l.PresentAmount)
		}
	}

	// An item measured for the first time starts from nothing.
	brick := line(t, second, "3.1")
	if brick.PreviousQty != 0 || brick.PreviousAmount != 0 {
		t.Errorf("item 3.1 is new in this bill; its previous column reads %s / %s", brick.PreviousQty, brick.PreviousAmount)
	}
	if brick.PresentAmount != rupees(276000, 0) {
		t.Errorf("item 3.1 claims %s, want 276,000.00", brick.PresentAmount)
	}
}

func TestThirdBillTakesBackAQuantityRevisedDownwards(t *testing.T) {
	bills := runningAccount(t)
	second, third := bills[1], bills[2]

	pcc := line(t, third, "2.1")
	if pcc.PreviousQty != units(55) {
		t.Fatalf("item 2.1 was certified at %s in bill 2, want 55 cum", pcc.PreviousQty)
	}
	if pcc.UpToDateQty != units(52) {
		t.Fatalf("item 2.1 now stands at %s, want 52 cum", pcc.UpToDateQty)
	}
	if want := -units(3); pcc.PresentQty != want {
		t.Errorf("item 2.1 claims %s, want %s — a minus quantity", pcc.PresentQty, want)
	}
	if want := rupees(-16200, 0); pcc.PresentAmount != want {
		t.Errorf("item 2.1 claims %s, want %s (280,800 less 297,000)", pcc.PresentAmount, want)
	}

	// The correction must come back out of the deductions as well, or the
	// contractor would be left having paid retention and tax on work it is no
	// longer being paid for.
	if third.ThisBill.ValueOfWork <= 0 {
		t.Fatalf("this bill claims %s", third.ThisBill.ValueOfWork)
	}
	wantCumulative := rupees(1864400, 0)
	if third.Cumulative.ValueOfWork != wantCumulative {
		t.Errorf("value of work up to date = %s, want %s", third.Cumulative.ValueOfWork, wantCumulative)
	}
	if want := wantCumulative - second.Cumulative.ValueOfWork; third.ThisBill.ValueOfWork != want {
		t.Errorf("value claimed = %s, want %s", third.ThisBill.ValueOfWork, want)
	}
	if want := rupees(1803934, 0); third.Cumulative.NetPayable != want {
		t.Errorf("net payable up to date = %s, want %s", third.Cumulative.NetPayable, want)
	}
	if want := rupees(612478, 0); third.ThisBill.NetPayable != want {
		t.Errorf("net claimed = %s, want %s", third.ThisBill.NetPayable, want)
	}

	// A downward correction is normal, and worth drawing attention to.
	minus := flagsWithCode(third, "minus-quantity")
	if len(minus) != 1 || minus[0].Item != "2.1" {
		t.Errorf("want one minus-quantity flag against item 2.1, got %+v", minus)
	}

	// The advance is fully recovered by this bill: 15% of 1,864,400 is more
	// than the 265,550 advanced, so the recovery stops at the advance.
	if want := rupees(265550, 0); third.Cumulative.AdvanceRecovery != want {
		t.Errorf("advance recovered to date = %s, want %s — the whole advance and no more", third.Cumulative.AdvanceRecovery, want)
	}
	if third.AdvanceOutstanding != 0 {
		t.Errorf("advance outstanding = %s, want nothing", third.AdvanceOutstanding)
	}
	if !hasFlag(third, "advance-recovered") {
		t.Error("the bill that finishes recovering the advance should say so")
	}
}

func TestFinalBillClosesTheAccount(t *testing.T) {
	bills := runningAccount(t)
	final := bills[3]

	if !final.Final {
		t.Fatal("the fourth bill is the final bill")
	}

	// 395 × 285 + 52 × 5,400 + 124 × 7,250 + 790 × 1,150 + 1,480 × 285 + 18 × 920.
	if want := rupees(2639235, 0); final.Cumulative.ValueOfWork != want {
		t.Errorf("value of work on completion = %s, want %s", final.Cumulative.ValueOfWork, want)
	}
	cases := []struct {
		name      string
		got, want Money
	}{
		{"tax on the supply", final.Cumulative.SupplyTax, rupees(475062, 30)},
		{"retention withheld", final.Cumulative.RetentionWithheld, rupees(131961, 75)},
		{"retention released", final.Cumulative.RetentionReleased, rupees(65980, 88)},
		{"retention still held", final.RetentionHeld, rupees(65980, 87)},
		{"advance recovered", final.Cumulative.AdvanceRecovery, rupees(265550, 0)},
		{"income-tax deducted", final.Cumulative.IncomeTaxTDS, rupees(52784, 70)},
		{"other recoveries", final.Cumulative.OtherRecoveries, rupees(25000, 0)},
		{"net payable to date", final.Cumulative.NetPayable, rupees(2704981, 73)},
		{"net claimed in the final bill", final.ThisBill.NetPayable, rupees(901047, 73)},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}

	// The retention ceiling was never reached: 5% of the work done is less than
	// 5% of the contract value.
	if want := contractTotal.Of(PctFromPercent(5)); final.RetentionCap != want {
		t.Errorf("retention ceiling = %s, want %s", final.RetentionCap, want)
	}
	if hasFlag(final, "retention-capped") {
		t.Error("the retention ceiling was not reached and should not be reported as reached")
	}

	// The balance of retention falls due at the end of the defect-liability
	// period, twelve months from the final bill.
	if want := NewDate(2027, time.July, 31); final.RetentionReleaseDue != want {
		t.Errorf("the balance of retention falls due %s, want %s", final.RetentionReleaseDue, want)
	}
	if !hasFlag(final, "retention-held-after-final") {
		t.Error("retention still held after the final bill should be reported")
	}

	// The extra item cannot be certified on this tool's word alone.
	extra := line(t, final, "9.1")
	if !extra.Extra || !extra.NeedsApproval {
		t.Errorf("item 9.1 is an extra item needing approval, got extra=%v approval=%v", extra.Extra, extra.NeedsApproval)
	}
	if !hasFlag(final, "extra-item") {
		t.Error("an extra item should be flagged for the client's written approval")
	}

	// RCC overran its contract quantity by 4 cum in 120, inside the 10%
	// deviation limit, so it is a note and not a stopper.
	rcc := line(t, final, "2.2")
	if rcc.ExcessQty != units(4) {
		t.Errorf("item 2.2 excess = %s, want 4 cum", rcc.ExcessQty)
	}
	if want := Pct(333); rcc.ExcessPct != want { // 4/120 = 3.33%
		t.Errorf("item 2.2 excess = %s%%, want %s%%", rcc.ExcessPct, want)
	}
	if rcc.NeedsApproval {
		t.Error("3.33% over is inside the 10% deviation limit and needs no variation order")
	}
	if !hasFlag(final, "over-contract-quantity") {
		t.Error("an item over its contract quantity should be noted even when inside the limit")
	}

	// Four items close below their contract quantity, which the deviation
	// statement has to account for.
	if !hasFlag(final, "final-under-run") {
		t.Error("items closing below their contract quantity should be reported on the final bill")
	}

	if !strings.HasPrefix(final.NetInWords, "Rupees nine lakh one thousand forty-seven") {
		t.Errorf("the net in words reads %q", final.NetInWords)
	}
}

// TestTheBillsAddUpToTheAccount is the identity this product exists to get
// right: every bill claims the figures up to date less the previous bill, so
// the bills of a running account must add up to the account itself — exactly,
// on every line, with no rounding left over.
func TestTheBillsAddUpToTheAccount(t *testing.T) {
	bills := runningAccount(t)

	chained, err := Chain(bills)
	if err != nil {
		t.Fatalf("the bills do not form a running account: %v", err)
	}

	rec := Reconcile(chained)
	if !rec.Balanced {
		t.Fatalf("the account does not reconcile:\n%s", strings.Join(rec.Differences, "\n"))
	}
	if rec.Bills != 4 {
		t.Errorf("reconciled %d bills, want 4", rec.Bills)
	}

	// Spelled out line by line rather than left to Reconcile, so that a change
	// to Reconcile cannot quietly stop checking it.
	var summed Ledger
	for _, b := range bills {
		summed = summed.Add(b.ThisBill)
	}
	upToDate := bills[len(bills)-1].Cumulative

	for _, c := range []struct {
		name          string
		summed, cumul Money
	}{
		{"value of work done", summed.ValueOfWork, upToDate.ValueOfWork},
		{"tax on the supply", summed.SupplyTax, upToDate.SupplyTax},
		{"retention withheld", summed.RetentionWithheld, upToDate.RetentionWithheld},
		{"retention released", summed.RetentionReleased, upToDate.RetentionReleased},
		{"advance recovery", summed.AdvanceRecovery, upToDate.AdvanceRecovery},
		{"income-tax deducted at source", summed.IncomeTaxTDS, upToDate.IncomeTaxTDS},
		{"other recoveries", summed.OtherRecoveries, upToDate.OtherRecoveries},
		{"net payable", summed.NetPayable, upToDate.NetPayable},
	} {
		if c.summed != c.cumul {
			t.Errorf("%s: the four bills add up to %s but the account stands at %s", c.name, c.summed, c.cumul)
		}
	}

	if want := rupees(2704981, 73); summed.NetPayable != want {
		t.Errorf("the four bills add up to a net of %s, want %s", summed.NetPayable, want)
	}

	// And item by item: what was claimed for an item across the bills is what
	// has been measured against it, no more and no less.
	for _, item := range rec.Items {
		if !item.Balanced {
			t.Errorf("item %s: claimed %s %s / %s across the bills, measured %s %s / %s",
				item.Code, item.SumOfBillsQty, item.Unit, item.SumOfBillsAmt,
				item.UpToDateQty, item.Unit, item.UpToDateAmt)
		}
	}

	// The item whose quantity was revised downwards is the one most likely to
	// go wrong, so check it by hand: 28 + 27 - 3 + 0 = 52 cum.
	var pccQty Qty
	var pccAmt Money
	for _, b := range bills {
		l := line(t, b, "2.1")
		pccQty += l.PresentQty
		pccAmt += l.PresentAmount
	}
	if pccQty != units(52) {
		t.Errorf("item 2.1 was claimed for %s across the bills, want 52 cum", pccQty)
	}
	if want := rupees(280800, 0); pccAmt != want {
		t.Errorf("item 2.1 was claimed for %s across the bills, want %s", pccAmt, want)
	}
}

func TestNothingIsPaidTwiceWhenABillIsPreparedAgain(t *testing.T) {
	boq := contractBOQ()
	terms := contractTerms()
	measured := map[string]Qty{"1.1": units(180), "2.1": units(28)}

	first := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: measured})

	// A second bill drawn up before any further work is measured must claim
	// nothing at all — not the same work over again.
	second := mustCompute(t, Input{No: 2, Date: NewDate(2026, time.May, 31), BOQ: boq, Terms: terms, Previous: &first, UpToDate: measured})

	if second.ThisBill != (Ledger{}) {
		t.Errorf("a bill with no further work measured claims %+v, want nothing on any line", second.ThisBill)
	}
	if second.Cumulative != first.Cumulative {
		t.Error("the account should stand exactly where it did")
	}
	for _, l := range second.Items {
		if l.PresentQty != 0 || l.PresentAmount != 0 {
			t.Errorf("item %s claims %s / %s again", l.Code, l.PresentQty, l.PresentAmount)
		}
		if l.UpToDateQty != l.PreviousQty {
			t.Errorf("item %s stands at %s against a previous %s", l.Code, l.UpToDateQty, l.PreviousQty)
		}
	}

	// And the account still reconciles across the pair.
	chained, err := Chain([]Bill{first, second})
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if rec := Reconcile(chained); !rec.Balanced {
		t.Errorf("the account does not reconcile: %v", rec.Differences)
	}
}

func TestAnItemAlreadyBilledIsNeverDroppedFromALaterBill(t *testing.T) {
	boq := contractBOQ()
	terms := contractTerms()

	first := mustCompute(t, Input{
		No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms,
		UpToDate: map[string]Qty{"1.1": units(180), "2.1": units(28)},
	})

	// The measurements for item 2.1 are missing from the second bill
	// altogether. It must still be in the bill, standing where it was, and the
	// omission must be reported rather than passed over.
	second := mustCompute(t, Input{
		No: 2, Date: NewDate(2026, time.May, 31), BOQ: boq, Terms: terms, Previous: &first,
		UpToDate: map[string]Qty{"1.1": units(360)},
	})

	pcc := line(t, second, "2.1")
	if !pcc.Carried {
		t.Error("item 2.1 has no measurement in this bill and should be marked as carried forward")
	}
	if pcc.UpToDateQty != units(28) || pcc.UpToDateAmount != rupees(151200, 0) {
		t.Errorf("item 2.1 stands at %s / %s, want 28 cum / 151,200.00", pcc.UpToDateQty, pcc.UpToDateAmount)
	}
	if pcc.PresentQty != 0 || pcc.PresentAmount != 0 {
		t.Errorf("a carried item claims %s / %s, want nothing", pcc.PresentQty, pcc.PresentAmount)
	}

	carried := flagsWithCode(second, "carried-forward")
	if len(carried) != 1 || carried[0].Item != "2.1" {
		t.Errorf("want one carried-forward flag against item 2.1, got %+v", carried)
	}

	// Carrying an item forward must not disturb the identity.
	chained, err := Chain([]Bill{first, second})
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	if rec := Reconcile(chained); !rec.Balanced {
		t.Errorf("the account does not reconcile: %v", rec.Differences)
	}
}

func TestAnItemDeductedInFullStaysInTheAccount(t *testing.T) {
	boq := contractBOQ()
	terms := contractTerms()

	first := mustCompute(t, Input{
		No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms,
		UpToDate: map[string]Qty{"1.1": units(180), "2.1": units(28)},
	})

	// Item 2.1 is deducted in full: the work was measured against the wrong
	// item and the whole quantity comes back out.
	second := mustCompute(t, Input{
		No: 2, Date: NewDate(2026, time.May, 31), BOQ: boq, Terms: terms, Previous: &first,
		UpToDate: map[string]Qty{"1.1": units(180), "2.1": 0},
	})

	pcc := line(t, second, "2.1")
	if pcc.Carried {
		t.Error("a quantity explicitly measured as nothing is a deduction, not a carried item")
	}
	if pcc.UpToDateQty != 0 || pcc.UpToDateAmount != 0 {
		t.Errorf("item 2.1 stands at %s / %s, want nothing", pcc.UpToDateQty, pcc.UpToDateAmount)
	}
	if pcc.PresentQty != -units(28) || pcc.PresentAmount != rupees(-151200, 0) {
		t.Errorf("item 2.1 claims %s / %s, want -28 cum / -151,200.00", pcc.PresentQty, pcc.PresentAmount)
	}

	// The whole bill is a net recovery and must say so.
	if second.ThisBill.NetPayable >= 0 {
		t.Errorf("this bill nets %s, want a recovery", second.ThisBill.NetPayable)
	}
	if !hasFlag(second, "net-recovery") {
		t.Error("a bill that is a net recovery should say so")
	}

	chained, err := Chain([]Bill{first, second})
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	rec := Reconcile(chained)
	if !rec.Balanced {
		t.Fatalf("the account does not reconcile: %v", rec.Differences)
	}
	// Nothing is left owing for the deducted item.
	for _, item := range rec.Items {
		if NormaliseCode(item.Code) == "2.1" {
			if item.SumOfBillsQty != 0 || item.SumOfBillsAmt != 0 {
				t.Errorf("item 2.1 nets %s / %s across the account, want nothing", item.SumOfBillsQty, item.SumOfBillsAmt)
			}
		}
	}
}

func TestRetentionStopsAtItsCeiling(t *testing.T) {
	boq := BOQ{{Code: "1", Description: "Lump-sum work", Unit: "nos", ContractQty: units(100), Rate: rupees(10000, 0)}}
	terms := DefaultTerms(MarketIN)
	terms.ContractValue = rupees(1000000, 0)
	terms.RetentionPct = PctFromPercent(10)   // 10% of the value of work
	terms.RetentionCapPct = PctFromPercent(5) // but never more than 5% of the contract

	// At 40% progress, 10% of the work done is 40,000 — under the 50,000 ceiling.
	part := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"1": units(40)}})
	if want := rupees(40000, 0); part.Cumulative.RetentionWithheld != want {
		t.Errorf("retention = %s, want %s", part.Cumulative.RetentionWithheld, want)
	}
	if hasFlag(part, "retention-capped") {
		t.Error("the ceiling has not been reached yet")
	}

	// At 80% progress, 10% would be 80,000, so it stops at the ceiling.
	most := mustCompute(t, Input{No: 2, Date: NewDate(2026, time.May, 31), BOQ: boq, Terms: terms, Previous: &part, UpToDate: map[string]Qty{"1": units(80)}})
	if want := rupees(50000, 0); most.Cumulative.RetentionWithheld != want {
		t.Errorf("retention = %s, want the %s ceiling", most.Cumulative.RetentionWithheld, want)
	}
	if most.RetentionCap != rupees(50000, 0) {
		t.Errorf("the ceiling reads %s", most.RetentionCap)
	}
	if !hasFlag(most, "retention-capped") {
		t.Error("reaching the ceiling should be reported")
	}
	// Only the 10,000 up to the ceiling is held in this bill.
	if want := rupees(10000, 0); most.ThisBill.RetentionWithheld != want {
		t.Errorf("retention held in this bill = %s, want %s", most.ThisBill.RetentionWithheld, want)
	}

	// Without a contract value there is nothing to take a ceiling of.
	uncapped := terms
	uncapped.ContractValue = 0
	uncapped.RetentionCapPct = 0
	free := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: uncapped, UpToDate: map[string]Qty{"1": units(80)}})
	if want := rupees(80000, 0); free.Cumulative.RetentionWithheld != want {
		t.Errorf("with no ceiling, retention = %s, want %s", free.Cumulative.RetentionWithheld, want)
	}
	if free.RetentionCap != 0 {
		t.Errorf("with no ceiling, the ceiling reads %s", free.RetentionCap)
	}
}

func TestAdvanceRecovery(t *testing.T) {
	boq := BOQ{{Code: "1", Description: "Lump-sum work", Unit: "nos", ContractQty: units(100), Rate: rupees(10000, 0)}}

	base := DefaultTerms(MarketIN)
	base.ContractValue = rupees(1000000, 0)
	base.AdvancePaid = rupees(100000, 0)

	// A bill of quantities that prices nothing, so there is no contract value
	// for progress to be measured against even by standing in for one.
	provisional := BOQ{{Code: "1", Description: "Provisional work", Unit: "nos", Rate: rupees(10000, 0)}}

	cases := []struct {
		name  string
		boq   BOQ
		terms func(Terms) Terms
		done  Qty
		final bool
		want  Money
	}{
		{
			name: "no advance, no recovery",
			terms: func(t Terms) Terms {
				t.AdvancePaid = 0
				return t
			},
			done: units(50), want: 0,
		},
		{
			name:  "a share of the value of work done",
			terms: func(t Terms) Terms { return t }, // 15% of 500,000
			done:  units(50), want: rupees(75000, 0),
		},
		{
			name:  "recovery stops at the advance paid",
			terms: func(t Terms) Terms { return t }, // 15% of 900,000 is 135,000
			done:  units(90), want: rupees(100000, 0),
		},
		{
			name:  "nothing done, nothing recovered",
			terms: func(t Terms) Terms { return t },
			done:  0, want: 0,
		},
		{
			name: "pro-rata: before recovery begins",
			terms: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				return t
			},
			done: units(10), want: 0, // 10% progress, recovery starts at 10%
		},
		{
			name: "pro-rata: halfway between the two points",
			terms: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				return t
			},
			done: units(50), want: rupees(50000, 0), // 50% of the way from 10% to 90%
		},
		{
			name: "pro-rata: at the finishing point",
			terms: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				return t
			},
			done: units(90), want: rupees(100000, 0),
		},
		{
			name: "pro-rata: past the finishing point",
			terms: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				return t
			},
			done: units(95), want: rupees(100000, 0),
		},
		{
			// With no contract value entered, the total of the bill of
			// quantities stands in for it, so progress can still be measured.
			name: "pro-rata against a contract value taken from the bill of quantities",
			terms: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				t.ContractValue = 0
				return t
			},
			done: units(50), want: rupees(50000, 0),
		},
		{
			name: "pro-rata with nothing at all to measure progress against",
			boq:  provisional,
			terms: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				t.ContractValue = 0
				return t
			},
			done: units(50), want: 0,
		},
		{
			name:  "a final bill recovers whatever is left",
			terms: func(t Terms) Terms { return t },
			done:  units(20), final: true, want: rupees(100000, 0),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			terms := c.terms(base)
			items := c.boq
			if items == nil {
				items = boq
			}
			bill, err := Compute(Input{
				No: 1, Date: NewDate(2026, time.April, 30), Final: c.final,
				BOQ: items, Terms: terms, UpToDate: map[string]Qty{"1": c.done},
			})
			if err != nil {
				t.Fatalf("Compute: %v", err)
			}
			if got := bill.Cumulative.AdvanceRecovery; got != c.want {
				t.Errorf("advance recovered = %s, want %s", got, c.want)
			}
			if got := bill.AdvanceOutstanding; got != terms.AdvancePaid-c.want {
				t.Errorf("advance outstanding = %s, want %s", got, terms.AdvancePaid-c.want)
			}
		})
	}
}

func TestIncomeTaxDeductionBase(t *testing.T) {
	boq := BOQ{{Code: "1", Description: "Lump-sum work", Unit: "nos", ContractQty: units(100), Rate: rupees(10000, 0)}}

	terms := DefaultTerms(MarketIN)
	terms.ContractValue = rupees(1000000, 0)
	terms.AdvancePaid = rupees(100000, 0)
	terms.IncomeTaxTDSPct = PctFromPercent(2)

	// Work done 500,000; advance recovered 15% = 75,000.
	onWork := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"1": units(50)}})
	if want := rupees(10000, 0); onWork.Cumulative.IncomeTaxTDS != want {
		t.Errorf("tax deducted on the value of work = %s, want %s (2%% of 500,000)", onWork.Cumulative.IncomeTaxTDS, want)
	}

	lessAdvance := terms
	lessAdvance.IncomeTaxTDSOn = TDSOnValueLessAdvanceRecovery
	net := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: lessAdvance, UpToDate: map[string]Qty{"1": units(50)}})
	if want := rupees(8500, 0); net.Cumulative.IncomeTaxTDS != want {
		t.Errorf("tax deducted on the value less the advance recovered = %s, want %s (2%% of 425,000)", net.Cumulative.IncomeTaxTDS, want)
	}

	// The base can never go below nothing, however much is being recovered.
	heavy := lessAdvance
	heavy.AdvanceRecoveryPct = PctFromPercent(100)
	heavy.AdvancePaid = rupees(900000, 0)
	zero := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: heavy, UpToDate: map[string]Qty{"1": units(50)}})
	if zero.Cumulative.IncomeTaxTDS != 0 {
		t.Errorf("tax deducted = %s, want nothing when the whole value is being recovered", zero.Cumulative.IncomeTaxTDS)
	}
}

func TestDeductionOutOfTheTaxOnTheSupplyIsOffUntilSwitchedOn(t *testing.T) {
	boq := BOQ{{Code: "1", Description: "Lump-sum work", Unit: "nos", ContractQty: units(100), Rate: rupees(10000, 0)}}
	terms := DefaultTerms(MarketIN)
	terms.ContractValue = rupees(1000000, 0)

	off := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"1": units(50)}})
	if off.Cumulative.SupplyTaxTDS != 0 {
		t.Errorf("deduction = %s, want nothing for a client that makes none", off.Cumulative.SupplyTaxTDS)
	}

	terms.SupplyTaxTDSApplies = true
	on := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"1": units(50)}})

	deduction := rupees(10000, 0) // 2% of 500,000
	if on.Cumulative.SupplyTaxTDS != deduction {
		t.Errorf("deduction = %s, want %s (2%% of 500,000)", on.Cumulative.SupplyTaxTDS, deduction)
	}
	if got, want := on.ThisBill.NetPayable, off.ThisBill.NetPayable-deduction; got != want {
		t.Errorf("net payable = %s, want %s — %s less than with no deduction", got, want, deduction)
	}
}

func TestPhilippineContractUsesPhilippineFiguresAndWords(t *testing.T) {
	boq := BOQ{{Code: "A.1", Description: "Structural concrete, Class A", Unit: "cu.m", ContractQty: units(500), Rate: rupees(8500, 0)}}
	terms := DefaultTerms(MarketPH)
	terms.AdvancePaid = 0

	if terms.RetentionPct != PctFromPercent(10) {
		t.Errorf("retention = %s%%, want the 10%% of the Revised IRR", terms.RetentionPct)
	}
	if terms.IncomeTaxTDSPct != PctFromPercent(2) {
		t.Errorf("expanded withholding tax = %s%%, want 2%%", terms.IncomeTaxTDSPct)
	}
	if terms.SupplyTaxPct != PctFromPercent(12) {
		t.Errorf("value-added tax = %s%%, want 12%%", terms.SupplyTaxPct)
	}

	bill := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"A.1": units(100)}})

	if bill.Currency != "PHP" || bill.Symbol != "₱" {
		t.Errorf("currency %q, symbol %q", bill.Currency, bill.Symbol)
	}
	// 100 × 8,500 = 850,000; VAT 102,000; retention 85,000; EWT 17,000.
	for _, c := range []struct {
		name      string
		got, want Money
	}{
		{"value of work", bill.Cumulative.ValueOfWork, rupees(850000, 0)},
		{"value-added tax", bill.Cumulative.SupplyTax, rupees(102000, 0)},
		{"retention", bill.Cumulative.RetentionWithheld, rupees(85000, 0)},
		{"expanded withholding tax", bill.Cumulative.IncomeTaxTDS, rupees(17000, 0)},
		{"net payable", bill.ThisBill.NetPayable, rupees(850000, 0)},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s", c.name, c.got, c.want)
		}
	}
	if !strings.HasPrefix(bill.NetInWords, "Pesos eight hundred fifty thousand") {
		t.Errorf("the net in words reads %q", bill.NetInWords)
	}
	if !strings.Contains(bill.Regulation, "RA 9184") {
		t.Errorf("the regulation footnote reads %q", bill.Regulation)
	}
}

func TestIndicativeRatesAreReportedUntilTheyAreChanged(t *testing.T) {
	boq := contractBOQ()

	shipped := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: contractTerms(), UpToDate: map[string]Qty{"1.1": units(100)}})
	if !shipped.DefaultRates || !hasFlag(shipped, "indicative-rates") {
		t.Error("a bill on the rates this tool ships with must say so")
	}

	own := contractTerms()
	own.RetentionPct = PctFromPercent(7)
	changed := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: own, UpToDate: map[string]Qty{"1.1": units(100)}})
	if changed.DefaultRates || hasFlag(changed, "indicative-rates") {
		t.Error("a bill on the contract's own rates should not be reported as being on the defaults")
	}
}

func TestComputeRefusesABillThatCannotBePrepared(t *testing.T) {
	boq := contractBOQ()
	terms := contractTerms()
	measured := map[string]Qty{"1.1": units(180)}

	first := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: measured})

	closed := first
	closed.Final = true

	cases := []struct {
		name string
		in   Input
		want string
	}{
		{
			name: "a bill numbered zero",
			in:   Input{No: 0, BOQ: boq, Terms: terms, UpToDate: measured},
			want: "numbered from one",
		},
		{
			name: "an account starting in the middle",
			in:   Input{No: 3, BOQ: boq, Terms: terms, UpToDate: measured},
			want: "cannot start in the middle",
		},
		{
			name: "a bill out of sequence",
			in:   Input{No: 4, BOQ: boq, Terms: terms, Previous: &first, UpToDate: measured},
			want: "bills follow one another",
		},
		{
			name: "a bill dated before the one before it",
			in:   Input{No: 2, Date: NewDate(2026, time.April, 1), BOQ: boq, Terms: terms, Previous: &first, UpToDate: measured},
			want: "before RA bill 1",
		},
		{
			name: "a bill after the final bill",
			in:   Input{No: 2, Date: NewDate(2026, time.May, 31), BOQ: boq, Terms: terms, Previous: &closed, UpToDate: measured},
			want: "closed the account",
		},
		{
			name: "work measured against an item that is not in the contract",
			in:   Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"7.7": units(10)}},
			want: "not in the bill of quantities",
		},
		{
			name: "an item already paid for taken out of the contract",
			in: Input{No: 2, Date: NewDate(2026, time.May, 31), BOQ: BOQ{boq[1], boq[2]}, Terms: terms,
				Previous: &first, UpToDate: map[string]Qty{"2.1": units(10)}},
			want: "cannot be removed",
		},
		{
			name: "a bill of quantities with the same item twice",
			in: Input{No: 1, Date: NewDate(2026, time.April, 30), Terms: terms, UpToDate: measured,
				BOQ: BOQ{boq[0], {Code: "1.1", Description: "Excavation again", Unit: "cum", Rate: rupees(300, 0)}}},
			want: "is used twice",
		},
		{
			name: "retention of more than everything",
			in: Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, UpToDate: measured,
				Terms: func() Terms { t := terms; t.RetentionPct = PctFromPercent(150); return t }()},
			want: "retention cannot be more than 100%",
		},
		{
			name: "an advance larger than the contract",
			in: Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, UpToDate: measured,
				Terms: func() Terms {
					t := terms
					t.ContractValue = rupees(100000, 0)
					t.AdvancePaid = rupees(200000, 0)
					return t
				}()},
			want: "cannot be more than the whole contract value",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Compute(c.in)
			if err == nil {
				t.Fatalf("Compute returned bill %d, want an error about %q", got.No, c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("Compute: %v\nwant it to mention %q", err, c.want)
			}
		})
	}
}

func TestExcessBeyondTheDeviationLimitNeedsAVariationOrder(t *testing.T) {
	boq := BOQ{{Code: "1", Description: "Lump-sum work", Unit: "cum", ContractQty: units(100), Rate: rupees(1000, 0)}}
	terms := DefaultTerms(MarketIN)
	terms.ContractValue = rupees(100000, 0)

	inside := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"1": units(108)}})
	l := line(t, inside, "1")
	if l.ExcessQty != units(8) || l.ExcessPct != PctFromPercent(8) {
		t.Errorf("excess = %s (%s%%), want 8 (8%%)", l.ExcessQty, l.ExcessPct)
	}
	if l.NeedsApproval {
		t.Error("8% over is inside the 10% limit")
	}
	if !hasFlag(inside, "over-contract-quantity") || hasFlag(inside, "excess-quantity") {
		t.Error("an excess inside the limit is a note, not a stopper")
	}

	outside := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: boq, Terms: terms, UpToDate: map[string]Qty{"1": units(125)}})
	l = line(t, outside, "1")
	if !l.NeedsApproval {
		t.Error("25% over is past the 10% limit and needs a variation order")
	}
	if !hasFlag(outside, "excess-quantity") {
		t.Error("an excess past the limit should be flagged")
	}
	if !hasFlag(outside, "over-contract-value") {
		t.Error("work worth more than the contract value should be flagged")
	}
	// The flag that will hold the bill up must come first.
	if outside.Flags[0].Level != LevelWarn {
		t.Errorf("the first flag is a %q, want a warning first", outside.Flags[0].Level)
	}

	// An item with no contract quantity is not limited by one.
	provisional := BOQ{{Code: "1", Description: "Provisional item", Unit: "cum", Rate: rupees(1000, 0)}}
	free := mustCompute(t, Input{No: 1, Date: NewDate(2026, time.April, 30), BOQ: provisional, Terms: terms, UpToDate: map[string]Qty{"1": units(500)}})
	if l := line(t, free, "1"); l.ExcessQty != 0 || l.NeedsApproval {
		t.Errorf("a provisional item reported an excess of %s", l.ExcessQty)
	}
}

func TestPartTotalsGroupTheBillBySubHead(t *testing.T) {
	bills := runningAccount(t)
	final := bills[3]

	byPart := make(map[string]PartTotal, len(final.Parts))
	for _, p := range final.Parts {
		byPart[p.Part] = p
	}

	if got := byPart["Concrete"]; got.UpToDate != rupees(1179800, 0) {
		t.Errorf("concrete up to date = %s, want 1,179,800.00 (280,800 + 899,000)", got.UpToDate)
	}
	if got := byPart["Extra items"]; got.UpToDate != rupees(16560, 0) || got.ContractAmt != 0 {
		t.Errorf("extra items = %s against a contract amount of %s, want 16,560.00 against nothing", got.UpToDate, got.ContractAmt)
	}

	// The sub-heads must add up to the bill.
	var sum, previous, thisBill Money
	for _, p := range final.Parts {
		sum += p.UpToDate
		previous += p.Previous
		thisBill += p.ThisBill
	}
	if sum != final.Cumulative.ValueOfWork {
		t.Errorf("the sub-heads add up to %s, the bill to %s", sum, final.Cumulative.ValueOfWork)
	}
	if thisBill != final.ThisBill.ValueOfWork {
		t.Errorf("the sub-heads claim %s, the bill claims %s", thisBill, final.ThisBill.ValueOfWork)
	}
	if previous != final.Previous.ValueOfWork {
		t.Errorf("the sub-heads' previous column is %s, the bill's is %s", previous, final.Previous.ValueOfWork)
	}
}

func TestLedgerArithmetic(t *testing.T) {
	a := Ledger{ValueOfWork: 100000, SupplyTax: 18000, RetentionWithheld: 5000, AdvanceRecovery: 15000, IncomeTaxTDS: 2000}
	a = a.settle()
	if want := Money(96000); a.NetPayable != want {
		t.Fatalf("net = %s, want %s", a.NetPayable, want)
	}
	if err := a.consistent(); err != nil {
		t.Fatalf("consistent: %v", err)
	}
	if want := Money(118000); a.GrossBill() != want {
		t.Errorf("gross bill = %s, want %s", a.GrossBill(), want)
	}
	if want := Money(22000); a.Deductions() != want {
		t.Errorf("deductions = %s, want %s", a.Deductions(), want)
	}

	b := Ledger{ValueOfWork: 40000, SupplyTax: 7200, RetentionWithheld: 2000, AdvanceRecovery: 6000, IncomeTaxTDS: 800}.settle()

	diff := a.Sub(b)
	if diff.NetPayable != a.NetPayable-b.NetPayable {
		t.Error("subtracting line by line must give the same net as subtracting the nets")
	}
	if err := diff.consistent(); err != nil {
		t.Errorf("a difference of two consistent ledgers must be consistent: %v", err)
	}
	if got := b.Add(diff); got != a {
		t.Errorf("adding the difference back gave %+v, want %+v", got, a)
	}

	broken := a
	broken.NetPayable += 1
	if err := broken.consistent(); err == nil {
		t.Error("a ledger whose net does not follow from its lines should be refused")
	}
}
