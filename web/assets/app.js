/* Running-account bill generator — the page's own behaviour.

   Every figure comes from the tool running on this machine; nothing else is
   contacted. The page builds its tables as text nodes, never as markup, so a
   description of work can never be read as anything but a description. */
(function () {
  "use strict";

  var MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

  var state = null;
  var editing = null; // the item code being edited, or null when adding

  /* ---------- small helpers ---------- */

  function $(id) { return document.getElementById(id); }

  function setText(node, s) { if (node) { node.textContent = s; } }

  function show(node, on) {
    if (!node) { return; }
    if (on) { node.removeAttribute("hidden"); } else { node.setAttribute("hidden", ""); }
  }

  function clear(node) { while (node && node.firstChild) { node.removeChild(node.firstChild); } }

  function symbol() { return (state && state.terms && state.terms.market === "PH") ? "₱" : "₹"; }

  function grouping() { return (state && state.terms && state.terms.market === "PH") ? "en-PH" : "en-IN"; }

  function money(v) {
    var n = Number(v) || 0;
    return symbol() + n.toLocaleString(grouping(), { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }

  function signedMoney(v) {
    var n = Number(v) || 0;
    if (n < 0) { return "(" + money(-n) + ")"; }
    return money(n);
  }

  function qty(v) {
    return (Number(v) || 0).toLocaleString(grouping(), { maximumFractionDigits: 3 });
  }

  function qtyUnit(v, unit) { return qty(v) + (unit ? " " + unit : ""); }

  function pct(v) { return (Number(v) || 0).toLocaleString("en-US", { maximumFractionDigits: 2 }) + "%"; }

  function dateText(iso) {
    if (!iso) { return "—"; }
    var parts = String(iso).split("-");
    if (parts.length !== 3) { return String(iso); }
    return Number(parts[2]) + " " + (MONTHS[Number(parts[1]) - 1] || parts[1]) + " " + parts[0];
  }

  function td(text, cls) {
    var cell = document.createElement("td");
    if (cls) { cell.className = cls; }
    cell.appendChild(document.createTextNode(text));
    return cell;
  }

  function th(text, cls) {
    var cell = document.createElement("th");
    if (cls) { cell.className = cls; }
    cell.appendChild(document.createTextNode(text));
    return cell;
  }

  function moneyCell(v, cls) {
    var n = Number(v) || 0;
    var cell = td(signedMoney(n), "num" + (cls ? " " + cls : ""));
    if (n < 0) { cell.classList.add("neg"); }
    return cell;
  }

  function qtyCell(v, unit, cls) {
    var n = Number(v) || 0;
    var cell = td(qtyUnit(n, unit), "num" + (cls ? " " + cls : ""));
    if (n < 0) { cell.classList.add("neg"); }
    return cell;
  }

  function rowButton(label, title, handler) {
    var button = document.createElement("button");
    button.type = "button";
    button.className = "rowbtn";
    button.setAttribute("aria-label", title);
    button.title = title;
    button.appendChild(document.createTextNode(label));
    button.addEventListener("click", handler);
    return button;
  }

  function message(node, text, problems) {
    if (!node) { return; }
    clear(node);
    if (!text) { show(node, false); return; }
    node.appendChild(document.createTextNode(text));
    if (problems && problems.length > 1) {
      var list = document.createElement("ul");
      problems.forEach(function (p) {
        var item = document.createElement("li");
        item.appendChild(document.createTextNode(p));
        list.appendChild(item);
      });
      node.appendChild(list);
    }
    show(node, true);
  }

  function banner(text, problems) { message($("banner"), text || "", problems); }

  /* ---------- talking to the tool ---------- */

  function api(method, url, body, contentType) {
    var options = { method: method, headers: {} };
    if (body !== undefined && body !== null) {
      options.headers["Content-Type"] = contentType || "application/json";
      options.body = contentType ? body : JSON.stringify(body);
    }
    return fetch(url, options).then(function (response) {
      return response.text().then(function (text) {
        var data = null;
        if (text) {
          try { data = JSON.parse(text); } catch (e) { data = null; }
        }
        if (!response.ok) {
          var err = new Error((data && data.error) || text || ("the tool answered " + response.status));
          err.problems = (data && data.problems) || null;
          throw err;
        }
        return data;
      });
    });
  }

  function refresh() {
    return api("GET", "/api/state").then(function (data) {
      state = data;
      render();
      return data;
    }).catch(function (err) {
      banner("Could not read the contract: " + err.message, err.problems);
    });
  }

  /* ---------- panels ---------- */

  function selectPanel(name) {
    var buttons = $("tabs").querySelectorAll("button");
    Array.prototype.forEach.call(buttons, function (b) {
      var on = b.getAttribute("data-panel") === name;
      b.setAttribute("aria-pressed", on ? "true" : "false");
      show($("panel-" + b.getAttribute("data-panel")), on);
    });
  }

  /* ---------- this bill ---------- */

  var MEMO_LINES = [
    { key: "valueOfWork", name: "Value of work done" },
    { key: "supplyTax", name: "Tax charged on the work" },
    { key: null, name: "Gross value of the bill", gross: true, rule: true },
    { key: "retentionWithheld", name: "Less retention withheld", minus: true },
    { key: "retentionReleased", name: "Add retention released" },
    { key: "advanceRecovery", name: "Less mobilisation-advance recovery", minus: true },
    { key: "incomeTaxTds", name: "Less income tax deducted at source", minus: true },
    { key: "supplyTaxTds", name: "Less deduction out of the tax on the work", minus: true },
    { key: "otherRecoveries", name: "Less other recoveries", minus: true },
    { key: "netPayable", name: "Net payable", total: true }
  ];

  function ledgerValue(ledger, line) {
    if (!ledger) { return 0; }
    if (line.gross) { return (Number(ledger.valueOfWork) || 0) + (Number(ledger.supplyTax) || 0); }
    return Number(ledger[line.key]) || 0;
  }

  function renderMemo(bill) {
    var body = $("b-memo");
    clear(body);
    if (!bill) { return; }

    MEMO_LINES.forEach(function (line) {
      var up = ledgerValue(bill.cumulative, line);
      var prev = ledgerValue(bill.previous, line);
      var now = ledgerValue(bill.thisBill, line);

      // A line nobody is using is left off the bill rather than printed as zero.
      if (!line.total && !line.gross && up === 0 && prev === 0 && now === 0) { return; }

      var tr = document.createElement("tr");
      if (line.total) { tr.className = "total"; }
      else if (line.minus) { tr.className = "sub"; }
      tr.appendChild(td(line.name));
      tr.appendChild(moneyCell(up));
      tr.appendChild(moneyCell(prev));
      tr.appendChild(moneyCell(now));
      body.appendChild(tr);
    });

    var notes = [];
    if (bill.contractValue) {
      notes.push("Progress " + pct(bill.progressPct) + " of a contract value of " + money(bill.contractValue) + ".");
    }
    if (bill.retentionHeld) {
      notes.push("Retention still held: " + money(bill.retentionHeld) + ".");
    }
    if (bill.terms && Number(bill.terms.advancePaid) > 0) {
      notes.push("Advance outstanding: " + money(bill.advanceOutstanding) + ".");
    }
    if (bill.paymentDueBy) {
      notes.push("On the terms entered, payment falls due by " + dateText(bill.paymentDueBy) + ".");
    }
    if (bill.retentionReleaseDue) {
      notes.push("The balance of retention falls due on " + dateText(bill.retentionReleaseDue) + ".");
    }
    setText($("b-memo-note"), notes.join(" "));
  }

  function renderBillItems(bill) {
    var body = $("b-items");
    var foot = $("b-items-foot");
    clear(body);
    clear(foot);

    var items = (bill && bill.items) || [];
    show($("b-items-table"), items.length > 0);
    show($("b-items-empty"), items.length === 0);
    if (items.length === 0) { return; }

    items.forEach(function (l) {
      var tr = document.createElement("tr");
      if (l.carried) { tr.className = "carried"; }

      var code = td(l.code);
      if (l.extra) {
        var tag = document.createElement("span");
        tag.className = "pill reg";
        tag.appendChild(document.createTextNode("extra"));
        code.appendChild(document.createTextNode(" "));
        code.appendChild(tag);
      }
      tr.appendChild(code);
      tr.appendChild(td(l.description, "desc"));
      tr.appendChild(td(l.unit));
      tr.appendChild(moneyCell(l.rate));
      tr.appendChild(qtyCell(l.previousQty, ""));
      tr.appendChild(qtyCell(l.presentQty, ""));
      tr.appendChild(qtyCell(l.upToDateQty, ""));
      tr.appendChild(moneyCell(l.previousAmount));
      tr.appendChild(moneyCell(l.presentAmount));
      tr.appendChild(moneyCell(l.upToDateAmount));
      body.appendChild(tr);
    });

    (bill.parts || []).forEach(function (p) {
      if ((bill.parts || []).length < 2) { return; }
      var tr = document.createElement("tr");
      tr.className = "sub";
      tr.appendChild(td(""));
      tr.appendChild(td(p.part ? p.part : "Other items", "desc"));
      tr.appendChild(td(""));
      tr.appendChild(td(""));
      tr.appendChild(td(""));
      tr.appendChild(td(""));
      tr.appendChild(td(""));
      tr.appendChild(moneyCell(p.previous));
      tr.appendChild(moneyCell(p.thisBill));
      tr.appendChild(moneyCell(p.upToDate));
      body.appendChild(tr);
    });

    var total = document.createElement("tr");
    total.appendChild(th("Value of work done"));
    total.appendChild(td("", "desc"));
    total.appendChild(td(""));
    total.appendChild(td(""));
    total.appendChild(td(""));
    total.appendChild(td(""));
    total.appendChild(td(""));
    total.appendChild(moneyCell(bill.previous.valueOfWork));
    total.appendChild(moneyCell(bill.thisBill.valueOfWork));
    total.appendChild(moneyCell(bill.cumulative.valueOfWork));
    foot.appendChild(total);
  }

  function renderFlags(bill) {
    var list = $("b-flags");
    clear(list);
    var flags = (bill && bill.flags) || [];
    show($("b-flags-card"), flags.length > 0);

    var words = { warn: "Needs attention", check: "Check this", note: "For information" };
    flags.forEach(function (f) {
      var li = document.createElement("li");
      li.className = f.level;
      var what = document.createElement("span");
      what.className = "what";
      what.appendChild(document.createTextNode(words[f.level] || f.level));
      li.appendChild(what);
      li.appendChild(document.createTextNode(f.message));
      list.appendChild(li);
    });
  }

  function renderBill() {
    var bill = state.openBill;
    var heading = $("bill-heading");

    if (state.closed) {
      setText(heading, "The account is closed");
      setText($("b-net"), "—");
      setText($("b-net-words"), "The final bill has been certified. Reopen it under Running account if something needs correcting.");
      renderMemo(null);
      renderBillItems(null);
      renderFlags(null);
      $("b-certify").disabled = true;
      return;
    }

    $("b-certify").disabled = false;

    if (!bill) {
      setText(heading, "RA bill " + state.openBillNo);
      setText($("b-net"), "—");
      setText($("b-net-words"), state.openError || "Nothing measured yet.");
      renderMemo(null);
      renderBillItems(null);
      renderFlags(null);
      if (state.openError) { banner("This bill cannot be drawn up yet: " + state.openError); }
      return;
    }

    var label = bill.final ? ("Final bill — RA bill " + bill.no) : ("RA bill " + bill.no);
    setText(heading, label);
    setText($("b-net"), signedMoney(bill.thisBill.netPayable));
    setText($("b-net-words"), bill.netInWords);

    renderMemo(bill);
    renderBillItems(bill);
    renderFlags(bill);
  }

  function renderDraftFields() {
    var d = state.draft || {};
    $("b-date").value = d.date || state.today;
    $("b-period-from").value = d.periodFrom || "";
    $("b-period-to").value = d.periodTo || "";
    $("b-other").value = Number(d.otherRecoveries) ? String(d.otherRecoveries) : "";
    $("b-released").value = Number(d.retentionReleased) ? String(d.retentionReleased) : "";
    $("b-note").value = d.note || "";
    $("b-final").checked = !!d.final;
    setText($("b-other-sym"), symbol());
    setText($("b-released-sym"), symbol());

    var no = state.openBillNo;
    $("b-print").href = "print.html?no=open";
    $("b-csv").href = "/api/bill.csv?no=open";
    $("b-print").textContent = "Printable RA bill " + no;
  }

  function saveDraft() {
    var body = {
      date: $("b-date").value || null,
      periodFrom: $("b-period-from").value || null,
      periodTo: $("b-period-to").value || null,
      final: $("b-final").checked,
      note: $("b-note").value,
      otherRecoveries: $("b-other").value || 0,
      retentionReleased: $("b-released").value || 0
    };
    setText($("b-saved"), "Saving…");
    return api("PUT", "/api/draft", body).then(function () {
      return refresh();
    }).then(function () {
      setText($("b-saved"), "Saved.");
    }).catch(function (err) {
      setText($("b-saved"), "");
      banner("Could not save: " + err.message, err.problems);
    });
  }

  /* ---------- measurements ---------- */

  function previewQuantity() {
    var nos = Number($("m-nos").value) || 0;
    var dims = [Number($("m-length").value) || 0, Number($("m-breadth").value) || 0, Number($("m-depth").value) || 0];

    var product = nos === 0 ? 1 : nos;
    var measured = nos !== 0;
    var shown = [];
    if (nos !== 0) { shown.push(String(nos)); }
    dims.forEach(function (d) {
      if (d !== 0) { product *= d; measured = true; shown.push(String(d)); }
    });

    if (!measured) {
      setText($("m-preview"), "—");
      setText($("m-preview-sub"), "Enter a number, or a length, or both.");
      return;
    }

    var rounded = Math.round(product * 1000) / 1000;
    if ($("m-deduct").checked) { rounded = -rounded; }

    var item = selectedItem();
    setText($("m-preview"), qtyUnit(rounded, item ? item.unit : ""));
    var working = shown.join(" × ");
    if ($("m-deduct").checked) { working = "deduct " + working; }
    setText($("m-preview-sub"), working + (shown.length > 1 ? " = " + qty(rounded) : ""));
  }

  function selectedItem() {
    var code = $("m-item").value;
    var found = null;
    (state.boq || []).forEach(function (i) { if (i.code === code) { found = i; } });
    return found;
  }

  function renderItemPicker() {
    var picker = $("m-item");
    var previous = picker.value;
    clear(picker);

    (state.boq || []).forEach(function (i) {
      var option = document.createElement("option");
      option.value = i.code;
      option.appendChild(document.createTextNode(i.code + " — " + i.description + " (" + i.unit + ")"));
      picker.appendChild(option);
    });
    if (previous) { picker.value = previous; }
    picker.disabled = (state.boq || []).length === 0;
  }

  function renderMeasuredTotals() {
    var body = $("m-totals");
    clear(body);
    var rows = (state.measured || []).filter(function (r) { return r.entries > 0; });
    show($("m-totals-table"), rows.length > 0);
    show($("m-totals-empty"), rows.length === 0);

    var byCode = {};
    (state.boq || []).forEach(function (i) { byCode[i.code] = i; });

    rows.forEach(function (r) {
      var item = byCode[r.code] || { unit: "" };
      var tr = document.createElement("tr");
      tr.appendChild(td(r.code));
      tr.appendChild(qtyCell(r.billed, item.unit));
      tr.appendChild(qtyCell(r.measured, item.unit));
      tr.appendChild(td(Number(item.contractQty) ? qtyUnit(item.contractQty, item.unit) : "—", "num quiet"));
      body.appendChild(tr);
    });
  }

  function measurementRow(m, withBill, withRemove) {
    var byCode = {};
    (state.boq || []).forEach(function (i) { byCode[i.code] = i; });
    var unit = (byCode[m.itemCode] || {}).unit || "";

    var product = m.nos || 0;
    var shown = [];
    if (m.nos) { shown.push(String(m.nos)); }
    [m.length, m.breadth, m.depth].forEach(function (d) { if (d) { shown.push(String(d)); } });
    product = shown.length ? shown.reduce(function (a, b) { return a * Number(b); }, 1) : 0;
    var quantity = Math.round(product * 1000) / 1000;
    if (m.deduct) { quantity = -quantity; }

    var tr = document.createElement("tr");
    if (withBill) { tr.appendChild(td(String(m.billNo || state.openBillNo))); }
    tr.appendChild(td(m.itemCode));
    tr.appendChild(td(m.where || (m.note || "—"), "desc"));
    tr.appendChild(td((m.deduct ? "deduct " : "") + shown.join(" × ")));
    tr.appendChild(qtyCell(quantity, unit));
    tr.appendChild(td(dateText(m.date)));
    if (withRemove) {
      var cell = document.createElement("td");
      cell.appendChild(rowButton("Remove", "Remove this measurement from the bill being drawn up", function () {
        api("DELETE", "/api/measurements?id=" + encodeURIComponent(m.id)).then(function () {
          banner("");
          return refresh();
        }).catch(function (err) { banner(err.message, err.problems); });
      }));
      tr.appendChild(cell);
    }
    return tr;
  }

  function renderMeasurements() {
    var openBody = $("m-open");
    var doneBody = $("m-done");
    clear(openBody);
    clear(doneBody);

    var all = state.measurements || [];
    var open = all.filter(function (m) { return !m.billNo; });
    var done = all.filter(function (m) { return !!m.billNo; });

    setText($("m-open-heading"), state.closed ? "The account is closed" : ("In RA bill " + state.openBillNo + ", being drawn up"));

    show($("m-open-table"), open.length > 0);
    show($("m-open-empty"), open.length === 0);
    open.forEach(function (m) { openBody.appendChild(measurementRow(m, false, true)); });

    show($("m-done-table"), done.length > 0);
    done.forEach(function (m) { doneBody.appendChild(measurementRow(m, true, false)); });

    $("m-add").disabled = state.closed || (state.boq || []).length === 0;
    renderMeasuredTotals();
  }

  function addMeasurement() {
    var body = {
      itemCode: $("m-item").value,
      where: $("m-where").value,
      page: $("m-page").value,
      date: $("m-date").value || null,
      nos: Number($("m-nos").value) || 0,
      length: Number($("m-length").value) || 0,
      breadth: Number($("m-breadth").value) || 0,
      depth: Number($("m-depth").value) || 0,
      deduct: $("m-deduct").checked,
      note: $("m-note").value
    };

    api("POST", "/api/measurements", body).then(function () {
      message($("m-problem"), "");
      ["m-nos", "m-length", "m-breadth", "m-depth", "m-note"].forEach(function (id) { $(id).value = ""; });
      $("m-deduct").checked = false;
      previewQuantity();
      return refresh();
    }).catch(function (err) {
      message($("m-problem"), err.message, err.problems);
    });
  }

  /* ---------- bill of quantities ---------- */

  function renderItems() {
    var body = $("i-rows");
    clear(body);
    var items = state.boq || [];
    show($("i-table"), items.length > 0);
    show($("i-empty"), items.length === 0);

    var measured = {};
    (state.measured || []).forEach(function (r) { measured[r.code] = r; });

    items.forEach(function (item) {
      var row = measured[item.code] || { measured: 0, billed: 0 };
      var contractAmount = (Number(item.contractQty) || 0) * (Number(item.rate) || 0);

      var tr = document.createElement("tr");
      var code = td(item.code);
      if (item.extra) {
        var tag = document.createElement("span");
        tag.className = "pill reg";
        tag.appendChild(document.createTextNode("extra"));
        code.appendChild(document.createTextNode(" "));
        code.appendChild(tag);
      }
      tr.appendChild(code);
      tr.appendChild(td(item.description, "desc"));
      tr.appendChild(td(item.unit));
      tr.appendChild(td(Number(item.contractQty) ? qty(item.contractQty) : "—", "num"));
      tr.appendChild(moneyCell(item.rate));
      tr.appendChild(td(Number(item.contractQty) ? money(contractAmount) : "—", "num"));
      tr.appendChild(qtyCell(row.measured, ""));
      tr.appendChild(qtyCell(row.billed, "", "quiet"));

      var actions = document.createElement("td");
      actions.appendChild(rowButton("Edit", "Edit item " + item.code, function () { startEdit(item); }));
      actions.appendChild(rowButton("Remove", "Remove item " + item.code, function () {
        api("DELETE", "/api/items?code=" + encodeURIComponent(item.code)).then(function () {
          banner("");
          return refresh();
        }).catch(function (err) { banner(err.message, err.problems); });
      }));
      tr.appendChild(actions);
      body.appendChild(tr);
    });

    setText($("i-count"), String(items.length));
    setText($("i-total"), money(state.boqTotal));
    setText($("i-contract"), money(state.contractValue));
    setText($("i-rate-sym"), symbol());
  }

  function startEdit(item) {
    editing = item.code;
    setText($("i-heading"), "Edit item " + item.code);
    $("i-code").value = item.code;
    $("i-part").value = item.part || "";
    $("i-desc").value = item.description || "";
    $("i-unit").value = item.unit || "";
    $("i-qty").value = Number(item.contractQty) ? String(item.contractQty) : "";
    $("i-rate").value = Number(item.rate) ? String(item.rate) : "";
    $("i-extra").checked = !!item.extra;
    setText($("i-save"), "Save the item");
    show($("i-cancel"), true);
    message($("i-problem"), "");
    $("i-code").focus();
  }

  function stopEdit() {
    editing = null;
    setText($("i-heading"), "Add an item");
    ["i-code", "i-part", "i-desc", "i-unit", "i-qty", "i-rate"].forEach(function (id) { $(id).value = ""; });
    $("i-extra").checked = false;
    setText($("i-save"), "Add the item");
    show($("i-cancel"), false);
    message($("i-problem"), "");
  }

  function saveItem() {
    var body = {
      code: $("i-code").value,
      part: $("i-part").value,
      description: $("i-desc").value,
      unit: $("i-unit").value,
      contractQty: $("i-qty").value || 0,
      rate: $("i-rate").value || 0,
      extra: $("i-extra").checked
    };
    var url = "/api/items";
    if (editing) { url += "?replacing=" + encodeURIComponent(editing); }

    api("POST", url, body).then(function () {
      stopEdit();
      banner("");
      return refresh();
    }).catch(function (err) {
      message($("i-problem"), err.message, err.problems);
    });
  }

  function importItems() {
    var file = $("i-file").files && $("i-file").files[0];
    if (!file) {
      message($("i-import-result"), "Choose a CSV file first.");
      return;
    }
    file.text().then(function (text) {
      return api("POST", "/api/items/import", text, "text/csv");
    }).then(function (result) {
      var said = [];
      if (result.added) { said.push(result.added + " item(s) added"); }
      if (result.updated) { said.push(result.updated + " updated"); }
      if (result.skipped) { said.push(result.skipped + " left alone"); }
      if (said.length === 0) { said.push("nothing came in"); }

      var problems = (result.errors || []).map(function (e) {
        return e.line ? ("line " + e.line + ": " + e.message) : e.message;
      });
      var text = said.join(", ") + ".";
      if (problems.length) { text += " " + problems.length + " row(s) could not be used:"; }
      message($("i-import-result"), text, problems.length ? problems : null);
      $("i-file").value = "";
      return refresh();
    }).catch(function (err) {
      message($("i-import-result"), err.message, err.problems);
    });
  }

  /* ---------- running account ---------- */

  function renderAccount() {
    var body = $("a-rows");
    clear(body);
    var bills = state.bills || [];
    show($("a-table"), bills.length > 0);
    show($("a-empty"), bills.length === 0);
    show($("a-reopen"), bills.length > 0);

    bills.forEach(function (b) {
      var tr = document.createElement("tr");
      var no = td("RA " + b.no + (b.final ? " (final)" : ""));
      tr.appendChild(no);
      tr.appendChild(td(dateText(b.date)));
      tr.appendChild(td(b.periodFrom || b.periodTo ? (dateText(b.periodFrom) + " – " + dateText(b.periodTo)) : "—"));
      tr.appendChild(moneyCell(b.thisBill.valueOfWork));
      tr.appendChild(moneyCell(b.cumulative.valueOfWork));
      tr.appendChild(moneyCell(b.thisBill.netPayable));

      var actions = document.createElement("td");
      var print = document.createElement("a");
      print.className = "rowbtn";
      print.href = "print.html?no=" + b.no;
      print.target = "_blank";
      print.rel = "noopener";
      print.appendChild(document.createTextNode("Print"));
      actions.appendChild(print);
      var csv = document.createElement("a");
      csv.className = "rowbtn";
      csv.href = "/api/bill.csv?no=" + b.no;
      csv.appendChild(document.createTextNode("CSV"));
      actions.appendChild(document.createTextNode(" "));
      actions.appendChild(csv);
      tr.appendChild(actions);
      body.appendChild(tr);
    });
  }

  function renderReconciliation() {
    var rec = state.reconciliation || { bills: 0, balanced: true, differences: [], items: [] };
    var rows = $("r-rows");
    clear(rows);

    if (!rec.bills) {
      setText($("r-verdict"), "Nothing to check yet");
      setText($("r-sub"), "The check runs once a bill has been certified.");
      show($("r-table"), false);
      show($("r-differences"), false);
      clear($("r-items"));
      show($("r-items-table"), false);
      show($("r-items-empty"), true);
      return;
    }

    setText($("r-verdict"), rec.balanced ? "The account adds up" : "The account does not add up");
    setText($("r-sub"), rec.bills + " bill(s) certified. " + (rec.balanced
      ? "Every line of every bill adds up to the account exactly."
      : "The differences are listed below and should be settled before anything goes to the client."));

    show($("r-table"), true);
    MEMO_LINES.forEach(function (line) {
      if (line.gross) { return; }
      var summed = ledgerValue(rec.sumOfBills, line);
      var upToDate = ledgerValue(rec.upToDate, line);
      if (!line.total && summed === 0 && upToDate === 0) { return; }

      // The memorandum names its lines "Less retention withheld"; the
      // reconciliation is not a memorandum, so it drops the "Less" and "Add"
      // and starts the line with a capital again.
      var name = line.name.replace(/^(Less|Add) /, "");
      name = name.charAt(0).toUpperCase() + name.slice(1);

      var tr = document.createElement("tr");
      if (line.total) { tr.className = "total"; }
      tr.appendChild(td(name));
      tr.appendChild(moneyCell(summed));
      tr.appendChild(moneyCell(upToDate));
      rows.appendChild(tr);
    });

    if (rec.differences && rec.differences.length) {
      message($("r-differences"), "What does not agree:", rec.differences);
    } else {
      show($("r-differences"), false);
    }

    var itemBody = $("r-items");
    clear(itemBody);
    var items = rec.items || [];
    show($("r-items-table"), items.length > 0);
    show($("r-items-empty"), items.length === 0);

    items.forEach(function (i) {
      var tr = document.createElement("tr");
      tr.appendChild(td(i.code));
      tr.appendChild(td(i.description, "desc"));
      tr.appendChild(td(i.unit));
      tr.appendChild(qtyCell(i.sumOfBillsQty, ""));
      tr.appendChild(qtyCell(i.upToDateQty, ""));
      tr.appendChild(td(String(i.bills), "num quiet"));
      var verdict = td(i.balanced ? "agrees" : "does not agree", i.balanced ? "pos" : "neg");
      tr.appendChild(verdict);
      itemBody.appendChild(tr);
    });
  }

  /* ---------- contract terms ---------- */

  function defaults() {
    var market = $("c-market").value || "IN";
    return (state.defaults && state.defaults[market]) || null;
  }

  function renderTermsHints() {
    var d = defaults();
    if (!d) { return; }
    var t = d.terms;
    var inIndia = d.market === "IN";

    setText($("c-market-hint"), "Named rules: " + d.regulation);
    setText($("c-retention-hint"), inIndia
      ? "Indicative: " + pct(t.retentionPct) + " of the value of work, to a ceiling of " + pct(t.retentionCapPct) + " of the contract value, which is the common CPWD-style term. The contract agreement governs."
      : "Indicative: " + pct(t.retentionPct) + " of every progress payment, to a ceiling of " + pct(t.retentionCapPct) + " of the contract price, per section 62.1 of the Revised IRR of RA 9184. The contract agreement governs.");
    setText($("c-advance-hint"), "Indicative: an advance of " + pct(d.advancePct) + " of the contract value" + (inIndia ? "" : ", the limit in section 88.2.1 of the Revised IRR") + ". Enter what was actually paid.");
    setText($("c-recovery-hint"), "Indicative: " + pct(t.advanceRecoveryPct) + " of each bill's value of work, until the advance is recovered in full. A final bill recovers whatever is left.");
    setText($("c-dlp-hint"), "Indicative: " + t.defectLiabilityMonths + " months from the final bill, after which the balance of retention falls due.");
    setText($("c-supply-tax-hint"), inIndia
      ? "Indicative: " + pct(t.supplyTaxPct) + " GST on a works contract to a non-government client. Concessional rates apply to some government and affordable-housing work."
      : "Indicative: " + pct(t.supplyTaxPct) + " VAT on the sale of services, under section 108 of the National Internal Revenue Code as amended.");
    setText($("c-income-tds-hint"), inIndia
      ? "Indicative: " + pct(t.incomeTaxTdsPct) + " under section 194C of the Income-tax Act where the payee is not an individual or a Hindu undivided family; " + pct(state.statutory.tds194cIndividualHufPct) + " where it is, and " + pct(state.statutory.tds206aaNoPanPct) + " under section 206AA with no PAN."
      : "Indicative: " + pct(t.incomeTaxTdsPct) + " expanded withholding tax on income payments to contractors, under Revenue Regulations 2-98 as amended.");
    setText($("c-supply-tds-hint"), inIndia
      ? "Indicative: " + pct(t.supplyTaxTdsPct) + " GST TDS under section 51 of the CGST Act, which a government department and the other notified deductors must deduct on a contract whose total value is more than " + money(state.statutory.gstTdsThresholdInr) + ". Leave it off for a private client."
      : "Indicative: " + pct(t.supplyTaxTdsPct) + " creditable withholding VAT, which a government payor withholds under section 114(C) of the National Internal Revenue Code. Leave it off for a private client.");
    setText($("c-variation-hint"), "Indicative: " + pct(t.variationLimitPct) + ", the limit in section 162.1 of the Revised IRR of RA 9184 and close to a CPWD-style deviation clause.");
  }

  function fillTerms(t) {
    $("c-market").value = t.market;
    $("c-contract-value").value = Number(t.contractValue) ? String(t.contractValue) : "";
    $("c-advance").value = Number(t.advancePaid) ? String(t.advancePaid) : "";
    $("c-retention").value = String(t.retentionPct);
    $("c-retention-cap").value = String(t.retentionCapPct);
    $("c-dlp").value = String(t.defectLiabilityMonths);
    $("c-payment-days").value = String(t.paymentDays);
    $("c-recovery-method").value = t.recoveryMethod;
    $("c-recovery-pct").value = String(t.advanceRecoveryPct);
    $("c-recovery-start").value = String(t.advanceRecoveryStartPct);
    $("c-recovery-end").value = String(t.advanceRecoveryEndPct);
    $("c-supply-tax").value = String(t.supplyTaxPct);
    $("c-income-tds").value = String(t.incomeTaxTdsPct);
    $("c-tds-base").value = t.incomeTaxTdsOn;
    $("c-supply-tds").value = String(t.supplyTaxTdsPct);
    $("c-supply-tds-applies").checked = !!t.supplyTaxTdsApplies;
    syncTermsVisibility();
    renderTermsHints();
  }

  function syncTermsVisibility() {
    var proRata = $("c-recovery-method").value === "pro-rata";
    show($("c-recovery-span"), proRata);
    show($("c-recovery-pct-field"), !proRata);
    show($("c-supply-tds-field"), $("c-supply-tds-applies").checked);

    var sym = $("c-market").value === "PH" ? "₱" : "₹";
    setText($("c-cv-sym"), sym);
    setText($("c-adv-sym"), sym);
  }

  function renderContract() {
    var p = state.project || {};
    $("c-work").value = p.work || "";
    $("c-client").value = p.client || "";
    $("c-contractor").value = p.contractor || "";
    $("c-agreement").value = p.agreementNo || "";
    $("c-order").value = p.workOrderNo || "";
    $("c-site").value = p.site || "";
    $("c-started").value = p.startedOn || "";
    $("c-due").value = p.dueOn || "";

    fillTerms(state.terms);
    setText($("c-file"), state.dataFile);

    var body = $("c-statutory");
    clear(body);
    var s = state.statutory;
    [
      ["Income-tax Act s.194C, payee an individual or a Hindu undivided family", pct(s.tds194cIndividualHufPct)],
      ["Income-tax Act s.194C, any other payee", pct(s.tds194cOtherPct)],
      ["Income-tax Act s.206AA, payee has furnished no PAN", pct(s.tds206aaNoPanPct)],
      ["s.194C(5) threshold, a single payment", "₹" + Number(s.tds194cSinglePaymentInr).toLocaleString("en-IN")],
      ["s.194C(5) threshold, in a financial year", "₹" + Number(s.tds194cAnnualInr).toLocaleString("en-IN")],
      ["CGST Act s.51, GST deducted at source", pct(s.gstTdsPct)],
      ["CGST Act s.51, contract-value threshold", "₹" + Number(s.gstTdsThresholdInr).toLocaleString("en-IN")],
      ["BIR RR 2-98 as amended, expanded withholding tax on contractors", pct(s.ewtContractorPctPh)],
      ["NIRC s.108, value-added tax on services", pct(s.vatPctPh)],
      ["NIRC s.114(C), withholding VAT by a government payor", pct(s.withholdingVatGovtPctPh)]
    ].forEach(function (pair) {
      var tr = document.createElement("tr");
      tr.appendChild(td(pair[0]));
      tr.appendChild(td(pair[1], "num"));
      body.appendChild(tr);
    });
  }

  function saveProject() {
    var body = {
      work: $("c-work").value,
      client: $("c-client").value,
      contractor: $("c-contractor").value,
      agreementNo: $("c-agreement").value,
      workOrderNo: $("c-order").value,
      site: $("c-site").value,
      startedOn: $("c-started").value || null,
      dueOn: $("c-due").value || null
    };
    api("PUT", "/api/project", body).then(function () {
      message($("c-project-problem"), "");
      banner("");
      return refresh();
    }).catch(function (err) {
      message($("c-project-problem"), err.message, err.problems);
    });
  }

  function saveTerms() {
    var body = {
      market: $("c-market").value,
      contractValue: $("c-contract-value").value || 0,
      retentionPct: $("c-retention").value || 0,
      retentionCapPct: $("c-retention-cap").value || 0,
      defectLiabilityMonths: Number($("c-dlp").value) || 0,
      advancePaid: $("c-advance").value || 0,
      recoveryMethod: $("c-recovery-method").value,
      advanceRecoveryPct: $("c-recovery-pct").value || 0,
      advanceRecoveryStartPct: $("c-recovery-start").value || 0,
      advanceRecoveryEndPct: $("c-recovery-end").value || 0,
      supplyTaxPct: $("c-supply-tax").value || 0,
      incomeTaxTdsPct: $("c-income-tds").value || 0,
      incomeTaxTdsOn: $("c-tds-base").value,
      supplyTaxTdsPct: $("c-supply-tds").value || 0,
      supplyTaxTdsApplies: $("c-supply-tds-applies").checked,
      variationLimitPct: $("c-variation").value || 0,
      paymentDays: Number($("c-payment-days").value) || 0
    };
    api("PUT", "/api/terms", body).then(function () {
      message($("c-terms-problem"), "");
      banner("");
      return refresh();
    }).catch(function (err) {
      message($("c-terms-problem"), err.message, err.problems);
    });
  }

  /* ---------- drawing everything ---------- */

  function render() {
    renderDraftFields();
    renderBill();
    renderItemPicker();
    renderMeasurements();
    renderItems();
    renderAccount();
    renderReconciliation();
    renderContract();
    previewQuantity();
    if (!$("m-date").value) { $("m-date").value = state.today; }
    setText($("i-rate-sym"), symbol());
  }

  /* ---------- the theme toggle ---------- */

  function wireTheme() {
    var root = document.documentElement;
    var button = document.querySelector("[data-themer]");
    if (!button) { return; }

    function stored() { try { return localStorage.getItem("omsme-theme"); } catch (e) { return null; } }
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
    if (window.matchMedia) {
      window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
        if (!stored()) { paint(); }
      });
    }
  }

  /* ---------- wiring ---------- */

  function wire() {
    wireTheme();

    Array.prototype.forEach.call($("tabs").querySelectorAll("button"), function (b) {
      b.addEventListener("click", function () { selectPanel(b.getAttribute("data-panel")); });
    });

    ["b-date", "b-period-from", "b-period-to", "b-other", "b-released", "b-note", "b-final"].forEach(function (id) {
      $(id).addEventListener("change", saveDraft);
    });

    $("b-certify").addEventListener("click", function () {
      api("POST", "/api/bills", {}).then(function (result) {
        banner("RA bill " + result.bill.no + " certified for " + signedMoney(result.bill.thisBill.netPayable) + ". The printable bill is under Running account.");
        return refresh();
      }).catch(function (err) { banner(err.message, err.problems); });
    });

    ["m-nos", "m-length", "m-breadth", "m-depth"].forEach(function (id) {
      $(id).addEventListener("input", previewQuantity);
    });
    $("m-deduct").addEventListener("change", previewQuantity);
    $("m-item").addEventListener("change", previewQuantity);
    $("m-add").addEventListener("click", addMeasurement);

    $("i-save").addEventListener("click", saveItem);
    $("i-cancel").addEventListener("click", stopEdit);
    $("i-import").addEventListener("click", importItems);

    $("a-reopen").addEventListener("click", function () {
      api("DELETE", "/api/bills").then(function (result) {
        banner("RA bill " + result.reopened + " has been reopened. Its measurements are back in the bill being drawn up.");
        return refresh();
      }).catch(function (err) { banner(err.message, err.problems); });
    });

    $("c-save-project").addEventListener("click", saveProject);
    $("c-save-terms").addEventListener("click", saveTerms);
    $("c-reset-terms").addEventListener("click", function () {
      var d = defaults();
      if (!d) { return; }
      var t = JSON.parse(JSON.stringify(d.terms));
      t.contractValue = $("c-contract-value").value || 0;
      t.advancePaid = $("c-advance").value || 0;
      t.supplyTaxTdsApplies = $("c-supply-tds-applies").checked;
      fillTerms(t);
      message($("c-terms-problem"), "These are the indicative defaults again. They are not saved until you save the terms.");
    });
    $("c-market").addEventListener("change", function () { syncTermsVisibility(); renderTermsHints(); });
    $("c-recovery-method").addEventListener("change", syncTermsVisibility);
    $("c-supply-tds-applies").addEventListener("change", syncTermsVisibility);
  }

  wire();
  refresh();
})();
