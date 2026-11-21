// Stack map: the bill drawn as layers, with lines between components that
// were seen in the same place. Loaded before app.js and called from its
// render(); it uses app.js helpers (el, svg, KINDS, SEVERITIES, ORIGIN_LABELS,
// kindOf, place, label, revealComponent, setState, syncControls,
// renderInventory) at call time. Everything from the bill goes in as text or
// as numeric SVG attributes; colour comes from classes in app.css.
"use strict";

const MAP_LAYERS = [
  { id: "agent", title: "Agent tooling", kinds: ["agent_tool"] },
  { id: "library", title: "AI libraries", kinds: ["library"] },
  { id: "model", title: "Models", kinds: ["model"] },
  { id: "serving", title: "Serving and providers", kinds: ["serving", "provider"] },
  { id: "data", title: "Data and platforms", kinds: ["data_store", "dataset", "platform", "other"] },
  { id: "accel", title: "Accelerators", kinds: ["accelerator"] },
];
const MAP_RAIL_KIND = "credential";
const MAP = {
  width: 1000, labelW: 150, mainX: 160, mainW: 650, railX: 846, railW: 144,
  top: 30, rowH: 78, nodeH: 42, gap: 10, maxNodeW: 150, railStep: 50,
};
const MAX_PER_LAYER = 6;
const MAX_RAIL = 9;
const MAX_GROUP_SIZE = 30;
const MAX_EDGES = 600;
const CHAR_W = 6.4;

const mapState = { selected: null, nodes: new Map(), edges: [], neighbours: new Map(), byUid: new Map() };

function truncate(value, width) {
  const max = Math.max(4, Math.floor(width / CHAR_W));
  return value.length > max ? value.slice(0, max - 1) + "…" : value;
}

/** Folder of a repo file when it sits at least two folders deep, e.g. services/search. */
function serviceFolder(path) {
  const parts = path.split("/").filter(Boolean);
  return parts.length >= 3 ? parts.slice(0, 2).join("/") : "";
}

/** Links components that share an evidence path (strong) or a repo service folder (weak). */
function deriveLinks(components) {
  const groups = new Map();
  const add = (key, component) => {
    const members = groups.get(key) || new Set();
    groups.set(key, new Set([...members, component.uid]));
  };
  for (const component of components) {
    for (const evidence of component.evidence) {
      if (!evidence.path) continue;
      add("same\u0000" + evidence.path, component);
      const folder = seenIn(component, "repo") ? serviceFolder(evidence.path) : "";
      if (folder) add("folder\u0000" + folder, component);
    }
  }
  const pairs = new Map();
  let skippedGroups = 0;
  for (const [key, members] of groups) {
    const [type, where] = key.split("\u0000");
    const uids = [...members].sort();
    if (uids.length < 2) continue;
    if (uids.length > MAX_GROUP_SIZE) { skippedGroups += 1; continue; }
    for (let i = 0; i < uids.length; i += 1) {
      for (let j = i + 1; j < uids.length; j += 1) {
        const id = uids[i] + "|" + uids[j];
        const prior = pairs.get(id) || { a: uids[i], b: uids[j], strong: false, places: [] };
        pairs.set(id, { ...prior, strong: prior.strong || type === "same", places: [...prior.places, where] });
      }
    }
  }
  return { links: [...pairs.values()], skippedGroups };
}

function layoutLayers(components) {
  const placed = new Map();
  const overflow = [];
  MAP_LAYERS.forEach((layer, row) => {
    const items = components.filter((c) => layer.kinds.includes(kindOf(c)))
      .sort((a, b) => (a.worst ?? 9) - (b.worst ?? 9) || a.name.localeCompare(b.name));
    const shown = items.length > MAX_PER_LAYER ? items.slice(0, MAX_PER_LAYER - 1) : items;
    const extra = items.length - shown.length;
    const slots = shown.length + (extra > 0 ? 1 : 0);
    const w = Math.min(MAP.maxNodeW, (MAP.mainW - (slots - 1) * MAP.gap) / Math.max(1, slots));
    const start = MAP.mainX + (MAP.mainW - (slots * w + (slots - 1) * MAP.gap)) / 2;
    const y = MAP.top + row * MAP.rowH + (MAP.rowH - MAP.nodeH) / 2;
    shown.forEach((c, i) => placed.set(c.uid, { x: start + i * (w + MAP.gap), y, w, h: MAP.nodeH, row }));
    if (extra > 0) overflow.push({ layer: layer.title, extra, x: start + shown.length * (w + MAP.gap), y, w });
  });
  const creds = components.filter((c) => c.kind === MAP_RAIL_KIND)
    .sort((a, b) => (a.worst ?? 9) - (b.worst ?? 9) || a.name.localeCompare(b.name));
  const railShown = creds.length > MAX_RAIL ? creds.slice(0, MAX_RAIL - 1) : creds;
  railShown.forEach((c, i) => placed.set(c.uid, { x: MAP.railX + 8, y: MAP.top + 8 + i * MAP.railStep, w: MAP.railW - 16, h: MAP.nodeH, row: -1 }));
  if (creds.length > railShown.length) {
    overflow.push({ layer: "Credentials", extra: creds.length - railShown.length, x: MAP.railX + 8, y: MAP.top + 8 + railShown.length * MAP.railStep, w: MAP.railW - 16 });
  }
  return { placed, overflow };
}

function edgePath(p, q) {
  const [a, b] = p.x <= q.x ? [p, q] : [q, p];
  const ax = a.x + a.w / 2, ay = a.y + a.h / 2, bx = b.x + b.w / 2, by = b.y + b.h / 2;
  if (a.row === -1 || b.row === -1) {
    const mid = (ax + bx) / 2;
    return `M${ax} ${ay} C${mid} ${ay} ${mid} ${by} ${bx} ${by}`;
  }
  if (a.row === b.row) {
    const lift = ay - MAP.nodeH / 2 - 14 - Math.min(18, Math.abs(bx - ax) / 20);
    return `M${ax} ${ay - a.h / 2} C${ax} ${lift} ${bx} ${lift} ${bx} ${by - b.h / 2}`;
  }
  const midY = (ay + by) / 2;
  return `M${ax} ${ay} C${ax} ${midY} ${bx} ${midY} ${bx} ${by}`;
}

function drawBands(height) {
  const bands = MAP_LAYERS.map((layer, row) => {
    const y = MAP.top + row * MAP.rowH;
    return svg("g", { class: "map-band" }, [
      svg("rect", { x: 0, y: y + 3, width: MAP.mainX + MAP.mainW, height: MAP.rowH - 6, rx: 8, class: "band-bg" }),
      svg("text", { x: 14, y: y + MAP.rowH / 2 + 4, class: "band-label" }, [layer.title]),
    ]);
  });
  const rail = svg("g", { class: "map-band" }, [
    svg("rect", { x: MAP.railX, y: MAP.top + 3, width: MAP.railW, height: height - MAP.top - 9, rx: 8, class: "band-bg band-rail" }),
    svg("text", { x: MAP.railX + 8, y: 18, class: "band-label" }, ["Credentials (names)"]),
  ]);
  return [...bands, rail];
}

function drawNode(component, box, degree) {
  const worst = component.worst === null ? null : SEVERITIES[component.worst];
  const sub = [ORIGIN_LABELS[component.origin] || "Unknown", component.version].filter(Boolean).join(" · ");
  const attrs = {
    class: "map-node", tabindex: 0, role: "button", "aria-pressed": "false", "data-origin": component.origin,
    "aria-label": label(component) + ", " + (ORIGIN_LABELS[component.origin] || "unknown source") +
      (worst ? ", " + worst + " risk" : "") + ", seen together with " + degree + (degree === 1 ? " other" : " others"),
  };
  if (worst) attrs["data-sev"] = worst;
  const children = [
    svg("title", {}, [label(component)]),
    svg("rect", { x: box.x, y: box.y, width: box.w, height: box.h, rx: 6, class: "node-box" }),
    svg("rect", { x: box.x, y: box.y + 6, width: 3, height: box.h - 12, rx: 1.5, class: "node-origin" }),
    svg("text", { x: box.x + 10, y: box.y + 17, class: "node-name" }, [truncate(component.name, box.w - (worst ? 26 : 16))]),
    svg("text", { x: box.x + 10, y: box.y + 32, class: "node-sub" }, [truncate(sub, box.w - 16)]),
  ];
  if (worst) children.push(svg("circle", { cx: box.x + box.w - 10, cy: box.y + 12, r: 4.5, class: "node-sev" }));
  return svg("g", attrs, children);
}

function applyHighlight(uid) {
  const canvas = document.getElementById("map-svg");
  if (!canvas) return;
  canvas.classList.toggle("is-focusing", Boolean(uid));
  const near = uid ? new Set([uid, ...(mapState.neighbours.get(uid) || [])]) : new Set();
  for (const [id, node] of mapState.nodes) {
    node.classList.toggle("is-near", near.has(id));
    node.classList.toggle("is-current", id === uid);
    node.setAttribute("aria-pressed", String(id === mapState.selected));
  }
  for (const edge of mapState.edges) {
    edge.node.classList.toggle("is-near", Boolean(uid) && (edge.a === uid || edge.b === uid));
  }
}

function filterToPlace(path) {
  setState({ origin: "all", severity: "any", kind: null, query: path });
  syncControls();
  renderKinds(state.bill);
  renderInventory(state.bill);
  document.getElementById("inventory-heading").scrollIntoView({ block: "start" });
}

function detailFor(component) {
  const neighbours = [...(mapState.neighbours.get(component.uid) || [])].map((id) => mapState.byUid.get(id)).filter(Boolean);
  const kindTitle = (KINDS.find(([k]) => k === component.kind) || [null, "Other"])[1];
  const meta = [kindTitle, ORIGIN_LABELS[component.origin] || "Unknown source", component.version].filter(Boolean).join(" · ");
  const places = component.evidence.map((e) => {
    const button = el("button", { className: "link-button mono", text: place(e), attrs: { type: "button", title: "Show everything seen at this place in the inventory" } });
    button.addEventListener("click", () => filterToPlace(e.path));
    return el("li", {}, [button]);
  });
  const together = neighbours.map((n) => {
    const button = el("button", { className: "chip-button", text: n.name, attrs: { type: "button" } });
    button.addEventListener("click", () => selectNode(n.uid, true));
    return el("li", {}, [button]);
  });
  const inventory = el("button", { className: "btn btn-small btn-ghost", text: "Show in inventory", attrs: { type: "button" } });
  inventory.addEventListener("click", () => revealComponent(component));
  return [
    el("h4", { className: "map-detail-title", text: component.name }),
    el("p", { className: "map-detail-meta", text: meta }),
    ...component.risks.map((r) => el("p", { className: "map-detail-risk" }, [
      el("span", { className: "sev-pill", attrs: { "data-sev": r.severity }, text: r.severity }), el("span", { text: r.summary })])),
    el("h5", { text: "Seen at" }),
    el("ul", { className: "map-detail-list" }, places.length ? places : [el("li", { text: "No location recorded." })]),
    el("h5", { text: "Seen together with" }),
    together.length ? el("ul", { className: "map-detail-chips" }, together) : el("p", { className: "panel-note", text: "Nothing else was seen in the same place." }),
    inventory,
  ];
}

function showDetail(uid) {
  const host = document.getElementById("map-detail");
  const component = uid ? mapState.byUid.get(uid) : null;
  host.replaceChildren(...(component ? detailFor(component) : [
    el("p", { className: "map-detail-empty", text: "Select a component to see where it was seen, why it is flagged, and what it was seen beside." }),
  ]));
}

function selectNode(uid, moveFocus) {
  mapState.selected = uid;
  applyHighlight(uid);
  showDetail(uid);
  if (moveFocus && uid) mapState.nodes.get(uid)?.focus();
}

function wireNode(node, uid) {
  node.addEventListener("mouseenter", () => applyHighlight(uid));
  node.addEventListener("mouseleave", () => applyHighlight(mapState.selected));
  node.addEventListener("focus", () => applyHighlight(uid));
  node.addEventListener("blur", () => applyHighlight(mapState.selected));
  node.addEventListener("click", () => selectNode(mapState.selected === uid ? null : uid, false));
  node.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      selectNode(mapState.selected === uid ? null : uid, false);
    }
  });
}

function renderMapText(components) {
  const lines = components.map((c) => {
    const names = [...(mapState.neighbours.get(c.uid) || [])].map((id) => mapState.byUid.get(id)?.name).filter(Boolean);
    return el("li", { text: c.name + (names.length ? " is seen together with " + names.join(", ") + "." : " is not seen together with anything else.") });
  });
  document.getElementById("map-text").replaceChildren(...lines);
}

function renderMap(bill) {
  const components = bill.components;
  const { links, skippedGroups } = deriveLinks(components);
  const { placed, overflow } = layoutLayers(components);
  const neighbours = new Map(components.map((c) => [c.uid, new Set()]));
  for (const link of links) {
    neighbours.get(link.a).add(link.b);
    neighbours.get(link.b).add(link.a);
  }
  mapState.selected = null;
  mapState.neighbours = neighbours;
  mapState.byUid = new Map(components.map((c) => [c.uid, c]));
  mapState.nodes = new Map();

  const railRows = Math.min(MAX_RAIL, components.filter((c) => c.kind === MAP_RAIL_KIND).length);
  const height = Math.max(MAP.top + MAP_LAYERS.length * MAP.rowH, MAP.top + 16 + railRows * MAP.railStep) + 6;
  const drawable = links.filter((l) => placed.has(l.a) && placed.has(l.b)).sort((x, y) => Number(y.strong) - Number(x.strong));
  mapState.edges = drawable.slice(0, MAX_EDGES).map((link) => ({
    a: link.a, b: link.b,
    node: svg("path", { d: edgePath(placed.get(link.a), placed.get(link.b)), class: "map-edge" + (link.strong ? " is-strong" : " is-folder") }),
  }));
  const nodes = components.filter((c) => placed.has(c.uid)).map((c) => {
    const node = drawNode(c, placed.get(c.uid), neighbours.get(c.uid).size);
    mapState.nodes.set(c.uid, node);
    wireNode(node, c.uid);
    return node;
  });
  const more = overflow.map((o) => svg("g", { class: "map-more" }, [
    svg("rect", { x: o.x, y: o.y, width: o.w, height: MAP.nodeH, rx: 6 }),
    svg("text", { x: o.x + o.w / 2, y: o.y + MAP.nodeH / 2 + 4, "text-anchor": "middle" }, ["+" + o.extra + " more"]),
  ]));
  const canvas = svg("svg", {
    id: "map-svg", class: "map-svg", viewBox: `0 0 ${MAP.width} ${height}`, "aria-labelledby": "map-heading map-note",
  }, [...drawBands(height), svg("g", { class: "map-edges" }, mapState.edges.map((e) => e.node)), ...nodes, ...more]);
  canvas.addEventListener("keydown", (event) => { if (event.key === "Escape") selectNode(null, false); });
  document.getElementById("map-canvas").replaceChildren(canvas);

  const caps = [];
  if (overflow.length) caps.push("Busy layers show their " + (MAX_PER_LAYER - 1) + " most at-risk components and a “+N more” box; lines to hidden components are not drawn.");
  if (skippedGroups) caps.push(skippedGroups + " place" + (skippedGroups === 1 ? " was" : "s were") + " shared by more than " + MAX_GROUP_SIZE + " components and not drawn as links.");
  if (drawable.length > MAX_EDGES) caps.push("Only the first " + MAX_EDGES + " of " + drawable.length + " links are drawn.");
  document.getElementById("map-caps").textContent = caps.join(" ");
  renderMapText(components);
  showDetail(null);
}
