package rabill

import (
	"strings"
	"testing"
)

func TestParseItemCSVTakesColumnsAsTheSpreadsheetNamesThem(t *testing.T) {
	// Headings in a different order, under the names a quantity surveyor's
	// spreadsheet actually uses, with rupee signs and lakh commas in the rates.
	rows := [][]string{
		{"Sl. No.", "Particulars", "Sub-head", "UOM", "Rate per unit", "BOQ Qty", "Extra Item"},
		{"1.1", "Excavation in foundation", "Earthwork", "cum", "₹285.00", "400", ""},
		{"2.1", "Plain cement concrete 1:4:8", "Concrete", "cum", "5,400.00", "60", ""},
		{"9.1", "Rock excavation by chiselling", "Extra items", "cum", "920", "", "yes"},
	}

	parsed, problems := ParseItemCSV(rows)
	if len(problems) != 0 {
		t.Fatalf("problems reading a sound file: %+v", problems)
	}
	if len(parsed) != 3 {
		t.Fatalf("read %d items, want 3", len(parsed))
	}

	first := parsed[0]
	if first.Line != 2 {
		t.Errorf("the first item came from line %d, want 2", first.Line)
	}
	if first.Item.Code != "1.1" || first.Item.Part != "Earthwork" || first.Item.Unit != "cum" {
		t.Errorf("the first item read as %+v", first.Item)
	}
	if first.Item.Rate != rupees(285, 0) || first.Item.ContractQty != units(400) {
		t.Errorf("the first item's rate is %s and quantity %s", first.Item.Rate, first.Item.ContractQty)
	}

	if parsed[1].Item.Rate != rupees(5400, 0) {
		t.Errorf("a rate with lakh commas read as %s", parsed[1].Item.Rate)
	}
	extra := parsed[2].Item
	if !extra.Extra {
		t.Error("an item marked as extra should come in as an extra item")
	}
	if extra.ContractQty != 0 {
		t.Errorf("an extra item with no quantity read as %s", extra.ContractQty)
	}
}

func TestParseItemCSVReportsEveryBadRowAndKeepsTheGoodOnes(t *testing.T) {
	rows := [][]string{
		{"item", "description", "unit", "contract_qty", "rate"},
		{"1.1", "Excavation", "cum", "400", "285.00"},
		{"", "No item number", "cum", "10", "100"},
		{"2.1", "", "cum", "10", "100"},
		{"2.2", "No unit", "", "10", "100"},
		{"2.3", "A rate that is not a number", "cum", "10", "two hundred"},
		{"2.4", "A negative rate", "cum", "10", "-100"},
		{"1.1", "The same item again", "cum", "10", "100"},
		{"", "", "", "", ""},
		{"3.1", "Brickwork", "sqm", "800", "1150"},
	}

	parsed, problems := ParseItemCSV(rows)

	if len(parsed) != 2 {
		t.Fatalf("read %d items, want the 2 sound ones", len(parsed))
	}
	if parsed[0].Item.Code != "1.1" || parsed[1].Item.Code != "3.1" {
		t.Errorf("read items %s and %s", parsed[0].Item.Code, parsed[1].Item.Code)
	}

	if len(problems) != 6 {
		t.Fatalf("reported %d problems, want 6: %+v", len(problems), problems)
	}
	wants := []struct {
		line int
		text string
	}{
		{3, "number from the bill of quantities"},
		{4, "description of the work"},
		{5, "unit of measurement"},
		{6, "not part of a number"},
		{7, "rate cannot be below zero"},
		{8, "already on line 2"},
	}
	for i, want := range wants {
		if problems[i].Line != want.line {
			t.Errorf("problem %d is on line %d, want %d (%+v)", i, problems[i].Line, want.line, problems[i])
		}
		if !strings.Contains(problems[i].Message, want.text) {
			t.Errorf("problem on line %d reads %q, want it to mention %q", problems[i].Line, problems[i].Message, want.text)
		}
	}
}

func TestParseItemCSVRefusesAFileItCannotRead(t *testing.T) {
	cases := []struct {
		name string
		rows [][]string
		want string
	}{
		{name: "an empty file", rows: nil, want: "the file is empty"},
		{
			name: "no item column",
			rows: [][]string{{"thing", "cost"}, {"a", "1"}},
			want: "must be the item number",
		},
		{
			name: "no rate column",
			rows: [][]string{{"item", "description", "unit"}, {"1.1", "Excavation", "cum"}},
			want: "no rate column",
		},
		{
			name: "headings but nothing under them",
			rows: [][]string{{"item", "description", "unit", "rate"}},
			want: "no rows of items",
		},
		{
			name: "headings and only blank rows",
			rows: [][]string{{"item", "description", "unit", "rate"}, {"", "", "", ""}},
			want: "no rows of items",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parsed, problems := ParseItemCSV(c.rows)
			if len(parsed) != 0 {
				t.Fatalf("read %d items from a file that cannot be read", len(parsed))
			}
			if len(problems) == 0 {
				t.Fatalf("no problem reported, want one about %q", c.want)
			}
			if !strings.Contains(problems[0].Message, c.want) {
				t.Errorf("problem reads %q, want it to mention %q", problems[0].Message, c.want)
			}
		})
	}
}

func TestBillOfQuantitiesRoundTripsThroughAFile(t *testing.T) {
	original := append(contractBOQ(), Item{
		Code: "9.1", Part: "Extra items", Description: "Rock excavation",
		Unit: "cum", Rate: rupees(920, 0), Extra: true,
	})

	rows := append([][]string{ItemCSVHeader()}, ItemCSVRows(original)...)
	parsed, problems := ParseItemCSV(rows)
	if len(problems) != 0 {
		t.Fatalf("an exported file did not read back: %+v", problems)
	}
	if len(parsed) != len(original) {
		t.Fatalf("read %d items back, want %d", len(parsed), len(original))
	}
	for i, got := range parsed {
		if got.Item != original[i] {
			t.Errorf("item %d came back as %+v, want %+v", i, got.Item, original[i])
		}
	}
}

func TestHeadingKeyAndTruthy(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Sl. No.", "slno"},
		{"Contract Qty", "contractqty"},
		{"  RATE  ", "rate"},
		{"Unit of Measurement", "unitofmeasurement"},
		{"", ""},
	} {
		if got := headingKey(c.in); got != c.want {
			t.Errorf("headingKey(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	for _, yes := range []string{"yes", "Y", "TRUE", "1", "extra", " x "} {
		if !truthy(yes) {
			t.Errorf("truthy(%q) = false", yes)
		}
	}
	for _, no := range []string{"", "no", "0", "false", "maybe"} {
		if truthy(no) {
			t.Errorf("truthy(%q) = true", no)
		}
	}
}

func TestBillCSVRowsCarryTheThreeColumnsOfARunningAccountBill(t *testing.T) {
	bills := runningAccount(t)
	third := bills[2]

	header := BillCSVHeader()
	rows := BillCSVRows(third)
	if len(rows) != len(third.Items)+1 {
		t.Fatalf("the file has %d rows for %d items plus a total", len(rows), len(third.Items))
	}
	for n, row := range rows {
		if len(row) != len(header) {
			t.Fatalf("row %d has %d cells against %d headings", n, len(row), len(header))
		}
	}

	at := func(name string) int {
		for i, h := range header {
			if h == name {
				return i
			}
		}
		t.Fatalf("the file has no %s column", name)
		return 0
	}

	// Item 2.1 is the one revised downwards; the file must carry the minus
	// figures, not hide them.
	var found bool
	for _, row := range rows {
		if row[at("item")] != "2.1" {
			continue
		}
		found = true
		if row[at("qty_previous")] != "55" || row[at("qty_up_to_date")] != "52" || row[at("qty_this_bill")] != "-3" {
			t.Errorf("item 2.1 reads %s previous, %s this bill, %s up to date",
				row[at("qty_previous")], row[at("qty_this_bill")], row[at("qty_up_to_date")])
		}
		if row[at("amount_this_bill")] != "-16200.00" {
			t.Errorf("item 2.1 claims %s", row[at("amount_this_bill")])
		}
	}
	if !found {
		t.Error("item 2.1 is missing from the file")
	}

	// The last row totals the value of work, and the total must agree with the
	// bill rather than be worked out again.
	total := rows[len(rows)-1]
	if total[at("amount_up_to_date")] != third.Cumulative.ValueOfWork.Decimal() {
		t.Errorf("the total row reads %s, the bill stands at %s", total[at("amount_up_to_date")], third.Cumulative.ValueOfWork)
	}
	if total[at("amount_this_bill")] != third.ThisBill.ValueOfWork.Decimal() {
		t.Errorf("the total row claims %s, the bill claims %s", total[at("amount_this_bill")], third.ThisBill.ValueOfWork)
	}
}

func TestMemorandumCSVRowsReadDownToTheNetPayable(t *testing.T) {
	bills := runningAccount(t)
	final := bills[3]

	rows := MemorandumCSVRows(final)
	if len(rows) != 10 {
		t.Fatalf("the memorandum has %d lines, want 10", len(rows))
	}
	for _, row := range rows {
		if len(row) != len(MemorandumCSVHeader()) {
			t.Fatalf("line %q has %d cells", row[0], len(row))
		}
	}

	byName := make(map[string][]string, len(rows))
	for _, row := range rows {
		byName[row[0]] = row
	}

	for name, want := range map[string]Money{
		"Value of work done":                 final.Cumulative.ValueOfWork,
		"Gross value of the bill":            final.Cumulative.GrossBill(),
		"Less retention withheld":            final.Cumulative.RetentionWithheld,
		"Add retention released":             final.Cumulative.RetentionReleased,
		"Less mobilisation-advance recovery": final.Cumulative.AdvanceRecovery,
		"Net payable":                        final.Cumulative.NetPayable,
	} {
		row, found := byName[name]
		if !found {
			t.Fatalf("the memorandum has no line %q", name)
		}
		if row[1] != want.Decimal() {
			t.Errorf("%q reads %s up to date, want %s", name, row[1], want)
		}
	}

	// The this-bill column of the net payable is what the contractor is paid.
	if got := byName["Net payable"][3]; got != final.ThisBill.NetPayable.Decimal() {
		t.Errorf("the net payable in this bill reads %s, want %s", got, final.ThisBill.NetPayable)
	}
}

func TestMeasurementCSVRowsAreTheBookInBookOrder(t *testing.T) {
	b := book()
	rows := MeasurementCSVRows(b, 4)

	if len(rows) != len(b) {
		t.Fatalf("the file has %d rows for %d entries", len(rows), len(b))
	}
	for _, row := range rows {
		if len(row) != len(MeasurementCSVHeader()) {
			t.Fatalf("a row has %d cells against %d headings", len(row), len(MeasurementCSVHeader()))
		}
	}

	// The first entry is bill 1's four footings: 4 × 3.25 × 0.23 × 0.15.
	if rows[0][0] != "1" || rows[0][1] != "2.1" {
		t.Errorf("the first row reads bill %q item %q", rows[0][0], rows[0][1])
	}
	if rows[0][5] != "4" || rows[0][6] != "3.25" || rows[0][7] != "0.23" || rows[0][8] != "0.15" {
		t.Errorf("the first row's dimensions read %q", rows[0][5:9])
	}
	if rows[0][10] != "0.449" {
		t.Errorf("the first row's quantity reads %q, want 0.449", rows[0][10])
	}

	// The deduction entry says so and carries a minus quantity.
	if rows[2][9] != "yes" || !strings.HasPrefix(rows[2][10], "-") {
		t.Errorf("the deduction row reads deduct=%q quantity=%q", rows[2][9], rows[2][10])
	}

	// The entry not yet certified is last and belongs to the open bill.
	last := rows[len(rows)-1]
	if last[0] != "4" || last[1] != "9.9" {
		t.Errorf("the last row reads bill %q item %q, want bill 4 item 9.9", last[0], last[1])
	}
}
