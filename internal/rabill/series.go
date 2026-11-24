package rabill

import (
	"fmt"
	"sort"
	"strings"
)

// Reconciliation is the check that the running account adds up: the bills of
// the account, added together, against the account itself.
//
// It is the one guarantee this tool makes. A running-account bill claims the
// work done to date less everything certified before, so the bills of a
// contract must add up to the last bill's cumulative figures — exactly, to the
// paise, line by line. If they do not, something has been paid twice or lost,
// and the figures should not go to a client until it is found.
type Reconciliation struct {
	Bills int `json:"bills"`
	// SumOfBills is every bill's "this bill" column added up.
	SumOfBills Ledger `json:"sumOfBills"`
	// UpToDate is the last bill's cumulative column.
	UpToDate Ledger `json:"upToDate"`
	// Balanced reports whether the two agree on every line.
	Balanced bool `json:"balanced"`
	// Differences names every line that does not agree, and every item whose
	// quantities do not.
	Differences []string `json:"differences"`
	// Items is the same check item by item.
	Items []ItemReconciliation `json:"items"`
}

// ItemReconciliation is the check on one item: the quantity and the value
// claimed across all the bills against the quantity and value measured to date.
type ItemReconciliation struct {
	Code          string `json:"code"`
	Description   string `json:"description"`
	Unit          string `json:"unit"`
	SumOfBillsQty Qty    `json:"sumOfBillsQty"`
	UpToDateQty   Qty    `json:"upToDateQty"`
	SumOfBillsAmt Money  `json:"sumOfBillsAmount"`
	UpToDateAmt   Money  `json:"upToDateAmount"`
	Balanced      bool   `json:"balanced"`
	// Bills is how many of the bills the item appeared in.
	Bills int `json:"bills"`
}

// Reconcile adds the bills of a running account up and checks them against the
// account. The bills must be in order, which Chain guarantees.
func Reconcile(bills []Bill) Reconciliation {
	out := Reconciliation{Bills: len(bills), Balanced: true}
	if len(bills) == 0 {
		return out
	}

	for _, bill := range bills {
		out.SumOfBills = out.SumOfBills.Add(bill.ThisBill)
	}
	out.UpToDate = bills[len(bills)-1].Cumulative

	for _, line := range []struct {
		name            string
		summed, cumul   Money
		alsoAcrossBills bool
	}{
		{name: "value of work done", summed: out.SumOfBills.ValueOfWork, cumul: out.UpToDate.ValueOfWork},
		{name: "tax on the supply", summed: out.SumOfBills.SupplyTax, cumul: out.UpToDate.SupplyTax},
		{name: "retention withheld", summed: out.SumOfBills.RetentionWithheld, cumul: out.UpToDate.RetentionWithheld},
		{name: "retention released", summed: out.SumOfBills.RetentionReleased, cumul: out.UpToDate.RetentionReleased},
		{name: "mobilisation-advance recovery", summed: out.SumOfBills.AdvanceRecovery, cumul: out.UpToDate.AdvanceRecovery},
		{name: "income-tax deducted at source", summed: out.SumOfBills.IncomeTaxTDS, cumul: out.UpToDate.IncomeTaxTDS},
		{name: "deduction out of the tax on the supply", summed: out.SumOfBills.SupplyTaxTDS, cumul: out.UpToDate.SupplyTaxTDS},
		{name: "other recoveries", summed: out.SumOfBills.OtherRecoveries, cumul: out.UpToDate.OtherRecoveries},
		{name: "net payable", summed: out.SumOfBills.NetPayable, cumul: out.UpToDate.NetPayable},
	} {
		if line.summed != line.cumul {
			out.Balanced = false
			out.Differences = append(out.Differences, fmt.Sprintf(
				"%s: the bills add up to %s but the account stands at %s, a difference of %s",
				line.name, line.summed, line.cumul, line.summed-line.cumul))
		}
	}

	out.Items = reconcileItems(bills)
	for _, item := range out.Items {
		if item.Balanced {
			continue
		}
		out.Balanced = false
		if item.SumOfBillsQty != item.UpToDateQty {
			out.Differences = append(out.Differences, fmt.Sprintf(
				"item %s: the quantities claimed across the bills add up to %s %s but %s %s has been measured",
				item.Code, item.SumOfBillsQty, item.Unit, item.UpToDateQty, item.Unit))
		}
		if item.SumOfBillsAmt != item.UpToDateAmt {
			out.Differences = append(out.Differences, fmt.Sprintf(
				"item %s: the amounts claimed across the bills add up to %s but the item stands at %s",
				item.Code, item.SumOfBillsAmt, item.UpToDateAmt))
		}
	}
	return out
}

// reconcileItems checks each item's quantities and amounts across the bills.
func reconcileItems(bills []Bill) []ItemReconciliation {
	type running struct {
		rec   ItemReconciliation
		order int
	}

	byCode := make(map[string]*running)
	var order []string

	for _, bill := range bills {
		for _, line := range bill.Items {
			key := NormaliseCode(line.Code)
			item, seen := byCode[key]
			if !seen {
				item = &running{rec: ItemReconciliation{
					Code: line.Code, Description: line.Description, Unit: line.Unit,
				}, order: len(order)}
				byCode[key] = item
				order = append(order, key)
			}
			item.rec.SumOfBillsQty += line.PresentQty
			item.rec.SumOfBillsAmt += line.PresentAmount
			// The last bill the item appears in carries its figure to date.
			item.rec.UpToDateQty = line.UpToDateQty
			item.rec.UpToDateAmt = line.UpToDateAmount
			item.rec.Bills++
		}
	}

	out := make([]ItemReconciliation, 0, len(order))
	for _, key := range order {
		rec := byCode[key].rec
		rec.Balanced = rec.SumOfBillsQty == rec.UpToDateQty && rec.SumOfBillsAmt == rec.UpToDateAmt
		out = append(out, rec)
	}
	return out
}

// Chain checks that a set of bills is a running account and not a pile of
// bills: numbered from one without a gap, in date order, with at most one final
// bill and that one last. It returns them in order.
func Chain(bills []Bill) ([]Bill, error) {
	out := append([]Bill(nil), bills...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].No < out[j].No })

	var problems []string
	for n, bill := range out {
		if bill.No != n+1 {
			problems = append(problems, fmt.Sprintf("the %s bill of the account is numbered %d — the bills of a running account are numbered from one without a gap", ordinal(n+1), bill.No))
		}
		if n > 0 {
			prev := out[n-1]
			if !bill.Date.IsZero() && !prev.Date.IsZero() && bill.Date.Before(prev.Date) {
				problems = append(problems, fmt.Sprintf("RA bill %d is dated %s, before RA bill %d on %s", bill.No, bill.Date, prev.No, prev.Date))
			}
			if prev.Final {
				problems = append(problems, fmt.Sprintf("RA bill %d was the final bill, so RA bill %d cannot follow it", prev.No, bill.No))
			}
		}
		if err := bill.Cumulative.consistent(); err != nil {
			problems = append(problems, fmt.Sprintf("RA bill %d: %s", bill.No, err))
		}
	}

	// Every bill's "this bill" column must be its own cumulative column less
	// the one before it. This is what makes the account add up, so it is
	// checked rather than assumed.
	var before Ledger
	for _, bill := range out {
		if want := bill.Cumulative.Sub(before); want != bill.ThisBill {
			problems = append(problems, fmt.Sprintf(
				"RA bill %d claims a net of %s, but its figures up to date less the previous bill's come to %s",
				bill.No, bill.ThisBill.NetPayable, want.NetPayable))
		}
		before = bill.Cumulative
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(problems, "\n"))
	}
	return out, nil
}

// ordinal writes a small number as first, second, third and so on.
func ordinal(n int) string {
	names := []string{"", "first", "second", "third", "fourth", "fifth", "sixth", "seventh", "eighth", "ninth", "tenth"}
	if n > 0 && n < len(names) {
		return names[n]
	}
	return fmt.Sprintf("%dth", n)
}
