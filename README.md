# RA bill — running-account bill generator

A free, offline, single-binary tool for civil contractors in **India** and the **Philippines**.
It keeps a contract's bill of quantities and measurement book, prepares each running-account
bill as the work done to date less everything certified in the bill before it, applies
retention, mobilisation-advance recovery and tax deducted at source, and produces a printable
bill for the contractor to submit themselves.

Explainer site: <https://sreenivas-sadhu-prabhakara.github.io/rabill-running-account-bill-generator/>

## What it does

A running-account bill is not an invoice. It is one account, re-stated every month. Each RA
bill carries, for every item, the quantity and value **up to date**; beside them, what the
**previous bill** was certified at; and in the third column the difference, which is what the
bill claims. The deductions work the same way — retention is a share of all the work done so
far, less the retention already held; advance recovery is the running total recouped, less what
was recouped before.

That subtraction is where spreadsheets go wrong, so this tool is built around not getting it
wrong:

- **The claim is a difference, never a product.** What a bill claims for an item is its value
  up to date less the value the previous bill was certified at — not the quantity measured this
  month at the rate. The two differ by the rounding on every earlier bill and by any rate
  corrected since.
- **An item that has ever been billed appears in every later bill.** With no measurement this
  time it stands at the previous figures and claims nothing. It cannot drop out unnoticed.
- **A quantity revised downwards is a minus quantity, not a deletion.** The correction gives
  back the value, the retention held on it and the tax deducted on it.
- **Nothing is paid twice.** Draw a bill up again before anything further is measured and it
  claims zero.
- **Retention and advance recovery cannot overshoot** their ceilings or the advance paid.

It then checks itself: the **reconciliation** adds every bill's claim up and sets it against
the account, line by line and item by item. They must agree exactly, to the last paise. A bill
that would break that is refused rather than certified.

Everything else it offers is in service of that: measurement-book entry (`nos × L × B × D`,
with a blank column meaning "does not apply"), deviation and extra-item flags, the amount in
words grouped the way the market groups digits, CSV in and out, and a printable bill with the
account of work done, the memorandum of payments, the measurements behind the quantities and
three signature blocks.

## Who it's for

The contractor or sub-contractor who bills a client progressively, and the project-management
consultant checking what arrives: small civil contractors, interior and finishing contractors,
road and drainage contractors, electrical and plumbing sub-contractors. It assumes a contract
with a bill of quantities, rates per unit and measurements taken on site. It needs no
accounting system and no internet connection.

## The named regulations

The *arithmetic* of a running account is a contract convention and is the same in both
markets. What the law sets is the **deductions**.

**India**

- **Income-tax Act, 1961, s.194C** — tax deducted from payments to a contractor at **1%**
  where the payee is an individual or a Hindu undivided family and **2%** otherwise; **s.206AA**
  raises it to **20%** with no PAN. The s.194C(5) thresholds are a single payment above
  **₹30,000** or **₹1,00,000** in aggregate in a financial year.
- **CGST Act, 2017, s.51** — a specified deductor (a government department, a local authority
  and the other notified persons) deducts **2%** (1% CGST + 1% SGST, or 2% IGST) on a contract
  whose total value exceeds **₹2,50,000**, on the value of the supply excluding the GST on it.
- A works contract is a supply of service; **18% GST** is the rate most commonly charged to a
  non-government client, with concessional rates for some government and affordable-housing work.
- **Retention and defect liability** are contract terms. A CPWD-style agreement commonly
  withholds **5%** of the value of work done to a ceiling of **5%** of the contract value,
  released after a **12-month** defect-liability period. A **mobilisation advance** of about
  **10%** of the contract value is recovered out of the bills.

**Philippines**

- **Revised IRR of RA 9184, s.62.1** — **retention money of 10%** of every progress payment,
  building up to **10% of the total contract price**, on government infrastructure contracts.
- **Revised IRR of RA 9184, s.88.2.1** — **advance payment of up to 15%** of the total contract
  price, recouped by deduction from the progress payments.
- **Revised IRR of RA 9184, s.162.1** — a variation order is limited to **10%** of the original
  contract cost.
- **BIR Revenue Regulations 2-98 as amended** — **2% expanded withholding tax** on income
  payments to contractors.
- **NIRC s.108** — **12% VAT** on the sale of services; **s.114(C)** — a government payor
  withholds a further **5%** as creditable withholding VAT.

Every one of those figures is a **named constant in one place** — a single block in
`internal/rabill/terms.go` — and every one is editable under **Contract terms** in the
interface. They are **indicative defaults**: the rates most often written into a small civil
contract, not rates this tool knows apply to yours. **The contract agreement governs**, then
the work order, then the tax law in force. The interface says so beside each field, the printed
bill carries it as a footnote, and the bill tells you while the rates are still the ones the
tool shipped with. Confirm each against the current Act, Rules and notifications, or with your
own tax adviser, before you rely on it.

## Install and run

One binary, the Go standard library and nothing else. No database, no packages to fetch, no
build step for the interface.

```sh
# build it
git clone https://github.com/sreenivas-sadhu-prabhakara/rabill-running-account-bill-generator.git
cd rabill-running-account-bill-generator
go build -o rabill .

# run it, then open http://127.0.0.1:8080 in a browser
./rabill

# or run it straight from the source
go run .

# another port, and one file per contract
./rabill -addr 127.0.0.1:9000 -data ~/contracts/office-block-plot-14.json
```

| Flag    | Default                | What it is                                      |
| ------- | ---------------------- | ----------------------------------------------- |
| `-addr` | `127.0.0.1:8080`       | the address the interface is served on          |
| `-data` | `./rabill-data.json`   | the file one contract is kept in                |

It binds to `127.0.0.1` — the machine it is running on, not the network. One file holds one
contract, so run a second copy on another port for another job. Stop it with Ctrl-C.

## Where your data is stored

One JSON file on your own machine, holding the contract, the bill of quantities, the
measurement book and every certified bill.

- **No outbound network calls.** Not for fonts, not for analytics, not for updates. The
  interface is built into the binary with `embed` and served from it. The process opens one
  listening socket on `127.0.0.1` and nothing else.
- **It submits nothing.** The tool prepares a bill for you to print and hand over. It has no
  connection to any client portal, any bank or any tax authority, and it emails nothing.
- **Written whole, atomically** — a temporary file beside the contract, renamed over the old
  one, so an interrupted save leaves the previous contract intact and never half of one. The
  file is written readable only by you.
- **Yours to read** — plain indented JSON with amounts as decimals. The bill of quantities
  exports to CSV that imports back unchanged, and every bill exports to CSV.
- **Checked on the way in.** If the bills in the file do not form a running account, the tool
  refuses to start and says which line does not add up.

## Scope and accuracy

This is a bill preparer and a calculator — not advice, not a certificate of payment, and not a
filing.

- It works figures out from the bill of quantities and the measurements you record. It cannot
  know what you did not measure, and it cannot know what your contract says.
- Every deduction rate is an **indicative default** until you change it. The contract agreement
  governs.
- It computes no price escalation, no secured advance on materials at site, no interest, no
  liquidated damages and no labour cess. It does not decide place of supply, applies no s.194C
  or s.51 threshold of its own, and issues no certificate of deduction.
- Amounts are held as whole paise or centavos, quantities as whole thousandths and rates as
  whole hundredths of a percent, with every multiplication on a 128-bit intermediate. There is
  no floating point in the arithmetic. That removes rounding drift; it does not remove the need
  to check the figures.
- Check every quantity against the measurement book, every rate against the agreement, and
  every deduction rate against the agreement and the current tax law or your own adviser,
  before the bill leaves your hands.

## Layout

```
main.go                 flags, the embedded interface, the local server
api.go                  the JSON interface the page talks to
store.go                the contract file: atomic writes, frozen bills, protected items
internal/rabill/        the rules, as pure functions with table-driven tests
  money.go              exact money, quantity and percentage arithmetic
  date.go               calendar months that clamp to month ends, and days
  terms.go              every named constant, in one block
  boq.go                the bill of quantities
  measure.go            measurement-book entry and its quantity arithmetic
  bill.go               the cumulative-minus-previous engine
  series.go             the reconciliation: do the bills add up to the account?
  words.go              the amount in words, grouped per market
  csv.go                CSV in and out
web/                    the interface, embedded in the binary
docs/                   the explainer site
```

Run the tests with `go test ./...`.

MIT licensed.
