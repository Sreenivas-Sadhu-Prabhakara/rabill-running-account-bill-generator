/* The printable running-account bill.

   It asks the tool on this machine for one bill and lays it out as a document
   to be signed. Everything is built as text nodes, so a description of work
   cannot be read as markup, and nothing is sent anywhere: the contractor prints
   this and submits it themselves. */
(function () {
  "use strict";

  var MONTHS = ["January", "February", "March", "April", "May", "June",
    "July", "August", "September", "October", "November", "December"];

  var market = "IN";

  function $(id) { return document.getElementById(id); }
  function setText(node, s) { if (node) { node.textContent = s; } }
  function show(node, on) {
    if (!node) { return; }
    if (on) { node.removeAttribute("hidden"); } else { node.setAttribute("hidden", ""); }
  }
  function clear(node) { while (node && node.firstChild) { node.removeChild(node.firstChild); } }

  function symbol() { return market === "PH" ? "₱" : "₹"; }
  function grouping() { return market === "PH" ? "en-PH" : "en-IN"; }

  function money(v) {
    var n = Number(v) || 0;
    var text = Math.abs(n).toLocaleString(grouping(), { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    return (n < 0 ? "(" : "") + symbol() + text + (n < 0 ? ")" : "");
  }
  function qty(v) { return (Number(v) || 0).toLocaleString(grouping(), { maximumFractionDigits: 3 }); }
  function pct(v) { return (Number(v) || 0).toLocaleString("en-US", { maximumFractionDigits: 2 }) + "%"; }

  function longDate(iso) {
    if (!iso) { return ""; }
    var parts = String(iso).split("-");
    if (parts.length !== 3) { return String(iso); }
    return Number(parts[2]) + " " + (MONTHS[Number(parts[1]) - 1] || parts[1]) + " " + parts[0];
  }

  function cell(tag, text, cls) {
    var node = document.createElement(tag);
    if (cls) { node.className = cls; }
    node.appendChild(document.createTextNode(text === undefined || text === null ? "" : text));
    return node;
  }
  function td(text, cls) { return cell("td", text, cls); }
  function th(text, cls) { return cell("th", text, cls); }

  function moneyCell(v) {
    var n = Number(v) || 0;
    var node = td(money(n), "num");
    if (n < 0) { node.classList.add("neg"); }
    return node;
  }
  function qtyCell(v) {
    var n = Number(v) || 0;
    var node = td(n === 0 ? "—" : qty(n), "num");
    if (n < 0) { node.classList.add("neg"); }
    return node;
  }

  function metaRow(body, key, value) {
    if (!value) { return; }
    var tr = document.createElement("tr");
    tr.appendChild(td(key, "k"));
    tr.appendChild(td(value, "v"));
    body.appendChild(tr);
  }

  var MEMO_LINES = [
    { key: "valueOfWork", name: "Value of work done" },
    { key: "supplyTax", name: "Add tax charged on the work" },
    { gross: true, name: "Gross value of the bill", rule: true },
    { key: "retentionWithheld", name: "Less retention / security withheld" },
    { key: "retentionReleased", name: "Add retention released" },
    { key: "advanceRecovery", name: "Less recovery of mobilisation advance" },
    { key: "incomeTaxTds", name: "Less income tax deducted at source" },
    { key: "supplyTaxTds", name: "Less deduction out of the tax on the work" },
    { key: "otherRecoveries", name: "Less other recoveries under the contract" },
    { key: "netPayable", name: "Net payable", net: true }
  ];

  function ledgerValue(ledger, line) {
    if (!ledger) { return 0; }
    if (line.gross) { return (Number(ledger.valueOfWork) || 0) + (Number(ledger.supplyTax) || 0); }
    return Number(ledger[line.key]) || 0;
  }

  function renderHeading(bill, project) {
    var title = bill.final ? ("Final bill — running account no. " + bill.no) : ("Running-account bill no. " + bill.no);
    setText($("p-title"), title);
    document.title = title + " · Open MSME";
    setText($("p-work"), project.work || "Name of work not entered");

    var body = $("p-meta");
    clear(body);
    metaRow(body, "Client / employer", project.client);
    metaRow(body, "Contractor", project.contractor);
    metaRow(body, "Agreement no.", project.agreementNo);
    metaRow(body, "Work-order no.", project.workOrderNo);
    metaRow(body, "Site", project.site);
    metaRow(body, "Bill date", longDate(bill.date));
    if (bill.periodFrom || bill.periodTo) {
      metaRow(body, "Period of work", longDate(bill.periodFrom) + " to " + longDate(bill.periodTo));
    }
    metaRow(body, "Contract value", bill.contractValue ? money(bill.contractValue) : "");
    metaRow(body, "Work done to date", money(bill.cumulative.valueOfWork) +
      (bill.contractValue ? " (" + pct(bill.progressPct) + " of the contract value)" : ""));
    metaRow(body, "Note", bill.note);

    setText($("p-sig-contractor"), project.contractor || "the contractor");
    setText($("p-sig-client"), project.client || "the client");
  }

  function renderItems(bill) {
    var body = $("p-items");
    var foot = $("p-items-foot");
    clear(body);
    clear(foot);

    var parts = {};
    (bill.parts || []).forEach(function (p) { parts[p.part] = p; });
    var manyParts = (bill.parts || []).length > 1;
    var lastPart = null;

    (bill.items || []).forEach(function (l) {
      if (manyParts && l.part !== lastPart) {
        lastPart = l.part;
        var head = document.createElement("tr");
        head.className = "part";
        var name = td(l.part ? l.part : "Other items");
        name.colSpan = 10;
        head.appendChild(name);
        body.appendChild(head);
      }

      var tr = document.createElement("tr");
      if (l.carried) { tr.className = "carried"; }
      tr.appendChild(td(l.code, "c-no"));

      var description = l.description;
      if (l.extra) { description += " [extra item — rate subject to approval]"; }
      if (l.excessQty && Number(l.excessQty) > 0) {
        description += " [" + qty(l.excessQty) + " " + l.unit + " over the contract quantity, " + pct(l.excessPct) + "]";
      }
      tr.appendChild(td(description));
      tr.appendChild(td(l.unit, "c-unit"));
      tr.appendChild(moneyCell(l.rate));
      tr.appendChild(qtyCell(l.previousQty));
      tr.appendChild(qtyCell(l.presentQty));
      tr.appendChild(qtyCell(l.upToDateQty));
      tr.appendChild(moneyCell(l.previousAmount));
      tr.appendChild(moneyCell(l.presentAmount));
      tr.appendChild(moneyCell(l.upToDateAmount));
      body.appendChild(tr);
    });

    if (manyParts) {
      (bill.parts || []).forEach(function (p) {
        var tr = document.createElement("tr");
        var name = td("Sub-total, " + (p.part ? p.part : "other items"));
        name.colSpan = 7;
        tr.appendChild(name);
        tr.appendChild(moneyCell(p.previous));
        tr.appendChild(moneyCell(p.thisBill));
        tr.appendChild(moneyCell(p.upToDate));
        foot.appendChild(tr);
      });
    }

    var total = document.createElement("tr");
    var label = th("Value of work done");
    label.colSpan = 7;
    total.appendChild(label);
    total.appendChild(moneyCell(bill.previous.valueOfWork));
    total.appendChild(moneyCell(bill.thisBill.valueOfWork));
    total.appendChild(moneyCell(bill.cumulative.valueOfWork));
    foot.appendChild(total);
  }

  function renderMemo(bill) {
    var body = $("p-memo");
    clear(body);

    MEMO_LINES.forEach(function (line) {
      var up = ledgerValue(bill.cumulative, line);
      var prev = ledgerValue(bill.previous, line);
      var now = ledgerValue(bill.thisBill, line);
      if (!line.net && !line.gross && up === 0 && prev === 0 && now === 0) { return; }

      var tr = document.createElement("tr");
      if (line.net) { tr.className = "net"; }
      else if (line.rule) { tr.className = "rule"; }
      tr.appendChild(td(line.name));
      tr.appendChild(moneyCell(up));
      tr.appendChild(moneyCell(prev));
      tr.appendChild(moneyCell(now));
      body.appendChild(tr);
    });

    setText($("p-net"), money(bill.thisBill.netPayable));
    setText($("p-words"), bill.netInWords);

    var dues = [];
    if (bill.retentionHeld) { dues.push("Retention still held: " + money(bill.retentionHeld) + "."); }
    if (bill.terms && Number(bill.terms.advancePaid) > 0) {
      dues.push("Mobilisation advance outstanding: " + money(bill.advanceOutstanding) + ".");
    }
    if (bill.paymentDueBy) { dues.push("Payment falls due by " + longDate(bill.paymentDueBy) + " on the terms recorded."); }
    if (bill.retentionReleaseDue) {
      dues.push("The balance of retention falls due on " + longDate(bill.retentionReleaseDue) +
        ", at the end of the " + bill.terms.defectLiabilityMonths + "-month defect-liability period.");
    }
    setText($("p-dues"), dues.join(" "));
  }

  function renderSheets(sheets) {
    var holder = $("p-sheets");
    clear(holder);
    show($("p-sheets-section"), (sheets || []).length > 0);

    (sheets || []).forEach(function (sheet) {
      var block = document.createElement("div");
      block.className = "sheet-item";

      var heading = document.createElement("h3");
      var code = document.createElement("span");
      code.className = "code";
      code.appendChild(document.createTextNode(sheet.code));
      heading.appendChild(code);
      heading.appendChild(document.createTextNode(sheet.description));
      block.appendChild(heading);

      var table = document.createElement("table");
      table.className = "acct";
      var head = document.createElement("thead");
      var headRow = document.createElement("tr");
      ["Page", "Where measured", "Nos", "Length", "Breadth", "Depth", "Quantity"].forEach(function (name, i) {
        headRow.appendChild(th(name, i >= 2 ? "num" : ""));
      });
      head.appendChild(headRow);
      table.appendChild(head);

      var body = document.createElement("tbody");
      (sheet.entries || []).forEach(function (m, i) {
        var tr = document.createElement("tr");
        tr.appendChild(td(m.page || "—"));
        var where = m.where || "";
        if (m.deduct) { where = "Deduct: " + where; }
        if (m.note) { where += (where ? " — " : "") + m.note; }
        tr.appendChild(td(where || "—"));
        [m.nos, m.length, m.breadth, m.depth].forEach(function (d) {
          tr.appendChild(td(Number(d) ? qty(d) : "", "num"));
        });
        tr.appendChild(qtyCell(sheet.quantities[i]));
        body.appendChild(tr);
      });
      table.appendChild(body);

      var foot = document.createElement("tfoot");
      var footRow = document.createElement("tr");
      var label = td("Total measured into this bill");
      label.colSpan = 6;
      footRow.appendChild(label);
      footRow.appendChild(td(qty(sheet.total) + " " + sheet.unit, "num"));
      foot.appendChild(footRow);
      table.appendChild(foot);

      block.appendChild(table);
      holder.appendChild(block);
    });
  }

  function renderFlags(bill) {
    var list = $("p-flags");
    clear(list);
    var flags = (bill.flags || []).filter(function (f) { return f.level !== "note"; });
    show($("p-flags-section"), flags.length > 0);
    flags.forEach(function (f) {
      var li = document.createElement("li");
      li.appendChild(document.createTextNode(f.message));
      list.appendChild(li);
    });
  }

  function renderRates(bill) {
    var body = $("p-rates");
    clear(body);
    var t = bill.terms;

    function rate(name, value) {
      var tr = document.createElement("tr");
      tr.appendChild(td(name));
      tr.appendChild(td(value, "num"));
      body.appendChild(tr);
    }

    rate("Retention / security withheld", pct(t.retentionPct) + " of the value of work done" +
      (Number(t.retentionCapPct) ? ", to a ceiling of " + pct(t.retentionCapPct) + " of the contract value" : ""));
    if (Number(t.advancePaid)) {
      rate("Mobilisation advance paid", money(t.advancePaid));
      rate("Recovered", t.recoveryMethod === "pro-rata"
        ? ("pro rata between " + pct(t.advanceRecoveryStartPct) + " and " + pct(t.advanceRecoveryEndPct) + " of progress")
        : (pct(t.advanceRecoveryPct) + " of the value of work done"));
    }
    if (Number(t.supplyTaxPct)) {
      rate(market === "PH" ? "Value-added tax charged" : "GST charged on the works contract", pct(t.supplyTaxPct));
    }
    if (Number(t.incomeTaxTdsPct)) {
      rate(market === "PH" ? "Expanded withholding tax" : "Income tax deducted at source (s.194C)",
        pct(t.incomeTaxTdsPct) + " of " + (t.incomeTaxTdsOn === "value-less-advance-recovery"
          ? "the value of work less the advance recovered" : "the value of work done"));
    }
    if (t.supplyTaxTdsApplies && Number(t.supplyTaxTdsPct)) {
      rate(market === "PH" ? "Creditable withholding VAT (NIRC s.114(C))" : "GST deducted at source (CGST s.51)", pct(t.supplyTaxTdsPct));
    }
    if (Number(t.defectLiabilityMonths)) {
      rate("Defect-liability period", t.defectLiabilityMonths + " months");
    }

    var note = "These are the rates recorded for this contract. ";
    note += bill.defaultRates
      ? "They are still the indicative defaults the tool ships with and have not been checked against the contract agreement."
      : "They have been entered for this contract.";
    note += " The contract agreement governs. Named rules: " + bill.regulation + ".";
    setText($("p-rates-note"), note);

    setText($("p-foot"), "Prepared on this contractor's own machine. Figures are worked out from the bill of quantities and " +
      "the measurement book recorded in it: the amount claimed on each item is its value up to date less the value the " +
      "previous bill was certified at. This is a bill for certification, not a certificate of payment and not tax advice.");
  }

  function wireTheme() {
    var root = document.documentElement;
    var button = document.querySelector("[data-themer]");
    if (!button) { return; }
    function current() {
      var t = root.getAttribute("data-theme");
      if (t) { return t; }
      return (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) ? "dark" : "light";
    }
    function paint() {
      var dark = current() === "dark";
      button.textContent = dark ? "☀ Light" : "☾ Dark";
      button.setAttribute("aria-label", dark ? "Switch to light theme" : "Switch to dark theme");
    }
    paint();
    button.addEventListener("click", function () {
      var next = current() === "dark" ? "light" : "dark";
      root.setAttribute("data-theme", next);
      try { localStorage.setItem("omsme-theme", next); } catch (e) { /* private browsing */ }
      paint();
    });
  }

  function load() {
    var asked = new URLSearchParams(window.location.search).get("no") || "open";
    var url = "/api/bill?no=" + encodeURIComponent(asked);
    $("p-csv").href = "/api/bill.csv?no=" + encodeURIComponent(asked);

    fetch(url).then(function (response) {
      return response.text().then(function (text) {
        var data = null;
        if (text) { try { data = JSON.parse(text); } catch (e) { data = null; } }
        if (!response.ok) {
          throw new Error((data && data.error) || ("the tool answered " + response.status));
        }
        return data;
      });
    }).then(function (data) {
      market = (data.bill.terms && data.bill.terms.market) || "IN";
      renderHeading(data.bill, data.project || {});
      renderItems(data.bill);
      renderMemo(data.bill);
      renderSheets(data.sheets);
      renderFlags(data.bill);
      renderRates(data.bill);
      show($("p-bill"), true);
      show($("p-problem"), false);
    }).catch(function (err) {
      var node = $("p-problem");
      clear(node);
      node.appendChild(document.createTextNode("This bill could not be prepared: " + err.message));
      show(node, true);
    });
  }

  wireTheme();
  $("p-print").addEventListener("click", function () { window.print(); });
  load();
})();
