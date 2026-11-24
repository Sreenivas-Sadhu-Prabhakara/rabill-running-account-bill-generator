package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sreenivas-sadhu-prabhakara/rabill-running-account-bill-generator/internal/rabill"
)

func rupees(major, paise int64) rabill.Money { return rabill.Money(major*100 + paise) }
func units(n int64) rabill.Qty               { return rabill.QtyFromUnits(n) }

// newStore opens an empty contract in a directory of its own.
func newStore(t *testing.T) *store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "contract.json")
	s, err := openStore(path)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	return s
}

// seeded is a store with a small contract and its terms already in it.
func seeded(t *testing.T) *store {
	t.Helper()
	s := newStore(t)

	terms := rabill.DefaultTerms(rabill.MarketIN)
	terms.ContractValue = rupees(1000000, 0)
	terms.AdvancePaid = rupees(100000, 0)
	if _, err := s.putTerms(terms); err != nil {
		t.Fatalf("putTerms: %v", err)
	}

	for _, item := range []rabill.Item{
		{Code: "1.1", Part: "Earthwork", Description: "Excavation in foundation", Unit: "cum", ContractQty: units(400), Rate: rupees(285, 0)},
		{Code: "2.1", Part: "Concrete", Description: "Plain cement concrete 1:4:8", Unit: "cum", ContractQty: units(60), Rate: rupees(5400, 0)},
		{Code: "3.1", Part: "Masonry", Description: "Brickwork 230 thick", Unit: "sqm", ContractQty: units(800), Rate: rupees(1150, 0)},
	} {
		if _, err := s.putItem(item, ""); err != nil {
			t.Fatalf("putItem %s: %v", item.Code, err)
		}
	}
	return s
}

func measure(t *testing.T, s *store, code string, nos, l, b, d float64, deduct bool) rabill.Measurement {
	t.Helper()
	m, err := s.addMeasurement(rabill.Measurement{
		ItemCode: code, Where: "test", Nos: nos, Length: l, Breadth: b, Depth: d, Deduct: deduct,
		Date: rabill.NewDate(2026, time.April, 10),
	})
	if err != nil {
		t.Fatalf("addMeasurement %s: %v", code, err)
	}
	return m
}

func TestOpenStoreStartsEmptyAndWritesNothingUntilAskedTo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contract.json")

	s, err := openStore(path)
	if err != nil {
		t.Fatalf("openStore: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("opening a contract that does not exist should not create a file")
	}

	data := s.snapshot()
	if data.Terms.Market != rabill.MarketIN {
		t.Errorf("a new contract starts in market %q, want India as the default", data.Terms.Market)
	}
	if !data.Terms.IsIndicativeDefault() {
		t.Error("a new contract starts on the indicative defaults")
	}
	if data.nextBillNo() != 1 {
		t.Errorf("a new contract's first bill is numbered %d", data.nextBillNo())
	}
	if data.closed() {
		t.Error("a new contract is not closed")
	}
	if data.lastBill() != nil {
		t.Error("a new contract has no last bill")
	}

	if _, err := s.putProject(project{Work: "Boundary wall"}); err != nil {
		t.Fatalf("putProject: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("the file should exist once something is saved: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the contract file is mode %v, want 0600 — it is the contractor's own rates", mode)
	}
}

func TestOpenStoreReadsBackWhatWasWritten(t *testing.T) {
	s := seeded(t)
	measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	before := s.snapshot()

	again, err := openStore(s.path)
	if err != nil {
		t.Fatalf("reopening the file: %v", err)
	}
	after := again.snapshot()

	if len(after.BOQ) != len(before.BOQ) || len(after.Bills) != len(before.Bills) {
		t.Fatalf("read back %d items and %d bills, want %d and %d", len(after.BOQ), len(after.Bills), len(before.BOQ), len(before.Bills))
	}
	if after.Bills[0].Cumulative != before.Bills[0].Cumulative {
		t.Error("the certified bill came back with different figures")
	}
	if after.Bills[0].ThisBill != before.Bills[0].ThisBill {
		t.Error("the certified bill came back claiming something else")
	}
	if after.Terms != before.Terms {
		t.Error("the terms came back changed")
	}
}

func TestOpenStoreRefusesAFileItCannotTrust(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "not JSON at all", body: "{nonsense", want: "move the file aside"},
		{name: "a newer format", body: `{"version":99}`, want: "newer version of this tool"},
		{
			name: "bills that do not add up",
			body: `{"version":1,"bills":[
				{"no":1,"cumulative":{"valueOfWork":1000,"netPayable":1000},"thisBill":{"valueOfWork":1000,"netPayable":9999}}
			]}`,
			want: "do not form a running account",
		},
		{
			name: "a gap in the bill numbers",
			body: `{"version":1,"bills":[
				{"no":1,"cumulative":{"valueOfWork":1000,"netPayable":1000},"thisBill":{"valueOfWork":1000,"netPayable":1000}},
				{"no":3,"cumulative":{"valueOfWork":2000,"netPayable":2000},"thisBill":{"valueOfWork":1000,"netPayable":1000}}
			]}`,
			want: "without a gap",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "contract.json")
			if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
				t.Fatalf("writing the file: %v", err)
			}
			_, err := openStore(path)
			if err == nil {
				t.Fatalf("openStore accepted the file, want an error about %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("openStore: %v\nwant it to mention %q", err, c.want)
			}
		})
	}

	// An empty file is a contract not yet started, not a corrupt one.
	path := filepath.Join(t.TempDir(), "contract.json")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("writing the file: %v", err)
	}
	if _, err := openStore(path); err != nil {
		t.Errorf("an empty file should start an empty contract, got %v", err)
	}
}

func TestCertifyFreezesTheBillAndTakesUpTheMeasurements(t *testing.T) {
	s := seeded(t)
	measure(t, s, "1.1", 1, 20, 3, 1.5, false) // 90 cum
	measure(t, s, "2.1", 1, 20, 3, 0.1, false) // 6 cum

	before := s.snapshot()
	if before.Measurements[0].BillNo != 0 {
		t.Fatal("a measurement starts in the open bill")
	}
	open, err := before.openBill()
	if err != nil {
		t.Fatalf("openBill: %v", err)
	}

	bill, err := s.certify()
	if err != nil {
		t.Fatalf("certify: %v", err)
	}
	if bill.No != 1 {
		t.Errorf("certified bill %d, want 1", bill.No)
	}
	if bill.Cumulative != open.Cumulative {
		t.Error("certifying changed the figures that were on screen")
	}

	after := s.snapshot()
	for _, m := range after.Measurements {
		if m.BillNo != 1 {
			t.Errorf("measurement %s is still in the open bill after certification", m.ID)
		}
	}
	if after.nextBillNo() != 2 {
		t.Errorf("the next bill is numbered %d, want 2", after.nextBillNo())
	}

	// The next bill starts empty, carrying forward the cumulative figures the
	// user entered so they are not re-entered or lost.
	if after.Draft.OtherRecoveries != bill.Cumulative.OtherRecoveries {
		t.Error("the next bill should start from the recoveries already made")
	}
	next, err := after.openBill()
	if err != nil {
		t.Fatalf("the next open bill: %v", err)
	}
	if next.ThisBill != (rabill.Ledger{}) {
		t.Errorf("the next bill claims %+v before anything is measured", next.ThisBill)
	}
	if next.Previous != bill.Cumulative {
		t.Error("the next bill's previous column must be the certified bill's cumulative column")
	}
}

func TestCertifyRefusesWhatCannotBeCertified(t *testing.T) {
	s := seeded(t)

	if _, err := s.certify(); err == nil {
		t.Error("certifying with nothing measured should be refused")
	} else if !strings.Contains(err.Error(), "nothing to bill") {
		t.Errorf("certify: %v", err)
	}

	measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	draftWithFinal := s.snapshot().Draft
	draftWithFinal.Final = true
	if _, err := s.putDraft(draftWithFinal); err != nil {
		t.Fatalf("putDraft: %v", err)
	}
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	// The account is closed, and everything that would add to it is refused.
	if _, err := s.certify(); err == nil || !strings.Contains(err.Error(), "account is closed") {
		t.Errorf("certifying after the final bill: %v", err)
	}
	if _, err := s.addMeasurement(rabill.Measurement{ItemCode: "1.1", Nos: 1}); err == nil || !strings.Contains(err.Error(), "account is closed") {
		t.Errorf("measuring after the final bill: %v", err)
	}
	if _, err := s.putDraft(draft{Date: rabill.Today()}); err == nil || !strings.Contains(err.Error(), "account is closed") {
		t.Errorf("changing the draft after the final bill: %v", err)
	}

	// And the open bill is no longer offered at all.
	if s.snapshot().closed() != true {
		t.Error("the account should read as closed")
	}
}

func TestReopenTakesTheLastBillBack(t *testing.T) {
	s := seeded(t)

	if _, err := s.reopen(); err == nil || !strings.Contains(err.Error(), "no bill has been certified") {
		t.Errorf("reopening an account with no bills: %v", err)
	}

	first := measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify 1: %v", err)
	}
	second := measure(t, s, "2.1", 1, 20, 3, 0.1, false)
	certified, err := s.certify()
	if err != nil {
		t.Fatalf("certify 2: %v", err)
	}

	no, err := s.reopen()
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if no != 2 {
		t.Errorf("reopened bill %d, want 2", no)
	}

	data := s.snapshot()
	if len(data.Bills) != 1 {
		t.Fatalf("%d bills remain, want 1", len(data.Bills))
	}
	// The reopened bill's own measurements come back into the open bill; the
	// first bill's stay where they are.
	for _, m := range data.Measurements {
		switch m.ID {
		case first.ID:
			if m.BillNo != 1 {
				t.Errorf("the first bill's measurement moved to bill %d", m.BillNo)
			}
		case second.ID:
			if m.BillNo != 0 {
				t.Errorf("the reopened bill's measurement is still in bill %d", m.BillNo)
			}
		}
	}
	// What was certified comes back as the draft, so nothing has to be retyped.
	if data.Draft.Date != certified.Date || data.Draft.Final != certified.Final {
		t.Errorf("the draft came back as %+v", data.Draft)
	}

	// Drawing the bill up again gives the same figures it was certified at.
	again, err := data.openBill()
	if err != nil {
		t.Fatalf("openBill: %v", err)
	}
	if again.Cumulative != certified.Cumulative || again.ThisBill != certified.ThisBill {
		t.Error("reopening and drawing the bill up again changed its figures")
	}
}

func TestACertifiedMeasurementIsNotErased(t *testing.T) {
	s := seeded(t)
	m := measure(t, s, "1.1", 1, 20, 3, 1.5, false)

	// While the bill is open it can be removed.
	if err := s.deleteMeasurement(m.ID); err != nil {
		t.Fatalf("deleteMeasurement on an open bill: %v", err)
	}
	if len(s.snapshot().Measurements) != 0 {
		t.Error("the measurement was not removed")
	}

	m = measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	err := s.deleteMeasurement(m.ID)
	if err == nil {
		t.Fatal("a certified measurement should not be erasable")
	}
	for _, want := range []string{"RA bill 1", "deduction entry"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("deleteMeasurement: %v\nwant it to mention %q", err, want)
		}
	}

	if err := s.deleteMeasurement("nothing"); err == nil {
		t.Error("removing a measurement that is not there should be refused")
	}
}

func TestAMeasurementNeedsAnItemToBeMeasuredAgainst(t *testing.T) {
	s := seeded(t)

	_, err := s.addMeasurement(rabill.Measurement{ItemCode: "9.9", Nos: 1})
	if err == nil {
		t.Fatal("measuring against an item that is not in the contract should be refused")
	}
	if !strings.Contains(err.Error(), "add the item first") {
		t.Errorf("addMeasurement: %v", err)
	}

	// An entry with nothing measured is refused before it reaches the book.
	if _, err := s.addMeasurement(rabill.Measurement{ItemCode: "1.1"}); err == nil {
		t.Error("an entry with no dimensions should be refused")
	}

	// A repeated identity is refused rather than overwriting an entry.
	m := measure(t, s, "1.1", 1, 2, 2, 2, false)
	if _, err := s.addMeasurement(rabill.Measurement{ID: m.ID, ItemCode: "1.1", Nos: 1}); err == nil {
		t.Error("a repeated identity should be refused")
	}

	// An entry with no date is dated today rather than left blank.
	dated, err := s.addMeasurement(rabill.Measurement{ItemCode: "1.1", Nos: 5})
	if err != nil {
		t.Fatalf("addMeasurement: %v", err)
	}
	if dated.Date != rabill.Today() {
		t.Errorf("an undated entry was dated %s, want today", dated.Date)
	}
	if dated.ID == "" {
		t.Error("an entry should be given an identity of its own")
	}
}

func TestAnItemThatHasBeenBilledIsProtected(t *testing.T) {
	s := seeded(t)
	measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	billed, _ := s.snapshot().BOQ.Find("1.1")

	// Its rate may be corrected: the difference belongs in the next bill.
	corrected := billed
	corrected.Rate = rupees(295, 0)
	if _, err := s.putItem(corrected, ""); err != nil {
		t.Fatalf("correcting a rate should be allowed: %v", err)
	}
	if got, _ := s.snapshot().BOQ.Find("1.1"); got.Rate != rupees(295, 0) {
		t.Errorf("the rate reads %s", got.Rate)
	}

	// Its number may not: the certified bill refers to it.
	renumbered := corrected
	renumbered.Code = "1.9"
	_, err := s.putItem(renumbered, "1.1")
	if err == nil || !strings.Contains(err.Error(), "number cannot be changed") {
		t.Errorf("renumbering a billed item: %v", err)
	}

	// Nor may its unit: the quantities already certified are in the old one.
	reunited := corrected
	reunited.Unit = "sqm"
	_, err = s.putItem(reunited, "")
	if err == nil || !strings.Contains(err.Error(), "unit cannot be changed") {
		t.Errorf("changing a billed item's unit: %v", err)
	}

	// And it cannot be removed at all.
	err = s.deleteItem("1.1")
	if err == nil || !strings.Contains(err.Error(), "stays in the account") {
		t.Errorf("removing a billed item: %v", err)
	}
}

func TestAnItemNotYetBilledCanBeChangedFreely(t *testing.T) {
	s := seeded(t)

	item, _ := s.snapshot().BOQ.Find("3.1")
	renumbered := item
	renumbered.Code = "3.2"
	renumbered.Unit = "cum"
	if _, err := s.putItem(renumbered, "3.1"); err != nil {
		t.Fatalf("renumbering an unbilled item: %v", err)
	}
	if _, found := s.snapshot().BOQ.Find("3.1"); found {
		t.Error("the old number is still there")
	}
	if _, found := s.snapshot().BOQ.Find("3.2"); !found {
		t.Error("the new number is not there")
	}

	// Measurements follow the item to its new number rather than being orphaned.
	measure(t, s, "3.2", 1, 10, 2, 0, false)
	again := s.snapshot()
	item, _ = again.BOQ.Find("3.2")
	renumbered = item
	renumbered.Code = "3.3"
	if _, err := s.putItem(renumbered, "3.2"); err != nil {
		t.Fatalf("renumbering again: %v", err)
	}
	for _, m := range s.snapshot().Measurements {
		if m.Key() != "3.3" {
			t.Errorf("a measurement was left pointing at item %s", m.ItemCode)
		}
	}

	// An item with measurements against it cannot be removed until they are.
	err := s.deleteItem("3.3")
	if err == nil || !strings.Contains(err.Error(), "measurement(s) against it") {
		t.Errorf("removing an item with measurements: %v", err)
	}

	if err := s.deleteItem("2.1"); err != nil {
		t.Fatalf("removing an untouched item: %v", err)
	}
	if err := s.deleteItem("2.1"); err == nil {
		t.Error("removing an item twice should be refused")
	}
}

func TestPutItemRefusesAClashingNumber(t *testing.T) {
	s := seeded(t)

	_, err := s.putItem(rabill.Item{Code: "1.1", Description: "Something else", Unit: "cum", Rate: 100}, "2.1")
	if err == nil || !strings.Contains(err.Error(), "already in the bill of quantities") {
		t.Errorf("renumbering onto another item's number: %v", err)
	}

	// Adding a brand-new item whose number is taken is the same clash.
	_, err = s.putItem(rabill.Item{Code: "1.1", Description: "Duplicate", Unit: "cum", Rate: 100}, "nothing-here")
	if err == nil || !strings.Contains(err.Error(), "already in the bill of quantities") {
		t.Errorf("adding a duplicate number: %v", err)
	}
}

func TestPutTermsValidatesBeforeSaving(t *testing.T) {
	s := seeded(t)
	before := s.snapshot().Terms

	bad := before
	bad.RetentionPct = rabill.PctFromPercent(150)
	if _, err := s.putTerms(bad); err == nil {
		t.Fatal("terms that cannot be used should be refused")
	}
	if s.snapshot().Terms != before {
		t.Error("refused terms were saved anyway")
	}

	good := before
	good.RetentionPct = rabill.PctFromPercent(7)
	saved, err := s.putTerms(good)
	if err != nil {
		t.Fatalf("putTerms: %v", err)
	}
	if saved.RetentionPct != rabill.PctFromPercent(7) || s.snapshot().Terms.RetentionPct != rabill.PctFromPercent(7) {
		t.Error("the new retention rate was not saved")
	}
}

func TestPutDraftRefusesFiguresThatMakeNoSense(t *testing.T) {
	s := seeded(t)

	for _, c := range []struct {
		name string
		in   draft
		want string
	}{
		{name: "a negative recovery", in: draft{OtherRecoveries: -100}, want: "cannot be below zero"},
		{name: "negative retention released", in: draft{RetentionReleased: -100}, want: "cannot be below zero"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := s.putDraft(c.in); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("putDraft: %v, want an error about %q", err, c.want)
			}
		})
	}

	saved, err := s.putDraft(draft{
		Date: rabill.NewDate(2026, time.April, 30), PeriodTo: rabill.NewDate(2026, time.April, 30),
		OtherRecoveries: rupees(1500, 0), Note: "  first bill  ",
	})
	if err != nil {
		t.Fatalf("putDraft: %v", err)
	}
	if saved.Note != "first bill" {
		t.Errorf("the note was saved as %q", saved.Note)
	}
	if s.snapshot().Draft.OtherRecoveries != rupees(1500, 0) {
		t.Error("the recovery was not saved")
	}
}

func TestContractValueFallsBackToTheBillOfQuantities(t *testing.T) {
	s := seeded(t)

	terms := s.snapshot().Terms
	terms.ContractValue = 0
	terms.AdvancePaid = 0
	if _, err := s.putTerms(terms); err != nil {
		t.Fatalf("putTerms: %v", err)
	}

	// 400 × 285 + 60 × 5,400 + 800 × 1,150 = 114,000 + 324,000 + 920,000.
	want := rupees(1358000, 0)
	if got := s.snapshot().contractValue(); got != want {
		t.Errorf("the contract value reads %s, want %s from the bill of quantities", got, want)
	}

	terms.ContractValue = rupees(1400000, 0)
	if _, err := s.putTerms(terms); err != nil {
		t.Fatalf("putTerms: %v", err)
	}
	if got := s.snapshot().contractValue(); got != rupees(1400000, 0) {
		t.Errorf("the contract value reads %s, want the figure entered", got)
	}
}

func TestImportBOQAddsUpdatesAndReportsByLine(t *testing.T) {
	s := seeded(t)
	measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	rows := [][]string{
		{"item", "description", "unit", "contract_qty", "rate"},
		{"1.1", "Excavation in foundation", "cum", "450", "290.00"}, // already billed, same unit
		{"2.1", "Plain cement concrete 1:4:8", "sqm", "60", "5400"}, // unit change on an unbilled item
		{"4.1", "Cement plaster 12 mm", "sqm", "1500", "285"},       // new
		{"5.1", "", "sqm", "100", "100"},                            // no description
	}
	parsed, rowErrors := rabill.ParseItemCSV(rows)

	result, err := s.importBOQ(parsed, rowErrors)
	if err != nil {
		t.Fatalf("importBOQ: %v", err)
	}
	// Item 1.1 is updated although it has been billed, because only its rate
	// and quantity move; item 2.1 is updated including its unit, because it has
	// not been billed; item 4.1 is new.
	if result.Added != 1 || result.Updated != 2 || result.Skipped != 0 {
		t.Errorf("import added %d, updated %d, skipped %d; want 1, 2, 0", result.Added, result.Updated, result.Skipped)
	}
	// One row could not be used at all: the one with no description, which the
	// parser refuses before the import sees it.
	if len(result.Errors) != 1 || result.Errors[0].Line != 5 {
		t.Errorf("import reported %+v, want one problem on line 5", result.Errors)
	}

	data := s.snapshot()
	if got, _ := data.BOQ.Find("1.1"); got.Rate != rupees(290, 0) || got.ContractQty != units(450) {
		t.Errorf("item 1.1 came in as %s at %s", got.ContractQty, got.Rate)
	}
	if got, _ := data.BOQ.Find("2.1"); got.Unit != "sqm" {
		t.Errorf("item 2.1's unit is %s, want sqm — it has not been billed", got.Unit)
	}
	if _, found := data.BOQ.Find("4.1"); !found {
		t.Error("the new item did not come in")
	}
}

func TestImportBOQWillNotChangeTheUnitOfABilledItem(t *testing.T) {
	s := seeded(t)
	measure(t, s, "1.1", 1, 20, 3, 1.5, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	rows := [][]string{
		{"item", "description", "unit", "contract_qty", "rate"},
		{"1.1", "Excavation in foundation", "sqm", "450", "290.00"},
	}
	parsed, rowErrors := rabill.ParseItemCSV(rows)

	result, err := s.importBOQ(parsed, rowErrors)
	if err != nil {
		t.Fatalf("importBOQ: %v", err)
	}
	if result.Skipped != 1 || result.Updated != 0 {
		t.Errorf("import updated %d and skipped %d, want 0 and 1", result.Updated, result.Skipped)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Message, "unit cannot be changed") {
		t.Errorf("import reported %+v", result.Errors)
	}
	if got, _ := s.snapshot().BOQ.Find("1.1"); got.Unit != "cum" {
		t.Errorf("item 1.1's unit became %s", got.Unit)
	}
}

func TestAChangeThatFailsIsRolledBackWholly(t *testing.T) {
	s := seeded(t)
	before := s.snapshot()

	err := s.change(func(d *fileData) error {
		d.BOQ = append(d.BOQ, rabill.Item{Code: "9.9", Description: "Something", Unit: "nos", Rate: 100})
		d.Project.Work = "changed"
		return errTest
	})
	if err != errTest {
		t.Fatalf("change returned %v, want the error the change gave", err)
	}

	after := s.snapshot()
	if len(after.BOQ) != len(before.BOQ) {
		t.Errorf("the bill of quantities has %d items, want the %d it had", len(after.BOQ), len(before.BOQ))
	}
	if after.Project.Work != before.Project.Work {
		t.Errorf("the heading reads %q, want %q", after.Project.Work, before.Project.Work)
	}
}

var errTest = errTestType("a change that fails")

type errTestType string

func (e errTestType) Error() string { return string(e) }

func TestTheSavedFileIsPlainReadableJSON(t *testing.T) {
	s := seeded(t)
	measure(t, s, "1.1", 4, 3.25, 0.23, 0.15, false)
	if _, err := s.certify(); err != nil {
		t.Fatalf("certify: %v", err)
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		t.Fatalf("reading the file: %v", err)
	}
	if !strings.Contains(string(raw), "\n  \"version\": 1") {
		t.Error("the file should be indented so a contractor can read it")
	}

	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatalf("the file is not JSON: %v", err)
	}
	for _, key := range []string{"version", "project", "terms", "boq", "measurements", "bills", "draft"} {
		if _, found := generic[key]; !found {
			t.Errorf("the file has no %q", key)
		}
	}

	// Amounts are written as plain decimal numbers, not as scaled integers, so
	// the file reads the way the bill does.
	if !strings.Contains(string(raw), `"rate": 285.00`) {
		t.Error("a rate should be written as 285.00")
	}
}
