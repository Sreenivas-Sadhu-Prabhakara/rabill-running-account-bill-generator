package rabill

import (
	"strings"
	"testing"
)

func TestItemAmountAndBOQTotal(t *testing.T) {
	boq := contractBOQ()

	cases := []struct {
		code string
		want Money
	}{
		{"1.1", rupees(114000, 0)},
		{"2.1", rupees(324000, 0)},
		{"2.2", rupees(870000, 0)},
		{"3.1", rupees(920000, 0)},
		{"4.1", rupees(427500, 0)},
	}
	for _, c := range cases {
		item, found := boq.Find(c.code)
		if !found {
			t.Fatalf("item %s is missing from the bill of quantities", c.code)
		}
		got, err := item.Amount()
		if err != nil {
			t.Fatalf("item %s: %v", c.code, err)
		}
		if got != c.want {
			t.Errorf("item %s = %s, want %s", c.code, got, c.want)
		}
	}

	total, err := boq.Total()
	if err != nil {
		t.Fatalf("Total: %v", err)
	}
	if total != contractTotal {
		t.Errorf("the contract totals %s, want %s", total, contractTotal)
	}

	// An extra item is outside the contract that was signed, so it is not part
	// of the contract value even once it has been priced.
	withExtra := append(contractBOQ(), Item{
		Code: "9.1", Description: "Rock excavation", Unit: "cum",
		ContractQty: units(20), Rate: rupees(920, 0), Extra: true,
	})
	total, err = withExtra.Total()
	if err != nil {
		t.Fatalf("Total with an extra item: %v", err)
	}
	if total != contractTotal {
		t.Errorf("with an extra item the contract totals %s, want the same %s", total, contractTotal)
	}

	empty := BOQ{}
	if got, err := empty.Total(); err != nil || got != 0 {
		t.Errorf("an empty bill of quantities totals %s, %v", got, err)
	}
}

func TestBOQFindMatchesTheWayCodesAreWritten(t *testing.T) {
	boq := BOQ{{Code: "CIV-11", Description: "Work", Unit: "nos", Rate: 100}}
	for _, code := range []string{"CIV-11", "civ-11", " civ-11 ", "Civ-11"} {
		if _, found := boq.Find(code); !found {
			t.Errorf("Find(%q) found nothing", code)
		}
	}
	if _, found := boq.Find("CIV-12"); found {
		t.Error("Find found an item that is not there")
	}
}

func TestItemValidate(t *testing.T) {
	cases := []struct {
		name string
		in   Item
		want string
	}{
		{
			name: "a sound item",
			in:   Item{Code: "2.1", Description: "Concrete", Unit: "cum", ContractQty: units(50), Rate: rupees(5400, 0)},
		},
		{
			name: "an item with no contract quantity is provisional, not wrong",
			in:   Item{Code: "2.1", Description: "Concrete", Unit: "cum", Rate: rupees(5400, 0)},
		},
		{
			name: "an item at no rate is allowed until a rate is agreed",
			in:   Item{Code: "9.1", Description: "Extra work", Unit: "cum", Extra: true},
		},
		{
			name: "no code",
			in:   Item{Description: "Concrete", Unit: "cum", Rate: 100},
			want: "number from the bill of quantities",
		},
		{
			name: "no description",
			in:   Item{Code: "2.1", Unit: "cum", Rate: 100},
			want: "description of the work",
		},
		{
			name: "no unit",
			in:   Item{Code: "2.1", Description: "Concrete", Rate: 100},
			want: "unit of measurement",
		},
		{
			name: "a negative rate",
			in:   Item{Code: "2.1", Description: "Concrete", Unit: "cum", Rate: -100},
			want: "rate cannot be below zero",
		},
		{
			name: "a rate beyond what the tool holds",
			in:   Item{Code: "2.1", Description: "Concrete", Unit: "cum", Rate: maxMoney + 1},
			want: "beyond what this tool holds exactly",
		},
		{
			name: "a quantity beyond what the tool holds",
			in:   Item{Code: "2.1", Description: "Concrete", Unit: "cum", ContractQty: maxQty + 1, Rate: 100},
			want: "beyond what this tool holds exactly",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.in.Validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want no error", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want an error about %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("Validate() = %q, want it to mention %q", err, c.want)
			}
		})
	}

	// Three faults at once are all reported.
	err := Item{}.Validate()
	if err == nil {
		t.Fatal("an empty item should be refused")
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) != 3 {
		t.Errorf("an empty item reported %d fault(s): %q", len(lines), err)
	}
}

func TestBOQValidateRefusesARepeatedItemNumber(t *testing.T) {
	if err := contractBOQ().Validate(); err != nil {
		t.Fatalf("a sound bill of quantities: %v", err)
	}

	twice := BOQ{
		{Code: "2.1", Description: "Concrete", Unit: "cum", Rate: 100},
		{Code: "2.1", Description: "Concrete again", Unit: "cum", Rate: 200},
	}
	err := twice.Validate()
	if err == nil {
		t.Fatal("the same item number twice should be refused")
	}
	if !strings.Contains(err.Error(), "used twice") {
		t.Errorf("Validate() = %q", err)
	}

	// The same code written differently is still the same code.
	spelled := BOQ{
		{Code: "civ 11", Description: "Work", Unit: "nos", Rate: 100},
		{Code: "CIV  11", Description: "Work again", Unit: "nos", Rate: 100},
	}
	if err := spelled.Validate(); err == nil {
		t.Error("the same item number spelled differently should still be refused")
	}

	// A fault in one item is reported with the item it belongs to.
	broken := BOQ{{Code: "2.1", Unit: "cum", Rate: 100}}
	err = broken.Validate()
	if err == nil || !strings.Contains(err.Error(), "item 1 (2.1)") {
		t.Errorf("Validate() = %v, want the fault attributed to item 1", err)
	}
}

func TestItemNormaliseTidiesWhatWasTyped(t *testing.T) {
	got := Item{
		Code: "  2.1 ", Part: "  Concrete  ", Description: "  Plain cement concrete  ",
		Unit: " cum ", ContractQty: -5,
	}.Normalise()

	if got.Code != "2.1" || got.Part != "Concrete" || got.Description != "Plain cement concrete" || got.Unit != "cum" {
		t.Errorf("Normalise gave %+v", got)
	}
	if got.ContractQty != 0 {
		t.Errorf("a negative contract quantity became %s, want nothing", got.ContractQty)
	}

	// A code with inner spacing keeps it in the code and loses it in the key.
	spaced := Item{Code: "CIV  11", Description: "Work", Unit: "nos"}.Normalise()
	if spaced.Code != "CIV 11" {
		t.Errorf("Normalise gave the code %q", spaced.Code)
	}
	if spaced.Key() != "CIV 11" {
		t.Errorf("Key() = %q", spaced.Key())
	}
}

func TestBOQPartsKeepTheirOrderAndPutTheUnnamedLast(t *testing.T) {
	boq := BOQ{
		{Code: "1", Part: "Earthwork", Description: "a", Unit: "cum"},
		{Code: "2", Description: "b", Unit: "cum"},
		{Code: "3", Part: "Concrete", Description: "c", Unit: "cum"},
		{Code: "4", Part: "Earthwork", Description: "d", Unit: "cum"},
	}
	got := boq.Parts()
	want := []string{"Earthwork", "Concrete", ""}
	if len(got) != len(want) {
		t.Fatalf("Parts() = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Parts() = %q, want %q", got, want)
		}
	}
}

func TestBOQIndex(t *testing.T) {
	index := contractBOQ().Index()
	if len(index) != 5 {
		t.Fatalf("the index holds %d items, want 5", len(index))
	}
	if got, found := index["2.2"]; !found || got.Rate != rupees(7250, 0) {
		t.Errorf("item 2.2 in the index = %+v", got)
	}
}
