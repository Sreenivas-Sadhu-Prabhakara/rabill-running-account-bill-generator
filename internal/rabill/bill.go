package rabill

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Ledger is the money side of a running-account bill. The same shape serves
// three columns of the printed bill: the figures up to date, the figures of the
// previous bill, and the difference between them — what this bill asks for.
//
// Every field is a signed amount, because the difference of two cumulative
// figures is routinely negative: a measurement corrected downwards gives back
// value, and giving it back gives back the retention and the tax deducted on it
// too.
type Ledger struct {
	// ValueOfWork is the value of work done: the measured quantity of every
	// item at its rate.
	ValueOfWork Money `json:"valueOfWork"`
	// SupplyTax is the GST or VAT charged to the client on that value.
	SupplyTax Money `json:"supplyTax"`
	// RetentionWithheld is retention or security held back out of the value.
	RetentionWithheld Money `json:"retentionWithheld"`
	// RetentionReleased is retention given back, against a completion
	// certificate or at the end of the defect-liability period.
	RetentionReleased Money `json:"retentionReleased"`
	// AdvanceRecovery is the mobilisation advance taken back.
	AdvanceRecovery Money `json:"advanceRecovery"`
	// IncomeTaxTDS is the income-tax deduction the client makes at source.
	IncomeTaxTDS Money `json:"incomeTaxTds"`
	// SupplyTaxTDS is the deduction the client makes out of the tax on the
	// supply.
	SupplyTaxTDS Money `json:"supplyTaxTds"`
	// OtherRecoveries is everything else the contract lets the client recover:
	// water and electricity charges, hire of plant, penalties, debit notes.
	OtherRecoveries Money `json:"otherRecoveries"`
	// NetPayable is what is left to pay.
	NetPayable Money `json:"netPayable"`
}

// net works out what is left to pay from the lines above it. Every other
// figure in this package is derived from a cumulative measurement; this is the
// one line that is derived from the others, so it lives in one function.
func (l Ledger) net() Money {
	return l.ValueOfWork + l.SupplyTax + l.RetentionReleased -
		l.RetentionWithheld - l.AdvanceRecovery - l.IncomeTaxTDS -
		l.SupplyTaxTDS - l.OtherRecoveries
}

// settle fills in the net payable from the lines above it.
func (l Ledger) settle() Ledger {
	l.NetPayable = l.net()
	return l
}

// GrossBill is the value of work plus the tax charged on it: the amount the
// bill is raised for, before anything is deducted from it.
func (l Ledger) GrossBill() Money { return l.ValueOfWork + l.SupplyTax }

// Deductions is everything held back, net of anything released.
func (l Ledger) Deductions() Money { return l.GrossBill() - l.NetPayable }

// Sub returns this ledger less another, line by line. Because the net payable
// is a plain sum of the other lines, subtracting line by line and subtracting
// the nets give the same answer — which is what makes the bills of a running
// account add up to the account.
func (l Ledger) Sub(o Ledger) Ledger {
	return Ledger{
		ValueOfWork:       l.ValueOfWork - o.ValueOfWork,
		SupplyTax:         l.SupplyTax - o.SupplyTax,
		RetentionWithheld: l.RetentionWithheld - o.RetentionWithheld,
		RetentionReleased: l.RetentionReleased - o.RetentionReleased,
		AdvanceRecovery:   l.AdvanceRecovery - o.AdvanceRecovery,
		IncomeTaxTDS:      l.IncomeTaxTDS - o.IncomeTaxTDS,
		SupplyTaxTDS:      l.SupplyTaxTDS - o.SupplyTaxTDS,
		OtherRecoveries:   l.OtherRecoveries - o.OtherRecoveries,
		NetPayable:        l.NetPayable - o.NetPayable,
	}
}

// Add returns the two ledgers added line by line.
func (l Ledger) Add(o Ledger) Ledger {
	return Ledger{
		ValueOfWork:       l.ValueOfWork + o.ValueOfWork,
		SupplyTax:         l.SupplyTax + o.SupplyTax,
		RetentionWithheld: l.RetentionWithheld + o.RetentionWithheld,
		RetentionReleased: l.RetentionReleased + o.RetentionReleased,
		AdvanceRecovery:   l.AdvanceRecovery + o.AdvanceRecovery,
		IncomeTaxTDS:      l.IncomeTaxTDS + o.IncomeTaxTDS,
		SupplyTaxTDS:      l.SupplyTaxTDS + o.SupplyTaxTDS,
		OtherRecoveries:   l.OtherRecoveries + o.OtherRecoveries,
		NetPayable:        l.NetPayable + o.NetPayable,
	}
}

// consistent reports whether the net payable still follows from the lines above
// it. It is checked on every bill, so that a change to this package can never
// quietly produce a bill that does not add up.
func (l Ledger) consistent() error {
	if want := l.net(); want != l.NetPayable {
		return fmt.Errorf("the net payable on this bill reads %s but its own lines add up to %s", l.NetPayable, want)
	}
	return nil
}

// Flag is something about a bill the person preparing it should look at before
// it goes to the client.
type Flag struct {
	// Level is how much it matters: "note" for information, "check" for
	// something to verify, "warn" for something that will hold the bill up.
	Level string `json:"level"`
	// Code names the kind of flag, so the interface can group them.
	Code string `json:"code"`
	// Item is the bill-of-quantities item it concerns, where it concerns one.
	Item string `json:"item,omitempty"`
	// Message says what it is, in words.
	Message string `json:"message"`
}

// The levels a flag carries.
const (
	LevelNote  = "note"
	LevelCheck = "check"
	LevelWarn  = "warn"
)

// ItemLine is one item as the bill accounts for it: the quantity and value up
// to date, what the previous bill had, and the difference this bill claims.
type ItemLine struct {
	Code        string `json:"code"`
	Part        string `json:"part"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
	Rate        Money  `json:"rate"`
	ContractQty Qty    `json:"contractQty"`
	Extra       bool   `json:"extra"`

	PreviousQty Qty `json:"previousQty"`
	PresentQty  Qty `json:"presentQty"`
	UpToDateQty Qty `json:"upToDateQty"`

	PreviousAmount Money `json:"previousAmount"`
	PresentAmount  Money `json:"presentAmount"`
	UpToDateAmount Money `json:"upToDateAmount"`

	// Carried marks an item the measurement book said nothing about in this
	// bill, so it stands at the previous bill's figures and claims nothing. It
	// is still printed: an item billed once is never left out of a later bill.
	Carried bool `json:"carried"`
	// ExcessQty is how far the measured quantity has run past the contract
	// quantity, and ExcessPct the same as a percentage of it.
	ExcessQty Qty `json:"excessQty"`
	ExcessPct Pct `json:"excessPct"`
	// NeedsApproval marks a line that cannot be certified without something in
	// writing from the client: an extra item with no agreed rate, or an excess
	// beyond the contract's deviation limit.
	NeedsApproval bool `json:"needsApproval"`
}

// PartTotal is the sub-total of one sub-head of the contract.
type PartTotal struct {
	Part        string `json:"part"`
	UpToDate    Money  `json:"upToDate"`
	Previous    Money  `json:"previous"`
	ThisBill    Money  `json:"thisBill"`
	ContractAmt Money  `json:"contractAmount"`
}

// Bill is one running-account bill, worked out and ready to print.
//
// Once it has been certified it is frozen: the next bill subtracts the figures
// in it exactly as they stand, not figures recomputed from today's rates. A
// rate corrected after bill 3 was certified therefore shows up as a difference
// in bill 4, which is where a running account puts it.
type Bill struct {
	No         int    `json:"no"`
	Date       Date   `json:"date"`
	PeriodFrom Date   `json:"periodFrom"`
	PeriodTo   Date   `json:"periodTo"`
	Final      bool   `json:"final"`
	Note       string `json:"note"`

	Items []ItemLine  `json:"items"`
	Parts []PartTotal `json:"parts"`

	// Cumulative is every figure up to date; Previous is the previous bill's
	// cumulative figures; ThisBill is Cumulative less Previous, which is what
	// the bill claims.
	Cumulative Ledger `json:"cumulative"`
	Previous   Ledger `json:"previous"`
	ThisBill   Ledger `json:"thisBill"`

	ContractValue      Money `json:"contractValue"`
	ProgressPct        Pct   `json:"progressPct"`
	RetentionHeld      Money `json:"retentionHeld"`
	RetentionCap       Money `json:"retentionCap"`
	AdvanceOutstanding Money `json:"advanceOutstanding"`

	PaymentDueBy        Date `json:"paymentDueBy"`
	RetentionReleaseDue Date `json:"retentionReleaseDue"`

	Terms        Terms  `json:"terms"`
	Currency     string `json:"currency"`
	Symbol       string `json:"symbol"`
	NetInWords   string `json:"netInWords"`
	Regulation   string `json:"regulation"`
	DefaultRates bool   `json:"defaultRates"`

	Flags []Flag `json:"flags"`
}

// Input is everything one bill is worked out from.
type Input struct {
	// No is the bill's number in the running account, counting from one.
	No int
	// Date is the date the bill is drawn up to.
	Date Date
	// PeriodFrom and PeriodTo are the period the work was done in. They are
	// printed on the bill and are not used in any arithmetic.
	PeriodFrom Date
	PeriodTo   Date
	// Final marks the last bill of the contract, which closes the account.
	Final bool
	Note  string

	// BOQ is the bill of quantities, which carries the rates.
	BOQ BOQ
	// UpToDate is the quantity measured against each item SO FAR — the whole
	// of it, not the part measured since the last bill. Book.Quantities
	// produces it from a measurement book.
	UpToDate map[string]Qty
	// Previous is the last certified bill, or nil for the first bill of the
	// account.
	Previous *Bill
	// Terms are the contract's money terms.
	Terms Terms

	// OtherRecoveries and RetentionReleased are cumulative figures the user
	// enters directly: everything recovered under the contract other than the
	// advance, and all retention given back so far.
	OtherRecoveries   Money
	RetentionReleased Money
}

// Compute works out one running-account bill.
//
// The whole of the arithmetic is cumulative. Every figure is worked out on the
// work measured since the contract started, rounded once; what the bill claims
// is that figure less the figure the previous bill was certified at. Nothing is
// ever worked out on "the work done this month", because a quantity revised in
// a later bill would then be either paid twice or lost.
func Compute(in Input) (Bill, error) {
	terms := in.Terms.Normalise()
	if err := terms.Validate(); err != nil {
		return Bill{}, err
	}

	boq := in.BOQ.Normalise()
	if err := boq.Validate(); err != nil {
		return Bill{}, err
	}

	if in.No < 1 {
		return Bill{}, errors.New("a running-account bill is numbered from one")
	}
	if in.Previous == nil {
		if in.No != 1 {
			return Bill{}, fmt.Errorf("this is bill %d but there is no bill %d before it — a running account cannot start in the middle", in.No, in.No-1)
		}
	} else {
		if in.Previous.Final {
			return Bill{}, fmt.Errorf("RA bill %d was the final bill and closed the account", in.Previous.No)
		}
		if in.No != in.Previous.No+1 {
			return Bill{}, fmt.Errorf("this is bill %d but the last certified bill is %d — bills follow one another", in.No, in.Previous.No)
		}
		if !in.Date.IsZero() && !in.Previous.Date.IsZero() && in.Date.Before(in.Previous.Date) {
			return Bill{}, fmt.Errorf("this bill is dated %s, before RA bill %d on %s", in.Date, in.Previous.No, in.Previous.Date)
		}
	}

	bill := Bill{
		No:         in.No,
		Date:       in.Date,
		PeriodFrom: in.PeriodFrom,
		PeriodTo:   in.PeriodTo,
		Final:      in.Final,
		Note:       strings.TrimSpace(in.Note),
		Terms:      terms,
		Currency:   terms.Market.Currency(),
		Symbol:     terms.Market.Symbol(),
		Regulation: terms.Regulation(),
	}
	bill.DefaultRates = terms.IsIndicativeDefault()

	items, flags, err := itemLines(boq, in.UpToDate, in.Previous, terms)
	if err != nil {
		return Bill{}, err
	}
	bill.Items = items
	bill.Flags = flags

	var valueOfWork Money
	for _, line := range items {
		valueOfWork += line.UpToDateAmount
	}

	contractValue := terms.ContractValue
	if contractValue == 0 {
		total, err := boq.Total()
		if err != nil {
			return Bill{}, err
		}
		contractValue = total
	}
	bill.ContractValue = contractValue
	if contractValue > 0 {
		bill.ProgressPct = percentOf(int64(valueOfWork), int64(contractValue))
	}

	cum := Ledger{ValueOfWork: valueOfWork}
	cum.SupplyTax = valueOfWork.Of(terms.SupplyTaxPct)

	// Retention: a share of the value of work done, held to a ceiling set as a
	// share of the contract value. Without a contract value there is nothing to
	// take the ceiling of, so there is no ceiling.
	cum.RetentionWithheld = valueOfWork.Of(terms.RetentionPct)
	if terms.RetentionCapPct > 0 && contractValue > 0 {
		bill.RetentionCap = contractValue.Of(terms.RetentionCapPct)
		if cum.RetentionWithheld > bill.RetentionCap {
			cum.RetentionWithheld = bill.RetentionCap
			bill.Flags = append(bill.Flags, Flag{
				Level: LevelNote, Code: "retention-capped",
				Message: fmt.Sprintf("Retention has reached its ceiling of %s (%s%% of the contract value), so no further retention is held.",
					bill.RetentionCap, terms.RetentionCapPct),
			})
		}
	}

	cum.RetentionReleased = in.RetentionReleased
	if cum.RetentionReleased < 0 {
		cum.RetentionReleased = 0
	}
	if cum.RetentionReleased > cum.RetentionWithheld {
		bill.Flags = append(bill.Flags, Flag{
			Level: LevelWarn, Code: "over-release",
			Message: fmt.Sprintf("The retention released to date, %s, is more than the %s held. It has been limited to what is held.",
				cum.RetentionReleased, cum.RetentionWithheld),
		})
		cum.RetentionReleased = cum.RetentionWithheld
	}
	bill.RetentionHeld = cum.RetentionWithheld - cum.RetentionReleased

	cum.AdvanceRecovery = advanceRecovery(terms, valueOfWork, contractValue, in.Final)
	bill.AdvanceOutstanding = terms.AdvancePaid - cum.AdvanceRecovery

	tdsBase := valueOfWork
	if terms.IncomeTaxTDSOn == TDSOnValueLessAdvanceRecovery {
		tdsBase = valueOfWork - cum.AdvanceRecovery
		if tdsBase < 0 {
			tdsBase = 0
		}
	}
	cum.IncomeTaxTDS = tdsBase.Of(terms.IncomeTaxTDSPct)

	if terms.SupplyTaxTDSApplies {
		cum.SupplyTaxTDS = valueOfWork.Of(terms.SupplyTaxTDSPct)
	}

	cum.OtherRecoveries = in.OtherRecoveries
	cum = cum.settle()

	bill.Cumulative = cum
	if in.Previous != nil {
		bill.Previous = in.Previous.Cumulative
	}
	bill.ThisBill = cum.Sub(bill.Previous)

	for _, l := range []Ledger{bill.Cumulative, bill.ThisBill} {
		if err := l.consistent(); err != nil {
			return Bill{}, err
		}
	}

	bill.Parts = partTotals(boq, items)
	bill.NetInWords = AmountInWords(bill.ThisBill.NetPayable, terms.Market)

	if !in.Date.IsZero() {
		if terms.PaymentDays > 0 {
			bill.PaymentDueBy = in.Date.AddDays(terms.PaymentDays)
		}
		if in.Final && terms.DefectLiabilityMonths > 0 {
			bill.RetentionReleaseDue = in.Date.AddMonths(terms.DefectLiabilityMonths)
		}
	}

	bill.Flags = append(bill.Flags, billFlags(bill, boq, contractValue)...)
	sortFlags(bill.Flags)
	return bill, nil
}

// itemLines builds one line per item, from the measured quantities and the
// previous bill.
//
// Two rules keep the account honest, and both are enforced here:
//
// An item that has ever been billed is in every later bill. If the measurement
// book says nothing about it this time it stands at the previous figures and
// claims nothing, so it can never fall out of the account unnoticed.
//
// What a bill claims for an item is the amount up to date less the amount the
// previous bill was certified at — never the quantity measured this time at the
// rate. Those two differ by the rounding on every earlier bill, and the
// difference is exactly how a spreadsheet comes to pay for the same work twice.
func itemLines(boq BOQ, upToDate map[string]Qty, previous *Bill, terms Terms) ([]ItemLine, []Flag, error) {
	var flags []Flag

	measured := make(map[string]Qty, len(upToDate))
	for code, q := range upToDate {
		measured[NormaliseCode(code)] = q
	}

	prevLines := make(map[string]ItemLine)
	if previous != nil {
		for _, line := range previous.Items {
			prevLines[NormaliseCode(line.Code)] = line
		}
	}

	index := boq.Index()

	// Every item the bill must account for: everything measured so far and
	// everything any earlier bill has already paid for.
	wanted := make(map[string]bool, len(measured)+len(prevLines))
	for code := range measured {
		wanted[code] = true
	}
	for code := range prevLines {
		wanted[code] = true
	}

	var missing []string
	for code := range wanted {
		if _, known := index[code]; !known {
			missing = append(missing, code)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		var problems []string
		for _, code := range missing {
			if _, billed := prevLines[code]; billed {
				problems = append(problems, fmt.Sprintf(
					"item %s has already been billed but is no longer in the bill of quantities — an item that has been paid for cannot be removed from it", code))
				continue
			}
			problems = append(problems, fmt.Sprintf(
				"item %s has been measured but is not in the bill of quantities — add it, marking it an extra item if the contract never priced it", code))
		}
		return nil, nil, fmt.Errorf("%s", strings.Join(problems, "\n"))
	}

	lines := make([]ItemLine, 0, len(wanted))
	for _, item := range boq {
		code := item.Key()
		if !wanted[code] {
			continue
		}
		prev := prevLines[code]

		line := ItemLine{
			Code:           item.Code,
			Part:           item.Part,
			Description:    item.Description,
			Unit:           item.Unit,
			Rate:           item.Rate,
			ContractQty:    item.ContractQty,
			Extra:          item.Extra,
			PreviousQty:    prev.UpToDateQty,
			PreviousAmount: prev.UpToDateAmount,
		}

		q, wasMeasured := measured[code]
		if wasMeasured {
			line.UpToDateQty = q
		} else {
			// Nothing measured against it this time: hold it where it was.
			line.UpToDateQty = prev.UpToDateQty
			line.Carried = true
			if prev.UpToDateAmount != 0 {
				flags = append(flags, Flag{
					Level: LevelCheck, Code: "carried-forward", Item: item.Code,
					Message: fmt.Sprintf("Item %s has no measurement in this bill, so it stands at the %s certified before and claims nothing. Check that is right.",
						item.Code, prev.UpToDateQty),
				})
			}
		}

		amount, err := Amount(line.UpToDateQty, line.Rate)
		if err != nil {
			return nil, nil, fmt.Errorf("item %s: %w", item.Code, err)
		}
		line.UpToDateAmount = amount
		line.PresentQty = line.UpToDateQty - line.PreviousQty
		line.PresentAmount = line.UpToDateAmount - line.PreviousAmount

		if item.ContractQty > 0 && line.UpToDateQty > item.ContractQty {
			line.ExcessQty = line.UpToDateQty - item.ContractQty
			line.ExcessPct = percentOf(int64(line.ExcessQty), int64(item.ContractQty))
			if terms.VariationLimitPct > 0 && line.ExcessPct > terms.VariationLimitPct {
				line.NeedsApproval = true
			}
		}
		if item.Extra {
			line.NeedsApproval = true
		}

		lines = append(lines, line)
	}
	return lines, flags, nil
}

// advanceRecovery works out how much of the mobilisation advance has been taken
// back by the time the value of work reaches a figure.
//
// It is always worked out from the cumulative value of work, never accumulated
// bill by bill, so that it cannot drift past the advance actually paid and
// cannot be taken twice. A final bill recovers whatever is left, because an
// advance not recovered by the final bill is not recovered at all.
func advanceRecovery(terms Terms, valueOfWork, contractValue Money, final bool) Money {
	if terms.AdvancePaid <= 0 {
		return 0
	}
	if final {
		return terms.AdvancePaid
	}

	var recovery Money
	switch terms.RecoveryMethod {
	case RecoveryProRata:
		if contractValue <= 0 {
			return 0
		}
		progress := percentOf(int64(valueOfWork), int64(contractValue))
		start, end := terms.AdvanceRecoveryStartPct, terms.AdvanceRecoveryEndPct
		switch {
		case end <= start:
			return 0
		case progress <= start:
			return 0
		case progress >= end:
			return terms.AdvancePaid
		}
		through := percentOf(int64(progress-start), int64(end-start))
		recovery = terms.AdvancePaid.Of(through)
	default:
		if valueOfWork <= 0 {
			return 0
		}
		recovery = valueOfWork.Of(terms.AdvanceRecoveryPct)
	}

	if recovery > terms.AdvancePaid {
		recovery = terms.AdvancePaid
	}
	if recovery < 0 {
		recovery = 0
	}
	return recovery
}

// partTotals sub-totals the bill by the sub-heads the items fall under.
func partTotals(boq BOQ, lines []ItemLine) []PartTotal {
	contract := make(map[string]Money)
	for _, item := range boq {
		if item.Extra {
			continue
		}
		if amount, err := item.Amount(); err == nil {
			contract[item.Part] += amount
		}
	}

	byPart := make(map[string]*PartTotal)
	var order []string
	for _, line := range lines {
		total, seen := byPart[line.Part]
		if !seen {
			total = &PartTotal{Part: line.Part, ContractAmt: contract[line.Part]}
			byPart[line.Part] = total
			order = append(order, line.Part)
		}
		total.UpToDate += line.UpToDateAmount
		total.Previous += line.PreviousAmount
		total.ThisBill += line.PresentAmount
	}

	out := make([]PartTotal, 0, len(order))
	for _, part := range order {
		out = append(out, *byPart[part])
	}
	return out
}

// billFlags is everything worth saying about the bill as a whole.
func billFlags(bill Bill, boq BOQ, contractValue Money) []Flag {
	var flags []Flag

	if bill.DefaultRates {
		flags = append(flags, Flag{
			Level: LevelCheck, Code: "indicative-rates",
			Message: "Every deduction rate on this bill is still the indicative default this tool ships with. Check each one against the contract agreement before the bill goes out.",
		})
	}

	for _, line := range bill.Items {
		switch {
		case line.Extra:
			flags = append(flags, Flag{
				Level: LevelWarn, Code: "extra-item", Item: line.Code,
				Message: fmt.Sprintf("Item %s is an extra item the contract never priced. Its rate needs the client's approval in writing before this bill can be certified.", line.Code),
			})
		case line.NeedsApproval:
			flags = append(flags, Flag{
				Level: LevelWarn, Code: "excess-quantity", Item: line.Code,
				Message: fmt.Sprintf("Item %s has been measured %s %s beyond its contract quantity, %s%% over, which is past the %s%% deviation limit. It needs a written deviation or variation order.",
					line.Code, line.ExcessQty, line.Unit, line.ExcessPct, bill.Terms.VariationLimitPct),
			})
		case line.ExcessQty > 0:
			flags = append(flags, Flag{
				Level: LevelNote, Code: "over-contract-quantity", Item: line.Code,
				Message: fmt.Sprintf("Item %s stands %s %s over its contract quantity, %s%% over, within the %s%% deviation limit.",
					line.Code, line.ExcessQty, line.Unit, line.ExcessPct, bill.Terms.VariationLimitPct),
			})
		}

		if line.PresentAmount < 0 {
			flags = append(flags, Flag{
				Level: LevelCheck, Code: "minus-quantity", Item: line.Code,
				Message: fmt.Sprintf("Item %s is a minus quantity in this bill: %s %s less than the last bill was certified at, recovering %s. That is how a measurement corrected downwards is accounted for, but check the correction is intended.",
					line.Code, line.PresentQty.Abs(), line.Unit, line.PresentAmount.Abs()),
			})
		}
	}

	if contractValue > 0 && bill.Cumulative.ValueOfWork > contractValue {
		over := bill.Cumulative.ValueOfWork - contractValue
		overPct := percentOf(int64(over), int64(contractValue))
		level := LevelNote
		if bill.Terms.VariationLimitPct > 0 && overPct > bill.Terms.VariationLimitPct {
			level = LevelWarn
		}
		flags = append(flags, Flag{
			Level: level, Code: "over-contract-value",
			Message: fmt.Sprintf("The value of work done, %s, is %s over the contract value of %s — %s%% over. A variation order covering it should be on record.",
				bill.Cumulative.ValueOfWork, over, contractValue, overPct),
		})
	}

	if bill.Terms.AdvancePaid > 0 {
		switch {
		case bill.AdvanceOutstanding <= 0:
			flags = append(flags, Flag{
				Level: LevelNote, Code: "advance-recovered",
				Message: fmt.Sprintf("The mobilisation advance of %s has been recovered in full.", bill.Terms.AdvancePaid),
			})
		case bill.Final:
			flags = append(flags, Flag{
				Level: LevelWarn, Code: "advance-outstanding",
				Message: fmt.Sprintf("%s of the mobilisation advance was still outstanding, and the whole balance has been recovered in this final bill.", bill.AdvanceOutstanding),
			})
		}
	}

	if bill.ThisBill.NetPayable < 0 {
		flags = append(flags, Flag{
			Level: LevelWarn, Code: "net-recovery",
			Message: fmt.Sprintf("This bill is a net recovery of %s rather than a payment: the deductions and the downward corrections together come to more than the work certified in it.", bill.ThisBill.NetPayable.Abs()),
		})
	}

	if bill.Final {
		var unmeasured, shortfall int
		for _, line := range bill.Items {
			if line.ContractQty > 0 && line.UpToDateQty < line.ContractQty {
				shortfall++
			}
		}
		billed := make(map[string]bool, len(bill.Items))
		for _, line := range bill.Items {
			billed[NormaliseCode(line.Code)] = true
		}
		for _, item := range boq {
			if !billed[item.Key()] {
				unmeasured++
			}
		}
		if shortfall > 0 {
			flags = append(flags, Flag{
				Level: LevelCheck, Code: "final-under-run",
				Message: fmt.Sprintf("This is the final bill and %d item(s) close below their contract quantity. An under-run on a final bill should be accounted for in the deviation statement.", shortfall),
			})
		}
		if unmeasured > 0 {
			flags = append(flags, Flag{
				Level: LevelWarn, Code: "final-unbilled-items",
				Message: fmt.Sprintf("This is the final bill and %d item(s) of the bill of quantities have never been measured at all. Confirm the work was not done before closing the account.", unmeasured),
			})
		}
		if bill.RetentionHeld > 0 {
			when := "the end of the defect-liability period"
			if !bill.RetentionReleaseDue.IsZero() {
				when = bill.RetentionReleaseDue.Long()
			}
			flags = append(flags, Flag{
				Level: LevelNote, Code: "retention-held-after-final",
				Message: fmt.Sprintf("%s of retention is still held after this final bill. On the terms entered it falls due on %s.", bill.RetentionHeld, when),
			})
		}
	}

	return flags
}

// sortFlags puts what will hold the bill up first.
func sortFlags(flags []Flag) {
	rank := map[string]int{LevelWarn: 0, LevelCheck: 1, LevelNote: 2}
	sort.SliceStable(flags, func(i, j int) bool { return rank[flags[i].Level] < rank[flags[j].Level] })
}
