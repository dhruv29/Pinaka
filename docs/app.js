// Renders a Pinaka bill of materials. A bill is untrusted input (names in it
// come from whatever was scanned), so every piece of it reaches the page as
// text through createElement/textContent, never as markup, and anything not
// shaped like a bill is refused before it is drawn.
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
const OTHER_KIND = "other";
const SEVERITIES = ["critical", "high", "medium", "low", "info"];
const ORIGIN_LABELS = { repo: "Code", cluster: "Cluster" };
const SEVERITY_FILTERS = {
  any: () => true,
  flagged: (c) => c.worst !== null,
  high: (c) => c.worst !== null && c.worst <= SEVERITIES.indexOf("high"),
  critical: (c) => c.worst === 0,
};
const MAX_FILE_BYTES = 20 * 1024 * 1024;
const MAX_COMPONENTS = 20000;
const MAX_TEXT = 600;
const MAX_EVIDENCE = 200;
const MAX_RISKS = 50;
const MAX_PROPERTIES = 40;
const MAX_NOTES = 200;
const MAX_PLACES_INLINE = 2;
const SVG_NS = "http://www.w3.org/2000/svg";

let state = { bill: null, origin: "all", query: "", severity: "any", kind: null, open: new Set(), isSample: true };

function setState(patch) {
  state = { ...state, ...patch };
}

/** Creates an element with text content and attributes; never parses HTML. */
function el(tag, options = {}, children = []) {
  const node = document.createElement(tag);
  if (options.text !== undefined) node.textContent = options.text;
  if (options.className) node.className = options.className;
  for (const [name, value] of Object.entries(options.attrs || {})) node.setAttribute(name, value);
  for (const child of children) node.append(child);
  return node;
}

/** Same for SVG; geometry goes in attributes, colour comes from the stylesheet. */
function svg(tag, attrs = {}, children = []) {
  const node = document.createElementNS(SVG_NS, tag);
  for (const [name, value] of Object.entries(attrs)) node.setAttribute(name, String(value));
  for (const child of children) node.append(child);
  return node;
}

function text(value) {
  return typeof value === "string" ? value.slice(0, MAX_TEXT) : "";
}

function severity(value) {
  return SEVERITIES.includes(value) ? value : "info";
}

function list(value, cap) {
  return (Array.isArray(value) ? value : []).slice(0, cap).filter((item) => item && typeof item === "object");
}

function cleanProperties(raw) {
  if (raw === null || typeof raw !== "object" || Array.isArray(raw)) return [];
  return Object.entries(raw).slice(0, MAX_PROPERTIES)
    .filter(([, value]) => ["string", "number", "boolean"].includes(typeof value))
    .map(([key, value]) => [text(key), text(String(value))])
    .filter(([key]) => key);
}

/** Copies only the fields this page shows, with the types it expects. */
function cleanComponent(raw, index) {
  if (raw === null || typeof raw !== "object") return null;
  const name = text(raw.name);
  if (!name) return null;
  const risks = list(raw.risks, MAX_RISKS).map((r) => ({ id: text(r.id), severity: severity(r.severity), summary: text(r.summary) }));
  const ranks = risks.map((r) => SEVERITIES.indexOf(r.severity));
  return {
    uid: "c" + index,
    kind: text(raw.kind),
    name,
    version: text(raw.version),
    supplier: text(raw.supplier),
    ecosystem: text(raw.ecosystem),
    origin: text(raw.origin),
    evidence: list(raw.evidence, MAX_EVIDENCE).map((e) => ({
      path: text(e.path),
      line: Number.isInteger(e.line) && e.line > 0 ? e.line : 0,
      detail: text(e.detail),
    })),
    risks,
    worst: ranks.length ? Math.min(...ranks) : null,
    properties: cleanProperties(raw.properties),
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
    notes: (Array.isArray(raw.notes) ? raw.notes : []).slice(0, MAX_NOTES).map(text).filter(Boolean),
  };
}

/* ---------- derived views ---------- */

const KNOWN_KINDS = new Set(KINDS.map(([kind]) => kind));

function kindOf(component) {
  return KNOWN_KINDS.has(component.kind) ? component.kind : OTHER_KIND;
}

function place(evidence) {
  let out = evidence.path || "(no path)";
  if (evidence.line) out += ":" + evidence.line;
  if (evidence.detail) out += "  ·  " + evidence.detail;
  return out;
}

function label(component) {
  return component.version ? component.name + " " + component.version : component.name;
}

function flagged(bill) {
  return bill.components
    .flatMap((component) => component.risks.map((risk) => ({ risk, component })))
    .sort((a, b) =>
      SEVERITIES.indexOf(a.risk.severity) - SEVERITIES.indexOf(b.risk.severity) ||
      a.component.name.localeCompare(b.component.name));
}

function haystack(component) {
  return [
    component.name, component.version, component.supplier, component.ecosystem, component.kind,
    ...component.evidence.flatMap((e) => [e.path, e.detail]),
    ...component.risks.flatMap((r) => [r.id, r.summary]),
  ].join("\n").toLowerCase();
}

function matches(component) {
  if (state.origin !== "all" && component.origin !== state.origin) return false;
  if (state.kind && kindOf(component) !== state.kind) return false;
  if (!SEVERITY_FILTERS[state.severity](component)) return false;
  const query = state.query.trim().toLowerCase();
  return !query || haystack(component).includes(query);
}

function countBy(items, key) {
  return items.reduce((acc, item) => ({ ...acc, [key(item)]: (acc[key(item)] || 0) + 1 }), {});
}

/* ---------- summary tiles ---------- */

function severityStack(risks) {
  const counts = countBy(risks, (f) => f.risk.severity);
  const total = risks.length || 1;
  let x = 0;
  const rects = SEVERITIES.filter((s) => counts[s]).map((s) => {
    const width = (counts[s] / total) * 100;
    const rect = svg("rect", { x, y: 0, width, height: 6, class: "fill", "data-sev": s });
    x += width;
    return rect;
  });
  return svg("svg", { class: "stack", viewBox: "0 0 100 6", preserveAspectRatio: "none", "aria-hidden": "true" }, rects);
}

function tile(title, value, sub, extra = {}) {
  const dd = el("dd", {}, [el("span", { className: "tile-value", text: String(value) })]);
  if (sub) dd.append(el("span", { className: "tile-sub", text: sub }));
  if (extra.chart) dd.append(extra.chart);
  return el("div", { className: "tile" + (extra.tone ? " tile-" + extra.tone : "") }, [el("dt", { text: title }), dd]);
}

function renderTally(bill) {
  const risks = flagged(bill);
  const counts = countBy(risks, (f) => f.risk.severity);
  const serious = (counts.critical || 0) + (counts.high || 0);
  const kinds = new Set(bill.components.map(kindOf)).size;
  const bySeverity = SEVERITIES.filter((s) => counts[s]).map((s) => counts[s] + " " + s).join(", ");
  document.getElementById("tally").replaceChildren(
    tile("Components", bill.components.length, kinds + (kinds === 1 ? " kind" : " kinds")),
    tile("Models", bill.components.filter((c) => c.kind === "model").length, "named or files"),
    tile("Risks flagged", risks.length, bySeverity || "none", { chart: severityStack(risks) }),
    tile("High or worse", serious, serious ? "look at these first" : "nothing serious", { tone: serious ? "alarm" : "calm" }),
  );
}

/* ---------- risk queue ---------- */

function revealComponent(component) {
  setState({ origin: "all", query: "", severity: "any", kind: null, open: new Set([...state.open, component.uid]) });
  syncControls();
  renderKinds(state.bill);
  renderInventory(state.bill);
  const toggle = document.getElementById("toggle-" + component.uid);
  if (!toggle) return;
  toggle.scrollIntoView({ block: "center", behavior: prefersReducedMotion() ? "auto" : "smooth" });
  toggle.focus({ preventScroll: true });
}

function renderRisks(bill) {
  const host = document.getElementById("risks");
  const risks = flagged(bill);
  if (risks.length === 0) {
    host.replaceChildren(el("li", { className: "empty", text: "Nothing was flagged in this bill." }));
    return;
  }
  host.replaceChildren(...risks.map(({ risk, component }) => {
    const target = el("button", { className: "risk-target", text: label(component), attrs: { type: "button" } });
    target.addEventListener("click", () => revealComponent(component));
    const meta = el("p", { className: "risk-meta" }, [
      el("span", { className: "origin-tag", attrs: { "data-origin": component.origin }, text: ORIGIN_LABELS[component.origin] || "Unknown" }),
      el("code", { className: "mono", text: component.evidence.length ? place(component.evidence[0]) : "no location recorded" }),
    ]);
    return el("li", { className: "risk", attrs: { "data-sev": risk.severity } }, [
      el("span", { className: "sev-pill", attrs: { "data-sev": risk.severity }, text: risk.severity }),
      el("div", { className: "risk-body" }, [target, el("p", { className: "risk-why", text: risk.summary }), meta]),
    ]);
  }));
}

/* ---------- breakdown ---------- */

function uniquePlaces(components) {
  return new Set(components.flatMap((c) => c.evidence.map((e) => e.path)).filter(Boolean)).size;
}

function renderWhere(bill) {
  const code = bill.components.filter((c) => c.origin === "repo");
  const cluster = bill.components.filter((c) => c.origin === "cluster");
  const total = code.length + cluster.length || 1;
  const codeWidth = (code.length / total) * 100;
  const bar = svg("svg", { class: "split", viewBox: "0 0 100 10", preserveAspectRatio: "none", "aria-hidden": "true" }, [
    svg("rect", { x: 0, y: 0, width: codeWidth, height: 10, class: "split-code" }),
    svg("rect", { x: codeWidth, y: 0, width: 100 - codeWidth, height: 10, class: "split-cluster" }),
  ]);
  const side = (name, items, places, unit) => el("div", { className: "where-side", attrs: { "data-origin": name } }, [
    el("span", { className: "where-label", text: ORIGIN_LABELS[name] }),
    el("span", { className: "where-value", text: String(items.length) }),
    el("span", { className: "where-sub", text: places + " " + unit }),
  ]);
  document.getElementById("where").replaceChildren(
    el("div", { className: "where-row" }, [
      side("repo", code, uniquePlaces(code), "files"),
      side("cluster", cluster, uniquePlaces(cluster), "workloads"),
    ]),
    bar,
  );
}

function renderKinds(bill) {
  const counts = countBy(bill.components, kindOf);
  const rows = [...KINDS, [OTHER_KIND, "Other"]].filter(([kind]) => counts[kind]);
  const max = Math.max(1, ...rows.map(([kind]) => counts[kind]));
  document.getElementById("kinds").replaceChildren(...rows.map(([kind, title]) => {
    const pressed = state.kind === kind;
    const button = el("button", { className: "kind-bar", attrs: { type: "button", "aria-pressed": String(pressed) } }, [
      el("span", { className: "kind-name", text: title }),
      el("span", { className: "kind-count", text: String(counts[kind]) }),
      svg("svg", { class: "kind-track", viewBox: "0 0 100 4", preserveAspectRatio: "none", "aria-hidden": "true" }, [
        svg("rect", { x: 0, y: 0, width: 100, height: 4, class: "track" }),
        svg("rect", { x: 0, y: 0, width: (counts[kind] / max) * 100, height: 4, class: "fill" }),
      ]),
    ]);
    button.addEventListener("click", () => {
      setState({ kind: pressed ? null : kind });
      renderKinds(state.bill);
      renderInventory(state.bill);
    });
    return el("li", {}, [button]);
  }));
}

/* ---------- inventory ---------- */

function detailRow(component, columns) {
  const blocks = [];
  blocks.push(el("div", { className: "detail-block" }, [
    el("h5", { text: "Evidence (" + component.evidence.length + ")" }),
    el("ul", { className: "evidence" }, component.evidence.length
      ? component.evidence.map((e) => el("li", { className: "mono", text: place(e) }))
      : [el("li", { text: "No location recorded." })]),
  ]));
  if (component.risks.length) {
    blocks.push(el("div", { className: "detail-block" }, [
      el("h5", { text: "Why it is flagged" }),
      el("ul", { className: "detail-risks" }, component.risks.map((r) => el("li", {}, [
        el("span", { className: "sev-pill", attrs: { "data-sev": r.severity }, text: r.severity }),
        el("span", { text: r.summary }),
        r.id ? el("code", { className: "mono risk-id", text: r.id }) : "",
      ]))),
    ]));
  }
  const facts = [["Supplier", component.supplier], ["Ecosystem", component.ecosystem], ...component.properties].filter(([, v]) => v);
  if (facts.length) {
    blocks.push(el("div", { className: "detail-block" }, [
      el("h5", { text: "Details" }),
      el("dl", { className: "facts-list" }, facts.flatMap(([k, v]) => [el("dt", { text: k }), el("dd", { text: v })])),
    ]));
  }
  const cell = el("td", { attrs: { colspan: String(columns) } }, [el("div", { className: "detail" }, blocks)]);
  return el("tr", { className: "detail-row", attrs: { id: "detail-" + component.uid } }, [cell]);
}

function componentRows(component) {
  const isOpen = state.open.has(component.uid);
  const toggle = el("button", {
    className: "row-toggle",
    attrs: { type: "button", id: "toggle-" + component.uid, "aria-expanded": String(isOpen), "aria-controls": "detail-" + component.uid },
  }, [el("span", { className: "chevron", attrs: { "aria-hidden": "true" } }), el("span", { text: component.name })]);
  toggle.addEventListener("click", () => {
    const open = new Set(state.open);
    if (open.has(component.uid)) open.delete(component.uid); else open.add(component.uid);
    setState({ open });
    renderInventory(state.bill);
    document.getElementById("toggle-" + component.uid)?.focus();
  });
  const worst = component.worst === null ? null : SEVERITIES[component.worst];
  const inline = component.evidence.slice(0, MAX_PLACES_INLINE).map((e) => e.path + (e.line ? ":" + e.line : "")).join(", ");
  const more = component.evidence.length - MAX_PLACES_INLINE;
  const row = el("tr", { className: "row" + (isOpen ? " is-open" : ""), attrs: worst ? { "data-sev": worst } : {} }, [
    el("td", { className: "cell-name", attrs: { "data-label": "Name" } }, [toggle]),
    el("td", { className: "mono", attrs: { "data-label": "Version" }, text: component.version || "–" }),
    el("td", { attrs: { "data-label": "Found in" } }, [
      el("span", { className: "origin-tag", attrs: { "data-origin": component.origin }, text: ORIGIN_LABELS[component.origin] || component.origin || "Unknown" }),
    ]),
    el("td", { className: "mono cell-seen", attrs: { "data-label": "Seen at" }, text: inline + (more > 0 ? " +" + more + " more" : "") }),
    el("td", { attrs: { "data-label": "Risk" } }, [
      worst ? el("span", { className: "sev-pill", attrs: { "data-sev": worst }, text: worst }) : el("span", { className: "muted", text: "–" }),
    ]),
  ]);
  return isOpen ? [row, detailRow(component, 5)] : [row];
}

function renderInventory(bill) {
  const shown = bill.components.filter(matches);
  const groups = [...KINDS, [OTHER_KIND, "Other"]]
    .map(([kind, title]) => [kind, title, shown.filter((c) => kindOf(c) === kind)])
    .filter(([, , items]) => items.length > 0);
  document.getElementById("shown").textContent = "Showing " + shown.length + " of " + bill.components.length;
  const host = document.getElementById("groups");
  if (groups.length === 0) {
    const clear = el("button", { className: "btn btn-small btn-ghost", text: "Clear filters", attrs: { type: "button" } });
    clear.addEventListener("click", clearFilters);
    host.replaceChildren(el("div", { className: "empty" }, [el("p", { text: "Nothing matches these filters." }), clear]));
    return;
  }
  host.replaceChildren(...groups.map(([kind, title, items]) => {
    const head = el("thead", {}, [el("tr", {}, ["Name", "Version", "Found in", "Seen at", "Risk"]
      .map((h) => el("th", { text: h, attrs: { scope: "col" } })))]);
    const table = el("table", { className: "inv-table" }, [head, el("tbody", {}, items.flatMap(componentRows))]);
    return el("section", { className: "group", attrs: { "aria-label": title } }, [
      el("h4", { className: "group-title" }, [el("span", { text: title }), el("span", { className: "group-count", text: String(items.length) })]),
      el("div", { className: "table-wrap" }, [table]),
    ]);
  }));
}

function renderGaps(bill) {
  document.getElementById("gaps-section").hidden = bill.notes.length === 0;
  document.getElementById("gaps").replaceChildren(...bill.notes.map((note) => el("li", { text: note })));
}

/* ---------- controls ---------- */

function syncControls() {
  for (const seg of document.querySelectorAll(".seg")) {
    seg.setAttribute("aria-pressed", String(seg.dataset.origin === state.origin));
  }
  document.getElementById("query").value = state.query;
  document.getElementById("severity").value = state.severity;
}

function clearFilters() {
  setState({ origin: "all", query: "", severity: "any", kind: null });
  syncControls();
  renderKinds(state.bill);
  renderInventory(state.bill);
}

function render() {
  const bill = state.bill;
  document.getElementById("bill-heading").textContent = bill.subject;
  document.getElementById("source-badge").textContent = state.isSample ? "Sample" : "Your file";
  document.getElementById("reset").hidden = state.isSample;
  syncControls();
  renderTally(bill);
  renderRisks(bill);
  renderWhere(bill);
  renderKinds(bill);
  renderMap(bill);
  renderInventory(bill);
  renderGaps(bill);
}

function say(message) {
  document.getElementById("status").textContent = message;
}

function prefersReducedMotion() {
  return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
}

function load(bill, isSample) {
  setState({ bill, isSample, origin: "all", query: "", severity: "any", kind: null, open: new Set() });
  render();
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
    load(cleanBill(parsed), false);
    say("Showing " + file.name.slice(0, 120) + ". It was read in this tab and not sent anywhere.");
    document.getElementById("bill").scrollIntoView();
  } catch (error) {
    say(error.message);
  }
}

function showSample() {
  try {
    load(cleanBill(window.PINAKA_SAMPLE), true);
    say("");
  } catch (error) {
    say("The sample bill could not be shown: " + error.message);
  }
}

function wireControls() {
  for (const seg of document.querySelectorAll(".seg")) {
    seg.addEventListener("click", () => {
      setState({ origin: seg.dataset.origin });
      syncControls();
      renderInventory(state.bill);
    });
  }
  document.getElementById("query").addEventListener("input", (event) => {
    setState({ query: event.target.value.slice(0, 200) });
    renderInventory(state.bill);
  });
  document.getElementById("severity").addEventListener("change", (event) => {
    setState({ severity: Object.hasOwn(SEVERITY_FILTERS, event.target.value) ? event.target.value : "any" });
    renderInventory(state.bill);
  });
  document.getElementById("file").addEventListener("change", (event) => {
    openFile(event.target.files[0]);
    event.target.value = "";
  });
  document.getElementById("reset").addEventListener("click", showSample);
}

function wireCopy() {
  const button = document.getElementById("copy");
  const command = document.getElementById("command-text");
  const done = (message) => {
    button.textContent = message;
    window.setTimeout(() => { button.textContent = "Copy"; }, 1600);
  };
  button.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(command.textContent);
      done("Copied");
    } catch {
      const range = document.createRange();
      range.selectNodeContents(command);
      window.getSelection().removeAllRanges();
      window.getSelection().addRange(range);
      done("Selected");
    }
  });
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

wireControls();
wireCopy();
wireDrop();
showSample();
