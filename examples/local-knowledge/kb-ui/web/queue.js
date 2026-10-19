// kg-ui queue tab — pool / ledger / failed / budget (Opt.4).

(function () {
  const { $, $$, fmtTime, escapeHTML, humanBytes } = window.kg;

  async function loadPool() {
    try {
      const r = await fetch("api/metrics");
      if (!r.ok) {
        const tbody = $("#pool-table tbody");
        if (tbody) tbody.innerHTML = '<tr><td colspan="6"><em class="empty">legacy worker pool unavailable in bashy search index</em></td></tr>';
        return;
      }
      const j = await r.json();
      renderPool(j);
      renderBudget(j.Budget || j.budget || {});
    } catch (_) {}
  }
  function renderPool(j) {
    const tbody = $("#pool-table tbody");
    if (!tbody) return;
    tbody.innerHTML = "";
    const pools = ["interactive", "batch", "background"];
    let totalRunning = 0, totalWaiting = 0;
    for (const p of pools) {
      const get = (m) => (j[m] && j[m][p] != null) ? j[m][p] : 0;
      const running = get("Running"), waiting = get("Waiting");
      totalRunning += running;
      totalWaiting += waiting;
      const row = document.createElement("tr");
      row.className = "pool-" + p;
      const activity = running > 0 ? " active" : "";
      row.classList.toggle("active", running > 0);
      row.innerHTML = `<td class="lane lane-${p}${activity}"><span class="lane-dot"></span>${p}</td>
        <td class="num">${get("Workers")}</td>
        <td class="num">${running}</td>
        <td class="num">${waiting}</td>
        <td class="num">${get("Submitted")}</td>
        <td class="num">${get("Rejected")}</td>`;
      tbody.appendChild(row);
    }
    const meta = $("#pool-meta");
    if (meta) meta.textContent = `running ${totalRunning} · waiting ${totalWaiting}`;
  }
  function renderBudget(b) {
    const card = $("#budget-card");
    const host = $("#budget-stats");
    if (!card || !host) return;
    if (!b || !b.max_cpu_percent) { card.hidden = true; return; }
    card.hidden = false;
    const parkedSec = Math.round((b.total_parked_ms || 0) / 100) / 10;
    host.innerHTML = `
      <div class="bud-tile"><div class="bud-num">${b.max_cpu_percent}%</div><div class="bud-label">CPU cap</div></div>
      <div class="bud-tile"><div class="bud-num">${b.waits || 0}</div><div class="bud-label">waits</div></div>
      <div class="bud-tile"><div class="bud-num">${parkedSec}s</div><div class="bud-label">parked total</div></div>`;
  }

  async function loadLedger() {
    try {
      const r = await fetch("api/queue/stats");
      const row = $("#ledger-row");
      if (!r.ok) { if (row) row.innerHTML = '<td colspan="4"><em class="empty">ledger unavailable in bashy search index</em></td>'; return; }
      const j = await r.json();
      if (row) row.innerHTML = `<td>${j.pending || 0}</td><td>${j.running || 0}</td><td>${j.done || 0}</td><td class="failed-cell">${j.failed || 0}</td>`;
    } catch (_) {}
  }
  async function loadFailed() {
    const tbody = $("#failed-table tbody");
    if (!tbody) return;
    try {
      const r = await fetch("api/queue/failed?limit=25");
      if (!r.ok) { tbody.innerHTML = '<tr><td colspan="5"><em class="empty">failed list unavailable in bashy search index</em></td></tr>'; return; }
      const rows = (await r.json()) || [];
      if (rows.length === 0) { tbody.innerHTML = '<tr><td colspan="5"><em class="empty">no failures recorded</em></td></tr>'; return; }
      tbody.innerHTML = "";
      for (const row of rows) {
        const tr = document.createElement("tr");
        const finished = row.finished_at ? new Date(row.finished_at).toLocaleString() : "";
        const pathCell = row.path
          ? `<a class="uri-link" data-uri="${escapeHTML(row.path)}" title="${escapeHTML(row.path)}">${escapeHTML(row.path)}</a>` : "";
        tr.innerHTML = `<td>${row.id}</td>
          <td class="path">${pathCell}</td>
          <td class="num">${row.attempts || 0}</td>
          <td>${finished}</td>
          <td class="err">${escapeHTML(row.last_error || "")}</td>`;
        tbody.appendChild(tr);
      }
    } catch (e) {
      tbody.innerHTML = `<tr><td colspan="5"><em class="empty">${escapeHTML(String(e))}</em></td></tr>`;
    }
  }

  window.kgQueueInit = function () {
    loadPool();
    loadLedger();
    loadFailed();
    setInterval(() => { loadPool(); loadLedger(); }, 2000);
    setInterval(loadFailed, 5000);
  };
})();
