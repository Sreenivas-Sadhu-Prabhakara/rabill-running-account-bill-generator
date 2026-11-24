package rabill

import (
	"fmt"
	"strings"
)

// The figures below are the whole of this tool's opinion about money. Every one
// of them is INDICATIVE. They are the rates and limits most often written into
// a small civil contract, not rates this tool knows apply to yours. The
// contract agreement, the work order and the tax law in force govern — in that
// order — and every figure here can be overridden on the Contract terms page
// before a single bill is prepared.
//
// They are gathered in one block on purpose: a rate changes, a contract departs
// from the norm, and whoever maintains this should have exactly one place to
// look.
const (
	// --- Retention / security deduction -------------------------------------
	// Retention is a slice of every progress payment held back against defects
	// and released after the defect-liability period. It is a contract term,
	// not a statutory one.
	//
	// India: 5% of the value of work done, held to a ceiling of 5% of the
	// contract value, is the common CPWD-style term; 10% appears too.
	//
	// Philippines: the Revised IRR of Republic Act 9184, section 62.1, provides
	// for retention money of 10% of every progress payment, building up to 10%
	// of the total contract price, on government infrastructure contracts.
	DefaultRetentionPctIN    = 500  // 5.00%
	DefaultRetentionCapPctIN = 500  // ceiling: 5.00% of the contract value
	DefaultRetentionPctPH    = 1000 // 10.00%
	DefaultRetentionCapPctPH = 1000 // ceiling: 10.00% of the contract value

	// DefaultDefectLiabilityMonths is how long after the final bill the balance
	// of retention is held. Twelve months from completion or from taking over
	// is the usual term in both markets.
	DefaultDefectLiabilityMonths = 12

	// DefaultFinalRetentionReleasePct is how much of the retention held is
	// commonly released against the completion certificate, the balance falling
	// due when the defect-liability period ends.
	DefaultFinalRetentionReleasePct = 5000 // 50.00% of the retention held

	// --- Mobilisation advance and its recovery ------------------------------
	// A mobilisation advance is paid at the start against a bank guarantee and
	// recovered out of the running-account bills.
	//
	// India: 10% of the contract value is a common term.
	// Philippines: section 88.2.1 of the Revised IRR of RA 9184 allows an
	// advance payment of up to 15% of the total contract price, recouped by
	// deduction from the progress payments.
	DefaultAdvancePctIN = 1000 // 10.00% of the contract value
	DefaultAdvancePctPH = 1500 // 15.00% of the contract price

	// DefaultAdvanceRecoveryPct is how much of each bill's value of work done
	// is applied against the advance until it is recovered in full.
	DefaultAdvanceRecoveryPct = 1500 // 15.00% of the value of work done

	// Pro-rata recovery instead takes the advance back in proportion to
	// progress, starting and finishing at these points of the contract value.
	DefaultAdvanceRecoveryStartPct = 1000 // begins at 10% progress
	DefaultAdvanceRecoveryEndPct   = 9000 // completes at 90% progress

	// --- Income-tax deduction at source -------------------------------------
	// India: section 194C of the Income-tax Act, 1961 requires tax to be
	// deducted from payments to a contractor at 1% where the payee is an
	// individual or a Hindu undivided family and 2% otherwise. Section 206AA
	// raises it to 20% where the payee has furnished no PAN. The thresholds in
	// section 194C(5) are a single payment above Rs 30,000 or Rs 1,00,000 in
	// aggregate during the financial year.
	TDS194CIndividualHUFPct = 100  // 1.00%
	TDS194COtherPct         = 200  // 2.00%
	TDS206AANoPANPct        = 2000 // 20.00%
	TDS194CSinglePaymentINR = 30000
	TDS194CAnnualINR        = 100000

	// Philippines: expanded withholding tax on income payments to contractors
	// is 2% under Revenue Regulations 2-98 as amended, withheld by the payor.
	EWTContractorPctPH = 200 // 2.00%

	// --- Tax on the supply itself -------------------------------------------
	// India: a works contract is a supply of service. The rate most commonly
	// charged to a non-government client is 18% GST; concessional rates apply
	// to some government and affordable-housing work.
	DefaultGSTWorksContractPct = 1800 // 18.00%

	// India: section 51 of the CGST Act, 2017 requires a specified deductor —
	// a government department, a local authority and the other persons
	// notified — to deduct 2% (1% CGST + 1% SGST, or 2% IGST) from payments
	// under a contract whose total value exceeds Rs 2,50,000. It is deducted on
	// the value of the supply, excluding the GST charged on it.
	GSTTDSPct                  = 200 // 2.00%
	GSTTDSContractThresholdINR = 250000

	// Philippines: value-added tax on the sale of services is 12% under section
	// 108 of the National Internal Revenue Code as amended. A government payor
	// also withholds 5% of the gross payment as creditable withholding VAT
	// under section 114(C).
	DefaultVATPctPH         = 1200 // 12.00%
	WithholdingVATGovtPctPH = 500  // 5.00%

	// --- Contract administration --------------------------------------------
	// DefaultVariationLimitPct is how far the measured quantity of an item may
	// run past its contract quantity before the excess needs a written
	// deviation or variation order. Ten per cent of the contract amount is the
	// limit in section 162.1 of the Revised IRR of RA 9184, and a similar
	// figure appears in CPWD-style deviation clauses.
	DefaultVariationLimitPct = 1000 // 10.00%

	// DefaultPaymentDays is how long after certification the contract allows
	// for payment of a running-account bill.
	DefaultPaymentDays = 30
)

// Market is the country whose defaults and named regulations a contract is read
// against.
type Market string

// The markets this tool ships defaults for.
const (
	MarketIN Market = "IN"
	MarketPH Market = "PH"
)

// Currency returns the currency a market's contracts are written in.
func (m Market) Currency() string {
	if m == MarketPH {
		return "PHP"
	}
	return "INR"
}

// Symbol returns the currency symbol a market's bills are printed with.
func (m Market) Symbol() string {
	if m == MarketPH {
		return "₱"
	}
	return "₹"
}

// RecoveryMethod is how a mobilisation advance is taken back out of the bills.
type RecoveryMethod string

// The two recovery methods contracts normally use.
const (
	// RecoveryPercentOfBill applies a fixed share of the value of work done to
	// the advance until it is recovered in full.
	RecoveryPercentOfBill RecoveryMethod = "percent-of-value"
	// RecoveryProRata takes the advance back in proportion to progress between
	// two points of the contract value.
	RecoveryProRata RecoveryMethod = "pro-rata"
)

// TDSBase is the figure income-tax deduction at source is worked out on.
type TDSBase string

// The two bases a contract may use for income-tax deduction.
const (
	// TDSOnValueOfWork deducts on the whole value of work done in the bill,
	// excluding the tax charged on the supply. This is the usual treatment.
	TDSOnValueOfWork TDSBase = "value-of-work"
	// TDSOnValueLessAdvanceRecovery deducts on the value of work less the
	// advance recovered in the bill, for a contract where tax was already
	// deducted when the advance was paid. Which of the two applies is a
	// question for the deductor and the contract, not for this tool.
	TDSOnValueLessAdvanceRecovery TDSBase = "value-less-advance-recovery"
)

// Terms are the money terms of one contract: the rates and limits every bill in
// the running account is worked out against. Every field starts at the
// indicative default for the market and is the user's to change.
type Terms struct {
	Market Market `json:"market"`

	// ContractValue is the agreed price of the contract, which the retention
	// ceiling and the progress percentages are measured against. Left at zero,
	// the total of the bill of quantities stands in for it.
	ContractValue Money `json:"contractValue"`

	RetentionPct          Pct `json:"retentionPct"`
	RetentionCapPct       Pct `json:"retentionCapPct"`
	DefectLiabilityMonths int `json:"defectLiabilityMonths"`

	AdvancePaid             Money          `json:"advancePaid"`
	RecoveryMethod          RecoveryMethod `json:"recoveryMethod"`
	AdvanceRecoveryPct      Pct            `json:"advanceRecoveryPct"`
	AdvanceRecoveryStartPct Pct            `json:"advanceRecoveryStartPct"`
	AdvanceRecoveryEndPct   Pct            `json:"advanceRecoveryEndPct"`

	// SupplyTaxPct is the GST or VAT the contractor charges the client on the
	// value of work done. Zero where the client pays the tax directly, or where
	// the contractor is not registered.
	SupplyTaxPct Pct `json:"supplyTaxPct"`

	// IncomeTaxTDSPct is the income-tax deduction the client makes: section
	// 194C in India, expanded withholding tax in the Philippines.
	IncomeTaxTDSPct Pct     `json:"incomeTaxTdsPct"`
	IncomeTaxTDSOn  TDSBase `json:"incomeTaxTdsOn"`

	// SupplyTaxTDSPct is the deduction the client makes out of the tax on the
	// supply: GST TDS under section 51 in India, creditable withholding VAT in
	// the Philippines. It applies only to some clients and some contracts, so
	// it is off until switched on.
	SupplyTaxTDSPct     Pct  `json:"supplyTaxTdsPct"`
	SupplyTaxTDSApplies bool `json:"supplyTaxTdsApplies"`

	VariationLimitPct Pct `json:"variationLimitPct"`
	PaymentDays       int `json:"paymentDays"`
}

// DefaultTerms returns the indicative terms for a market. They are a starting
// point to be checked against the contract, never a statement of what it says.
func DefaultTerms(m Market) Terms {
	t := Terms{
		Market:                  m,
		DefectLiabilityMonths:   DefaultDefectLiabilityMonths,
		RecoveryMethod:          RecoveryPercentOfBill,
		AdvanceRecoveryPct:      DefaultAdvanceRecoveryPct,
		AdvanceRecoveryStartPct: DefaultAdvanceRecoveryStartPct,
		AdvanceRecoveryEndPct:   DefaultAdvanceRecoveryEndPct,
		IncomeTaxTDSOn:          TDSOnValueOfWork,
		VariationLimitPct:       DefaultVariationLimitPct,
		PaymentDays:             DefaultPaymentDays,
	}
	switch m {
	case MarketPH:
		t.RetentionPct = DefaultRetentionPctPH
		t.RetentionCapPct = DefaultRetentionCapPctPH
		t.SupplyTaxPct = DefaultVATPctPH
		t.IncomeTaxTDSPct = EWTContractorPctPH
		t.SupplyTaxTDSPct = WithholdingVATGovtPctPH
	default:
		t.Market = MarketIN
		t.RetentionPct = DefaultRetentionPctIN
		t.RetentionCapPct = DefaultRetentionCapPctIN
		t.SupplyTaxPct = DefaultGSTWorksContractPct
		t.IncomeTaxTDSPct = TDS194COtherPct
		t.SupplyTaxTDSPct = GSTTDSPct
	}
	return t
}

// IsIndicativeDefault reports whether every rate in the terms is still the one
// this tool shipped with, so the interface can say so plainly rather than let a
// user assume the figures were checked against their contract.
func (t Terms) IsIndicativeDefault() bool {
	d := DefaultTerms(t.Market)
	d.ContractValue = t.ContractValue
	d.AdvancePaid = t.AdvancePaid
	d.SupplyTaxTDSApplies = t.SupplyTaxTDSApplies
	return t.Normalise() == d.Normalise()
}

// Normalise fills in anything missing and squares away values that make no
// sense, so the rest of the package can take the terms as read.
func (t Terms) Normalise() Terms {
	if t.Market != MarketPH {
		t.Market = MarketIN
	}
	if t.RecoveryMethod != RecoveryProRata {
		t.RecoveryMethod = RecoveryPercentOfBill
	}
	if t.IncomeTaxTDSOn != TDSOnValueLessAdvanceRecovery {
		t.IncomeTaxTDSOn = TDSOnValueOfWork
	}
	if t.DefectLiabilityMonths < 0 {
		t.DefectLiabilityMonths = 0
	}
	if t.PaymentDays < 0 {
		t.PaymentDays = 0
	}
	if t.ContractValue < 0 {
		t.ContractValue = 0
	}
	if t.AdvancePaid < 0 {
		t.AdvancePaid = 0
	}
	for _, p := range []*Pct{
		&t.RetentionPct, &t.RetentionCapPct, &t.AdvanceRecoveryPct,
		&t.AdvanceRecoveryStartPct, &t.AdvanceRecoveryEndPct,
		&t.SupplyTaxPct, &t.IncomeTaxTDSPct, &t.SupplyTaxTDSPct, &t.VariationLimitPct,
	} {
		if *p < 0 {
			*p = 0
		}
	}
	if t.AdvanceRecoveryEndPct < t.AdvanceRecoveryStartPct {
		t.AdvanceRecoveryStartPct, t.AdvanceRecoveryEndPct = t.AdvanceRecoveryEndPct, t.AdvanceRecoveryStartPct
	}
	return t
}

// Validate reports every term that cannot be used, as one error listing them
// all so a form can show them together.
func (t Terms) Validate() error {
	var problems []string

	if t.RetentionPct > pctScale {
		problems = append(problems, "retention cannot be more than 100% of the value of work done")
	}
	if t.RetentionCapPct > pctScale {
		problems = append(problems, "the retention ceiling cannot be more than 100% of the contract value")
	}
	if t.AdvanceRecoveryPct > pctScale {
		problems = append(problems, "advance recovery cannot be more than 100% of the value of work done")
	}
	if t.SupplyTaxPct > pctScale {
		problems = append(problems, "tax on the supply cannot be more than 100%")
	}
	if t.IncomeTaxTDSPct > pctScale {
		problems = append(problems, "income-tax deduction at source cannot be more than 100%")
	}
	if t.SupplyTaxTDSPct > pctScale {
		problems = append(problems, "deduction out of the tax on the supply cannot be more than 100%")
	}
	if t.RecoveryMethod == RecoveryProRata && t.AdvanceRecoveryEndPct == t.AdvanceRecoveryStartPct && t.AdvancePaid > 0 {
		problems = append(problems, "pro-rata recovery needs a start and a finish that are not the same point of progress")
	}
	if t.AdvancePaid > 0 && t.ContractValue > 0 && t.AdvancePaid > t.ContractValue {
		problems = append(problems, "the advance paid cannot be more than the whole contract value")
	}
	if t.DefectLiabilityMonths > 600 {
		problems = append(problems, "a defect-liability period of more than fifty years is not a contract term")
	}
	if t.PaymentDays > 3650 {
		problems = append(problems, "a payment period of more than ten years is not a contract term")
	}

	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}

// Regulation names the rules the deductions in these terms answer to, for the
// footnote a printed bill carries.
func (t Terms) Regulation() string {
	if t.Market == MarketPH {
		return "RA 9184 Revised IRR (retention, advance payment) · BIR RR 2-98 as amended (2% EWT) · NIRC s.108 and s.114(C) (VAT)"
	}
	return "Income-tax Act s.194C (TDS) · CGST Act s.51 (GST TDS) · contract retention and defect-liability clauses"
}
