package rabill

import (
	"strings"
	"testing"
)

func TestDefaultTermsAreTheFiguresDocumented(t *testing.T) {
	in := DefaultTerms(MarketIN)
	for _, c := range []struct {
		name      string
		got, want Pct
	}{
		{"retention", in.RetentionPct, DefaultRetentionPctIN},
		{"retention ceiling", in.RetentionCapPct, DefaultRetentionCapPctIN},
		{"advance recovery", in.AdvanceRecoveryPct, DefaultAdvanceRecoveryPct},
		{"GST on a works contract", in.SupplyTaxPct, DefaultGSTWorksContractPct},
		{"TDS under s.194C", in.IncomeTaxTDSPct, TDS194COtherPct},
		{"GST TDS under s.51", in.SupplyTaxTDSPct, GSTTDSPct},
		{"deviation limit", in.VariationLimitPct, DefaultVariationLimitPct},
	} {
		if c.got != c.want {
			t.Errorf("India: %s = %s%%, want %s%%", c.name, c.got, c.want)
		}
	}
	if in.Market != MarketIN || in.Market.Currency() != "INR" {
		t.Errorf("India: market %q, currency %q", in.Market, in.Market.Currency())
	}
	if in.DefectLiabilityMonths != DefaultDefectLiabilityMonths || in.PaymentDays != DefaultPaymentDays {
		t.Errorf("India: defect liability %d months, payment %d days", in.DefectLiabilityMonths, in.PaymentDays)
	}
	if in.RecoveryMethod != RecoveryPercentOfBill || in.IncomeTaxTDSOn != TDSOnValueOfWork {
		t.Errorf("India: recovery %q on base %q", in.RecoveryMethod, in.IncomeTaxTDSOn)
	}
	// A deduction out of the tax on the supply applies only to some clients,
	// so it must be off until it is switched on deliberately.
	if in.SupplyTaxTDSApplies {
		t.Error("the deduction out of GST should be off by default")
	}

	ph := DefaultTerms(MarketPH)
	for _, c := range []struct {
		name      string
		got, want Pct
	}{
		{"retention", ph.RetentionPct, DefaultRetentionPctPH},
		{"retention ceiling", ph.RetentionCapPct, DefaultRetentionCapPctPH},
		{"VAT", ph.SupplyTaxPct, DefaultVATPctPH},
		{"expanded withholding tax", ph.IncomeTaxTDSPct, EWTContractorPctPH},
		{"withholding VAT", ph.SupplyTaxTDSPct, WithholdingVATGovtPctPH},
	} {
		if c.got != c.want {
			t.Errorf("Philippines: %s = %s%%, want %s%%", c.name, c.got, c.want)
		}
	}
	if ph.Market.Currency() != "PHP" {
		t.Errorf("Philippines: currency %q", ph.Market.Currency())
	}

	// The named constants themselves, so that changing one is a deliberate act
	// that shows up here.
	for _, c := range []struct {
		name      string
		got, want Pct
	}{
		{"s.194C, individual or HUF payee", TDS194CIndividualHUFPct, 100},
		{"s.194C, other payee", TDS194COtherPct, 200},
		{"s.206AA, no PAN", TDS206AANoPANPct, 2000},
		{"s.51 GST TDS", GSTTDSPct, 200},
		{"EWT on contractors", EWTContractorPctPH, 200},
		{"VAT", DefaultVATPctPH, 1200},
		{"withholding VAT on government payors", WithholdingVATGovtPctPH, 500},
		{"retention released against completion", DefaultFinalRetentionReleasePct, 5000},
		{"advance, India", DefaultAdvancePctIN, 1000},
		{"advance, Philippines", DefaultAdvancePctPH, 1500},
	} {
		if c.got != c.want {
			t.Errorf("%s = %s%%, want %s%%", c.name, c.got, c.want)
		}
	}
	if GSTTDSContractThresholdINR != 250000 {
		t.Errorf("the s.51 contract threshold is %d, want 250000", GSTTDSContractThresholdINR)
	}
	if TDS194CSinglePaymentINR != 30000 || TDS194CAnnualINR != 100000 {
		t.Errorf("the s.194C thresholds are %d and %d", TDS194CSinglePaymentINR, TDS194CAnnualINR)
	}
}

func TestTermsNormalise(t *testing.T) {
	odd := Terms{
		Market:                  "ZZ",
		RecoveryMethod:          "guesswork",
		IncomeTaxTDSOn:          "whatever",
		DefectLiabilityMonths:   -3,
		PaymentDays:             -1,
		ContractValue:           -100,
		AdvancePaid:             -100,
		RetentionPct:            -500,
		AdvanceRecoveryStartPct: PctFromPercent(80),
		AdvanceRecoveryEndPct:   PctFromPercent(20),
	}
	got := odd.Normalise()

	if got.Market != MarketIN {
		t.Errorf("an unknown market became %q, want India as the fallback", got.Market)
	}
	if got.RecoveryMethod != RecoveryPercentOfBill {
		t.Errorf("an unknown recovery method became %q", got.RecoveryMethod)
	}
	if got.IncomeTaxTDSOn != TDSOnValueOfWork {
		t.Errorf("an unknown deduction base became %q", got.IncomeTaxTDSOn)
	}
	for name, v := range map[string]int64{
		"defect liability": int64(got.DefectLiabilityMonths),
		"payment days":     int64(got.PaymentDays),
		"contract value":   int64(got.ContractValue),
		"advance paid":     int64(got.AdvancePaid),
		"retention":        int64(got.RetentionPct),
	} {
		if v < 0 {
			t.Errorf("%s stayed negative at %d", name, v)
		}
	}
	// A start and finish entered the wrong way round are put right rather than
	// quietly recovering nothing.
	if got.AdvanceRecoveryStartPct != PctFromPercent(20) || got.AdvanceRecoveryEndPct != PctFromPercent(80) {
		t.Errorf("pro-rata recovery runs from %s%% to %s%%, want 20%% to 80%%", got.AdvanceRecoveryStartPct, got.AdvanceRecoveryEndPct)
	}

	// Normalising twice changes nothing more.
	if again := got.Normalise(); again != got {
		t.Error("normalising an already-normalised set of terms changed it")
	}
}

func TestTermsValidate(t *testing.T) {
	sound := DefaultTerms(MarketIN)
	sound.ContractValue = rupees(1000000, 0)
	sound.AdvancePaid = rupees(100000, 0)
	if err := sound.Validate(); err != nil {
		t.Fatalf("sound terms: %v", err)
	}

	cases := []struct {
		name  string
		spoil func(Terms) Terms
		want  string
	}{
		{
			name:  "retention of more than everything",
			spoil: func(t Terms) Terms { t.RetentionPct = PctFromPercent(101); return t },
			want:  "retention cannot be more than 100%",
		},
		{
			name:  "a ceiling of more than the whole contract",
			spoil: func(t Terms) Terms { t.RetentionCapPct = PctFromPercent(150); return t },
			want:  "ceiling cannot be more than 100%",
		},
		{
			name:  "recovering more than the bill",
			spoil: func(t Terms) Terms { t.AdvanceRecoveryPct = PctFromPercent(120); return t },
			want:  "advance recovery cannot be more than 100%",
		},
		{
			name:  "tax of more than everything",
			spoil: func(t Terms) Terms { t.SupplyTaxPct = PctFromPercent(120); return t },
			want:  "tax on the supply cannot be more than 100%",
		},
		{
			name:  "deducting more than everything",
			spoil: func(t Terms) Terms { t.IncomeTaxTDSPct = PctFromPercent(120); return t },
			want:  "income-tax deduction at source cannot be more than 100%",
		},
		{
			name: "pro-rata recovery with no span to recover over",
			spoil: func(t Terms) Terms {
				t.RecoveryMethod = RecoveryProRata
				t.AdvanceRecoveryStartPct = PctFromPercent(50)
				t.AdvanceRecoveryEndPct = PctFromPercent(50)
				return t
			},
			want: "not the same point of progress",
		},
		{
			name:  "an advance larger than the contract",
			spoil: func(t Terms) Terms { t.AdvancePaid = rupees(2000000, 0); return t },
			want:  "more than the whole contract value",
		},
		{
			name:  "a defect-liability period of a lifetime",
			spoil: func(t Terms) Terms { t.DefectLiabilityMonths = 1200; return t },
			want:  "fifty years",
		},
		{
			name:  "a payment period of a lifetime",
			spoil: func(t Terms) Terms { t.PaymentDays = 5000; return t },
			want:  "ten years",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.spoil(sound).Normalise().Validate()
			if err == nil {
				t.Fatalf("Validate() = nil, want an error about %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("Validate() = %q, want it to mention %q", err, c.want)
			}
		})
	}

	// Several faults at once are all reported, so a form can show them together.
	many := sound
	many.RetentionPct = PctFromPercent(150)
	many.SupplyTaxPct = PctFromPercent(150)
	err := many.Validate()
	if err == nil {
		t.Fatal("two faults should be refused")
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) != 2 {
		t.Errorf("two faults reported as %d line(s): %q", len(lines), err)
	}
}

func TestIsIndicativeDefaultIgnoresWhatIsNotARate(t *testing.T) {
	terms := DefaultTerms(MarketIN)
	if !terms.IsIndicativeDefault() {
		t.Fatal("the shipped defaults must read as the shipped defaults")
	}

	// The contract value, the advance paid and whether the client deducts out
	// of the tax are facts about a contract, not rates this tool chose.
	withFacts := terms
	withFacts.ContractValue = rupees(2655500, 0)
	withFacts.AdvancePaid = rupees(265550, 0)
	withFacts.SupplyTaxTDSApplies = true
	if !withFacts.IsIndicativeDefault() {
		t.Error("filling in the contract's own figures is not changing a rate")
	}

	for _, spoil := range []func(Terms) Terms{
		func(t Terms) Terms { t.RetentionPct = PctFromPercent(7); return t },
		func(t Terms) Terms { t.SupplyTaxPct = PctFromPercent(12); return t },
		func(t Terms) Terms { t.DefectLiabilityMonths = 24; return t },
		func(t Terms) Terms { t.PaymentDays = 45; return t },
		func(t Terms) Terms { t.RecoveryMethod = RecoveryProRata; return t },
		func(t Terms) Terms { t.IncomeTaxTDSOn = TDSOnValueLessAdvanceRecovery; return t },
	} {
		if spoil(terms).IsIndicativeDefault() {
			t.Errorf("a changed rate still reads as the shipped default: %+v", spoil(terms))
		}
	}

	// The Philippine defaults are their own defaults, not India's.
	if !DefaultTerms(MarketPH).IsIndicativeDefault() {
		t.Error("the Philippine defaults must read as defaults too")
	}
	crossed := DefaultTerms(MarketPH)
	crossed.Market = MarketIN
	if crossed.IsIndicativeDefault() {
		t.Error("the Philippine rates under an Indian contract are not the Indian defaults")
	}
}

func TestRegulationNamesTheRulesOfTheMarket(t *testing.T) {
	in := DefaultTerms(MarketIN).Regulation()
	for _, want := range []string{"194C", "s.51"} {
		if !strings.Contains(in, want) {
			t.Errorf("the Indian footnote %q does not mention %q", in, want)
		}
	}
	ph := DefaultTerms(MarketPH).Regulation()
	for _, want := range []string{"RA 9184", "RR 2-98", "NIRC"} {
		if !strings.Contains(ph, want) {
			t.Errorf("the Philippine footnote %q does not mention %q", ph, want)
		}
	}
}

func TestMarketSymbols(t *testing.T) {
	if got := MarketIN.Symbol(); got != "₹" {
		t.Errorf("India's symbol is %q", got)
	}
	if got := MarketPH.Symbol(); got != "₱" {
		t.Errorf("the Philippines' symbol is %q", got)
	}
	// An unknown market falls back to India rather than printing nothing.
	if got := Market("ZZ").Symbol(); got != "₹" {
		t.Errorf("an unknown market's symbol is %q", got)
	}
	if got := Market("ZZ").Currency(); got != "INR" {
		t.Errorf("an unknown market's currency is %q", got)
	}
}
