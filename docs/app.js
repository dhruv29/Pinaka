// Renders a Pinaka bill of materials. A bill is untrusted input (names in it
// come from whatever was scanned), so every piece of it reaches the page as
// text, never as markup, and anything not shaped like a bill is refused.
"use strict";

const KINDS = [
  ["model", "Models"],
  ["serving", "Model serving"],
  ["provider", "Hosted AI providers"],
  ["agent_tool", "Agent tooling"],
  ["library", "AI libraries"],
  ["data_store", "AI data stores"],
  ["dataset", "Datasets"],
  ["platform", "ML platforms"],
  ["accelerator", "Accelerators"],
  ["credential", "Credentials (names only)"],
];
const SEVERITIES = ["critical", "high", "medium", "low", "info"];
const ORIGIN_LABELS = { repo: "code", cluster: "cluster" };
const MAX_FILE_BYTES = 20 * 1024 * 1024;
const MAX_COMPONENTS = 20000;
const MAX_TEXT = 600;
const MAX_PLACES_SHOWN = 3;

const state = { bill: null, origin: "all", isSample: true };

/** Creates an element with text content and attributes; never parses HTML. */
function el(tag, options = {}, children = []) {
  const node = document.createElement(tag);
  if (options.text !== undefined) node.textContent = options.text;
  if (options.className) node.className = options.className;
  for (const [name, value] of Object.entries(options.attrs || {})) node.setAttribute(name, value);
  for (const child of children) node.append(child);
  return node;
}

function text(value) {
  return typeof value === "string" ? value.slice(0, MAX_TEXT) : "";
}

function severity(value) {
  return SEVERITIES.includes(value) ? value : "info";
}

/** Copies only the fields this page shows, with the types it expects. */
function cleanComponent(raw) {
  if (raw === null || typeof raw !== "object") return null;
  const name = text(raw.name);
  if (!name) return null;
  const list = (value) => (Array.isArray(value) ? value : []);
  return {
    kind: text(raw.kind),
    name,
    version: text(raw.version),
    origin: text(raw.origin),
    evidence: list(raw.evidence).filter((e) => e && typeof e === "object").map((e) => ({
      path: text(e.path),
      line: Number.isInteger(e.line) && e.line > 0 ? e.line : 0,
      detail: text(e.detail),
    })),
    risks: list(raw.risks).filter((r) => r && typeof r === "object").map((r) => ({
      severity: severity(r.severity),
      summary: text(r.summary),
    })),
  };
}

function cleanBill(raw) {
  if (raw === null || typeof raw !== "object" || !Array.isArray(raw.components)) {
    throw new Error("That file is not a Pinaka bill: it has no list of components.");
  }
  if (raw.components.length > MAX_COMPONENTS) {
    throw new Error("That bill has more components than this page will show.");
  }
  return {
    subject: text(raw.subject) || "Unnamed scan",
    components: raw.components.map(cleanComponent).filter(Boolean),
    notes: (Array.isArray(raw.notes) ? raw.notes : []).map(text).filter(Boolean),
  };
}

function place(evidence) {
  let out = evidence.path;
  if (evidence.line) out += ":" + evidence.line;
  if (evidence.detail) out += " (" + evidence.detail + ")";
  return out;
}

function places(component) {
  const shown = component.evidence.slice(0, MAX_PLACES_SHOWN).map(place);
  const more = component.evidence.length - shown.length;
  if (more > 0) shown.push("and " + more + " more");
  return shown.join("; ");
}

function flagged(bill) {
  const out = [];
  for (const component of bill.components) {
    for (const risk of component.risks) out.push({ risk, component });
  }
  return out.sort((a, b) =>
    SEVERITIES.indexOf(a.risk.severity) - SEVERITIES.indexOf(b.risk.severity) ||
    a.component.name.localeCompare(b.component.name));
}

function visible(bill) {
  return state.origin === "all" ? bill.components : bill.components.filter((c) => c.origin === state.origin);
}

function renderTally(bill) {
  const tally = document.getElementById("tally");
  const risks = flagged(bill);
  const serious = risks.filter((f) => f.risk.severity === "critical" || f.risk.severity === "high").length;
  const cells = [
    ["Components", bill.components.length, false],
    ["Models", bill.components.filter((c) => c.kind === "model").length, false],
    ["In code", bill.components.filter((c) => c.origin === "repo").length, false],
    ["Running", bill.components.filter((c) => c.origin === "cluster").length, false],
    ["Risks", risks.length, false],
    ["High or worse", serious, serious > 0],
  ];
  tally.replaceChildren(...cells.map(([label, count, alarm]) =>
    el("div", { className: alarm ? "alarm" : "" }, [el("dt", { text: label }), el("dd", { text: String(count) })])));
}

function renderRisks(bill) {
  const list = document.getElementById("risks");
  const risks = flagged(bill);
  if (risks.length === 0) {
    list.replaceChildren(el("li", { className: "empty", text: "Nothing was flagged." }));
    return;
  }
  list.replaceChildren(...risks.map(({ risk, component }) => {
    const label = component.version ? component.name + " " + component.version : component.name;
    const what = el("div", { className: "what" }, [el("span", { className: "sev", text: risk.severity }), label]);
    const item = el("li", { attrs: { "data-sev": risk.severity } }, [
      what,
      el("div", { className: "why", text: risk.summary }),
      el("div", { className: "where mono", text: component.evidence.length ? place(component.evidence[0]) : "" }),
    ]);
    return item;
  }));
}

function row(component) {
  const worst = component.risks.map((r) => r.severity).sort((a, b) => SEVERITIES.indexOf(a) - SEVERITIES.indexOf(b))[0];
  const name = el("td", { className: "name" });
  if (worst) name.append(el("span", { className: "dot", attrs: { "data-sev": worst, title: "Flagged: " + worst } }));
  name.append(component.name);
  return el("tr", {}, [
    name,
    el("td", { text: component.version || "–" }),
    el("td", {}, [el("span", { className: "origin", text: ORIGIN_LABELS[component.origin] || component.origin || "?" })]),
    el("td", { className: "seen mono", text: places(component) }),
  ]);
}

function renderSections(bill) {
  const host = document.getElementById("sections");
  const components = visible(bill);
  const known = new Set(KINDS.map(([kind]) => kind));
  const groups = KINDS.map(([kind, title]) => [title, components.filter((c) => c.kind === kind)]);
  groups.push(["Other", components.filter((c) => !known.has(c.kind))]);
  const sections = groups.filter(([, items]) => items.length > 0).map(([title, items]) => {
    const head = el("thead", {}, [el("tr", {}, ["Name", "Version", "Found in", "Seen at"].map((h) => el("th", { text: h, attrs: { scope: "col" } })))]);
    const table = el("table", {}, [head, el("tbody", {}, items.map(row))]);
    return el("section", { className: "kind" }, [
      el("h4", {}, [title, el("span", { text: String(items.length) })]),
      el("div", { className: "table-wrap" }, [table]),
    ]);
  });
  host.replaceChildren(...(sections.length ? sections : [el("p", { className: "empty", text: "Nothing from this source." })]));
}

function renderGaps(bill) {
  const section = document.getElementById("gaps-section");
  section.hidden = bill.notes.length === 0;
  document.getElementById("gaps").replaceChildren(...bill.notes.map((note) => el("li", { text: note })));
}

function render() {
  const bill = state.bill;
  document.getElementById("bill-heading").textContent = bill.subject;
  document.getElementById("reset").hidden = state.isSample;
  renderTally(bill);
  renderRisks(bill);
  renderSections(bill);
  renderGaps(bill);
}

function say(message) {
  document.getElementById("status").textContent = message;
}

async function openFile(file) {
  if (!file) return;
  try {
    if (file.size > MAX_FILE_BYTES) throw new Error("That file is too large to be a bill this page can show.");
    let parsed;
    try {
      parsed = JSON.parse(await file.text());
    } catch {
      throw new Error("That file is not JSON. Make one with: pinaka scan -format json");
    }
    state.bill = cleanBill(parsed);
    state.isSample = false;
    say("");
    render();
    document.getElementById("bill").scrollIntoView();
  } catch (error) {
    say(error.message);
  }
}

function showSample() {
  state.bill = cleanBill(window.PINAKA_SAMPLE);
  state.isSample = true;
  say("");
  render();
}

function wireFilters() {
  for (const chip of document.querySelectorAll(".chip")) {
    chip.addEventListener("click", () => {
      state.origin = chip.dataset.origin;
      for (const other of document.querySelectorAll(".chip")) {
        other.setAttribute("aria-pressed", String(other === chip));
      }
      renderSections(state.bill);
    });
  }
}

function wireDrop() {
  const overlay = document.getElementById("drop");
  let depth = 0;
  const carriesFile = (event) => Array.from(event.dataTransfer?.types || []).includes("Files");
  window.addEventListener("dragenter", (event) => {
    if (!carriesFile(event)) return;
    depth += 1;
    overlay.hidden = false;
  });
  window.addEventListener("dragleave", () => {
    depth = Math.max(0, depth - 1);
    overlay.hidden = depth === 0;
  });
  window.addEventListener("dragover", (event) => {
    if (carriesFile(event)) event.preventDefault();
  });
  window.addEventListener("drop", (event) => {
    if (!carriesFile(event)) return;
    event.preventDefault();
    depth = 0;
    overlay.hidden = true;
    openFile(event.dataTransfer.files[0]);
  });
}

document.getElementById("file").addEventListener("change", (event) => {
  openFile(event.target.files[0]);
  event.target.value = "";
});
document.getElementById("reset").addEventListener("click", showSample);
wireFilters();
wireDrop();
showSample();
