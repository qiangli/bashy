// kg-ui insights tab — artifact distribution + corpus breakdown +
// top entities. Lazy-loaded on first activation; refreshed every 30s
// while the tab is visible.

(function () {
  const { $, $$, escapeHTML, escapeAttr, colorForArtifact, onEvent } = window.kg;

  let booted = false;
  let refreshTimer = 0;

  async function loadArtifacts() {
    try {
      const r = await fetch("api/query", {
        method: "POST",
        body: `{ q(func: has(artifact)) @groupby(artifact) { count(uid) } }`,
      });
      if (!r.ok) return;
      const j = await r.json();
      const rows = (j.q && j.q[0] && j.q[0]["@groupby"]) || [];
      renderArtifacts(rows);
    } catch (_) {}
  }
  function renderArtifacts(rows) {
    const host = $("#artifacts-list");
    if (!host) return;
    if (rows.length === 0) { host.innerHTML = `<div class="empty">no artifacts indexed yet</div>`; return; }
    const max = rows.reduce((m, r) => Math.max(m, r.count), 0);
    host.innerHTML = rows.sort((a, b) => b.count - a.count).map((r) => {
      const pct = max > 0 ? Math.round((r.count / max) * 100) : 0;
      const color = colorForArtifact(r.artifact);
      return `<div class="art-bar-row">
        <div class="art-bar-label">
          <span class="art-dot" style="background:${color}"></span>
          ${escapeHTML(r.artifact)}
        </div>
        <div class="art-bar-track"><div class="art-bar-fill" style="width:${pct}%;background:${color}"></div></div>
        <div class="art-bar-num">${r.count}</div>
      </div>`;
    }).join("");
  }

  async function loadCorpora() {
    try {
      const r = await fetch("api/query", {
        method: "POST",
        body: `{ q(func: has(corpus_id)) @filter(eq(node_type, "Document")) @groupby(corpus_id) { count(uid) } }`,
      });
      if (!r.ok) return;
      const j = await r.json();
      const rows = (j.q && j.q[0] && j.q[0]["@groupby"]) || [];
      renderCorpora(rows);
    } catch (_) {}
  }
  function renderCorpora(rows) {
    const host = $("#corpus-list");
    if (!host) return;
    if (rows.length === 0) { host.innerHTML = `<div class="empty">user-authored only — no tagged corpora yet</div>`; return; }
    host.innerHTML = rows.sort((a, b) => b.count - a.count).map((r) => {
      return `<div class="corpus-row">
        <span class="corpus-label" title="${escapeAttr(r.corpus_id)}">${escapeHTML(r.corpus_id)}</span>
        <span class="corpus-num">${r.count}</span>
      </div>`;
    }).join("");
  }

  // Top entities: walks every entity NodeType, counts ~mentions for
  // each. One DQL pass with @count avoids fetching the mention rows.
  async function loadTopEntities() {
    try {
      const types = ["Person", "Organization", "Project", "Place", "Concept", "Tool", "Event"];
      const filters = types.map((t) => `eq(node_type, "${t}")`).join(" OR ");
      const r = await fetch("api/query", {
        method: "POST",
        body: `{
          q(func: has(node_type), first: 100) @filter((${filters}) AND NOT has(alias_of) AND NOT has(deleted_at)) {
            node_id node_type label
            mentions: count(~mentions)
          }
        }`,
      });
      if (!r.ok) return;
      const j = await r.json();
      let rows = (j.q || []).filter((r) => (r.mentions || 0) > 0);
      rows.sort((a, b) => (b.mentions || 0) - (a.mentions || 0));
      rows = rows.slice(0, 20);
      renderTopEntities(rows);
    } catch (_) {}
  }
  function renderTopEntities(rows) {
    const tbody = $("#entities-table tbody");
    if (!tbody) return;
    if (rows.length === 0) {
      tbody.innerHTML = `<tr><td colspan="3" class="empty">no entities yet — run an NER-enabled daemon</td></tr>`;
      return;
    }
    tbody.innerHTML = rows.map((r) => {
      const color = window.kg.colorForType(r.node_type);
      return `<tr>
        <td><span class="ent-label">${escapeHTML(r.label || "(unlabeled)")}</span></td>
        <td><span class="badge" style="--badge-accent:${color}">${escapeHTML(r.node_type)}</span></td>
        <td class="num">${r.mentions}</td>
      </tr>`;
    }).join("");
  }

  function refreshAll() {
    loadArtifacts();
    loadCorpora();
    loadTopEntities();
  }

  window.kgInsightsInit = function () {
    if (!booted) {
      booted = true;
      refreshAll();
      refreshTimer = setInterval(refreshAll, 30000);
      onEvent("ingest_done", () => { /* throttled — just refresh occasionally */ });
    } else {
      refreshAll();
    }
  };
})();
