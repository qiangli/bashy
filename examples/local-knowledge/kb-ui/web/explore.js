// kg-ui explore tab — search + 3d-force-graph (with 2D fallback) +
// per-hit enrichment dots + node detail panel. Uses ForceGraph3D
// (vendored) and falls back to vis-network when the user clicks the
// engine toggle in the canvas header.

(function () {
  const { $, $$, escapeHTML, escapeAttr, shortenURI,
          colorForType, colorForArtifact, onEvent } = window.kg;

  const EDGE_PREDICATES = [
    "mentions", "authored_by", "part_of", "relates_to", "references",
    "attended", "works_on", "occurred_at",
    "calls", "imports", "defines",
    "extracted_by",
  ];

  // Graph state. nodes/links are shared between the 3D + 2D engines —
  // the engine reads the same data structure, just renders it
  // differently. The Map dedupes by id; addOrUpdate updates in place.
  const graph = {
    engine: "3d",            // "3d" | "2d"
    fg3d: null,              // ForceGraph3D instance
    net2d: null,             // vis.Network instance
    nodes: new Map(),        // id → { id, label, type, artifact, ... }
    links: new Map(),        // "src→rel→tgt" → { source, target, rel }
    visNodes: null,          // vis.DataSet
    visEdges: null,          // vis.DataSet
  };

  let lastHits = [];
  let activeTypes = new Set();
  let activeArtifacts = new Set();

  // ─── search ─────────────────────────────────────────────────────
  let searchTimer = 0;
  function onSearchInput(ev) {
    const q = ev.target.value.trim();
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => doSearch(q), 200);
  }
  async function doSearch(q) {
    const list = $("#search-results");
    const counter = $("#results-count");
    if (!q) {
      if (list) list.innerHTML = `<li class="empty">type a query, hit Enter or wait</li>`;
      if (counter) counter.textContent = "";
      return;
    }
    try {
      const mode = $("#search-mode")?.value || "hybrid";
      const corpus = $("#search-corpus")?.value || "";
      const params = new URLSearchParams({ q, mode });
      if (corpus) params.set("corpus", corpus);
      const r = await fetch("api/search?" + params.toString());
      if (!r.ok) throw new Error("HTTP " + r.status);
      const j = await r.json();
      lastHits = j.hits || [];
      if (counter) counter.textContent = `${lastHits.length} hits · ${j.mode || "auto"}${j.fallback ? " fallback" : ""} · ${j.elapsed_ms || 0}ms`;
      renderSearchResults(lastHits);
    } catch (e) {
      if (list) list.innerHTML = `<li class="empty">search failed: ${escapeHTML(String(e))}</li>`;
    }
  }
  function renderSearchResults(hits) {
    const list = $("#search-results");
    if (!list) return;
    const filtered = hits.filter((h) =>
      (activeTypes.size === 0 || activeTypes.has(h.node_type || h.type)) &&
      (activeArtifacts.size === 0 || activeArtifacts.has(h.artifact))
    );
    if (filtered.length === 0) { list.innerHTML = `<li class="empty">no matches</li>`; return; }
    list.innerHTML = "";
    for (const h of filtered) {
      const id = h.id || h.node_id;
      const type = h.type || h.node_type || "?";
      const li = document.createElement("li");
      const score = (h.score != null) ? ` <span class="hit-score">${h.score.toFixed(3)}</span>` : "";
      li.style.setProperty("--hit-accent", colorForType(type));
      li.innerHTML = `
        <div class="hit-row">
          <span class="hit-type-dot" title="${escapeAttr(type)}"></span>
          <span class="hit-label">${escapeHTML(h.label || "(unlabeled)")}</span>${score}
        </div>
        <div class="hit-meta">
          <span class="hit-type">${escapeHTML(type)}</span>
          <span class="hit-enrichment" data-id="${escapeAttr(id)}"></span>
        </div>`;
      li.addEventListener("click", () => {
        loadNodeDetail(id);
        expandNode(id);
      });
      list.appendChild(li);
      // Async: load enrichment dots for Documents only.
      if (type === "Document" && h.source_uri) loadEnrichmentDots(h.source_uri, li.querySelector(".hit-enrichment"));
      else if (type === "Document") loadEnrichmentByID(id, li.querySelector(".hit-enrichment"));
    }
  }

  // ─── enrichment dots (Opt.1) ────────────────────────────────────
  async function loadEnrichmentDots(uri, host) {
    if (!host) return;
    try {
      const r = await fetch("api/enrichment/by-uri?uri=" + encodeURIComponent(uri));
      if (!r.ok) return;
      const j = await r.json();
      renderDots(j.stages || {}, host);
    } catch (_) {}
  }
  async function loadEnrichmentByID(id, host) {
    // Fallback: look up the Document's URI from /api/nodes/{id}, then
    // call /api/enrichment/by-uri. Cheap — happens once per hit.
    try {
      const r = await fetch("api/nodes/" + encodeURIComponent(id));
      if (!r.ok) return;
      const j = await r.json();
      const n = (j.node || [])[0];
      if (n && n.source_uri) loadEnrichmentDots(n.source_uri, host);
    } catch (_) {}
  }
  function renderDots(stages, host) {
    const order = ["structural", "entity", "relation", "embed"];
    host.innerHTML = order.map((s) => {
      const cls = stages[s] === "ready" ? "ready" : "missing";
      return `<span class="enr-dot ${cls} dot-${s}" title="${s}: ${stages[s] || "missing"}"></span>`;
    }).join("");
  }

  // ─── type filter + artifact filter ──────────────────────────────
  async function loadTypeFilter() {
    try {
      const r = await fetch("api/stats");
      if (!r.ok) return;
      const counts = await r.json();
      const box = $("#type-filter");
      if (!box) return;
      box.innerHTML = "";
      Object.keys(counts).sort().forEach((t) => {
        const b = document.createElement("button");
        b.className = "chip";
        b.style.setProperty("--chip-accent", colorForType(t));
        b.textContent = `${t} ${counts[t]}`;
        b.dataset.type = t;
        b.addEventListener("click", () => {
          if (activeTypes.has(t)) { activeTypes.delete(t); b.classList.remove("active"); }
          else { activeTypes.add(t); b.classList.add("active"); }
          renderSearchResults(lastHits);
        });
        box.appendChild(b);
      });
    } catch (_) {}
  }
  async function loadArtifactFilter() {
    try {
      const r = await fetch("api/query", {
        method: "POST",
        body: `{ q(func: has(artifact)) @groupby(artifact) { count(uid) } }`,
      });
      if (!r.ok) return;
      const j = await r.json();
      const rows = (j.q && j.q[0] && j.q[0]["@groupby"]) || [];
      const box = $("#artifact-filter");
      if (!box) return;
      if (rows.length === 0) { box.innerHTML = `<span class="empty">none indexed yet</span>`; return; }
      box.innerHTML = "";
      rows.forEach((row) => {
        const t = row.artifact;
        const n = row.count;
        const b = document.createElement("button");
        b.className = "chip";
        b.style.setProperty("--chip-accent", colorForArtifact(t));
        b.textContent = `${t} ${n}`;
        b.dataset.artifact = t;
        b.addEventListener("click", () => {
          if (activeArtifacts.has(t)) { activeArtifacts.delete(t); b.classList.remove("active"); }
          else { activeArtifacts.add(t); b.classList.add("active"); }
          renderSearchResults(lastHits);
        });
        box.appendChild(b);
      });
    } catch (_) {}
  }

  // ─── node detail + expand ───────────────────────────────────────
  async function loadNodeDetail(nodeId) {
    try {
      const r = await fetch("api/nodes/" + encodeURIComponent(nodeId));
      if (!r.ok) throw new Error("HTTP " + r.status);
      const j = await r.json();
      const arr = j.node || [];
      const host = $("#node-detail");
      if (!host) return;
      if (arr.length === 0) { host.innerHTML = `<p class="empty">node not found</p>`; return; }
      renderNodeDetail(arr[0]);
    } catch (e) {
      const host = $("#node-detail");
      if (host) host.innerHTML = `<p class="empty">load failed: ${escapeHTML(String(e))}</p>`;
    }
  }
  function renderNodeDetail(n) {
    const root = $("#node-detail");
    if (!root) return;
    const scalars = ["node_id", "node_type", "label", "source_uri", "content_hash",
                     "artifact", "bucket", "tier", "corpus_id", "markdown_hash",
                     "extractor_id", "extractor_ver", "created_at", "updated_at", "deleted_at"];
    let html = `<div class="nd-head"><h3>${escapeHTML(n.label || "(unlabeled)")}</h3>
                <span class="nd-type" style="--nd-accent:${colorForType(n.node_type)}">${escapeHTML(n.node_type || "?")}</span></div>`;
    html += `<dl class="nd-dl">`;
    for (const k of scalars) {
      if (n[k] === undefined || n[k] === null || n[k] === "") continue;
      let value;
      if (k === "source_uri" && typeof n[k] === "string" && n[k].startsWith("file://")) {
        value = `<a class="uri-link" data-uri="${escapeAttr(n[k])}" title="${escapeAttr(n[k])}">${escapeHTML(String(n[k]))}</a>`;
      } else if (k === "markdown_hash" && n.source_uri) {
        value = `<a class="uri-link" data-uri="${escapeAttr(n.source_uri)}" data-mode="compare" title="open side-by-side">${escapeHTML(String(n[k]))}</a>`;
      } else if (k === "artifact") {
        value = `<span class="artifact-chip" style="--chip-accent:${colorForArtifact(n[k])}">${escapeHTML(String(n[k]))}</span>`;
      } else {
        value = escapeHTML(String(n[k]));
      }
      html += `<dt>${escapeHTML(k)}</dt><dd>${value}</dd>`;
    }
    html += `</dl>`;
    if (n.text) html += `<h4>text</h4><div class="text-block">${escapeHTML(n.text)}</div>`;
    for (const pred of EDGE_PREDICATES) {
      const ts = n[pred];
      if (!Array.isArray(ts) || ts.length === 0) continue;
      html += `<h4>${escapeHTML(pred)} → ${ts.length}</h4><ul class="edge-list">`;
      for (const t of ts) {
        html += `<li data-id="${escapeAttr(t.node_id)}" data-pred="${escapeAttr(pred)}">
          <span class="rel">→</span>
          <span class="tgt-type" style="--nd-accent:${colorForType(t.node_type)}">${escapeHTML(t.node_type || "?")}</span>
          <span>${escapeHTML(t.label || t.node_id)}</span></li>`;
      }
      html += `</ul>`;
    }
    for (const rp of ["~mentions", "~defines"]) {
      const ts = n[rp];
      if (!Array.isArray(ts) || ts.length === 0) continue;
      html += `<h4>${escapeHTML(rp)} ← ${ts.length}</h4><ul class="edge-list">`;
      for (const t of ts) {
        html += `<li data-id="${escapeAttr(t.node_id)}" data-pred="${escapeAttr(rp)}">
          <span class="rel">←</span>
          <span class="tgt-type" style="--nd-accent:${colorForType(t.node_type)}">${escapeHTML(t.node_type || "?")}</span>
          <span>${escapeHTML(t.label || t.node_id)}</span></li>`;
      }
      html += `</ul>`;
    }
    root.innerHTML = html;
    root.querySelectorAll(".edge-list li").forEach((li) => {
      li.addEventListener("click", () => {
        const id = li.getAttribute("data-id");
        loadNodeDetail(id);
        expandNode(id);
      });
    });
    addOrUpdateNode({ node_id: n.node_id, node_type: n.node_type, label: n.label, artifact: n.artifact });
    redraw();
  }

  async function expandNode(nodeId) {
    try {
      const r = await fetch("api/nodes/" + encodeURIComponent(nodeId));
      if (!r.ok) return;
      const j = await r.json();
      const arr = j.node || [];
      if (arr.length === 0) return;
      const n = arr[0];
      addOrUpdateNode({ node_id: n.node_id, node_type: n.node_type, label: n.label, artifact: n.artifact });
      for (const pred of EDGE_PREDICATES) {
        const ts = n[pred];
        if (!Array.isArray(ts)) continue;
        for (const t of ts) {
          addOrUpdateNode({ node_id: t.node_id, node_type: t.node_type, label: t.label, artifact: t.artifact });
          addLink(n.node_id, t.node_id, pred);
        }
      }
      for (const rp of ["~mentions", "~defines"]) {
        const ts = n[rp];
        if (!Array.isArray(ts)) continue;
        for (const t of ts) {
          addOrUpdateNode({ node_id: t.node_id, node_type: t.node_type, label: t.label, artifact: t.artifact });
          addLink(t.node_id, n.node_id, rp.slice(1));
        }
      }
      redraw();
    } catch (_) {}
  }

  // ─── shared graph state ─────────────────────────────────────────
  function addOrUpdateNode(n) {
    if (!n.node_id) return;
    graph.nodes.set(n.node_id, {
      id: n.node_id,
      label: shortLabel(n.label),
      type: n.node_type || "?",
      artifact: n.artifact || "",
    });
  }
  function addLink(srcId, tgtId, rel) {
    if (!srcId || !tgtId) return;
    const key = srcId + "→" + rel + "→" + tgtId;
    if (graph.links.has(key)) return;
    graph.links.set(key, { source: srcId, target: tgtId, rel });
  }
  function shortLabel(label) {
    if (!label) return "(unlabeled)";
    if (label.length > 36) return label.slice(0, 34) + "…";
    return label;
  }
  function clearGraph() {
    graph.nodes.clear();
    graph.links.clear();
    redraw();
  }
  function updateCanvasMeta() {
    const meta = $("#canvas-meta");
    if (meta) meta.textContent = `${graph.nodes.size} nodes · ${graph.links.size} edges`;
  }

  // ─── 3d-force-graph driver ──────────────────────────────────────
  function init3D() {
    const host = $("#graph-3d");
    if (!host || typeof ForceGraph3D !== "function") return;
    const themeBG = () => getComputedStyle(document.documentElement)
      .getPropertyValue("--bg-canvas").trim() || "#0b0d10";
    graph.fg3d = ForceGraph3D()(host)
      .backgroundColor(themeBG())
      .nodeLabel((n) => `${n.type}: ${n.label}`)
      .nodeColor((n) => colorForType(n.type))
      .nodeOpacity(0.95)
      .nodeVal((n) => 1 + (n._degree || 0) * 0.5)
      .linkColor(() => "rgba(160,170,190,0.35)")
      .linkOpacity(0.5)
      .linkDirectionalArrowLength(2.5)
      .linkDirectionalArrowRelPos(1)
      .onNodeClick((n) => {
        loadNodeDetail(n.id);
      })
      .onNodeRightClick((n) => {
        expandNode(n.id);
      });
    // Cooler camera so the canvas isn't too zoomed in on first paint.
    graph.fg3d.cameraPosition({ x: 0, y: 0, z: 240 });
    // Resize handler — the host's flex layout means initial size may
    // be 0 until the tab activates.
    window.addEventListener("resize", () => sizeGraph3D());
  }
  function sizeGraph3D() {
    if (!graph.fg3d) return;
    const host = $("#graph-3d");
    if (!host) return;
    const w = host.clientWidth, h = host.clientHeight;
    if (w > 0 && h > 0) graph.fg3d.width(w).height(h);
  }

  // ─── 2D fallback (vis-network) — lazy init only on toggle ────────
  function init2D() {
    if (graph.net2d) return;
    const host = $("#graph-2d");
    if (!host || typeof vis !== "object") return;
    graph.visNodes = new vis.DataSet([]);
    graph.visEdges = new vis.DataSet([]);
    graph.net2d = new vis.Network(host, { nodes: graph.visNodes, edges: graph.visEdges }, {
      interaction: { hover: true, tooltipDelay: 200 },
      physics: { enabled: true, solver: "forceAtlas2Based", stabilization: { iterations: 80 } },
      nodes: { shape: "dot", size: 14, font: { color: "#e6e9ef", size: 12 }, borderWidth: 1.5 },
      edges: { color: { color: "#374151", highlight: "#60a5fa" }, arrows: { to: { enabled: true, scaleFactor: 0.4 } }, smooth: { type: "dynamic" }, width: 1 },
    });
    graph.net2d.on("click", (params) => { if (params.nodes.length > 0) loadNodeDetail(params.nodes[0]); });
    graph.net2d.on("doubleClick", (params) => { if (params.nodes.length > 0) expandNode(params.nodes[0]); });
  }

  // redraw pushes the current graph state into whichever engine is
  // active. Cheap on small graphs, but bounded — we cap at 200 nodes
  // by default so a runaway expand doesn't kill performance.
  const NODE_CAP = 200;
  function nodeDegrees() {
    const deg = new Map();
    for (const k of graph.links.keys()) {
      const l = graph.links.get(k);
      const a = l.source.id || l.source;
      const b = l.target.id || l.target;
      deg.set(a, (deg.get(a) || 0) + 1);
      deg.set(b, (deg.get(b) || 0) + 1);
    }
    return deg;
  }
  function snapshotForEngine() {
    const deg = nodeDegrees();
    const nodes = Array.from(graph.nodes.values()).slice(0, NODE_CAP).map((n) => ({
      ...n, _degree: deg.get(n.id) || 0,
    }));
    const keep = new Set(nodes.map((n) => n.id));
    const links = Array.from(graph.links.values())
      .filter((l) => keep.has(l.source.id || l.source) && keep.has(l.target.id || l.target));
    return { nodes, links };
  }
  function redraw() {
    updateCanvasMeta();
    if (graph.engine === "3d") {
      if (!graph.fg3d) return;
      const { nodes, links } = snapshotForEngine();
      graph.fg3d.graphData({ nodes, links });
      sizeGraph3D();
    } else {
      init2D();
      if (!graph.net2d) return;
      const { nodes, links } = snapshotForEngine();
      // vis.DataSet keyed by id — clear + add each tick.
      graph.visNodes.clear();
      graph.visEdges.clear();
      for (const n of nodes) {
        graph.visNodes.add({
          id: n.id, label: n.label, title: `${n.type}: ${n.label}`,
          color: { background: colorForType(n.type), border: "#60a5fa" },
        });
      }
      for (const l of links) {
        const src = l.source.id || l.source;
        const tgt = l.target.id || l.target;
        graph.visEdges.add({ id: src + "→" + l.rel + "→" + tgt, from: src, to: tgt, label: l.rel });
      }
    }
  }

  function toggleEngine() {
    graph.engine = graph.engine === "3d" ? "2d" : "3d";
    const btn = $("#canvas-engine");
    if (btn) btn.textContent = graph.engine === "3d" ? "2D" : "3D";
    $("#graph-3d").hidden = graph.engine !== "3d";
    $("#graph-2d").hidden = graph.engine !== "2d";
    redraw();
    if (graph.engine === "3d") sizeGraph3D();
  }

  // ─── seed canvas (latest extraction runs → Documents) ──────────
  async function seedCanvas() {
    try {
      const r = await fetch("api/runs?limit=5");
      if (!r.ok) return;
      const j = await r.json();
      const runs = (j.runs || []).filter((x) => x.source_uri);
      if (runs.length === 0) return;
      const uris = runs.map((x) => x.source_uri);
      const q = `{ docs(func: eq(source_uri, [${uris.map(quoteForDQL).join(",")}]))
                     @filter(eq(node_type, "Document")) { node_id } }`;
      const dr = await fetch("api/query", { method: "POST", body: q });
      if (!dr.ok) return;
      const dj = await dr.json();
      const docs = (dj.docs || []).slice(0, 5);
      for (const d of docs) {
        if (d.node_id) {
          // eslint-disable-next-line no-await-in-loop
          await expandNode(d.node_id);
        }
      }
    } catch (_) {}
  }
  function quoteForDQL(s) {
    return '"' + String(s).replace(/\\/g, "\\\\").replace(/"/g, '\\"') + '"';
  }

  // ─── wiring ─────────────────────────────────────────────────────
  let bootDone = false;
  window.kgExploreInit = function () {
    if (bootDone) { sizeGraph3D(); return; }
    bootDone = true;
    // The shared index has no graph service. Keep the retained graph assets
    // dormant until a graph adapter is available.
    const canvas = $("#graph-3d");
    if (canvas) canvas.textContent = "Graph enrichment unavailable";
    $("#canvas-engine")?.setAttribute("disabled", "");
    loadTypeFilter();
    loadArtifactFilter();
    $("#search-input")?.addEventListener("input", onSearchInput);
    $("#search-mode")?.addEventListener("change", () => doSearch($("#search-input")?.value.trim() || ""));
    $("#search-corpus")?.addEventListener("change", () => doSearch($("#search-input")?.value.trim() || ""));
    $("#canvas-clear")?.addEventListener("click", clearGraph);
    $("#canvas-fit")?.addEventListener("click", () => {
      if (graph.engine === "3d" && graph.fg3d) graph.fg3d.zoomToFit(400);
      else if (graph.engine === "2d" && graph.net2d) graph.net2d.fit();
    });
    $("#canvas-engine")?.addEventListener("click", toggleEngine);
    // Refresh enrichment dots when ingest events fire — they may have
    // just lifted a stage and the dots should update.
    onEvent("enricher_tick", () => {
      $$(".hit-enrichment").forEach((host) => {
        const id = host.dataset.id || host.getAttribute("data-id");
        if (!id) return;
        loadEnrichmentByID(id, host);
      });
    });
    // When the explore tab is shown later (router lazy-init), resize
    // the WebGL canvas — the host may have started at width=0.
    window.addEventListener("tab:show", (ev) => {
      if (ev.detail.tab === "explore") sizeGraph3D();
    });
  };
})();
