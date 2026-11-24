package rabill

import (
	"fmt"
	"strings"
)

// A bill of quantities almost always exists in a spreadsheet before it exists
// anywhere else, and a bill has to be able to go back into one for the client's
// own checking. So the bill of quantities comes in and goes out as CSV, and so
// does every bill.

// RowError is a row of an imported file that could not be used, named by its
// line in the file so it can be found and corrected.
type RowError struct {
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ParsedItem is one item read from a file, with the line it came from.
type ParsedItem struct {
	Line int  `json:"line"`
	Item Item `json:"item"`
}

// ItemCSVHeader is the column heading of the bill-of-quantities file. A file
// exported under these headings imports back unchanged.
func ItemCSVHeader() []string {
	return []string{"item", "part", "description", "unit", "contract_qty", "rate", "extra"}
}

// ItemCSVRows renders a bill of quantities for a file.
func ItemCSVRows(boq BOQ) [][]string {
	rows := make([][]string, 0, len(boq))
	for _, item := range boq {
		extra := ""
		if item.Extra {
			extra = "yes"
		}
		rows = append(rows, []string{
			item.Code, item.Part, item.Description, item.Unit,
			item.ContractQty.Decimal(), item.Rate.Decimal(), extra,
		})
	}
	return rows
}

// columnAliases are the headings a spreadsheet is likely to carry for each
// field this tool needs, so a file does not have to be renamed before it can be
// used.
var columnAliases = map[string][]string{
	"item":         {"item", "itemno", "itemnumber", "code", "slno", "sno", "srno", "boqitem", "itemcode"},
	"part":         {"part", "subhead", "head", "section", "group", "category"},
	"description":  {"description", "particulars", "descriptionofwork", "itemdescription", "work", "nameofwork"},
	"unit":         {"unit", "uom", "units", "unitofmeasurement", "uqc"},
	"contract_qty": {"contractqty", "qty", "quantity", "boqqty", "agreementqty", "tenderqty", "contractquantity"},
	"rate":         {"rate", "unitrate", "rateperunit", "amountperunit", "price"},
	"extra":        {"extra", "extraitem", "nonboq", "deviation"},
}

// headingKey reduces a column heading to the form aliases are matched on.
func headingKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// mapColumns works out which column of a file holds which field.
func mapColumns(heading []string) map[string]int {
	lookup := make(map[string]string)
	for field, aliases := range columnAliases {
		for _, alias := range aliases {
			lookup[alias] = field
		}
	}

	out := make(map[string]int)
	for i, cell := range heading {
		field, known := lookup[headingKey(cell)]
		if !known {
			continue
		}
		if _, taken := out[field]; !taken {
			out[field] = i
		}
	}
	return out
}

// truthy reads the several ways a spreadsheet says yes.
func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "yes", "y", "true", "1", "extra", "x":
		return true
	}
	return false
}

// ParseItemCSV reads a bill of quantities out of the rows of a CSV file. It
// takes the columns from the headings, in whatever order they come and under
// whatever name the spreadsheet gives them, and reports every row it cannot
// use rather than stopping at the first.
func ParseItemCSV(rows [][]string) ([]ParsedItem, []RowError) {
	var parsed []ParsedItem
	var problems []RowError

	if len(rows) == 0 {
		return nil, []RowError{{Line: 0, Message: "the file is empty"}}
	}

	columns := mapColumns(rows[0])
	if _, found := columns["item"]; !found {
		return nil, []RowError{{Line: 1, Message: "the first row of the file must name the columns, and one of them must be the item number — none of " + strings.Join(rows[0], ", ") + " reads as one"}}
	}
	if _, found := columns["rate"]; !found {
		return nil, []RowError{{Line: 1, Message: "the file has no rate column, so there is nothing to price the work at"}}
	}

	cell := func(row []string, field string) string {
		at, found := columns[field]
		if !found || at >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[at])
	}

	seen := make(map[string]int)
	for n, row := range rows[1:] {
		line := n + 2 // the heading is line 1

		if strings.TrimSpace(strings.Join(row, "")) == "" {
			continue
		}

		item := Item{
			Code:        cell(row, "item"),
			Part:        cell(row, "part"),
			Description: cell(row, "description"),
			Unit:        cell(row, "unit"),
			Extra:       truthy(cell(row, "extra")),
		}

		var faults []string
		qty, err := ParseQty(cell(row, "contract_qty"))
		if err != nil {
			faults = append(faults, err.Error())
		}
		item.ContractQty = qty

		rate, err := ParseMoney(cell(row, "rate"))
		if err != nil {
			faults = append(faults, err.Error())
		}
		item.Rate = rate

		item = item.Normalise()
		if err := item.Validate(); err != nil {
			faults = append(faults, strings.Split(err.Error(), "\n")...)
		}
		if len(faults) > 0 {
			problems = append(problems, RowError{Line: line, Message: strings.Join(faults, "; ")})
			continue
		}
		if first, repeated := seen[item.Key()]; repeated {
			problems = append(problems, RowError{Line: line, Message: fmt.Sprintf("item %s is already on line %d of this file", item.Code, first)})
			continue
		}
		seen[item.Key()] = line
		parsed = append(parsed, ParsedItem{Line: line, Item: item})
	}

	if len(parsed) == 0 && len(problems) == 0 {
		problems = append(problems, RowError{Line: 0, Message: "the file has headings but no rows of items under them"})
	}
	return parsed, problems
}

// BillCSVHeader is the column heading of a bill exported as a file: the three
// columns a running-account bill is read across.
func BillCSVHeader() []string {
	return []string{
		"item", "part", "description", "unit", "rate", "contract_qty",
		"qty_previous", "qty_this_bill", "qty_up_to_date",
		"amount_previous", "amount_this_bill", "amount_up_to_date",
		"excess_qty", "needs_approval", "carried_forward",
	}
}

// BillCSVRows renders a bill's items for a file.
func BillCSVRows(b Bill) [][]string {
	yes := func(v bool) string {
		if v {
			return "yes"
		}
		return ""
	}

	rows := make([][]string, 0, len(b.Items)+1)
	for _, l := range b.Items {
		rows = append(rows, []string{
			l.Code, l.Part, l.Description, l.Unit, l.Rate.Decimal(), l.ContractQty.Decimal(),
			l.PreviousQty.Decimal(), l.PresentQty.Decimal(), l.UpToDateQty.Decimal(),
			l.PreviousAmount.Decimal(), l.PresentAmount.Decimal(), l.UpToDateAmount.Decimal(),
			l.ExcessQty.Decimal(), yes(l.NeedsApproval), yes(l.Carried),
		})
	}
	rows = append(rows, []string{
		"", "", "Value of work done", "", "", "", "", "", "",
		b.Previous.ValueOfWork.Decimal(), b.ThisBill.ValueOfWork.Decimal(), b.Cumulative.ValueOfWork.Decimal(),
		"", "", "",
	})
	return rows
}

// MemorandumCSVHeader is the column heading of the memorandum of payments: the
// deduction ledger read across the same three columns.
func MemorandumCSVHeader() []string {
	return []string{"line", "up_to_date", "previous", "this_bill"}
}

// MemorandumCSVRows renders the memorandum of payments for a file, in the order
// a bill prints it.
func MemorandumCSVRows(b Bill) [][]string {
	row := func(name string, pick func(Ledger) Money) []string {
		return []string{name, pick(b.Cumulative).Decimal(), pick(b.Previous).Decimal(), pick(b.ThisBill).Decimal()}
	}
	return [][]string{
		row("Value of work done", func(l Ledger) Money { return l.ValueOfWork }),
		row("Tax on the supply", func(l Ledger) Money { return l.SupplyTax }),
		row("Gross value of the bill", Ledger.GrossBill),
		row("Less retention withheld", func(l Ledger) Money { return l.RetentionWithheld }),
		row("Add retention released", func(l Ledger) Money { return l.RetentionReleased }),
		row("Less mobilisation-advance recovery", func(l Ledger) Money { return l.AdvanceRecovery }),
		row("Less income-tax deducted at source", func(l Ledger) Money { return l.IncomeTaxTDS }),
		row("Less deduction out of the tax on the supply", func(l Ledger) Money { return l.SupplyTaxTDS }),
		row("Less other recoveries", func(l Ledger) Money { return l.OtherRecoveries }),
		row("Net payable", func(l Ledger) Money { return l.NetPayable }),
	}
}

// MeasurementCSVHeader is the column heading of the measurement sheet.
func MeasurementCSVHeader() []string {
	return []string{"bill_no", "item", "date", "page", "where", "nos", "length", "breadth", "depth", "deduct", "quantity", "note"}
}

// MeasurementCSVRows renders a measurement book for a file, in book order.
// openBillNo is the number the entries not yet certified belong to.
func MeasurementCSVRows(book Book, openBillNo int) [][]string {
	rows := make([][]string, 0, len(book))
	for _, m := range book.Sorted() {
		no := m.BillNo
		if no == 0 {
			no = openBillNo
		}
		deduct := ""
		if m.Deduct {
			deduct = "yes"
		}
		quantity := ""
		if q, err := m.Quantity(); err == nil {
			quantity = q.Decimal()
		}
		rows = append(rows, []string{
			fmt.Sprintf("%d", no), m.ItemCode, m.Date.String(), m.Page, m.Where,
			trimFloat(m.Nos), trimFloat(m.Length), trimFloat(m.Breadth), trimFloat(m.Depth),
			deduct, quantity, m.Note,
		})
	}
	return rows
}
