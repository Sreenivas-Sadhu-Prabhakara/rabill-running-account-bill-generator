package rabill

import (
	"strings"
	"testing"
	"time"
)

func TestMeasurementQuantity(t *testing.T) {
	cases := []struct {
		name string
		in   Measurement
		want Qty
	}{
		{
			name: "nothing measured is nothing",
			in:   Measurement{},
			want: 0,
		},
		{
			name: "a count item is the number alone",
			in:   Measurement{Nos: 24},
			want: QtyFromUnits(24),
		},
		{
			name: "a quantity taken from a weighbridge slip",
			in:   Measurement{Nos: 12.75},
			want: 12750,
		},
		{
			name: "a length: no number, length only",
			in:   Measurement{Length: 18.4},
			want: 18400,
		},
		{
			name: "an area: length by breadth",
			in:   Measurement{Length: 4.5, Breadth: 3.2},
			want: 14400,
		},
		{
			name: "a volume: length by breadth by depth",
			in:   Measurement{Length: 4.5, Breadth: 3.2, Depth: 0.15},
			want: 2160,
		},
		{
			// 4 × 3.25 × 0.23 × 0.15 = 0.4485 cum of footing concrete.
			name: "four like footings",
			in:   Measurement{Nos: 4, Length: 3.25, Breadth: 0.23, Depth: 0.15},
			want: 449, // 0.4485 rounds away from zero at the thousandth
		},
		{
			name: "a blank breadth multiplies by nothing, not by zero",
			in:   Measurement{Nos: 6, Length: 2.4, Breadth: 0, Depth: 0.1},
			want: 1440,
		},
		{
			name: "a blank number counts as one",
			in:   Measurement{Length: 10, Breadth: 2, Depth: 0.5},
			want: QtyFromUnits(10),
		},
		{
			name: "a deduction subtracts",
			in:   Measurement{Nos: 2, Length: 1.2, Breadth: 2.1, Deduct: true},
			want: -5040,
		},
		{
			name: "a deducted count subtracts",
			in:   Measurement{Nos: 3, Deduct: true},
			want: -QtyFromUnits(3),
		},
		{
			name: "a fractional number of like things",
			in:   Measurement{Nos: 0.5, Length: 4, Breadth: 2},
			want: QtyFromUnits(4),
		},
		{
			name: "rounds to the thousandth, down",
			in:   Measurement{Length: 1.00049},
			want: 1000,
		},
		{
			name: "rounds to the thousandth, up",
			in:   Measurement{Length: 1.00051},
			want: 1001,
		},
		{
			name: "binary noise in a product does not leak through",
			in:   Measurement{Nos: 3, Length: 0.1},
			want: 300,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.in.Quantity()
			if err != nil {
				t.Fatalf("Quantity(): %v", err)
			}
			if got != c.want {
				t.Errorf("Quantity() = %s (%d), want %s (%d)", got, int64(got), c.want, int64(c.want))
			}
		})
	}
}

func TestMeasurementQuantityRefusesAnImpossibleFigure(t *testing.T) {
	if _, err := (Measurement{Nos: 1e9, Length: 1e9}).Quantity(); err == nil {
		t.Error("a quantity beyond what the tool holds exactly should be refused")
	}
}

func TestMeasurementDimensions(t *testing.T) {
	cases := []struct {
		in   Measurement
		want string
	}{
		{Measurement{Nos: 4, Length: 3.25, Breadth: 0.23, Depth: 0.15}, "4 × 3.25 × 0.23 × 0.15"},
		{Measurement{Length: 4.5, Breadth: 3.2}, "4.5 × 3.2"},
		{Measurement{Nos: 24}, "24"},
		{Measurement{Nos: 6, Length: 2.4, Depth: 0.1}, "6 × 2.4 × 0.1"},
		{Measurement{}, ""},
	}
	for _, c := range cases {
		if got := c.in.Dimensions(); got != c.want {
			t.Errorf("Dimensions() = %q, want %q", got, c.want)
		}
	}
}

func TestMeasurementValidate(t *testing.T) {
	cases := []struct {
		name string
		in   Measurement
		want string // a fragment the error must carry; empty means it must pass
	}{
		{
			name: "a sound entry",
			in:   Measurement{ItemCode: "2.1", Nos: 4, Length: 3.25, Breadth: 0.23, Depth: 0.15},
		},
		{
			name: "a count entry is sound",
			in:   Measurement{ItemCode: "5.2", Nos: 12},
		},
		{
			name: "no item",
			in:   Measurement{Nos: 4},
			want: "which item",
		},
		{
			name: "nothing measured",
			in:   Measurement{ItemCode: "2.1"},
			want: "at least a number or one dimension",
		},
		{
			name: "a negative length",
			in:   Measurement{ItemCode: "2.1", Length: -4},
			want: "the length cannot be below zero",
		},
		{
			name: "a negative number",
			in:   Measurement{ItemCode: "2.1", Nos: -4, Length: 1},
			want: "the number cannot be below zero",
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
}

// book builds a small measurement book spread across three bills: two items,
// an addition, a deduction and a later correction.
func book() Book {
	return Book{
		{ID: "m1", ItemCode: "2.1", BillNo: 1, Date: NewDate(2026, time.April, 10), Where: "Footings F1-F4", Nos: 4, Length: 3.25, Breadth: 0.23, Depth: 0.15},
		{ID: "m2", ItemCode: "3.2", BillNo: 1, Date: NewDate(2026, time.April, 12), Where: "Ground-floor walls", Length: 18, Breadth: 3, Depth: 0.23},
		{ID: "m3", ItemCode: "3.2", BillNo: 1, Date: NewDate(2026, time.April, 12), Where: "Door openings", Nos: 2, Length: 1.2, Breadth: 2.1, Depth: 0.23, Deduct: true},
		{ID: "m4", ItemCode: "2.1", BillNo: 2, Date: NewDate(2026, time.May, 18), Where: "Footings F5-F8", Nos: 4, Length: 3.25, Breadth: 0.23, Depth: 0.15},
		{ID: "m5", ItemCode: "2.1", BillNo: 3, Date: NewDate(2026, time.June, 20), Where: "F3 re-measured, depth 0.15 not 0.18", Nos: 1, Length: 3.25, Breadth: 0.23, Depth: 0.03, Deduct: true},
		{ID: "m6", ItemCode: "9.9", Date: NewDate(2026, time.July, 2), Where: "Extra: rock excavation", Nos: 1, Length: 6, Breadth: 2, Depth: 1},
	}
}

func TestBookQuantities(t *testing.T) {
	b := book()

	cases := []struct {
		name       string
		billNo     int
		openBillNo int
		want       map[string]Qty
	}{
		{
			name: "before any bill nothing has been measured",
			// 4 × 3.25 × 0.23 × 0.15 = 0.4485 -> 0.449
			billNo: 0, openBillNo: 4, want: map[string]Qty{},
		},
		{
			name:   "the first bill",
			billNo: 1, openBillNo: 4,
			// item 3.2: 18 × 3 × 0.23 = 12.42, less 2 × 1.2 × 2.1 × 0.23 = 1.1592 -> 1.159
			want: map[string]Qty{"2.1": 449, "3.2": 12420 - 1159},
		},
		{
			name:   "the second bill adds four more footings",
			billNo: 2, openBillNo: 4,
			want: map[string]Qty{"2.1": 898, "3.2": 11261},
		},
		{
			name:   "the third bill carries a downward correction",
			billNo: 3, openBillNo: 4,
			// less 1 × 3.25 × 0.23 × 0.03 = 0.022425 -> 0.022
			want: map[string]Qty{"2.1": 898 - 22, "3.2": 11261},
		},
		{
			name:   "the open bill takes in the entry not yet certified",
			billNo: 4, openBillNo: 4,
			want: map[string]Qty{"2.1": 876, "3.2": 11261, "9.9": QtyFromUnits(12)},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := b.Quantities(c.billNo, c.openBillNo)
			if err != nil {
				t.Fatalf("Quantities(%d, %d): %v", c.billNo, c.openBillNo, err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("Quantities(%d, %d) returned %d items (%v), want %d", c.billNo, c.openBillNo, len(got), got, len(c.want))
			}
			for code, want := range c.want {
				if got[code] != want {
					t.Errorf("item %s = %s, want %s", code, got[code], want)
				}
			}
		})
	}
}

func TestBookQuantitiesAreCumulativeAndNeverFallBehind(t *testing.T) {
	b := book()

	// The up-to-date quantity of an item may be corrected downwards, but it is
	// always the whole of the work measured against it, never only the part
	// measured since the last bill. Bill 2's figure must contain bill 1's.
	first, err := b.Quantities(1, 4)
	if err != nil {
		t.Fatalf("bill 1: %v", err)
	}
	second, err := b.Quantities(2, 4)
	if err != nil {
		t.Fatalf("bill 2: %v", err)
	}
	if second["2.1"] <= first["2.1"] {
		t.Errorf("item 2.1 up to bill 2 = %s, which does not contain bill 1's %s", second["2.1"], first["2.1"])
	}
	if second["3.2"] != first["3.2"] {
		t.Errorf("item 3.2 was not measured again in bill 2, so it should stand at %s, not %s", first["3.2"], second["3.2"])
	}
}

func TestBookEntriesForOneBill(t *testing.T) {
	b := book()

	entries := b.EntriesFor("3.2", 1, 4)
	if len(entries) != 2 {
		t.Fatalf("item 3.2 in bill 1 has %d entries, want 2", len(entries))
	}
	if entries[0].ID != "m2" || entries[1].ID != "m3" {
		t.Errorf("entries came back as %s, %s; want m2 then m3", entries[0].ID, entries[1].ID)
	}
	if !entries[1].Deduct {
		t.Error("the second entry is the deduction and should say so")
	}

	if got := b.EntriesFor("3.2", 2, 4); len(got) != 0 {
		t.Errorf("item 3.2 was not measured in bill 2, got %d entries", len(got))
	}

	// An entry not yet certified belongs to the open bill.
	open := b.EntriesFor("9.9", 4, 4)
	if len(open) != 1 || open[0].ID != "m6" {
		t.Errorf("the open bill should hold m6, got %+v", open)
	}

	// Matching ignores case and stray spacing in the item code.
	if got := b.EntriesFor("  2.1 ", 1, 4); len(got) != 1 {
		t.Errorf("an item code with stray spacing should still match, got %d entries", len(got))
	}
}

func TestBookSortedPutsTheOpenBillLast(t *testing.T) {
	b := Book{
		{ID: "open", BillNo: 0, Date: NewDate(2026, time.January, 1)},
		{ID: "third", BillNo: 3, Date: NewDate(2026, time.June, 1)},
		{ID: "first", BillNo: 1, Date: NewDate(2026, time.April, 1)},
	}
	got := b.Sorted()
	want := []string{"first", "third", "open"}
	for i, id := range want {
		if got[i].ID != id {
			t.Fatalf("sorted order = %s, %s, %s; want %v", got[0].ID, got[1].ID, got[2].ID, want)
		}
	}
}

func TestBookValidate(t *testing.T) {
	if err := book().Validate(); err != nil {
		t.Fatalf("a sound book: %v", err)
	}

	bad := Book{
		{ID: "a", ItemCode: "2.1", Nos: 1},
		{ID: "a", ItemCode: "2.2", Nos: 1},
	}
	err := bad.Validate()
	if err == nil || !strings.Contains(err.Error(), "identity already used") {
		t.Errorf("a repeated identity should be refused, got %v", err)
	}

	err = Book{{ItemCode: "", Nos: 0}}.Validate()
	if err == nil {
		t.Fatal("an empty entry should be refused")
	}
	if lines := strings.Split(err.Error(), "\n"); len(lines) != 2 {
		t.Errorf("an entry with two faults should report both, got %q", err)
	}
}

func TestNormaliseCode(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2.1", "2.1"},
		{"  2.1  ", "2.1"},
		{"civ-11", "CIV-11"},
		{"Part  A   3", "PART A 3"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormaliseCode(c.in); got != c.want {
			t.Errorf("NormaliseCode(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
