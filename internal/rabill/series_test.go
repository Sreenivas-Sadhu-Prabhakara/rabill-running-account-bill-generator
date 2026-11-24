package rabill

import (
	"strings"
	"testing"
	"time"
)

func TestChainPutsTheBillsInOrder(t *testing.T) {
	bills := runningAccount(t)
	shuffled := []Bill{bills[2], bills[0], bills[3], bills[1]}

	chained, err := Chain(shuffled)
	if err != nil {
		t.Fatalf("Chain: %v", err)
	}
	for i, bill := range chained {
		if bill.No != i+1 {
			t.Fatalf("bill %d of the chain is numbered %d", i+1, bill.No)
		}
	}
}

func TestChainRefusesWhatIsNotARunningAccount(t *testing.T) {
	bills := runningAccount(t)

	cases := []struct {
		name string
		in   []Bill
		want string
	}{
		{
			name: "a gap in the numbering",
			in:   []Bill{bills[0], bills[2]},
			want: "without a gap",
		},
		{
			name: "a bill dated before the one before it",
			in: func() []Bill {
				out := []Bill{bills[0], bills[1]}
				out[1].Date = NewDate(2026, time.January, 1)
				return out
			}(),
			want: "before RA bill 1",
		},
		{
			name: "a bill after the final bill",
			in: func() []Bill {
				out := []Bill{bills[0], bills[1]}
				out[0].Final = true
				return out
			}(),
			want: "cannot follow it",
		},
		{
			name: "a bill whose own lines do not add up",
			in: func() []Bill {
				out := []Bill{bills[0]}
				out[0].Cumulative.NetPayable += 100
				return out
			}(),
			want: "its own lines add up to",
		},
		{
			name: "a bill claiming more than the account moved",
			in: func() []Bill {
				out := []Bill{bills[0], bills[1]}
				out[1].ThisBill.NetPayable += 100
				return out
			}(),
			want: "less the previous bill's come to",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Chain(c.in)
			if err == nil {
				t.Fatalf("Chain accepted %d bills, want an error about %q", len(got), c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("Chain: %v\nwant it to mention %q", err, c.want)
			}
		})
	}
}

func TestChainAcceptsAnEmptyAccountAndASingleBill(t *testing.T) {
	if got, err := Chain(nil); err != nil || len(got) != 0 {
		t.Errorf("Chain(nil) = %v, %v; want an empty account and no error", got, err)
	}
	bills := runningAccount(t)
	if _, err := Chain([]Bill{bills[0]}); err != nil {
		t.Errorf("Chain of a single bill: %v", err)
	}
}

func TestReconcileReportsADifferenceRatherThanHidingIt(t *testing.T) {
	bills := runningAccount(t)

	// Tamper with one bill's claim, as a hand-edited data file or a bug in a
	// later version of this package might.
	tampered := append([]Bill(nil), bills...)
	tampered[1].ThisBill.ValueOfWork += rupees(1000, 0)
	tampered[1].ThisBill.NetPayable += rupees(1000, 0)

	rec := Reconcile(tampered)
	if rec.Balanced {
		t.Fatal("a tampered account must not reconcile")
	}
	joined := strings.Join(rec.Differences, "\n")
	for _, want := range []string{"value of work done", "net payable"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the differences do not mention %q:\n%s", want, joined)
		}
	}
}

func TestReconcileChecksItemsOneByOne(t *testing.T) {
	bills := runningAccount(t)

	tampered := append([]Bill(nil), bills...)
	items := append([]ItemLine(nil), tampered[2].Items...)
	for i := range items {
		if NormaliseCode(items[i].Code) == "2.2" {
			items[i].PresentQty += units(5)
		}
	}
	tampered[2].Items = items

	rec := Reconcile(tampered)
	if rec.Balanced {
		t.Fatal("an item claimed for more than was measured must not reconcile")
	}

	var found bool
	for _, item := range rec.Items {
		if NormaliseCode(item.Code) != "2.2" {
			if !item.Balanced {
				t.Errorf("item %s was not touched and should still balance", item.Code)
			}
			continue
		}
		found = true
		if item.Balanced {
			t.Error("item 2.2 was tampered with and should not balance")
		}
	}
	if !found {
		t.Fatal("item 2.2 is missing from the reconciliation")
	}
	if !strings.Contains(strings.Join(rec.Differences, "\n"), "item 2.2") {
		t.Errorf("the differences do not name item 2.2:\n%s", strings.Join(rec.Differences, "\n"))
	}
}

func TestReconcileCountsTheBillsEachItemAppearsIn(t *testing.T) {
	bills := runningAccount(t)
	rec := Reconcile(bills)

	counts := map[string]int{}
	for _, item := range rec.Items {
		counts[NormaliseCode(item.Code)] = item.Bills
	}
	// Excavation was measured in the first bill and is in all four.
	if counts["1.1"] != 4 {
		t.Errorf("item 1.1 appears in %d bills, want 4", counts["1.1"])
	}
	// Plaster was first measured in the third bill.
	if counts["4.1"] != 2 {
		t.Errorf("item 4.1 appears in %d bills, want 2", counts["4.1"])
	}
	// The extra item only appears in the final bill.
	if counts["9.1"] != 1 {
		t.Errorf("item 9.1 appears in %d bills, want 1", counts["9.1"])
	}
}

func TestReconcileAnEmptyAccount(t *testing.T) {
	rec := Reconcile(nil)
	if !rec.Balanced || rec.Bills != 0 || len(rec.Differences) != 0 {
		t.Errorf("an empty account reconciled as %+v", rec)
	}
}

// TestALongAccountWithEveryAwkwardTurnStillAddsUp walks a longer account
// through everything that goes wrong on site at once — a new item, a rate
// corrected after certification, a quantity revised downwards, an item
// deducted in full, an excess, a retention ceiling reached and a final bill —
// and checks the identity survives all of it.
func TestALongAccountWithEveryAwkwardTurnStillAddsUp(t *testing.T) {
	boq := BOQ{
		{Code: "A", Description: "Excavation", Unit: "cum", ContractQty: units(300), Rate: rupees(287, 50)},
		{Code: "B", Description: "Concrete", Unit: "cum", ContractQty: units(90), Rate: rupees(6333, 33)},
		{Code: "C", Description: "Steel", Unit: "kg", ContractQty: units(12000), Rate: rupees(78, 95)},
		{Code: "D", Description: "Shuttering", Unit: "sqm", ContractQty: units(700), Rate: rupees(412, 75)},
	}

	terms := DefaultTerms(MarketIN)
	terms.ContractValue = rupees(2000000, 0)
	terms.AdvancePaid = rupees(200000, 0)
	terms.RetentionPct = PctFromPercent(10)
	terms.RetentionCapPct = PctFromPercent(5)
	terms.SupplyTaxTDSApplies = true

	steps := []struct {
		date     Date
		final    bool
		measured map[string]Qty
		released Money
		other    Money
		boq      BOQ
	}{
		{
			date:     NewDate(2026, time.February, 29), // a leap day, for good measure
			measured: map[string]Qty{"A": 123456, "B": 7777},
		},
		{
			date:     NewDate(2026, time.March, 31),
			measured: map[string]Qty{"A": 201333, "B": 23111, "C": 1234567},
		},
		{
			// Item B is revised downwards and the rate on C is corrected after
			// bill 2 was certified: the difference belongs in this bill.
			date: NewDate(2026, time.April, 30),
			boq: BOQ{
				boq[0],
				boq[1],
				{Code: "C", Description: "Steel", Unit: "kg", ContractQty: units(12000), Rate: rupees(81, 20)},
				boq[3],
			},
			measured: map[string]Qty{"A": 201333, "B": 19999, "C": 3456789, "D": 155555},
			other:    rupees(1250, 50),
		},
		{
			// Item D is deducted in full; A runs past its contract quantity.
			date:     NewDate(2026, time.May, 31),
			measured: map[string]Qty{"A": 341000, "B": 89500, "C": 8765432, "D": 0},
			other:    rupees(1250, 50),
		},
		{
			date:     NewDate(2026, time.June, 30),
			final:    true,
			measured: map[string]Qty{"A": 341000, "B": 90250, "C": 11987654, "D": 688888},
			released: rupees(50000, 0),
			other:    rupees(3775, 25),
		},
	}

	var bills []Bill
	var previous *Bill
	for n, step := range steps {
		items := step.boq
		if items == nil {
			items = boq
		}
		bill, err := Compute(Input{
			No: n + 1, Date: step.date, Final: step.final,
			BOQ: items, Terms: terms, Previous: previous,
			UpToDate:          step.measured,
			RetentionReleased: step.released,
			OtherRecoveries:   step.other,
		})
		if err != nil {
			t.Fatalf("bill %d: %v", n+1, err)
		}
		bills = append(bills, bill)
		previous = &bills[len(bills)-1]
	}

	chained, err := Chain(bills)
	if err != nil {
		t.Fatalf("the account is not a running account: %v", err)
	}
	rec := Reconcile(chained)
	if !rec.Balanced {
		t.Fatalf("the account does not reconcile:\n%s", strings.Join(rec.Differences, "\n"))
	}

	// The rate on item C was corrected from 78.95 to 81.20 after bill 2 had
	// been certified. Bill 2 stands at 1,234.567 kg × 78.95 = 97,469.06, and
	// bill 3 stands at 3,456.789 kg × 81.20 = 280,691.27, so what bill 3
	// claims is the difference — which carries the correction on the earlier
	// quantity with it. A bill that worked the claim out as "the quantity
	// measured this time at the rate" would miss that correction entirely.
	steel := line(t, bills[2], "C")
	if want := rupees(97469, 6); steel.PreviousAmount != want {
		t.Errorf("item C's previous amount is %s, want %s — the figure bill 2 was certified at, not one recomputed at the new rate", steel.PreviousAmount, want)
	}
	if want := rupees(280691, 27); steel.UpToDateAmount != want {
		t.Errorf("item C up to date = %s, want %s", steel.UpToDateAmount, want)
	}
	if want := rupees(183222, 21); steel.PresentAmount != want {
		t.Errorf("item C claims %s in bill 3, want %s", steel.PresentAmount, want)
	}
	if atNewRateOnly, _ := Amount(steel.PresentQty, steel.Rate); atNewRateOnly == steel.PresentAmount {
		t.Error("the claim happens to equal the quantity measured this time at the rate, so this case no longer tests what it is here to test")
	}

	// The retention ceiling binds on the later bills.
	last := bills[len(bills)-1]
	if want := terms.ContractValue.Of(PctFromPercent(5)); last.Cumulative.RetentionWithheld != want {
		t.Errorf("retention withheld = %s, want the %s ceiling", last.Cumulative.RetentionWithheld, want)
	}

	// Item D was deducted in full in bill 4 and measured again in the final
	// bill, and the account still agrees on it.
	for _, item := range rec.Items {
		if NormaliseCode(item.Code) == "D" {
			if item.SumOfBillsQty != 688888 {
				t.Errorf("item D nets %s across the account, want 688.888 sqm", item.SumOfBillsQty)
			}
			if item.Bills != 3 {
				t.Errorf("item D appears in %d bills, want 3", item.Bills)
			}
		}
	}

	// And the whole account: the five bills add up to the account exactly.
	var summed Ledger
	for _, b := range bills {
		summed = summed.Add(b.ThisBill)
	}
	if summed != last.Cumulative {
		t.Errorf("the five bills add up to a net of %s, the account stands at %s", summed.NetPayable, last.Cumulative.NetPayable)
	}
}
