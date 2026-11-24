package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sreenivas-sadhu-prabhakara/rabill-running-account-bill-generator/internal/rabill"
)

// fileFormatVersion is written into the data file so a later version of the
// tool can tell what it is reading.
const fileFormatVersion = 1

// project is who the contract is between and what it is for — the heading every
// bill is printed under.
type project struct {
	Work        string      `json:"work"`
	Client      string      `json:"client"`
	Contractor  string      `json:"contractor"`
	AgreementNo string      `json:"agreementNo"`
	WorkOrderNo string      `json:"workOrderNo"`
	Site        string      `json:"site"`
	StartedOn   rabill.Date `json:"startedOn"`
	DueOn       rabill.Date `json:"dueOn"`
}

// normalise trims the text fields.
func (p project) normalise() project {
	p.Work = strings.TrimSpace(p.Work)
	p.Client = strings.TrimSpace(p.Client)
	p.Contractor = strings.TrimSpace(p.Contractor)
	p.AgreementNo = strings.TrimSpace(p.AgreementNo)
	p.WorkOrderNo = strings.TrimSpace(p.WorkOrderNo)
	p.Site = strings.TrimSpace(p.Site)
	return p
}

// draft is what the open bill carries that the measurement book cannot say: its
// date, the period it covers, whether it closes the account, and the cumulative
// figures the user enters by hand.
type draft struct {
	Date              rabill.Date  `json:"date"`
	PeriodFrom        rabill.Date  `json:"periodFrom"`
	PeriodTo          rabill.Date  `json:"periodTo"`
	Final             bool         `json:"final"`
	Note              string       `json:"note"`
	OtherRecoveries   rabill.Money `json:"otherRecoveries"`
	RetentionReleased rabill.Money `json:"retentionReleased"`
}

// fileData is the whole contents of the data file.
type fileData struct {
	Version      int           `json:"version"`
	Project      project       `json:"project"`
	Terms        rabill.Terms  `json:"terms"`
	BOQ          rabill.BOQ    `json:"boq"`
	Measurements rabill.Book   `json:"measurements"`
	Bills        []rabill.Bill `json:"bills"`
	Draft        draft         `json:"draft"`
}

// normalise fills in anything missing.
func (d fileData) normalise() fileData {
	d.Version = fileFormatVersion
	d.Project = d.Project.normalise()
	d.Terms = d.Terms.Normalise()
	d.BOQ = d.BOQ.Normalise()
	d.Measurements = d.Measurements.Normalise()
	if d.Draft.Date.IsZero() {
		d.Draft.Date = rabill.Today()
	}
	d.Draft.Note = strings.TrimSpace(d.Draft.Note)
	return d
}

// store keeps the contract in memory and writes the whole file out after every
// change. One contract's bill of quantities, measurement book and bills come to
// a few thousand records at most, so rewriting the file is both simpler and
// safer than editing it in place.
type store struct {
	mu   sync.RWMutex
	path string
	data fileData
}

// openStore reads the data file, or starts an empty contract if there is no file
// yet. It does not create the file until something is saved.
func openStore(path string) (*store, error) {
	s := &store{
		path: path,
		data: fileData{Terms: rabill.DefaultTerms(rabill.MarketIN)}.normalise(),
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return s, nil
	}

	var loaded fileData
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return nil, fmt.Errorf("reading %s: %w (move the file aside to start again)", path, err)
	}
	if loaded.Version > fileFormatVersion {
		return nil, fmt.Errorf("%s was written by a newer version of this tool (format %d, this build reads %d)",
			path, loaded.Version, fileFormatVersion)
	}
	s.data = loaded.normalise()

	// A file that has been edited by hand, or written by a version with a bug
	// in it, must not be allowed to produce bills that do not add up. Say so on
	// the way in rather than on the way out.
	if _, err := rabill.Chain(s.data.Bills); err != nil {
		return nil, fmt.Errorf("the bills already in %s do not form a running account:\n%w", path, err)
	}
	return s, nil
}

// snapshot is a copy of the contract that a reader can use without holding the
// lock.
func (s *store) snapshot() fileData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := s.data
	out.BOQ = append(rabill.BOQ(nil), s.data.BOQ...)
	out.Measurements = append(rabill.Book(nil), s.data.Measurements...)
	out.Bills = append([]rabill.Bill(nil), s.data.Bills...)
	return out
}

// change applies a change and saves the file. If the change leaves the contract
// in a state that cannot produce a bill, or if saving fails, it is rolled back,
// so what is in memory always matches what is on disk.
func (s *store) change(apply func(d *fileData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	before := s.data
	before.BOQ = append(rabill.BOQ(nil), s.data.BOQ...)
	before.Measurements = append(rabill.Book(nil), s.data.Measurements...)
	before.Bills = append([]rabill.Bill(nil), s.data.Bills...)

	rollback := func() { s.data = before }

	if err := apply(&s.data); err != nil {
		rollback()
		return err
	}
	s.data = s.data.normalise()
	if err := s.save(); err != nil {
		rollback()
		return err
	}
	return nil
}

// save writes the whole contract to the data file atomically: a temporary file
// in the same directory, flushed to disk, then renamed over the old one. A
// crash therefore leaves either the old file or the new one, never half of
// either.
func (s *store) save() error {
	s.data.Version = fileFormatVersion

	body, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("preparing the contract: %w", err)
	}
	body = append(body, '\n')

	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(s.path)+".tmp*")
	if err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // a no-op once the rename has succeeded

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", s.path, err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("replacing %s: %w", s.path, err)
	}
	return nil
}

// nextBillNo is the number the open bill will carry.
func (d fileData) nextBillNo() int { return len(d.Bills) + 1 }

// lastBill is the last certified bill, or nil when the account has not started.
func (d fileData) lastBill() *rabill.Bill {
	if len(d.Bills) == 0 {
		return nil
	}
	return &d.Bills[len(d.Bills)-1]
}

// closed reports whether the final bill has been certified.
func (d fileData) closed() bool {
	last := d.lastBill()
	return last != nil && last.Final
}

// contractValue is the figure the terms give, or the total of the bill of
// quantities where the terms give none.
func (d fileData) contractValue() rabill.Money {
	if d.Terms.ContractValue > 0 {
		return d.Terms.ContractValue
	}
	total, err := d.BOQ.Total()
	if err != nil {
		return 0
	}
	return total
}

// openBill works out the bill now being drawn up, from the measurement book as
// it stands.
func (d fileData) openBill() (rabill.Bill, error) {
	no := d.nextBillNo()
	quantities, err := d.Measurements.Quantities(no, no)
	if err != nil {
		return rabill.Bill{}, err
	}
	return rabill.Compute(rabill.Input{
		No:                no,
		Date:              d.Draft.Date,
		PeriodFrom:        d.Draft.PeriodFrom,
		PeriodTo:          d.Draft.PeriodTo,
		Final:             d.Draft.Final,
		Note:              d.Draft.Note,
		BOQ:               d.BOQ,
		UpToDate:          quantities,
		Previous:          d.lastBill(),
		Terms:             d.Terms,
		OtherRecoveries:   d.Draft.OtherRecoveries,
		RetentionReleased: d.Draft.RetentionReleased,
	})
}

// newID makes an identity for a measurement-book entry.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Only reached if the machine has no randomness at all, which it does.
		return fmt.Sprintf("m%d", len(b))
	}
	return hex.EncodeToString(b[:])
}

// putProject replaces the contract's heading.
func (s *store) putProject(next project) (project, error) {
	next = next.normalise()
	err := s.change(func(d *fileData) error {
		d.Project = next
		return nil
	})
	return next, err
}

// putTerms replaces the contract's money terms. They are validated first,
// because every bill in the account is worked out from them.
func (s *store) putTerms(next rabill.Terms) (rabill.Terms, error) {
	next = next.Normalise()
	if err := next.Validate(); err != nil {
		return next, err
	}
	err := s.change(func(d *fileData) error {
		d.Terms = next
		return nil
	})
	return next, err
}

// putDraft replaces what the open bill carries of its own.
func (s *store) putDraft(next draft) (draft, error) {
	if next.OtherRecoveries < 0 {
		return next, errors.New("a recovery cannot be below zero — enter what has been recovered, not what is left")
	}
	if next.RetentionReleased < 0 {
		return next, errors.New("retention released cannot be below zero")
	}
	next.Note = strings.TrimSpace(next.Note)
	err := s.change(func(d *fileData) error {
		if d.closed() {
			return errors.New("the final bill has been certified and the account is closed")
		}
		d.Draft = next
		return nil
	})
	return next, err
}

// putItem adds an item to the bill of quantities, or replaces one already
// there. An item that has been billed keeps its code and its unit: changing
// either would silently detach it from the quantities already certified
// against it.
func (s *store) putItem(item rabill.Item, replacing string) (rabill.Item, error) {
	item = item.Normalise()
	if err := item.Validate(); err != nil {
		return item, err
	}

	err := s.change(func(d *fileData) error {
		target := rabill.NormaliseCode(replacing)
		if target == "" {
			target = item.Key()
		}

		at := -1
		for i, existing := range d.BOQ {
			if existing.Key() == target {
				at = i
				break
			}
		}

		if at < 0 {
			if _, clash := d.BOQ.Find(item.Code); clash {
				return fmt.Errorf("item %s is already in the bill of quantities", item.Code)
			}
			d.BOQ = append(d.BOQ, item)
			return nil
		}

		if billed, where := d.billedItem(target); billed {
			if item.Key() != target {
				return fmt.Errorf("item %s has already been billed in RA bill %d, so its number cannot be changed — the bills already certified refer to it", d.BOQ[at].Code, where)
			}
			if item.Unit != d.BOQ[at].Unit {
				return fmt.Errorf("item %s has already been billed in RA bill %d in %s, so its unit cannot be changed to %s", item.Code, where, d.BOQ[at].Unit, item.Unit)
			}
		}
		if item.Key() != target {
			if _, clash := d.BOQ.Find(item.Code); clash {
				return fmt.Errorf("item %s is already in the bill of quantities", item.Code)
			}
			// Carry the measurements over to the new number, or they would be
			// left pointing at an item that no longer exists.
			for i, m := range d.Measurements {
				if m.Key() == target {
					d.Measurements[i].ItemCode = item.Code
				}
			}
		}
		d.BOQ[at] = item
		return nil
	})
	return item, err
}

// billedItem reports whether an item appears in a certified bill, and the
// number of the first bill it appears in.
func (d fileData) billedItem(key string) (bool, int) {
	for _, bill := range d.Bills {
		for _, line := range bill.Items {
			if rabill.NormaliseCode(line.Code) == key {
				return true, bill.No
			}
		}
	}
	return false, 0
}

// deleteItem removes an item from the bill of quantities. An item that has been
// billed cannot be removed, and neither can one with measurements against it
// until those are removed first.
func (s *store) deleteItem(code string) error {
	return s.change(func(d *fileData) error {
		key := rabill.NormaliseCode(code)
		at := -1
		for i, item := range d.BOQ {
			if item.Key() == key {
				at = i
				break
			}
		}
		if at < 0 {
			return fmt.Errorf("there is no item %s in the bill of quantities", code)
		}
		if billed, where := d.billedItem(key); billed {
			return fmt.Errorf("item %s was billed in RA bill %d and cannot be removed — an item that has been paid for stays in the account", code, where)
		}

		var holding int
		for _, m := range d.Measurements {
			if m.Key() == key {
				holding++
			}
		}
		if holding > 0 {
			return fmt.Errorf("item %s has %d measurement(s) against it — remove those first", code, holding)
		}

		d.BOQ = append(d.BOQ[:at], d.BOQ[at+1:]...)
		return nil
	})
}

// addMeasurement records an entry in the open bill.
func (s *store) addMeasurement(m rabill.Measurement) (rabill.Measurement, error) {
	m = m.Normalise()
	m.BillNo = 0
	if m.ID == "" {
		m.ID = newID()
	}
	if m.Date.IsZero() {
		m.Date = rabill.Today()
	}
	if err := m.Validate(); err != nil {
		return m, err
	}

	err := s.change(func(d *fileData) error {
		if d.closed() {
			return errors.New("the final bill has been certified and the account is closed")
		}
		if _, found := d.BOQ.Find(m.ItemCode); !found {
			return fmt.Errorf("there is no item %s in the bill of quantities — add the item first, marking it an extra item if the contract never priced it", m.ItemCode)
		}
		for _, existing := range d.Measurements {
			if existing.ID == m.ID {
				return fmt.Errorf("a measurement with the identity %s is already recorded", m.ID)
			}
		}
		d.Measurements = append(d.Measurements, m)
		return nil
	})
	return m, err
}

// deleteMeasurement removes an entry from the open bill. An entry taken into a
// certified bill cannot be removed: a measurement book is not erased, and a
// correction goes in as a fresh entry in the bill being drawn up.
func (s *store) deleteMeasurement(id string) error {
	return s.change(func(d *fileData) error {
		at := -1
		for i, m := range d.Measurements {
			if m.ID == id {
				at = i
				break
			}
		}
		if at < 0 {
			return fmt.Errorf("there is no measurement %s in the book", id)
		}
		if no := d.Measurements[at].BillNo; no != 0 {
			return fmt.Errorf("this measurement was taken into RA bill %d. A certified measurement is not erased — record a deduction entry in the bill being drawn up instead, which is what a measurement book does", no)
		}
		d.Measurements = append(d.Measurements[:at], d.Measurements[at+1:]...)
		return nil
	})
}

// certify freezes the open bill as the next running-account bill. Everything
// measured but not yet billed is taken into it, and the bill's own figures are
// kept exactly as certified, because the next bill subtracts them.
func (s *store) certify() (rabill.Bill, error) {
	var certified rabill.Bill

	err := s.change(func(d *fileData) error {
		if d.closed() {
			return errors.New("the final bill has been certified and the account is closed")
		}

		bill, err := d.openBill()
		if err != nil {
			return err
		}
		if len(bill.Items) == 0 {
			return errors.New("there is nothing to bill: no work has been measured against any item")
		}

		no := bill.No
		for i, m := range d.Measurements {
			if m.BillNo == 0 {
				d.Measurements[i].BillNo = no
			}
		}
		d.Bills = append(d.Bills, bill)

		// Check the whole account before the change is saved, so a bill that
		// would not add up is never written to the file at all.
		if _, err := rabill.Chain(d.Bills); err != nil {
			return err
		}

		// The next bill starts from today, over the period after this one.
		d.Draft = draft{
			Date:              rabill.Today(),
			PeriodFrom:        bill.PeriodTo,
			OtherRecoveries:   bill.Cumulative.OtherRecoveries,
			RetentionReleased: bill.Cumulative.RetentionReleased,
		}
		certified = bill
		return nil
	})
	return certified, err
}

// reopen takes the last certified bill back into the open bill, so a mistake
// found before the bill goes out can be put right. Only the last bill can be
// reopened: an earlier one is subtracted by every bill after it.
func (s *store) reopen() (int, error) {
	var no int
	err := s.change(func(d *fileData) error {
		if len(d.Bills) == 0 {
			return errors.New("no bill has been certified yet")
		}
		last := d.Bills[len(d.Bills)-1]
		no = last.No

		for i, m := range d.Measurements {
			if m.BillNo == no {
				d.Measurements[i].BillNo = 0
			}
		}
		d.Bills = d.Bills[:len(d.Bills)-1]
		d.Draft = draft{
			Date:              last.Date,
			PeriodFrom:        last.PeriodFrom,
			PeriodTo:          last.PeriodTo,
			Final:             last.Final,
			Note:              last.Note,
			OtherRecoveries:   last.Cumulative.OtherRecoveries,
			RetentionReleased: last.Cumulative.RetentionReleased,
		}
		return nil
	})
	return no, err
}

// importResult reports what an imported file did to the bill of quantities.
type importResult struct {
	Added   int               `json:"added"`
	Updated int               `json:"updated"`
	Skipped int               `json:"skipped"`
	Errors  []rabill.RowError `json:"errors"`
}

// importBOQ adds the rows of a parsed file to the bill of quantities. A row
// that cannot be used is reported by line and the rest still come in. An item
// already billed keeps its number and unit, as it does when edited by hand.
func (s *store) importBOQ(rows []rabill.ParsedItem, rowErrors []rabill.RowError) (importResult, error) {
	result := importResult{Errors: rowErrors}

	err := s.change(func(d *fileData) error {
		for _, row := range rows {
			item := row.Item
			at := -1
			for i, existing := range d.BOQ {
				if existing.Key() == item.Key() {
					at = i
					break
				}
			}
			if at < 0 {
				d.BOQ = append(d.BOQ, item)
				result.Added++
				continue
			}
			if billed, where := d.billedItem(item.Key()); billed && item.Unit != d.BOQ[at].Unit {
				result.Skipped++
				result.Errors = append(result.Errors, rabill.RowError{
					Line: row.Line,
					Message: fmt.Sprintf("item %s was billed in RA bill %d in %s, so its unit cannot be changed to %s",
						item.Code, where, d.BOQ[at].Unit, item.Unit),
				})
				continue
			}
			d.BOQ[at] = item
			result.Updated++
		}
		return nil
	})
	return result, err
}
