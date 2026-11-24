package rabill

import (
	"fmt"
	"sort"
	"strings"
)

// Item is one line of the bill of quantities: the work the contract agreed a
// rate for. An item measured on site that the contract never priced is still an
// Item, marked Extra, because it has to appear in the bill somewhere — the
// alternative is work done and not claimed.
type Item struct {
	// Code is the item number in the bill of quantities, such as "2.4" or
	// "CIV-11". It is how a measurement finds its item, so it must be unique.
	Code string `json:"code"`
	// Part is the sub-head of the contract the item sits under, such as
	// "Earthwork" or "Plumbing". It only groups the printed bill.
	Part string `json:"part"`
	// Description is the item's description of work, as the contract words it.
	Description string `json:"description"`
	// Unit is the unit of measurement: cum, sqm, m, kg, MT, nos, litre.
	Unit string `json:"unit"`
	// ContractQty is the quantity the contract provides for. Zero means the
	// quantity is not limited by the contract, which is normal for a
	// provisional or an extra item.
	ContractQty Qty `json:"contractQty"`
	// Rate is the agreed rate per unit.
	Rate Money `json:"rate"`
	// Extra marks an item the contract did not price, whose rate therefore
	// needs the client's written approval before it can be paid.
	Extra bool `json:"extra"`
}

// NormaliseCode reduces an item code to the form measurements are matched on:
// trimmed, inner runs of space collapsed, and compared without regard to case.
func NormaliseCode(s string) string {
	return strings.Join(strings.Fields(strings.ToUpper(s)), " ")
}

// Key is the item's code in the form measurements are matched on.
func (i Item) Key() string { return NormaliseCode(i.Code) }

// Normalise tidies an item's text fields.
func (i Item) Normalise() Item {
	i.Code = strings.Join(strings.Fields(i.Code), " ")
	i.Part = strings.TrimSpace(i.Part)
	i.Description = strings.TrimSpace(i.Description)
	i.Unit = strings.TrimSpace(i.Unit)
	if i.ContractQty < 0 {
		i.ContractQty = 0
	}
	return i
}

// Amount is the item's contract amount: its contract quantity at its rate.
func (i Item) Amount() (Money, error) { return Amount(i.ContractQty, i.Rate) }

// Validate reports everything wrong with an item, as one error listing them all.
func (i Item) Validate() error {
	var problems []string
	if strings.TrimSpace(i.Code) == "" {
		problems = append(problems, "an item needs its number from the bill of quantities")
	}
	if strings.TrimSpace(i.Description) == "" {
		problems = append(problems, "an item needs a description of the work")
	}
	if strings.TrimSpace(i.Unit) == "" {
		problems = append(problems, "an item needs a unit of measurement, such as cum, sqm, m, kg or nos")
	}
	if i.Rate < 0 {
		problems = append(problems, "a rate cannot be below zero")
	}
	if i.Rate > maxMoney {
		problems = append(problems, fmt.Sprintf("a rate of %s is beyond what this tool holds exactly", i.Rate))
	}
	if i.ContractQty > maxQty {
		problems = append(problems, fmt.Sprintf("a contract quantity of %s is beyond what this tool holds exactly", i.ContractQty))
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}

// BOQ is the whole bill of quantities, in the order it is to be billed in.
type BOQ []Item

// Normalise tidies every item.
func (b BOQ) Normalise() BOQ {
	out := make(BOQ, len(b))
	for i, item := range b {
		out[i] = item.Normalise()
	}
	return out
}

// Validate checks every item and refuses a repeated code, since a repeated code
// would make it impossible to say which item a measurement belongs to.
func (b BOQ) Validate() error {
	var problems []string
	seen := make(map[string]string, len(b))
	for n, item := range b {
		if err := item.Validate(); err != nil {
			for _, p := range strings.Split(err.Error(), "\n") {
				problems = append(problems, fmt.Sprintf("item %d (%s): %s", n+1, item.Code, p))
			}
			continue
		}
		if first, dup := seen[item.Key()]; dup {
			problems = append(problems, fmt.Sprintf("item number %s is used twice — once for %q and again for %q", item.Code, first, item.Description))
			continue
		}
		seen[item.Key()] = item.Description
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(problems, "\n"))
}

// Find returns the item with a code, and whether there was one.
func (b BOQ) Find(code string) (Item, bool) {
	key := NormaliseCode(code)
	for _, item := range b {
		if item.Key() == key {
			return item, true
		}
	}
	return Item{}, false
}

// Index maps every item code to its item.
func (b BOQ) Index() map[string]Item {
	out := make(map[string]Item, len(b))
	for _, item := range b {
		out[item.Key()] = item
	}
	return out
}

// Total is the sum of every item's contract amount — the value of the contract
// as the bill of quantities prices it. Extra items are left out, because an
// extra item is by definition outside the contract that was signed.
func (b BOQ) Total() (Money, error) {
	var total Money
	for _, item := range b {
		if item.Extra {
			continue
		}
		amount, err := item.Amount()
		if err != nil {
			return 0, fmt.Errorf("item %s: %w", item.Code, err)
		}
		total += amount
	}
	return total, nil
}

// Parts lists the sub-heads the items fall under, in the order they first
// appear, with items that name no sub-head last under an empty name.
func (b BOQ) Parts() []string {
	var order []string
	seen := make(map[string]bool)
	for _, item := range b {
		if !seen[item.Part] {
			seen[item.Part] = true
			order = append(order, item.Part)
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i] != "" && order[j] == "" })
	return order
}
