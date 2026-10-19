// kg-ui monitor tab — discovery + enrichment + stats + pipeline + events.
// Uses helpers from core.js. Re-uses kg.onEvent so we don't open a
// second EventSource.

(function () {
  const { $, $$, fmtTime, escapeHTML, shortenURI, shortenPath, humanBytes,
          compactNumber, bucketClass, onEvent } = window.kg;

  // ─── /api/stats ─────────────────────────────────────────────────
  async function loadStats() {
    try {
      const r = await fetch("api/stats");
      if (!r.ok) throw new Error("HTTP " + r.status);
      const counts = await r.json();
      renderStats(counts);
    } catch (e) { renderStatsError(e); }
  }
  function renderStats(counts) {
    const grid = $("#stats-grid");
    if (!grid) return;
    grid.innerHTML = "";
    const entries = Object.entries(counts).sort((a, b) => b[1] - a[1]);
    if (entries.length === 0) {
      grid.innerHTML = `<div class="stat empty">graph is empty</div>`;
      return;
    }
    let total = 0;
    for (const [label, value] of entries) {
      total += value;
      const card = document.createElement("div");
      card.className = "stat";
      card.innerHTML = `<div class="label">${escapeHTML(label)}</div><div class="value">${value}</div>`;
      const color = window.kg.colorForType(label);
      card.style.setProperty("--accent-color", color);
      grid.appendChild(card);
    }
    const totalCard = document.createElement("div");
    totalCard.className = "stat total";
    totalCard.innerHTML = `<div class="label">total</div><div class="value">${total}</div>`;
    grid.appendChild(totalCard);
  }
  function renderStatsError(e) {
    const grid = $("#stats-grid");
    if (grid) grid.innerHTML = `<div class="stat empty">stats unavailable: ${escapeHTML(String(e))}</div>`;
  }

  // ─── /api/enrichment/summary (Opt.1) ────────────────────────────
  async function loadEnrichment() {
    try {
      const r = await fetch("api/enrichment/summary");
      if (!r.ok) { hideEnrichment(); return; }
      const j = await r.json();
      renderEnrichment(j);
    } catch (_) { hideEnrichment(); }
  }
  function hideEnrichment() {
    const c = $("#enrichment-card");
    if (c) c.hidden = true;
  }
  function renderEnrichment(j) {
    const card = $("#enrichment-card");
    const host = $("#enrichment-grid");
    if (!card || !host) return;
    const total = j.indexed || 0;
    if (total === 0) { hideEnrichment(); return; }
    card.hidden = false;
    const by = j.by_stage || {};
    const stages = [
      ["indexed",    total,                "indexed"],
      ["structural", by.structural || 0,   "structural"],
      ["entity",     by.entity || 0,       "entity"],
      ["relation",   by.relation || 0,     "relation"],
      ["embed",      by.embed || 0,        "embed"],
    ];
    host.innerHTML = stages.map(([k, v, cls]) => {
      const pct = total > 0 && k !== "indexed" ? Math.round((v / total) * 100) : null;
      return `<div class="enr-tile enr-${cls}">
        <div class="enr-num">${compactNumber(v)}</div>
        <div class="enr-label">${escapeHTML(k)}</div>
        ${pct !== null ? `<div class="enr-pct">${pct}%</div>` : ""}
      </div>`;
    }).join("");
  }

  // ─── /api/runs ──────────────────────────────────────────────────
  async function loadRuns() {
    try {
      const r = await fetch("api/runs?limit=20");
      if (!r.ok) throw new Error("HTTP " + r.status);
      const j = await r.json();
      renderRuns(j.runs || []);
    } catch (e) {
      const tb = $("#runs-body");
      if (tb) tb.innerHTML = `<tr><td colspan="5" class="empty">runs unavailable: ${escapeHTML(String(e))}</td></tr>`;
    }
  }
  // recentDurations tracks the last 50 (uri → ingest_done.duration_ms)
  // mappings from SSE events. The Duration column on the runs table
  // hydrates from this map so the user sees per-doc elapsed without
  // a separate API call.
  const recentDurations = new Map();
  function rememberDuration(uri, durMs) {
    if (!uri || typeof durMs !== "number") return;
    recentDurations.set(uri, durMs);
    if (recentDurations.size > 50) {
      const firstKey = recentDurations.keys().next().value;
      recentDurations.delete(firstKey);
    }
  }
  function renderRuns(rows) {
    const body = $("#runs-body");
    if (!body) return;
    if (rows.length === 0) {
      body.innerHTML = `<tr><td colspan="5" class="empty">no ingestion runs yet</td></tr>`;
      return;
    }
    body.innerHTML = "";
    for (const r of rows) {
      const tr = document.createElement("tr");
      const uri = r.source_uri || "";
      const srcCell = uri
        ? `<a class="uri-link" data-uri="${escapeHTML(uri)}" title="${escapeHTML(uri)}">${escapeHTML(shortenURI(uri))}</a>`
        : `<span class="muted">—</span>`;
      const dur = recentDurations.get(uri);
      const durCell = (typeof dur === "number")
        ? `${dur} ms`
        : `<span class="muted">—</span>`;
      tr.innerHTML = `
        <td class="when">${fmtTime(r.run_at)}</td>
        <td><span class="badge">${escapeHTML(r.extractor_id || "?")}</span></td>
        <td class="src">${srcCell}</td>
        <td class="num">${r.bonsai_ts ?? ""}</td>
        <td class="num">${durCell}</td>`;
      body.appendChild(tr);
    }
  }

  // ─── /api/scan/live (discovery card) ────────────────────────────
  async function loadScan() {
    try {
      const r = await fetch("api/scan/live");
      if (!r.ok) { const c = $("#discovery-card"); if (c) c.hidden = true; return; }
      const j = await r.json();
      renderDiscovery(j);
    } catch (_) { const c = $("#discovery-card"); if (c) c.hidden = true; }
  }
  function renderDiscovery(snap) {
    const card = $("#discovery-card");
    if (!card) return;
    card.hidden = false;
    const totals = $("#discovery-totals");
    const rate = (snap.roots || []).reduce((a, r) => a + (r.emits_per_sec || 0), 0);
    const finishedAll = (snap.roots || []).length > 0 && (snap.roots || []).every((r) => r.finished && r.finished !== "0001-01-01T00:00:00Z");
    const stateBadge = finishedAll
      ? `<span class="pill done">walk complete</span>`
      : `<span class="pill walking">walking…</span>`;
    totals.innerHTML = `${stateBadge}
      <span class="k">emitted</span>=<span class="v">${snap.total_emitted}</span>
      <span class="k">bytes</span>=<span class="v">${humanBytes(snap.total_bytes)}</span>
      <span class="k">skipped</span>=<span class="v">${snap.total_skipped}</span>
      <span class="k">rate</span>=<span class="v">${rate.toFixed(0)}/s</span>`;

    const now = $("#discovery-now");
    if (snap.current_path) {
      const uri = "file://" + snap.current_path;
      now.innerHTML = `<span class="now-label">now:</span> <a class="uri-link" data-uri="${escapeHTML(uri)}" title="${escapeHTML(uri)}">${escapeHTML(shortenPath(snap.current_path))}</a>`;
    } else now.textContent = "";

    const byBucket = {};
    for (const r of (snap.roots || [])) {
      for (const [b, n] of Object.entries(r.bucket_emitted || {})) byBucket[b] = (byBucket[b] || 0) + n;
    }
    const entries = Object.entries(byBucket).sort((a, b) => b[1] - a[1]);
    const max = entries.length ? entries[0][1] : 0;
    const host = $("#discovery-buckets");
    if (entries.length === 0) host.innerHTML = `<div class="empty">no files discovered yet</div>`;
    else host.innerHTML = entries.map(([b, n]) => {
      const pct = max > 0 ? Math.round((n / max) * 100) : 0;
      return `<div class="disc-bar-row">
        <div class="disc-bar-label">${escapeHTML(b)}</div>
        <div class="disc-bar-track"><div class="disc-bar-fill ${bucketClass(b)}" style="width:${pct}%"></div></div>
        <div class="disc-bar-num">${n}</div>
      </div>`;
    }).join("");

    const skipHost = $("#discovery-skips");
    const skips = (snap.recent_skips || []).slice(-8).reverse();
    if (skips.length === 0) skipHost.innerHTML = `<li class="empty">nothing filtered</li>`;
    else skipHost.innerHTML = skips.map((s) => {
      const reason = s.reason || "filter";
      const uri = "file://" + s.path;
      return `<li>
        <span class="io-ts">${fmtTime(s.at)}</span>
        <span class="badge muted">${escapeHTML(reason)}</span>
        <a class="uri-link" data-uri="${escapeHTML(uri)}" title="${escapeHTML(uri)}">${escapeHTML(shortenPath(s.path))}</a>
      </li>`;
    }).join("");
  }

  // ─── /api/substages + /api/substages/live (pipeline panel) ───────

  // STAGE_GROUPS maps the 6 conceptual stages of the redesigned DAG
  // (see ~/.claude/plans/digress-a-little-show-peppy-gem.md) onto the
  // 15 internal substages the daemon's chain actually runs. The
  // grouping is a UI-layer rendering decision — daemon endpoints
  // continue to expose per-substage counters.
  //
  // Stages that have no constituent substage in the chain (Cluster,
  // Serve) declare an `external` endpoint the DAG node fetches its
  // counters from. They render gracefully when the endpoint isn't
  // wired (falls back to a "no sweep yet" / "not wired" payload).
  const STAGE_GROUPS = [
    { id: 'scan',      order: 0, title: 'Scan + Cache', substages: ['read', 'cache-check'] },
    { id: 'classify',  order: 1, title: 'Classify',      substages: ['classify'] },
    { id: 'convert',   order: 2, title: 'Convert',       substages: ['convert', 'llm-convert'], llm: 'convert' },
    { id: 'construct', order: 3, title: 'Construct',     substages: ['structural', 'doc-decompose', 'coref', 'entity', 'relation', 'fs-emit', 'project-mint'], llm: 'construct' },
    { id: 'commit',    order: 4, title: 'Commit',        substages: ['upsert', 'ledger', 'embed'] },
    { id: 'cluster',   order: 5, title: 'Cluster',       substages: [], external: 'api/cluster' },
    { id: 'serve',     order: 6, title: 'Serve',         substages: [], external: 'api/serve' },
  ];

  // Reverse index: substage name → STAGE_GROUPS entry. Built once.
  const SUBSTAGE_TO_GROUP = (() => {
    const m = {};
    for (const g of STAGE_GROUPS) for (const s of g.substages) m[s] = g;
    return m;
  })();

  // aggregateStage rolls one group's substage counters into a single
  // virtual stage record matching the shape `/api/substages/live`
  // already returns per substage. The aggregate carries:
  //   - in_flight / completed / errored: simple sums
  //   - throughput_per_min: sum across substages (each chain step
  //     contributes its own throughput)
  //   - p50_ms / p99_ms: max across substages (worst-case is what
  //     hurts; per-step averages would be misleading)
  //   - recent_inputs / recent_outputs: merged chronologically with
  //     each entry tagged with its source substage name so the
  //     focused panel can attribute rows
  //   - last_error: the most recent across substages
  //   - members: the original per-substage records, used by the
  //     focused panel's constituent-substages table
  function aggregateStage(group, liveStages) {
    const byName = new Map();
    for (const s of liveStages || []) byName.set(s.name, s);
    const members = group.substages
      .map((name) => byName.get(name))
      .filter(Boolean);

    let in_flight = 0, completed = 0, errored = 0, throughput_per_min = 0;
    let p50_ms = 0, p99_ms = 0;
    const recent_inputs = [];
    const recent_outputs = [];
    let last_error = null;

    for (const m of members) {
      in_flight += m.in_flight || 0;
      completed += m.completed || 0;
      errored += m.errored || 0;
      throughput_per_min += m.throughput_per_min || 0;
      if ((m.p50_ms || 0) > p50_ms) p50_ms = m.p50_ms;
      if ((m.p99_ms || 0) > p99_ms) p99_ms = m.p99_ms;
      for (const x of m.recent_inputs || []) recent_inputs.push({ ...x, _substage: m.name });
      for (const x of m.recent_outputs || []) recent_outputs.push({ ...x, _substage: m.name });
      if (m.last_error) {
        const candidate = { ...m.last_error, _substage: m.name };
        if (!last_error || new Date(candidate.at) > new Date(last_error.at)) last_error = candidate;
      }
    }
    // Sort newest first; cap so the focused panel doesn't unbound.
    recent_inputs.sort((a, b) => new Date(b.at) - new Date(a.at));
    recent_outputs.sort((a, b) => new Date(b.at) - new Date(a.at));
    return {
      id: group.id,
      order: group.order,
      title: group.title,
      llm: group.llm || null,
      external: group.external || null,
      members,
      in_flight,
      completed,
      errored,
      throughput_per_min,
      p50_ms,
      p99_ms,
      recent_inputs: recent_inputs.slice(0, 20),
      recent_outputs: recent_outputs.slice(0, 20),
      last_error,
    };
  }

  // aggregateAllStages returns every STAGE_GROUPS entry rolled up
  // against the latest live snapshot. Stages without substages (cluster,
  // serve) come back with zero counters; the DAG renderer hits their
  // `external` endpoint when those values matter.
  function aggregateAllStages(snap) {
    const live = (snap && snap.stages) || [];
    return STAGE_GROUPS.map((g) => aggregateStage(g, live));
  }

  // stageStatus collapses an aggregate (or a substage record) into a
  // single status pill matching the today's existing CSS classes.
  function stageStatusFor(agg) {
    if (agg.in_flight > 0) return 'running';
    if (agg.errored > 0) return 'failed';
    if (agg.completed > 0) return 'done';
    return 'pending';
  }

  const pipelineState = { snap: null, selected: null, manifests: {} };

  async function loadSubstageManifests() {
    try {
      const r = await fetch("api/substages");
      if (!r.ok) return;
      const arr = await r.json();
      if (!Array.isArray(arr)) return;
      for (const m of arr) if (m && m.name) pipelineState.manifests[m.name] = m;
      if (pipelineState.selected) renderFocusedDetail();
    } catch (_) {}
  }
  async function loadPipeline() {
    try {
      const r = await fetch("api/substages/live");
      if (!r.ok) { renderPipelineEmpty(r.status === 503 ? "substages telemetry not configured" : `HTTP ${r.status}`); return; }
      const j = await r.json();
      renderPipeline(j);
    } catch (e) { renderPipelineEmpty(String(e)); }
  }
  function renderPipelineEmpty(msg) {
    const f = $("#pipeline-flow"); if (f) f.innerHTML = `<div class="stage empty">${escapeHTML(msg)}</div>`;
    const t = $("#pipeline-totals"); if (t) t.innerHTML = "";
    const d = $("#pipeline-detail"); if (d) d.hidden = true;
  }
  function renderPipeline(snap) {
    pipelineState.snap = snap;
    renderSummaryRow(snap);
    renderPipelineTotals(snap);
    renderDAG(snap);
    renderFocusedDetail();
  }
  // renderSummaryRow paints the strip above the DAG. Numbers come from
  // /api/substages/live (snap.totals + snap.queue + snap.eta); pause +
  // disk state are owned by core.js which manages the page-level
  // banners + pills, so the summary row just reflects the current
  // header pause button visibility.
  function renderSummaryRow(snap) {
    const setMetric = (id, val) => {
      const el = $(`#${id} .summary-value`);
      if (el) el.textContent = val;
    };
    const t = (snap && snap.totals) || {};
    const q = (snap && snap.queue) || {};
    const e = (snap && snap.eta) || {};
    setMetric("summary-submitted", compactNumber(t.submitted || 0));
    setMetric("summary-completed", compactNumber(t.completed || 0));
    setMetric("summary-skipped",   compactNumber(t.skipped   || 0));
    setMetric("summary-errored",   compactNumber(t.errored   || 0));
    // tp = sum of throughput_per_min across stages, computed from the
    // already-aggregated stages array.
    const tp = (snap && snap.stages || []).reduce((s, st) => s + (st.throughput_per_min || 0), 0);
    setMetric("summary-throughput", tp ? `${tp.toFixed(1)}/min` : "—");
    setMetric("summary-queue", (q.depth || q.cap) ? `${q.depth ?? 0}/${q.cap ?? 0}` : "—");
    if (e.eta_seconds && e.jobs_inflight) {
      setMetric("summary-eta", e.eta_seconds < 60 ? `${e.eta_seconds}s` : `${Math.round(e.eta_seconds / 60)}m`);
    } else {
      setMetric("summary-eta", "—");
    }
    // Pause indicator mirrors the header pause/resume button visibility
    // — core.js already toggles those. When resume button is visible,
    // ingest is paused.
    const ctlHost = $("#summary-ctl");
    if (ctlHost) {
      const resumeBtn = $("#btn-resume");
      const paused = resumeBtn && !resumeBtn.hidden;
      ctlHost.innerHTML = paused
        ? `<span class="pill failed">▮▮ paused</span>`
        : `<span class="pill done">▶ running</span>`;
    }
  }
  function renderPipelineTotals(snap) {
    const tot = $("#pipeline-totals");
    if (!tot) return;
    const t = snap.totals || {};
    const q = snap.queue || {};
    const parts = [];
    if (q.workers) parts.push(`<span class="k">workers</span>=<span class="v">${q.workers}</span>`);
    if (q.depth || q.cap) parts.push(`<span class="k">in-flight</span>=<span class="v">${q.depth ?? 0}/${q.cap ?? 0}</span>`);
    if (t.submitted) parts.push(`<span class="k">submitted</span>=<span class="v">${t.submitted}</span>`);
    if (t.completed) parts.push(`<span class="k">completed</span>=<span class="v done">${t.completed}</span>`);
    if (t.skipped)   parts.push(`<span class="k">skipped</span>=<span class="v">${t.skipped}</span>`);
    if (t.errored)   parts.push(`<span class="k">errored</span>=<span class="v failed">${t.errored}</span>`);
    tot.innerHTML = parts.join(" &nbsp; ");
  }
  // renderDAG paints the 6-stage pipeline as inline SVG into
  // #pipeline-flow. The SVG layout is fixed (1280x300 viewBox) and
  // scales to the container via CSS; horizontal scroll kicks in only
  // when the container is narrower than the SVG's min-width.
  function renderDAG(snap) {
    const host = $("#pipeline-flow");
    if (!host) return;
    const groups = aggregateAllStages(snap);
    if (!pipelineState.selected) {
      const active = groups.find((g) => g.in_flight > 0 || g.errored > 0) || groups.find((g) => g.completed > 0) || groups[0];
      pipelineState.selected = active ? active.id : null;
    }
    host.innerHTML = buildDAGSVG(groups);
    host.querySelectorAll('[data-stage-id]').forEach((el) => {
      el.addEventListener('click', () => {
        pipelineState.selected = el.dataset.stageId;
        renderDAG(pipelineState.snap); // re-render so .active highlight updates
        renderFocusedDetail();
      });
      el.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); el.click(); }
      });
    });
  }
  // buildDAGSVG returns the SVG string for the 6-stage flow. Top row:
  // Scan → Classify → Convert → Construct → Commit → Sinks. Below
  // Commit/Sinks: Cluster reads sinks; Serve reads sinks too and
  // emits to agents. Producers label hangs off Scan's left edge.
  function buildDAGSVG(groups) {
    const W = 1280, H = 300;
    const byId = Object.fromEntries(groups.map((g) => [g.id, g]));
    const NW = 168, NH = 70; // stage node width/height
    const Y_TOP = 60;        // top row y
    const X0 = 120;          // first stage x
    const STEP = 180;        // x stride between stages
    const positions = {
      scan:      { x: X0,            y: Y_TOP, w: NW, h: NH },
      classify:  { x: X0 + STEP * 1, y: Y_TOP, w: NW, h: NH },
      convert:   { x: X0 + STEP * 2, y: Y_TOP, w: NW, h: NH },
      construct: { x: X0 + STEP * 3, y: Y_TOP, w: NW, h: NH },
      commit:    { x: X0 + STEP * 4, y: Y_TOP, w: NW, h: NH },
      cluster:   { x: X0 + STEP * 4, y: 170,   w: NW, h: 54 },
      serve:     { x: X0 + STEP * 4, y: 240,   w: NW, h: 54 },
    };
    const sinks = { x: X0 + STEP * 5, y: Y_TOP, w: 116, h: NH };
    const sel = pipelineState.selected;

    let svg = `<svg class="dag-svg" viewBox="0 0 ${W} ${H}" preserveAspectRatio="xMidYMid meet" role="group" aria-label="pipeline DAG">`;
    svg += `<defs><marker id="dag-arr" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto"><path d="M0,0 L10,5 L0,10 z" fill="currentColor"/></marker></defs>`;

    // Producers label + arrow into Scan.
    const pScan = positions.scan;
    svg += `<text x="40" y="${Y_TOP + NH / 2 + 4}" class="dag-edge-label" text-anchor="middle">producers</text>`;
    svg += `<line x1="80" y1="${Y_TOP + NH / 2}" x2="${pScan.x - 4}" y2="${Y_TOP + NH / 2}" class="dag-arrow" marker-end="url(#dag-arr)"/>`;

    // Top-row arrows (scan→classify→convert→construct→commit→sinks).
    const topRow = ['scan', 'classify', 'convert', 'construct', 'commit'];
    for (let i = 0; i < topRow.length - 1; i++) {
      const a = positions[topRow[i]], b = positions[topRow[i + 1]];
      svg += `<line x1="${a.x + a.w}" y1="${Y_TOP + NH / 2}" x2="${b.x - 4}" y2="${Y_TOP + NH / 2}" class="dag-arrow" marker-end="url(#dag-arr)"/>`;
    }
    // commit → sinks arrow
    const pCommit = positions.commit;
    svg += `<line x1="${pCommit.x + pCommit.w}" y1="${Y_TOP + NH / 2}" x2="${sinks.x - 4}" y2="${Y_TOP + NH / 2}" class="dag-arrow" marker-end="url(#dag-arr)"/>`;

    // Sinks rendering (pseudo-block; not clickable, not a stage).
    svg += `<g class="dag-sinks">
      <rect x="${sinks.x}" y="${sinks.y}" width="${sinks.w}" height="${sinks.h}" rx="8"/>
      <text x="${sinks.x + sinks.w / 2}" y="${sinks.y + 28}" text-anchor="middle" class="dag-sinks-title">Sinks</text>
      <text x="${sinks.x + sinks.w / 2}" y="${sinks.y + 48}" text-anchor="middle" class="dag-sinks-sub">bonsai · bluge · vec</text>
    </g>`;

    // Cluster reads from sinks (arrow up from cluster top edge).
    const pCluster = positions.cluster;
    svg += `<line x1="${pCluster.x + pCluster.w / 2}" y1="${pCluster.y}" x2="${sinks.x + sinks.w / 2 - 24}" y2="${sinks.y + sinks.h}" class="dag-arrow dag-arrow-side" marker-end="url(#dag-arr)"/>`;
    // Serve reads from sinks too (arrow from sinks down through cluster region).
    const pServe = positions.serve;
    svg += `<line x1="${sinks.x + sinks.w / 2 + 24}" y1="${sinks.y + sinks.h}" x2="${pServe.x + pServe.w / 2}" y2="${pServe.y}" class="dag-arrow dag-arrow-side" marker-end="url(#dag-arr)"/>`;
    // Serve → agents (arrow leaves right edge).
    svg += `<line x1="${pServe.x + pServe.w}" y1="${pServe.y + pServe.h / 2}" x2="${W - 60}" y2="${pServe.y + pServe.h / 2}" class="dag-arrow" marker-end="url(#dag-arr)"/>`;
    svg += `<text x="${W - 30}" y="${pServe.y + pServe.h / 2 + 4}" class="dag-edge-label" text-anchor="middle">agents</text>`;

    // Stage node rectangles + click targets.
    for (const id of ['scan', 'classify', 'convert', 'construct', 'commit', 'cluster', 'serve']) {
      svg += renderStageNode(byId[id], positions[id], sel === id);
    }
    svg += `</svg>`;
    return svg;
  }
  function renderStageNode(agg, pos, isSelected) {
    if (!agg) return "";
    const status = stageStatusFor(agg);
    let brief = "";
    if (agg.in_flight > 0) brief = `${agg.in_flight}↻`;
    else if (agg.errored > 0) brief = `${agg.errored} err`;
    else if (agg.completed > 0) brief = compactNumber(agg.completed);
    const cls = `dag-node ${status}${isSelected ? " active" : ""}${agg.llm ? " has-llm" : ""}`;
    return `<g class="${cls}" data-stage-id="${agg.id}" tabindex="0" role="button" aria-label="${escapeHTML(agg.title)} stage, status ${status}">
      <rect x="${pos.x}" y="${pos.y}" width="${pos.w}" height="${pos.h}" rx="8"/>
      <text class="dag-node-num" x="${pos.x + 12}" y="${pos.y + 22}">${agg.order + 1}</text>
      <text class="dag-node-title" x="${pos.x + pos.w / 2}" y="${pos.y + 30}" text-anchor="middle">${escapeHTML(agg.title)}</text>
      <text class="dag-node-stat" x="${pos.x + pos.w / 2}" y="${pos.y + pos.h - 14}" text-anchor="middle">${escapeHTML(brief)}</text>
    </g>`;
  }
  function stageStatus(st) {
    if (st.in_flight > 0) return "running";
    if (st.errored > 0) return "failed";
    if (st.completed > 0) return "done";
    return "pending";
  }
  function renderFocusedDetail() {
    const host = $("#pipeline-detail");
    if (!host) return;
    const snap = pipelineState.snap;
    if (!snap || !pipelineState.selected) { host.hidden = true; return; }
    const group = STAGE_GROUPS.find((g) => g.id === pipelineState.selected);
    if (!group) { host.hidden = true; return; }
    const agg = aggregateStage(group, snap.stages || []);
    host.hidden = false;
    const status = stageStatusFor(agg);
    const meta = [];
    meta.push(`<span class="k">order</span>=<span class="v">${agg.order + 1}</span>`);
    meta.push(`<span class="k">in-flight</span>=<span class="v ${agg.in_flight ? "warn" : ""}">${agg.in_flight}</span>`);
    meta.push(`<span class="k">done</span>=<span class="v done">${agg.completed}</span>`);
    if (agg.errored) meta.push(`<span class="k">err</span>=<span class="v failed">${agg.errored}</span>`);
    if (agg.throughput_per_min) meta.push(`<span class="k">tp</span>=<span class="v">${agg.throughput_per_min.toFixed(1)}/min</span>`);
    if (agg.p50_ms || agg.p99_ms) meta.push(`<span class="k">p50</span>=<span class="v">${agg.p50_ms}ms</span> <span class="k">p99</span>=<span class="v">${agg.p99_ms}ms</span>`);
    const constituents = renderConstituentsTable(agg);
    const errBar = agg.last_error
      ? `<div class="stage-error"><span class="pill failed">last error</span>
           <span class="meta-line">${agg.last_error._substage ? `<span class="chip">${escapeHTML(agg.last_error._substage)}</span> ` : ""}<span class="v failed">${escapeHTML(agg.last_error.msg)}</span>${
             agg.last_error.uri ? ` &nbsp; ${renderInlineURI(agg.last_error.uri)}` : ""}</span></div>`
      : "";
    const ins = (agg.recent_inputs || []).slice(0, 10);
    const outs = (agg.recent_outputs || []).slice(0, 10);
    const ioBlock = (agg.members.length > 0)
      ? `<div class="detail-io">
          <div class="io-col">
            <div class="io-head">recent inputs (${ins.length})</div>
            ${ins.length ? `<ul class="io-list">${ins.map(renderInputItemWithSub).join("")}</ul>` : `<div class="io-empty">no recent inputs</div>`}
          </div>
          <div class="io-col">
            <div class="io-head">recent outputs (${outs.length})</div>
            ${outs.length ? `<ul class="io-list">${outs.map(renderOutputItemWithSub).join("")}</ul>` : `<div class="io-empty">no recent outputs</div>`}
          </div>
        </div>`
      : renderExternalStagePanel(agg);
    host.innerHTML = `
      <div class="detail-head">
        <div class="detail-title">
          <span class="stage-order">${agg.order + 1}</span>
          <span class="stage-name">${escapeHTML(agg.title)}</span>
          <span class="pill ${status}">${status}</span>
        </div>
        <div class="meta-line">${meta.join(" &nbsp; ")}</div>
      </div>
      ${constituents}
      ${errBar}
      ${renderContextualLLMCards(group)}
      ${ioBlock}`;
  }
  // renderContextualLLMCards inlines compact summaries of the
  // models + scheduler tables when an LLM-bearing stage is focused.
  // Reads the hidden host tables (loadModels + loadScheduler keep them
  // populated) and projects a 3-column condensed view per surface.
  // Empty string for non-LLM stages.
  function renderContextualLLMCards(group) {
    if (!group || !group.llm) return "";
    const modelsBody = $("#models-body");
    const schedBody  = $("#scheduler-body");
    const buildModelsRows = () => {
      if (!modelsBody) return "";
      const rows = Array.from(modelsBody.querySelectorAll("tr"))
        .map((tr) => {
          const tds = tr.querySelectorAll("td");
          if (tds.length < 5 || tr.querySelector(".empty")) return "";
          return `<tr>
            <td class="cell-name">${tds[0].textContent.trim()}</td>
            <td class="cell-name">${tds[1].textContent.trim()}</td>
            <td>${tds[4].innerHTML.trim()}</td>
          </tr>`;
        })
        .filter(Boolean)
        .join("");
      return rows || `<tr><td colspan="3" class="empty">no models loaded</td></tr>`;
    };
    const buildSchedRows = () => {
      if (!schedBody) return "";
      const rows = Array.from(schedBody.querySelectorAll("tr"))
        .map((tr) => {
          const tds = tr.querySelectorAll("td");
          if (tds.length < 7 || tr.querySelector(".empty")) return "";
          return `<tr>
            <td class="cell-name">${tds[2].textContent.trim()}</td>
            <td class="cell-num">${tds[3].textContent.trim()}</td>
            <td class="cell-num">${tds[4].textContent.trim()}</td>
            <td class="cell-num">${tds[6].textContent.trim()}</td>
          </tr>`;
        })
        .filter(Boolean)
        .join("");
      return rows || `<tr><td colspan="4" class="empty">scheduler idle</td></tr>`;
    };
    return `<div class="contextual-llm-cards">
      <div class="contextual-card">
        <div class="contextual-head">models in use</div>
        <table class="constituents-table">
          <thead><tr><th>role</th><th>model</th><th>state</th></tr></thead>
          <tbody>${buildModelsRows()}</tbody>
        </table>
      </div>
      <div class="contextual-card">
        <div class="contextual-head">enrichment queue</div>
        <table class="constituents-table">
          <thead><tr><th>model</th><th class="num">depth</th><th class="num">drained</th><th class="num">batch</th></tr></thead>
          <tbody>${buildSchedRows()}</tbody>
        </table>
      </div>
    </div>`;
  }
  // renderConstituentsTable renders one row per constituent substage
  // inside the focused stage's detail panel. Each row carries its own
  // pill, name, counters, and p50. Empty for cluster/serve where no
  // substage is registered.
  function renderConstituentsTable(agg) {
    if (!agg.members || agg.members.length === 0) return "";
    const rows = agg.members.map((m) => {
      const mf = pipelineState.manifests[m.name] || {};
      const subStatus = stageStatus(m);
      return `<tr class="constituent-row" data-substage="${escapeHTML(m.name)}">
        <td><span class="pill ${subStatus}">${subStatus}</span></td>
        <td class="cell-name">${escapeHTML(m.name)} ${renderLLMBadge(mf.llm)}</td>
        <td class="cell-num">${m.in_flight}</td>
        <td class="cell-num done">${m.completed}</td>
        <td class="cell-num ${m.errored ? "failed" : ""}">${m.errored}</td>
        <td class="cell-num">${m.p50_ms || "·"}<span class="cell-suffix">ms</span></td>
      </tr>`;
    }).join("");
    return `<div class="constituents-block">
      <div class="constituents-head">constituent substages (${agg.members.length})</div>
      <table class="constituents-table">
        <thead><tr><th></th><th>name</th><th>flight</th><th>done</th><th>err</th><th>p50</th></tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
  }
  // renderExternalStagePanel renders the placeholder for stages that
  // have no per-Job substages (cluster, serve). When /api/cluster +
  // /api/serve land (slice G) these grow real counters; today they
  // explain themselves.
  function renderExternalStagePanel(agg) {
    if (agg.id === "cluster") {
      return `<div class="external-stage-panel">
        <p>Background community-detection stage. Runs out-of-band on a periodic scheduler; no per-Job substages.</p>
        <p>Run <code>kg cluster</code> to force an immediate sweep, or wait for the next scheduler tick.</p>
        <p class="meta-line"><span class="k">endpoint</span>=<span class="v">${escapeHTML(agg.external || "api/cluster")}</span> <span class="chip">not wired yet</span></p>
      </div>`;
    }
    if (agg.id === "serve") {
      return `<div class="external-stage-panel">
        <p>Read-only external surface (MCP stdio + REST + SSE). Counters: MCP clients, tool-call totals, REST request rate.</p>
        <p class="meta-line"><span class="k">endpoint</span>=<span class="v">${escapeHTML(agg.external || "api/serve")}</span> <span class="chip">not wired yet</span></p>
      </div>`;
    }
    return "";
  }
  // renderInputItemWithSub / renderOutputItemWithSub wrap the existing
  // render*Item helpers and prepend a substage chip so the focused
  // panel attributes each row to its source substage.
  function renderInputItemWithSub(in_) {
    const base = renderInputItem(in_);
    if (!in_._substage) return base;
    return base.replace(/^<li>/, `<li><span class="chip chip-sub">${escapeHTML(in_._substage)}</span> `);
  }
  function renderOutputItemWithSub(o) {
    const base = renderOutputItem(o);
    if (!o._substage) return base;
    return base.replace(/^<li>/, `<li><span class="chip chip-sub">${escapeHTML(o._substage)}</span> `);
  }
  function renderStageDescription(mf) {
    if (!mf || (!mf.description && !(mf.algorithms && mf.algorithms.length) && !(mf.depends_on && mf.depends_on.length))) return "";
    const depsLine = (mf.depends_on && mf.depends_on.length)
      ? `<div class="stage-deps"><span class="deps-label">runs after</span> ${mf.depends_on.map((d) => `<span class="chip">${escapeHTML(d)}</span>`).join("")}</div>` : "";
    const algoBlock = (mf.algorithms && mf.algorithms.length)
      ? `<details class="stage-algos"><summary>algorithms (${mf.algorithms.length})</summary><ul>${mf.algorithms.map((a) => `<li>${escapeHTML(a)}</li>`).join("")}</ul></details>` : "";
    return `<div class="stage-about">${mf.description ? `<p class="stage-desc">${escapeHTML(mf.description)}</p>` : ""}${depsLine}${algoBlock}</div>`;
  }
  function renderLLMBadge(llm) {
    if (!llm || llm === "none") return "";
    const cls = llm.includes("embed") ? "llm-embed" : llm.includes("vision") ? "llm-vision" : "llm-chat";
    const label = llm.length > 20 ? llm.slice(0, 18) + "…" : llm;
    return `<span class="pill ${cls}" title="LLM-bound: ${escapeHTML(llm)}">${escapeHTML(label)}</span>`;
  }
  function renderInlineURI(uri) {
    return `<a class="uri-link" data-uri="${escapeHTML(uri)}" title="${escapeHTML(uri)}">${escapeHTML(shortenURI(uri))}</a>`;
  }
  function renderInputItem(in_) {
    const uri = in_.uri || "";
    const isArchive = /\.(zip|tar|tar\.gz|tgz|tbz2|7z|rar)$/i.test(uri);
    const extra = isArchive ? ` <a class="dl-link" href="api/file?uri=${encodeURIComponent(uri)}&download=1" title="download">⬇</a>` : "";
    return `<li><span class="io-ts">${fmtTime(in_.at)}</span>
      <a class="uri-link" data-uri="${escapeHTML(uri)}" title="${escapeHTML(uri)}">${escapeHTML(shortenURI(uri))}</a>${extra}</li>`;
  }
  function renderOutputItem(o) {
    const label = o.label || o.id || "";
    if (o.kind === "converted" || o.kind === "cache-hit") {
      const sourceURI = correlateOutputURI(o);
      if (sourceURI) {
        return `<li><span class="io-ts">${fmtTime(o.at)}</span>
          <span class="badge">${escapeHTML(o.kind)}</span>
          <a class="uri-link" data-uri="${escapeHTML(sourceURI)}" data-mode="compare" title="open side-by-side">${escapeHTML(label)}</a></li>`;
      }
    }
    return `<li><span class="io-ts">${fmtTime(o.at)}</span>
      <span class="badge">${escapeHTML(o.kind || "")}</span>
      <span class="io-label">${escapeHTML(label)}</span></li>`;
  }
  function correlateOutputURI(o) {
    const snap = pipelineState.snap;
    if (!snap || !pipelineState.selected) return "";
    const st = (snap.stages || []).find((s) => s.name === pipelineState.selected);
    if (!st) return "";
    const ins = st.recent_inputs || [];
    if (!ins.length) return "";
    const outAt = new Date(o.at).getTime();
    let best = null, bestGap = Infinity;
    for (const in_ of ins) {
      const inAt = new Date(in_.at).getTime();
      const gap = Math.abs(outAt - inAt);
      if (gap < bestGap) { best = in_; bestGap = gap; }
    }
    return best ? best.uri : "";
  }

  // ─── /api/query → projects card ─────────────────────────────────
  async function loadProjects() {
    try {
      const r = await fetch("api/query", {
        method: "POST",
        body: `{ q(func: eq(node_type, "Project"), first: 20) { node_id label path corpus_id } }`,
      });
      if (!r.ok) throw new Error("HTTP " + r.status);
      const j = await r.json();
      renderProjects(j.q || []);
    } catch (e) {
      const host = $("#projects-list");
      if (host) host.innerHTML = `<div class="stat empty">projects unavailable: ${escapeHTML(String(e))}</div>`;
    }
  }
  function renderProjects(rows) {
    const host = $("#projects-list");
    if (!host) return;
    if (rows.length === 0) { host.innerHTML = `<div class="stat empty">no codebases detected yet</div>`; return; }
    host.innerHTML = "";
    for (const p of rows) {
      const row = document.createElement("div");
      row.className = "project-row";
      const corpus = p.corpus_id
        ? `<span class="pill accent">corpus ${escapeHTML(p.corpus_id)}</span>`
        : `<span class="pill skipped">summary</span>`;
      const uri = p.path ? "file://" + p.path : "";
      const link = uri
        ? `<a class="uri-link" data-uri="${escapeHTML(uri)}" title="${escapeHTML(p.path)}">${escapeHTML(shortenURI(uri))}</a>`
        : `<span class="muted">—</span>`;
      row.innerHTML = `<div class="project-head"><span class="project-label">${escapeHTML(p.label || "(unnamed)")}</span>${corpus}</div>
        <div class="meta-line">${link}</div>`;
      host.appendChild(row);
    }
  }

  // ─── SSE event log ─────────────────────────────────────────────
  const EVENT_LIMIT = 200;
  let firstEvent = true;
  function appendEvent(ev) {
    const log = $("#events-log");
    if (!log) return;
    if (firstEvent) { log.innerHTML = ""; firstEvent = false; }
    const row = document.createElement("div");
    row.className = "event t-" + (ev.type || "unknown").replace(/[^a-z0-9_]/gi, "_");
    row.innerHTML = `<span class="ts">${fmtTime(ev.at)}</span>
      <span class="type">${escapeHTML(ev.type || "?")}</span>
      <span class="body">${renderPayload(ev.payload)}</span>`;
    log.appendChild(row);
    while (log.children.length > EVENT_LIMIT) log.removeChild(log.firstChild);
    log.scrollTop = log.scrollHeight;
  }
  function renderPayload(p) {
    if (!p) return "";
    if (typeof p === "string") return escapeHTML(p);
    const parts = [];
    for (const [k, v] of Object.entries(p)) {
      if (v === null || v === undefined || v === "") continue;
      if (typeof v === "object") {
        const inner = Object.entries(v).map(([kk, vv]) => `${escapeHTML(kk)}=${escapeHTML(String(vv))}`).join(" ");
        parts.push(`<span class="k">${escapeHTML(k)}</span>=${inner}`);
        continue;
      }
      let valueHTML, cls = "v";
      if (k === "uri" && typeof v === "string" && v.startsWith("file://")) {
        valueHTML = `<a class="uri-link" data-uri="${escapeHTML(v)}" title="${escapeHTML(v)}">${escapeHTML(shortenURI(v))}</a>`;
        cls = "v uri";
      } else {
        cls = k === "error" ? "v err" : (k === "uri" ? "v uri" : "v");
        valueHTML = escapeHTML(String(v));
      }
      parts.push(`<span class="k">${escapeHTML(k)}</span>=<span class="${cls}">${valueHTML}</span>`);
    }
    return parts.join(" &nbsp; ");
  }

  // ─── /api/llm/pool (model picker catalog) ───────────────────────
  // Cached so each row's <select> renders without an extra fetch.
  // Refreshed alongside loadModels() so a model that appeared on the
  // upstream while the UI was open shows up within one poll cycle.
  let __pickerPool = null;
  let __pickerPoolError = "";
  async function loadPickerPool() {
    try {
      const r = await fetch("api/llm/pool");
      if (!r.ok) {
        __pickerPool = null;
        // Body is the upstream error message — surface it dimly so
        // operators know why the picker is empty (e.g. 401 → token).
        __pickerPoolError = (await r.text()).trim();
        return;
      }
      __pickerPool = await r.json();
      __pickerPoolError = "";
    } catch (e) {
      __pickerPool = null;
      __pickerPoolError = String(e);
    }
  }

  function renderPickerMeta() {
    const meta = $("#picker-pool-meta");
    if (!meta) return;
    if (Array.isArray(__pickerPool) && __pickerPool.length > 0) {
      meta.hidden = false;
      meta.textContent = `picker catalog: ${__pickerPool.length} model${__pickerPool.length === 1 ? "" : "s"} from /api/llm/pool`;
    } else if (__pickerPoolError) {
      meta.hidden = false;
      meta.textContent = `picker catalog unavailable: ${__pickerPoolError.slice(0, 200)}`;
    } else {
      meta.hidden = true;
    }
  }

  async function swapRole(role, model, statusEl) {
    if (statusEl) {
      statusEl.hidden = false;
      statusEl.textContent = `swapping ${role} → ${model}…`;
      statusEl.style.color = "#888";
    }
    try {
      const r = await fetch("api/llm/swap", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ role, model }),
      });
      const txt = (await r.text()).trim();
      if (!r.ok) {
        if (statusEl) {
          statusEl.style.color = "#c33";
          statusEl.textContent = `swap failed (${r.status}): ${txt.slice(0, 300)}`;
        }
        return;
      }
      if (statusEl) {
        statusEl.style.color = "#2a7";
        statusEl.textContent = `swapped ${role} → ${model}`;
      }
      // Refresh the table so the new model name + state appear immediately.
      await loadModels();
    } catch (e) {
      if (statusEl) {
        statusEl.style.color = "#c33";
        statusEl.textContent = `swap error: ${e}`;
      }
    }
  }

  // ─── /api/models (M5: Models in use card) ───────────────────────
  async function loadModels() {
    const card = $("#models-card");
    const banner = $("#banner-llm-down");
    // Refresh the picker catalog in parallel — independent fetch.
    const poolP = loadPickerPool();
    try {
      const r = await fetch("api/models");
      if (!r.ok) {
        if (card) card.hidden = true;
        if (banner) banner.hidden = true;
        return;
      }
      const rows = await r.json();
      if (!Array.isArray(rows) || rows.length === 0) {
        if (card) card.hidden = true;
        if (banner) banner.hidden = true;
        return;
      }
      await poolP;
      if (card) card.hidden = false;
      const body = $("#models-body");
      const statusEl = $("#picker-status");
      if (body) {
        body.innerHTML = "";
        const poolModels = Array.isArray(__pickerPool) ? __pickerPool : [];
        for (const m of rows) {
          const tr = document.createElement("tr");
          const stateClass = (m.state === "up") ? "pill ok" : (m.state === "degraded" ? "pill warn" : "pill err");
          const errCell = m.last_error ? ` <span class="muted" title="${escapeHTML(m.last_error)}">⚠</span>` : "";

          // Action cell: dropdown of pool models + an Apply button. Hidden
          // when the pool fetch failed so the picker doesn't pretend.
          let actionHTML = "";
          if (poolModels.length > 0) {
            const opts = poolModels
              .map((p) => `<option value="${escapeHTML(p.id)}"${p.id === m.model ? " selected" : ""}>${escapeHTML(p.id)}</option>`)
              .join("");
            actionHTML = `
              <select class="picker-select" data-role="${escapeHTML(m.role || "")}">${opts}</select>
              <button class="picker-apply" data-role="${escapeHTML(m.role || "")}">apply</button>`;
          } else {
            actionHTML = `<span class="muted">no catalog</span>`;
          }

          tr.innerHTML = `
            <td><span class="badge">${escapeHTML(m.role || "")}</span></td>
            <td>${escapeHTML(m.model || "")}</td>
            <td><code class="endpoint">${escapeHTML(m.endpoint || "")}</code></td>
            <td>${escapeHTML(m.backend || "")}</td>
            <td><span class="${stateClass}">${escapeHTML(m.state || "")}</span>${errCell}</td>
            <td>${actionHTML}</td>`;
          body.appendChild(tr);
        }
        // Wire up apply buttons; one event listener per row is fine —
        // the table re-renders on every loadModels poll.
        body.querySelectorAll("button.picker-apply").forEach((btn) => {
          btn.addEventListener("click", () => {
            const role = btn.dataset.role || "";
            const sel = body.querySelector(`select.picker-select[data-role="${role}"]`);
            const model = sel ? sel.value : "";
            if (!role || !model) return;
            swapRole(role, model, statusEl);
          });
        });
      }
      renderPickerMeta();
      // Banner: every configured role is down → LLM enrichment unavailable.
      const allDown = rows.every((m) => m.state === "down");
      if (banner) banner.hidden = !allDown;
    } catch (e) {
      if (card) card.hidden = true;
      if (banner) banner.hidden = true;
    }
  }

  // ─── /api/scheduler/live (M5: Enrichment queue card) ────────────
  async function loadScheduler() {
    const card = $("#scheduler-card");
    try {
      const r = await fetch("api/scheduler/live");
      if (!r.ok) {
        if (card) card.hidden = true;
        return;
      }
      const j = await r.json();
      const qs = j.queues || [];
      if (qs.length === 0) {
        if (card) card.hidden = true;
        return;
      }
      if (card) card.hidden = false;
      const body = $("#scheduler-body");
      if (body) {
        body.innerHTML = "";
        for (const q of qs) {
          const tr = document.createElement("tr");
          const activeMark = q.drainer_active ? `<span class="pill ok">●</span>` : `<span class="muted">○</span>`;
          tr.innerHTML = `
            <td>${escapeHTML(q.backend || "")}</td>
            <td><code class="endpoint">${escapeHTML(q.endpoint || "")}</code></td>
            <td>${escapeHTML(q.model || "")} ${activeMark}</td>
            <td class="num">${q.depth ?? 0}</td>
            <td class="num">${q.drained_total ?? 0}</td>
            <td class="num">${q.overload_count ?? 0}</td>
            <td class="num">${(q.last_batch_count ?? 0)} × ${q.last_batch_ms ?? 0}ms</td>`;
          body.appendChild(tr);
        }
      }
    } catch (e) {
      if (card) card.hidden = true;
    }
  }

  // ─── ETA panel (under the pipeline card) ────────────────────────
  function renderETA(live) {
    const panel = $("#pipeline-eta");
    if (!panel || !live || !live.eta) return;
    const e = live.eta;
    if (!e.avg_doc_ms && !e.eta_seconds) {
      panel.hidden = true;
      return;
    }
    panel.hidden = false;
    const fmt = (s) => {
      if (s <= 0) return "idle";
      if (s < 60) return `${s}s`;
      if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`;
      return `≥1h`;
    };
    panel.innerHTML = `
      <span class="meta-line">
        ETA <strong>${fmt(e.eta_seconds || 0)}</strong>
        &nbsp;·&nbsp; avg <strong>${e.avg_doc_ms || 0} ms</strong>/doc
        &nbsp;·&nbsp; ${e.jobs_inflight || 0} in flight
      </span>`;
  }

  // Hook ETA rendering onto loadPipeline.
  const _origLoadPipeline = loadPipeline;
  loadPipeline = async function () {
    await _origLoadPipeline();
    try {
      const r = await fetch("api/substages/live");
      if (r.ok) renderETA(await r.json());
    } catch (_) { /* keep panel as-is */ }
  };

  // ─── /api/roots ─────────────────────────────────────────────────
  async function loadRoots() {
    const tbody = $("#roots-body");
    if (!tbody) return;
    try {
      const r = await fetch("api/roots");
      if (!r.ok) return;
      const j = await r.json();
      renderRoots(j.roots || []);
    } catch (_) {}
  }
  function renderRoots(roots) {
    const tbody = $("#roots-body");
    const meta = $("#roots-totals");
    if (!tbody) return;
    if (meta) meta.textContent = `${roots.length} roots`;
    if (roots.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="empty">no roots indexed yet — add one above</td></tr>';
      return;
    }
    tbody.innerHTML = "";
    for (const r of roots) {
      const tr = document.createElement("tr");
      const inc = (r.includes || []).join(", ") || "*";
      const exc = (r.excludes || []).join(", ") || "-";
      const statusClass = r.status === "active" ? "ok" : (r.status === "paused" ? "warn" : "muted");
      tr.innerHTML = `
        <td><strong>${escapeHTML(r.path)}</strong></td>
        <td><small class="muted">${escapeHTML(r.resolved)}</small></td>
        <td><span class="pill ${statusClass}">${escapeHTML(r.status || "active")}</span></td>
        <td class="num">${r.file_count || 0}</td>
        <td><small>${escapeHTML(inc)}</small></td>
        <td><small>${escapeHTML(exc)}</small></td>
        <td>
          <button type="button" class="btn-mini btn-remove-root" data-path="${escapeHTML(r.path)}">forget</button>
        </td>
      `;
      tbody.appendChild(tr);
    }
    tbody.querySelectorAll(".btn-remove-root").forEach((btn) => {
      btn.addEventListener("click", async () => {
        const p = btn.getAttribute("data-path");
        if (!p) return;
        const response = await fetch("api/roots?path=" + encodeURIComponent(p), {
          method: "DELETE",
          headers: { "X-KB-UI": "1" },
        });
        if (!response.ok) { const meta=$("#roots-totals"); if(meta) meta.textContent=await response.text(); return; }
        loadRoots();
      });
    });
  }
  function wireRoots() {
    const addBtn = $("#btn-add-root");
    const input = $("#root-input");
    if (addBtn && input) {
      addBtn.addEventListener("click", async () => {
        const p = input.value.trim();
        if (!p) return;
        const response = await fetch("api/roots", {
          method: "POST",
          headers: { "Content-Type": "application/json", "X-KB-UI": "1" },
          body: JSON.stringify({ path: p }),
        });
        if (!response.ok) { const meta=$("#roots-totals"); if(meta) meta.textContent=await response.text(); return; }
        input.value = "";
        loadRoots();
      });
      input.addEventListener("keydown", (e) => {
        if (e.key === "Enter") addBtn.click();
      });
    }
  }

  // ─── Wiring ─────────────────────────────────────────────────────
  window.kgMonitorInit = function () {
    loadRoots();
    wireRoots();
    loadStats();
    loadEnrichment();
    loadRuns();
    loadPipeline();
    loadSubstageManifests();
    loadProjects();
    loadScan();
    loadModels();
    loadScheduler();
    // Backstop polls.
    setInterval(() => { loadRoots(); loadStats(); loadRuns(); loadProjects(); }, 15000);
    setInterval(loadEnrichment, 10000);
    setInterval(loadPipeline, 2000);
    setInterval(loadScan, 2000);
    setInterval(loadModels, 5000);
    setInterval(loadScheduler, 2000);
    // SSE → cheap refreshes.
    onEvent("*", appendEvent);
    onEvent("root_added",   loadRoots);
    onEvent("root_removed", loadRoots);
    onEvent("ingest_done", (ev) => {
      const p = ev && ev.payload;
      if (p && p.uri && typeof p.duration_ms === "number") rememberDuration(p.uri, p.duration_ms);
      loadRoots(); loadStats(); loadRuns(); loadPipeline(); loadEnrichment();
    });
    onEvent("forget",      () => { loadRoots(); loadStats(); loadRuns(); loadPipeline(); loadEnrichment(); });
    ["ingest_start", "ingest_error", "ingest_skip"].forEach((t) => onEvent(t, loadPipeline));
    onEvent("ingest_deferred", loadScheduler);
    onEvent("llm_batch_done",   loadScheduler);
    onEvent("enricher_tick", loadEnrichment);
  };
})();
