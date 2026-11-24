package rabill

import (
	"fmt"
	"sort"
	"strings"
)

// Measurement is one entry in the measurement book: a thing measured on site,
// at a place, with the dimensions it was measured by.
//
// A measurement book has four numeric columns — No., L, B and D — and the
// quantity is their product. Which of them apply depends on the item: a wall is
// No. × L × B × H, a plinth beam No. × L × B, a skirting No. × L, a door by
// number alone. A column that does not apply is left blank on the page, so a
// blank column here multiplies by nothing rather than by zero.
type Measurement struct {
	// ID is this entry's own identity, so an entry can be referred to and
	// removed while it is still part of an open bill.
	ID string `json:"id"`
	// ItemCode is the bill-of-quantities item this entry is measured against.
	ItemCode string `json:"itemCode"`
	// BillNo is the running-account bill this entry was taken into. Zero means
	// it is in the open bill and not yet certified.
	BillNo int `json:"billNo"`
	// Date is the day the measurement was recorded.
	Date Date `json:"date"`
	// Where is the location measured, such as "Footing F3, grid C2" or
	// "Second floor, rooms 201-204". It is what makes a measurement checkable
	// against the work.
	Where string `json:"where"`
	// Page is the measurement-book page and entry it was written on, kept so a
	// bill can be traced back to the book.
	Page string `json:"page"`

	// Nos is the number of like things measured. Left at zero it counts as one
	// when any dimension is given, and as the quantity itself when none is.
	Nos float64 `json:"nos"`
	// Length, Breadth and Depth are the dimensions measured, in the item's own
	// unit of length. Zero means the dimension does not apply.
	Length  float64 `json:"length"`
	Breadth float64 `json:"breadth"`
	Depth   float64 `json:"depth"`

	// Deduct marks an entry that takes quantity away rather than adding it: an
	// opening in a wall, a recess in a slab, a length measured twice and found
	// once. A deduction is written as positive dimensions and subtracts.
	Deduct bool `json:"deduct"`
	// Note is anything the measurer wants to record against the entry.
	Note string `json:"note"`
}

// Normalise tidies an entry's text fields and squares away a negative figure.
func (m Measurement) Normalise() Measurement {
	m.ID = strings.TrimSpace(m.ID)
	m.ItemCode = strings.Join(strings.Fields(m.ItemCode), " ")
	m.Where = strings.TrimSpace(m.Where)
	m.Page = strings.TrimSpace(m.Page)
	m.Note = strings.TrimSpace(m.Note)
	if m.BillNo < 0 {
		m.BillNo = 0
	}
	return m
}

// Key is the item code this entry is measured against, in matching form.
func (m Measurement) Key() string { return NormaliseCode(m.ItemCode) }

// Quantity is the quantity this entry contributes, rounded to the thousandth
// and negative for a deduction entry.
//
// The product is Nos × every dimension that was measured. A dimension left at
// zero is one that does not apply to the item, so it is left out of the product
// rather than taken as a zero that would wipe the entry out. An entry with no
// dimensions at all is the number itself, which is how a count item and a
// quantity taken from a weighbridge slip or a supplier's invoice are recorded.
func (m Measurement) Quantity() (Qty, error) {
	product := m.Nos
	if product == 0 {
		product = 1
	}
	measured := m.Nos != 0
	for _, dim := range []float64{m.Length, m.Breadth, m.Depth} {
		if dim != 0 {
			product *= dim
			measured = true
		}
	}
	if !measured {
		return 0, nil
	}

	q, err := QtyFromFloat(product)
	if err != nil {
		return 0, err
	}
	if m.Deduct {
		return -q, nil
	}
	return q, nil
}

// Dimensions renders the entry the way the measurement book reads it, such as
// "4 × 3.25 × 0.23 × 0.15", for the printed bill's measurement sheet.
func (m Measurement) Dimensions() string {
	var parts []string
	if m.Nos != 0 {
		parts = append(parts, trimFloat(m.Nos))
	}
	for _, dim := range []float64{m.Length, m.Breadth, m.Depth} {
		if dim != 0 {
			parts = append(parts, trimFloat(dim))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " × ")
}

// trimFloat renders a dimension without trailing zeros.
func trimFloat(f float64) string { return trimZeros(fmt.Sprintf("%.4f", f)) }

// Validate reports everything wrong with an entry, as one error listing all of
// it.
func (m Measurement) Validate() error {
	var problems []string

	if strings.TrimSpace(m.ItemCode) == "" {
		problems = append(problems, "a measurement must say which item it is measured against")
	}
	for name, v := range map[string]float64{
		"the number":  m.Nos,
		"the length":  m.Length,
		"the breadth": m.Breadth,
		"the depth":   m.Depth,
	} {
		if v < 0 {
			problems = append(problems, name+" cannot be below zero — a deduction is written as positive dimensions and marked as a deduction")
		}
	}
	if m.Nos == 0 && m.Length == 0 && m.Breadth == 0 && m.Depth == 0 {
		problems = append(problems, "a measurement needs at least a number or one dimension")
	}
	if len(problems) == 0 {
		if _, err := m.Quantity(); err != nil {
			problems = append(problems, err.Error())
		}
	}

	if len(problems) == 0 {
		return nil
	}
	sort.Strings(problems)
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}

// Book is a whole measurement book: every entry recorded against the contract,
// certified and open together.
type Book []Measurement

// Normalise tidies every entry.
func (b Book) Normalise() Book {
	out := make(Book, len(b))
	for i, m := range b {
		out[i] = m.Normalise()
	}
	return out
}

// Sorted returns the entries in the order a bill reads them: by the bill they
// belong to, then by date, then by the order they were recorded in. Entries in
// the open bill come last, because that is where they are being added.
func (b Book) Sorted() Book {
	out := append(Book(nil), b...)
	order := make(map[string]int, len(b))
	for i, m := range b {
		order[m.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, z := out[i], out[j]
		ab, zb := a.BillNo, z.BillNo
		if ab == 0 {
			ab = 1 << 30
		}
		if zb == 0 {
			zb = 1 << 30
		}
		if ab != zb {
			return ab < zb
		}
		if !a.Date.Equal(z.Date) {
			return a.Date.Before(z.Date)
		}
		return order[a.ID] < order[z.ID]
	})
	return out
}

// upTo reports whether an entry belongs to the account as it stands at a bill.
// An entry in the open bill (BillNo zero) counts towards the bill being drawn
// up and towards no earlier one.
func upTo(m Measurement, billNo, openBillNo int) bool {
	effective := m.BillNo
	if effective == 0 {
		effective = openBillNo
	}
	return effective >= 1 && effective <= billNo
}

// Quantities totals the book, item by item, as it stands at a bill: every entry
// taken into that bill or any earlier one, additions less deductions.
//
// This is the "up to date" quantity a running-account bill is built on. It is
// cumulative by construction — the whole of the work measured so far, not the
// work measured since the last bill — because that is the only figure a
// measurement book can be asked for honestly. Entries are never edited away;
// a correction is a later entry, and it moves the cumulative figure.
//
// openBillNo is the number the open bill will carry, so entries not yet
// certified are counted towards it.
func (b Book) Quantities(billNo, openBillNo int) (map[string]Qty, error) {
	out := make(map[string]Qty)
	for _, m := range b {
		if !upTo(m, billNo, openBillNo) {
			continue
		}
		q, err := m.Quantity()
		if err != nil {
			return nil, fmt.Errorf("measurement %s against item %s: %w", m.Page, m.ItemCode, err)
		}
		key := m.Key()
		total := out[key] + q
		if total > maxQty || total < -maxQty {
			return nil, fmt.Errorf("item %s: %w", m.ItemCode, ErrOutOfRange)
		}
		out[key] = total
	}
	return out, nil
}

// EntriesFor returns the entries measured against one item in a bill, in book
// order. It is what a bill's measurement sheet prints under the item.
func (b Book) EntriesFor(itemCode string, billNo, openBillNo int) Book {
	key := NormaliseCode(itemCode)
	var out Book
	for _, m := range b.Sorted() {
		if m.Key() != key {
			continue
		}
		effective := m.BillNo
		if effective == 0 {
			effective = openBillNo
		}
		if effective != billNo {
			continue
		}
		out = append(out, m)
	}
	return out
}

// Validate checks every entry and refuses a repeated identity.
func (b Book) Validate() error {
	var problems []string
	seen := make(map[string]bool, len(b))
	for n, m := range b {
		if err := m.Validate(); err != nil {
			for _, p := range strings.Split(err.Error(), "\n") {
				problems = append(problems, fmt.Sprintf("measurement %d (item %s): %s", n+1, m.ItemCode, p))
			}
			continue
		}
		if m.ID != "" && seen[m.ID] {
			problems = append(problems, fmt.Sprintf("measurement %d carries an identity already used", n+1))
			continue
		}
		seen[m.ID] = true
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}
