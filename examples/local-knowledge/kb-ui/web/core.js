// kg-ui core — shared helpers + SSE hub + hash router + header pills.
// Loaded before every per-tab module so they can subscribe to events
// and share one EventSource connection across tabs.

const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

const fmtTime = (iso) => {
  if (!iso) return "";
  const d = new Date(iso);
  return d.toISOString().substring(11, 19);
};

function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  }[c]));
}
const escapeAttr = escapeHTML;

function shortenURI(uri) {
  if (!uri) return "";
  if (!uri.startsWith("file://")) return uri;
  const path = uri.slice("file://".length);
  if (path.length < 60) return path;
  const parts = path.split("/").filter(Boolean);
  return "…/" + parts.slice(-3).join("/");
}

function shortenPath(p) {
  if (!p) return "";
  if (p.length < 80) return p;
  const parts = p.split("/").filter(Boolean);
  return "…/" + parts.slice(-3).join("/");
}

function humanBytes(n) {
  if (n == null) return "unknown";
  if (n <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
  return `${n.toFixed(n < 10 ? 1 : 0)} ${units[i]}`;
}

function compactNumber(n) {
  if (n < 1000) return String(n);
  if (n < 10_000) return (n / 1000).toFixed(1).replace(/\.0$/, "") + "k";
  return Math.round(n / 1000) + "k";
}

// ─── Color palette for NodeType + Artifact + Bucket ──────────────────
// Shared between the 3d graph, the hit list, the discovery card bars,
// and the type/artifact filter chips so the user can scan all four
// surfaces with the same color memory.
const TYPE_COLORS = {
  Document:        "#5b9eff",
  Topic:           "#a78bfa",
  Person:          "#fb923c",
  Organization:    "#fbbf24",
  Project:         "#22d3ee",
  Concept:         "#84cc16",
  Task:            "#f472b6",
  Place:           "#94a3b8",
  Tool:            "#a78bfa",
  Event:           "#5eead4",
  File:            "#64748b",
  Directory:       "#475569",
  "Code:Module":   "#94a3b8",
  "Code:Function": "#60a5fa",
  "Code:Class":    "#c084fc",
  ExtractionRun:   "#374151",
};
function colorForType(t) { return TYPE_COLORS[t] || "#60a5fa"; }

const ARTIFACT_COLORS = {
  log:           "#facc15",
  trace:         "#f97316",
  metric:        "#06b6d4",
  "oci-image":   "#a3e635",
  "llm-model":   "#c084fc",
  "agent-skill": "#22d3ee",
  "project-repo": "#34d399",
};
function colorForArtifact(a) { return ARTIFACT_COLORS[a] || "#94a3b8"; }

const BUCKET_CLASS = {
  "file/document": "b-document",
  "file/code":     "b-code",
  "file/media":    "b-media",
  "file/log":      "b-log",
  "file/binary":   "b-binary",
  "file/archive":  "b-archive",
  "file/symlink":  "b-symlink",
  "file/trace":    "b-trace",
  "file/metric":   "b-metric",
};
function bucketClass(b) {
  if (BUCKET_CLASS[b]) return BUCKET_CLASS[b];
  if (b.startsWith("dir/")) return "b-dir";
  return "b-misc";
}

// ─── Tab router ──────────────────────────────────────────────────────
// Hash-based: #/explore #/monitor #/queue #/insights. Default explore.
// Active tab fires `tab:show` so per-tab modules can lazy-init.
const TAB_DEFAULT = "explore";
const VALID_TABS = ["explore", "monitor", "queue", "insights"];

function currentTab() {
  const h = (location.hash || "").replace(/^#\/?/, "");
  return VALID_TABS.includes(h) ? h : TAB_DEFAULT;
}

function activateTab(name) {
  $$(".tab").forEach((a) => a.classList.toggle("active", a.dataset.tab === name));
  $$(".tab-panel").forEach((p) => { p.hidden = p.dataset.tab !== name; });
  window.dispatchEvent(new CustomEvent("tab:show", { detail: { tab: name } }));
}

window.addEventListener("hashchange", () => activateTab(currentTab()));

// ─── Theme toggle ────────────────────────────────────────────────────
function initTheme() {
  const saved = localStorage.getItem("kg.theme");
  if (saved) document.documentElement.setAttribute("data-theme", saved);
  const btn = $("#btn-theme");
  if (btn) btn.addEventListener("click", () => {
    const next = document.documentElement.getAttribute("data-theme") === "dark" ? "light" : "dark";
    document.documentElement.setAttribute("data-theme", next);
    localStorage.setItem("kg.theme", next);
  });
}

// ─── SSE hub ─────────────────────────────────────────────────────────
// One EventSource for the whole SPA. Modules subscribe via
// kg.onEvent(type, handler). Wildcard '*' fires on every event.
const sseSubs = new Map(); // type → [fn]
function onEvent(type, fn) {
  const list = sseSubs.get(type) || [];
  list.push(fn);
  sseSubs.set(type, list);
}
function fireEvent(ev) {
  const t = ev.type || "unknown";
  (sseSubs.get(t) || []).forEach((fn) => { try { fn(ev); } catch (_) {} });
  (sseSubs.get("*") || []).forEach((fn) => { try { fn(ev); } catch (_) {} });
}

function setConn(state, msg) {
  const el = $("#meta-conn");
  if (!el) return;
  el.className = "pill conn " + state;
  el.textContent = msg;
}

let sseConnOk = false;
function openSSE() {
  setConn("", "● connecting…");
  const es = new EventSource("events");
  es.onopen = () => { sseConnOk = true; setConn("ok", "● live"); };
  es.onerror = () => {
    setConn("fail", sseConnOk ? "● reconnecting…" : "● daemon unreachable");
    sseConnOk = false;
  };
  es.onmessage = (msg) => {
    try { fireEvent(JSON.parse(msg.data)); } catch (_) {}
  };
}

// ─── Service pills (system / ollama / podman / disk) ─────────────────
async function loadServicePill(name, sel, decorate) {
  const pill = $(sel);
  if (!pill) return;
  try {
    const r = await fetch("api/" + name);
    if (!r.ok) { pill.hidden = true; return; }
    const j = await r.json();
    pill.hidden = false;
    let label = name + " " + (j.state || "?");
    if (decorate && (j.state === "up" || j.state === "degraded")) {
      let detail = j.detail;
      if (typeof detail === "string") { try { detail = JSON.parse(detail); } catch (_) { detail = null; } }
      const extra = decorate(detail) || [];
      if (extra.length) label += " · " + extra.join(" · ");
      if (j.latency_ms) label += " · " + j.latency_ms + "ms";
    }
    pill.textContent = label;
    pill.className = "pill " + name + " " + j.state;
  } catch (_) { pill.hidden = true; }
}

async function loadSystemPill() {
  await loadServicePill("system", "#meta-system", (d) => {
    if (!d) return [];
    const parts = [];
    if (d.cpu_percent != null) parts.push("cpu " + d.cpu_percent + "%");
    if (d.mem_percent != null) parts.push("mem " + d.mem_percent + "%");
    if (d.quiet_active) parts.push("quiet ▦");
    return parts;
  });
}
async function loadOllamaPill() {
  await loadServicePill("ollama", "#meta-ollama", (d) => {
    if (!d || !d.model_count) return [];
    return [d.model_count + " models"];
  });
}
async function loadPodmanPill() {
  await loadServicePill("podman", "#meta-podman", (d) => {
    if (!d) return [];
    const parts = [];
    if (d.containers != null) parts.push(d.containers + " containers");
    if (d.images != null) parts.push(d.images + " images");
    return parts;
  });
}

async function loadDisk() {
  try {
    const r = await fetch("api/disk");
    if (!r.ok) return;
    const j = await r.json();
    renderDisk(j);
  } catch (_) {}
}
function renderDisk(state) {
  const pill = $("#meta-disk");
  const banner = $("#banner-disk");
  if (!state || !state.level) {
    if (pill) pill.hidden = true;
    if (banner) banner.hidden = true;
    return;
  }
  if (pill) {
    pill.hidden = false;
    pill.textContent = `disk ${humanBytes(state.avail_bytes)} free`;
    pill.className = "pill disk " + state.level;
  }
  if (state.level === "warn") {
    banner.hidden = false;
    banner.className = "banner warn";
    banner.innerHTML = `⚠ low disk — ${humanBytes(state.avail_bytes)} free under ${escapeHTML(state.path)}. ingest auto-pauses when free space crosses the pause threshold.`;
  } else if (state.level === "pause") {
    banner.hidden = false;
    banner.className = "banner critical";
    banner.innerHTML = `🛑 disk critical — ${humanBytes(state.avail_bytes)} free under ${escapeHTML(state.path)}. ingest auto-paused; free space then click resume.`;
  } else if (banner) {
    banner.hidden = true;
  }
}

// ─── Pause / resume control bar ──────────────────────────────────────
async function loadControl() {
  try {
    const r = await fetch("api/control");
    if (!r.ok) return;
    const j = await r.json();
    renderControl(j);
  } catch (_) {}
}
function renderControl(state) {
  const pauseBtn = $("#btn-pause");
  const resumeBtn = $("#btn-resume");
  const banner = $("#banner-pause");
  if (!pauseBtn || !resumeBtn) return;
  if (state.paused) {
    pauseBtn.hidden = true;
    resumeBtn.hidden = false;
    if (banner) {
      banner.hidden = false;
      const since = state.since && state.since !== "0001-01-01T00:00:00Z"
        ? ` since ${new Date(state.since).toLocaleTimeString()}` : "";
      banner.className = "banner warn";
      banner.innerHTML = `▮▮ ingestion paused${since} — click resume to continue.`;
    }
  } else {
    pauseBtn.hidden = false;
    resumeBtn.hidden = true;
    if (banner) banner.hidden = true;
  }
}
async function pauseDaemon()  { await fetch("api/control/pause",  { method: "POST", headers: { "X-KB-UI": "1" } }); loadControl(); }
async function resumeDaemon() { await fetch("api/control/resume", { method: "POST", headers: { "X-KB-UI": "1" } }); loadControl(); }

// ─── Public surface — kg namespace ───────────────────────────────────
window.kg = {
  $, $$, fmtTime, escapeHTML, escapeAttr, shortenURI, shortenPath,
  humanBytes, compactNumber,
  colorForType, colorForArtifact, bucketClass,
  onEvent,
  currentTab, activateTab,
  TYPE_COLORS, ARTIFACT_COLORS,
};

// ─── Initial wiring (called from app.js after every module loads) ───
window.kgCoreInit = function () {
  initTheme();
  // Wire control buttons.
  $("#btn-pause")?.addEventListener("click", pauseDaemon);
  $("#btn-resume")?.addEventListener("click", resumeDaemon);
  // Initial pill load.
  loadSystemPill();
  loadOllamaPill();
  loadPodmanPill();
  loadDisk();
  loadControl();
  // Polling backstops; SSE pushes drive immediate updates below.
  setInterval(loadSystemPill, 10000);
  setInterval(loadOllamaPill, 10000);
  setInterval(loadPodmanPill, 30000);
  setInterval(loadDisk,        5000);
  setInterval(loadControl,     5000);
  // SSE → pill refresh hooks.
  ["system_up", "system_degraded", "system_down", "system_unknown"].forEach((t) => onEvent(t, loadSystemPill));
  ["ollama_up", "ollama_degraded", "ollama_down", "ollama_unknown"].forEach((t) => onEvent(t, loadOllamaPill));
  ["podman_up", "podman_degraded", "podman_down", "podman_unknown"].forEach((t) => onEvent(t, loadPodmanPill));
  ["disk_ok", "disk_warn", "disk_pause"].forEach((t) => onEvent(t, () => { loadDisk(); if (t === "disk_pause") loadControl(); }));
  ["ingest_paused", "ingest_resumed"].forEach((t) => onEvent(t, loadControl));
  // Open the shared SSE stream.
  openSSE();
};
