package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sreenivas-sadhu-prabhakara/rabill-running-account-bill-generator/internal/rabill"
)

// api serves the JSON interface the page talks to. Every call is same-origin
// and nothing leaves the machine.
type api struct {
	store *store
}

// routes registers every endpoint.
func (a *api) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/state", a.getState)
	mux.HandleFunc("PUT /api/project", a.putProject)
	mux.HandleFunc("PUT /api/terms", a.putTerms)
	mux.HandleFunc("PUT /api/draft", a.putDraft)

	mux.HandleFunc("POST /api/items", a.postItem)
	mux.HandleFunc("DELETE /api/items", a.deleteItem)
	mux.HandleFunc("POST /api/items/import", a.postImport)

	mux.HandleFunc("POST /api/measurements", a.postMeasurement)
	mux.HandleFunc("DELETE /api/measurements", a.deleteMeasurement)

	mux.HandleFunc("GET /api/bill", a.getBill)
	mux.HandleFunc("POST /api/bills", a.postCertify)
	mux.HandleFunc("DELETE /api/bills", a.deleteBill)

	mux.HandleFunc("GET /api/bill.csv", a.getBillCSV)
	mux.HandleFunc("GET /api/boq.csv", a.getBOQCSV)
	mux.HandleFunc("GET /api/boq-template.csv", a.getTemplateCSV)
	mux.HandleFunc("GET /api/measurements.csv", a.getMeasurementsCSV)
}

// indicativeDefaults are the figures this build ships with, sent to the page so
// that it can offer them and say where they came from rather than carry a
// second copy of them.
type indicativeDefaults struct {
	Market                   rabill.Market `json:"market"`
	Currency                 string        `json:"currency"`
	Symbol                   string        `json:"symbol"`
	Terms                    rabill.Terms  `json:"terms"`
	AdvancePct               rabill.Pct    `json:"advancePct"`
	FinalRetentionReleasePct rabill.Pct    `json:"finalRetentionReleasePct"`
	Regulation               string        `json:"regulation"`
}

// defaultsFor describes a market's shipped figures.
func defaultsFor(m rabill.Market) indicativeDefaults {
	terms := rabill.DefaultTerms(m)
	advance := rabill.Pct(rabill.DefaultAdvancePctIN)
	if m == rabill.MarketPH {
		advance = rabill.DefaultAdvancePctPH
	}
	return indicativeDefaults{
		Market:                   terms.Market,
		Currency:                 terms.Market.Currency(),
		Symbol:                   terms.Market.Symbol(),
		Terms:                    terms,
		AdvancePct:               advance,
		FinalRetentionReleasePct: rabill.DefaultFinalRetentionReleasePct,
		Regulation:               terms.Regulation(),
	}
}

// statutoryFigures are the thresholds the law sets rather than the contract, so
// the page can state them beside the rates.
type statutoryFigures struct {
	TDS194CIndividualHUFPct rabill.Pct `json:"tds194cIndividualHufPct"`
	TDS194COtherPct         rabill.Pct `json:"tds194cOtherPct"`
	TDS206AANoPANPct        rabill.Pct `json:"tds206aaNoPanPct"`
	TDS194CSinglePaymentINR int64      `json:"tds194cSinglePaymentInr"`
	TDS194CAnnualINR        int64      `json:"tds194cAnnualInr"`
	GSTTDSPct               rabill.Pct `json:"gstTdsPct"`
	GSTTDSThresholdINR      int64      `json:"gstTdsThresholdInr"`
	EWTContractorPctPH      rabill.Pct `json:"ewtContractorPctPh"`
	VATPctPH                rabill.Pct `json:"vatPctPh"`
	WithholdingVATGovtPctPH rabill.Pct `json:"withholdingVatGovtPctPh"`
}

// itemQuantity is how much of an item has been measured, for the bill-of-
// quantities page to show beside the contract quantity.
type itemQuantity struct {
	Code        string       `json:"code"`
	Measured    rabill.Qty   `json:"measured"`
	Billed      rabill.Qty   `json:"billed"`
	Entries     int          `json:"entries"`
	OpenEntries int          `json:"openEntries"`
	Amount      rabill.Money `json:"amount"`
}

// statePayload is everything the page needs to draw itself.
type statePayload struct {
	Today          rabill.Date                   `json:"today"`
	Project        project                       `json:"project"`
	Terms          rabill.Terms                  `json:"terms"`
	Draft          draft                         `json:"draft"`
	BOQ            rabill.BOQ                    `json:"boq"`
	Measurements   rabill.Book                   `json:"measurements"`
	Measured       []itemQuantity                `json:"measured"`
	Bills          []rabill.Bill                 `json:"bills"`
	OpenBill       *rabill.Bill                  `json:"openBill"`
	OpenBillNo     int                           `json:"openBillNo"`
	OpenError      string                        `json:"openError"`
	Closed         bool                          `json:"closed"`
	ContractValue  rabill.Money                  `json:"contractValue"`
	BOQTotal       rabill.Money                  `json:"boqTotal"`
	Reconciliation rabill.Reconciliation         `json:"reconciliation"`
	Defaults       map[string]indicativeDefaults `json:"defaults"`
	Statutory      statutoryFigures              `json:"statutory"`
	DataFile       string                        `json:"dataFile"`
}

// getState answers with the whole contract as it stands.
func (a *api) getState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.state())
}

// state gathers everything the page draws itself from.
func (a *api) state() statePayload {
	data := a.store.snapshot()

	out := statePayload{
		Today:          rabill.Today(),
		Project:        data.Project,
		Terms:          data.Terms,
		Draft:          data.Draft,
		BOQ:            data.BOQ,
		Measurements:   data.Measurements.Sorted(),
		Bills:          data.Bills,
		OpenBillNo:     data.nextBillNo(),
		Closed:         data.closed(),
		ContractValue:  data.contractValue(),
		Reconciliation: rabill.Reconcile(data.Bills),
		Defaults: map[string]indicativeDefaults{
			string(rabill.MarketIN): defaultsFor(rabill.MarketIN),
			string(rabill.MarketPH): defaultsFor(rabill.MarketPH),
		},
		Statutory: statutoryFigures{
			TDS194CIndividualHUFPct: rabill.TDS194CIndividualHUFPct,
			TDS194COtherPct:         rabill.TDS194COtherPct,
			TDS206AANoPANPct:        rabill.TDS206AANoPANPct,
			TDS194CSinglePaymentINR: rabill.TDS194CSinglePaymentINR,
			TDS194CAnnualINR:        rabill.TDS194CAnnualINR,
			GSTTDSPct:               rabill.GSTTDSPct,
			GSTTDSThresholdINR:      rabill.GSTTDSContractThresholdINR,
			EWTContractorPctPH:      rabill.EWTContractorPctPH,
			VATPctPH:                rabill.DefaultVATPctPH,
			WithholdingVATGovtPctPH: rabill.WithholdingVATGovtPctPH,
		},
		DataFile: a.store.path,
	}

	if total, err := data.BOQ.Total(); err == nil {
		out.BOQTotal = total
	}

	if !data.closed() {
		if bill, err := data.openBill(); err == nil {
			out.OpenBill = &bill
		} else {
			out.OpenError = err.Error()
		}
	}

	out.Measured = measuredQuantities(data)
	return out
}

// measuredQuantities totals the measurement book per item: what has been
// measured altogether, and what of it has been certified.
func measuredQuantities(data fileData) []itemQuantity {
	open := data.nextBillNo()
	all, err := data.Measurements.Quantities(open, open)
	if err != nil {
		all = map[string]rabill.Qty{}
	}
	billed, err := data.Measurements.Quantities(open-1, open)
	if err != nil {
		billed = map[string]rabill.Qty{}
	}

	entries := map[string]int{}
	openEntries := map[string]int{}
	for _, m := range data.Measurements {
		entries[m.Key()]++
		if m.BillNo == 0 {
			openEntries[m.Key()]++
		}
	}

	out := make([]itemQuantity, 0, len(data.BOQ))
	for _, item := range data.BOQ {
		row := itemQuantity{
			Code:        item.Code,
			Measured:    all[item.Key()],
			Billed:      billed[item.Key()],
			Entries:     entries[item.Key()],
			OpenEntries: openEntries[item.Key()],
		}
		if amount, err := rabill.Amount(row.Measured, item.Rate); err == nil {
			row.Amount = amount
		}
		out = append(out, row)
	}
	return out
}

// putProject saves the contract's heading.
func (a *api) putProject(w http.ResponseWriter, r *http.Request) {
	var in project
	if !readJSON(w, r, &in) {
		return
	}
	saved, err := a.store.putProject(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": saved})
}

// putTerms saves the contract's money terms.
func (a *api) putTerms(w http.ResponseWriter, r *http.Request) {
	var in rabill.Terms
	if !readJSON(w, r, &in) {
		return
	}
	saved, err := a.store.putTerms(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"terms": saved})
}

// putDraft saves what the open bill carries of its own.
func (a *api) putDraft(w http.ResponseWriter, r *http.Request) {
	var in draft
	if !readJSON(w, r, &in) {
		return
	}
	saved, err := a.store.putDraft(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"draft": saved})
}

// postItem adds an item to the bill of quantities or replaces one in it. The
// item being replaced is named by ?replacing= when its number is changing.
func (a *api) postItem(w http.ResponseWriter, r *http.Request) {
	var in rabill.Item
	if !readJSON(w, r, &in) {
		return
	}
	saved, err := a.store.putItem(in, r.URL.Query().Get("replacing"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": saved})
}

// deleteItem removes an item from the bill of quantities.
func (a *api) deleteItem(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		writeProblem(w, http.StatusBadRequest, errors.New("which item? pass ?code=<item number>"))
		return
	}
	if err := a.store.deleteItem(code); err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": code})
}

// postMeasurement records an entry in the measurement book.
func (a *api) postMeasurement(w http.ResponseWriter, r *http.Request) {
	var in rabill.Measurement
	if !readJSON(w, r, &in) {
		return
	}
	saved, err := a.store.addMeasurement(in)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	quantity, _ := saved.Quantity()
	writeJSON(w, http.StatusCreated, map[string]any{"measurement": saved, "quantity": quantity})
}

// deleteMeasurement removes an entry from the open bill.
func (a *api) deleteMeasurement(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		writeProblem(w, http.StatusBadRequest, errors.New("which measurement? pass ?id=<identity>"))
		return
	}
	if err := a.store.deleteMeasurement(id); err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": id})
}

// getBill answers with one bill: a certified one by number, or the one being
// drawn up when no number is given.
func (a *api) getBill(w http.ResponseWriter, r *http.Request) {
	bill, err := a.bill(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	data := a.store.snapshot()
	writeJSON(w, http.StatusOK, map[string]any{
		"bill":    bill,
		"project": data.Project,
		"sheets":  measurementSheets(data, bill),
	})
}

// measurementSheet is one item's measurements as they were taken into a bill —
// the working behind the quantity, which is what a client's engineer checks.
type measurementSheet struct {
	Code        string       `json:"code"`
	Description string       `json:"description"`
	Unit        string       `json:"unit"`
	Entries     rabill.Book  `json:"entries"`
	Quantities  []rabill.Qty `json:"quantities"`
	Total       rabill.Qty   `json:"total"`
}

// measurementSheets gathers the measurements taken into a bill, item by item.
func measurementSheets(data fileData, bill rabill.Bill) []measurementSheet {
	open := data.nextBillNo()
	var out []measurementSheet
	for _, item := range data.BOQ {
		entries := data.Measurements.EntriesFor(item.Code, bill.No, open)
		if len(entries) == 0 {
			continue
		}
		sheet := measurementSheet{Code: item.Code, Description: item.Description, Unit: item.Unit, Entries: entries}
		for _, m := range entries {
			q, _ := m.Quantity()
			sheet.Quantities = append(sheet.Quantities, q)
			sheet.Total += q
		}
		out = append(out, sheet)
	}
	return out
}

// bill resolves the bill a request asks for.
func (a *api) bill(r *http.Request) (rabill.Bill, error) {
	data := a.store.snapshot()

	raw := strings.TrimSpace(r.URL.Query().Get("no"))
	if raw == "" || raw == "open" || raw == "draft" {
		if data.closed() {
			return rabill.Bill{}, errors.New("the final bill has been certified and the account is closed")
		}
		return data.openBill()
	}

	no, err := strconv.Atoi(raw)
	if err != nil {
		return rabill.Bill{}, fmt.Errorf("%q is not a bill number", raw)
	}
	if no == data.nextBillNo() && !data.closed() {
		return data.openBill()
	}
	if no < 1 || no > len(data.Bills) {
		return rabill.Bill{}, fmt.Errorf("there is no RA bill %d in this account", no)
	}
	return data.Bills[no-1], nil
}

// postCertify freezes the bill being drawn up.
func (a *api) postCertify(w http.ResponseWriter, r *http.Request) {
	bill, err := a.store.certify()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"bill": bill})
}

// deleteBill reopens the last certified bill.
func (a *api) deleteBill(w http.ResponseWriter, r *http.Request) {
	no, err := a.store.reopen()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reopened": no})
}

// postImport brings a bill of quantities in from a CSV file.
func (a *api) postImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCSVBody)
	defer r.Body.Close()

	reader := csv.NewReader(r.Body)
	reader.FieldsPerRecord = -1 // rows may be ragged; a short row is reported per row
	reader.TrimLeadingSpace = true

	rows, err := reader.ReadAll()
	if err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Errorf("this does not read as a CSV file: %w", err))
		return
	}

	parsed, rowErrors := rabill.ParseItemCSV(rows)
	result, err := a.store.importBOQ(parsed, rowErrors)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// getBillCSV answers with a bill as a file: the account of work done, then the
// memorandum of payments under it.
func (a *api) getBillCSV(w http.ResponseWriter, r *http.Request) {
	bill, err := a.bill(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err)
		return
	}

	rows := rabill.BillCSVRows(bill)
	rows = append(rows, nil)
	rows = append(rows, []string{"Memorandum of payments", "up to date", "previous bill", "this bill"})
	rows = append(rows, rabill.MemorandumCSVRows(bill)...)

	name := fmt.Sprintf("ra-bill-%d.csv", bill.No)
	if bill.Final {
		name = fmt.Sprintf("ra-bill-%d-final.csv", bill.No)
	}
	writeCSV(w, name, rabill.BillCSVHeader(), rows)
}

// getBOQCSV answers with the bill of quantities as a file. The same file
// imports back unchanged, so it is also the backup.
func (a *api) getBOQCSV(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()
	writeCSV(w, "bill-of-quantities.csv", rabill.ItemCSVHeader(), rabill.ItemCSVRows(data.BOQ))
}

// getTemplateCSV answers with an empty file carrying only the column headings.
func (a *api) getTemplateCSV(w http.ResponseWriter, r *http.Request) {
	writeCSV(w, "bill-of-quantities-template.csv", rabill.ItemCSVHeader(), nil)
}

// getMeasurementsCSV answers with the whole measurement book as a file.
func (a *api) getMeasurementsCSV(w http.ResponseWriter, r *http.Request) {
	data := a.store.snapshot()
	writeCSV(w, "measurement-book.csv", rabill.MeasurementCSVHeader(),
		rabill.MeasurementCSVRows(data.Measurements, data.nextBillNo()))
}

// readJSON reads a JSON request body, answering with a problem and reporting
// false when it cannot.
func readJSON(w http.ResponseWriter, r *http.Request, into any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		writeProblem(w, http.StatusBadRequest, fmt.Errorf("this request did not read as JSON: %w", err))
		return false
	}
	return true
}

// writeJSON answers with a JSON body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "could not prepare the answer", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// writeProblem answers with an error, splitting the several problems a form can
// have into a list the page can show against the fields.
func writeProblem(w http.ResponseWriter, status int, err error) {
	problems := strings.Split(err.Error(), "\n")
	writeJSON(w, status, map[string]any{
		"error":    strings.Join(problems, "; "),
		"problems": problems,
	})
}

// writeCSV answers with a CSV file for the browser to save.
func writeCSV(w http.ResponseWriter, filename string, header []string, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))

	out := csv.NewWriter(w)
	if err := out.Write(header); err != nil {
		return
	}
	for _, row := range rows {
		if err := out.Write(row); err != nil {
			return
		}
	}
	out.Flush()
}
