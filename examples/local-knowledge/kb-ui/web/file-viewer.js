// file-viewer.js — singleton modal source-file viewer.
//
// Loaded by every kg-ui page (monitor / explorer / queue). Exposes
// window.openFileViewer(uri) which fetches /api/file?uri=… and
// renders the response in a native <dialog>. Anchors anywhere on
// the page with data-uri="file://…" trigger it via one document-
// level click delegator — no inline onclick handlers, no XSS
// surface (every uri lives in an HTML attribute, never an href
// or template literal that's eval'd).
//
// URL hash carries #view=<uri> so the open viewer is shareable —
// pasting the link in another tab auto-opens the same file.

(function () {
  let dialog;
  let elUri, elBody, elFooter, elCopyBtn, elCloseBtn;

  let elCompareWrap, elBodyL, elBodyR, elTabs;
  let currentMode = "single"; // "single" | "compare"
  let currentURI = "";

  function ensureDialog() {
    if (dialog) return dialog;
    dialog = document.createElement("dialog");
    dialog.id = "file-viewer";
    dialog.className = "file-viewer";
    dialog.innerHTML = `
      <div class="fv-header">
        <span class="fv-uri" title=""></span>
        <div class="fv-tabs" role="tablist">
          <button type="button" class="fv-tab" data-mode="single" aria-selected="true">indexed text</button>
          <button type="button" class="fv-tab" data-mode="compare" aria-selected="false" disabled>comparison unavailable</button>
        </div>
        <a class="fv-btn fv-download" title="Download as file" target="_blank" rel="noopener">⬇ download</a>
        <button type="button" class="fv-btn fv-copy" title="Copy URI">copy</button>
        <button type="button" class="fv-btn fv-close" title="Close (Esc)" aria-label="Close">×</button>
      </div>
      <pre class="fv-body code"><em>loading…</em></pre>
      <div class="fv-compare" hidden>
        <div class="fv-pane fv-pane-l">
          <div class="fv-pane-head">original</div>
          <pre class="fv-pane-body code"><em>loading…</em></pre>
        </div>
        <div class="fv-pane fv-pane-r">
          <div class="fv-pane-head">canonical markdown</div>
          <pre class="fv-pane-body code"><em>loading…</em></pre>
        </div>
      </div>
      <div class="fv-footer meta-line"></div>
    `;
    document.body.appendChild(dialog);

    elUri    = dialog.querySelector(".fv-uri");
    elBody   = dialog.querySelector(".fv-body");
    elCompareWrap = dialog.querySelector(".fv-compare");
    elBodyL  = dialog.querySelectorAll(".fv-pane-body")[0];
    elBodyR  = dialog.querySelectorAll(".fv-pane-body")[1];
    elFooter = dialog.querySelector(".fv-footer");
    elCopyBtn  = dialog.querySelector(".fv-copy");
    elCloseBtn = dialog.querySelector(".fv-close");
    elTabs   = dialog.querySelector(".fv-tabs");

    // Tab toggle.
    elTabs.addEventListener("click", (e) => {
      const btn = e.target.closest(".fv-tab");
      if (!btn) return;
      setMode(btn.dataset.mode);
    });

    elCloseBtn.addEventListener("click", closeViewer);
    elCopyBtn.addEventListener("click", () => {
      const uri = elUri.dataset.uri || "";
      if (!uri) return;
      navigator.clipboard?.writeText(uri).then(
        () => flashButton(elCopyBtn, "copied"),
        () => flashButton(elCopyBtn, "copy failed"),
      );
    });
    // Backdrop click (clicking the dialog itself, not its children) closes.
    dialog.addEventListener("click", (e) => {
      if (e.target === dialog) closeViewer();
    });
    dialog.addEventListener("close", () => {
      // Strip the #view= fragment on close so reload doesn't auto-reopen.
      if (location.hash.startsWith("#view=")) {
        history.replaceState(null, "", location.pathname + location.search);
      }
    });
    return dialog;
  }

  function flashButton(btn, msg) {
    const old = btn.textContent;
    btn.textContent = msg;
    setTimeout(() => { btn.textContent = old; }, 1100);
  }

  function closeViewer() {
    if (dialog?.open) dialog.close();
  }

  function setHash(uri) {
    const target = "#view=" + encodeURIComponent(uri);
    if (location.hash !== target) {
      history.replaceState(null, "", location.pathname + location.search + target);
    }
  }

  // Open the modal for the given uri. Public API. opts.mode may be
  // "single" (default) or "compare" — the latter loads /api/file and
  // /api/markdown side by side. The "download" affordance always
  // points at /api/file?uri=…&download=1.
  function openFileViewer(uri, opts) {
    if (!uri) return;
    opts = opts || {};
    const dlg = ensureDialog();
    currentURI = uri;
    elUri.textContent = shortenURI(uri);
    elUri.title = uri;
    elUri.dataset.uri = uri;
    dialog.querySelector(".fv-download").href =
      "api/file?uri=" + encodeURIComponent(uri) + "&download=1";
    elFooter.innerHTML = "";
    setHash(uri);
    if (!dlg.open) dlg.showModal();
    setMode(opts.mode || "single");
  }

  // setMode switches between single-pane and side-by-side. Loads the
  // appropriate sources on entry to each mode.
  function setMode(mode) {
    // This adapter serves indexed text, not a separate original/markdown cache.
    mode = "single";
    if (!dialog) return;
    if (mode !== "single" && mode !== "compare") mode = "single";
    currentMode = mode;
    // Update tab a11y state.
    dialog.querySelectorAll(".fv-tab").forEach((b) => {
      const active = b.dataset.mode === mode;
      b.setAttribute("aria-selected", active ? "true" : "false");
      b.classList.toggle("active", active);
    });
    elBody.hidden = mode !== "single";
    elCompareWrap.hidden = mode !== "compare";
    if (mode === "single") {
      loadIntoPane(currentURI, elBody, elFooter);
    } else {
      loadIntoPane(currentURI, elBodyL, null);
      loadMarkdownIntoPane(currentURI, elBodyR);
    }
  }

  // loadIntoPane fetches /api/file?uri=… and renders into `pane`.
  // `footer` (optional) is updated with mime + bytes + sha when set.
  function loadIntoPane(uri, pane, footer) {
    pane.innerHTML = "";
    pane.textContent = "loading…";
    fetch("api/file?uri=" + encodeURIComponent(uri))
      .then(async (r) => {
        if (r.status === 415) {
          const j = await r.json().catch(() => ({}));
          pane.textContent = "";
          const note = document.createElement("div");
          note.className = "fv-binary";
          note.textContent = `binary file — preview unavailable (download via the ⬇ button)`;
          pane.appendChild(note);
          if (footer) {
            footer.innerHTML =
              `<span class="k">mime</span> <span class="v">${escapeText(j.mime || "?")}</span>` +
              `<span class="sep">·</span>` +
              `<span class="k">bytes</span> <span class="v">${j.bytes ?? "?"}</span>`;
          }
          return;
        }
        if (!r.ok) {
          const text = await r.text().catch(() => `HTTP ${r.status}`);
          pane.textContent = "";
          const err = document.createElement("div");
          err.className = "fv-error";
          err.textContent = `${r.status} — ${text || "error"}`;
          pane.appendChild(err);
          if (footer) footer.innerHTML = "";
          return;
        }
        const body = await r.text();
        const bytes = r.headers.get("X-Kg-Bytes") || "?";
        const sha   = r.headers.get("X-Kg-Sha256") || "";
        const mime  = (r.headers.get("Content-Type") || "?").split(";")[0];
        pane.textContent = body;
        if (footer) {
          footer.innerHTML =
            `<span class="k">mime</span> <span class="v">${escapeText(mime)}</span>` +
            `<span class="sep">·</span>` +
            `<span class="k">bytes</span> <span class="v">${bytes}</span>` +
            (sha ? `<span class="sep">·</span><span class="k">sha256</span> <span class="v">${sha.slice(0, 12)}…</span>` : "");
        }
      })
      .catch((e) => {
        pane.textContent = "";
        const err = document.createElement("div");
        err.className = "fv-error";
        err.textContent = String(e);
        pane.appendChild(err);
      });
  }

  // loadMarkdownIntoPane fetches /api/markdown?uri=… and renders.
  function loadMarkdownIntoPane(uri, pane) {
    pane.innerHTML = "";
    pane.textContent = "loading…";
    fetch("api/markdown?uri=" + encodeURIComponent(uri))
      .then(async (r) => {
        if (r.status === 404) {
          pane.textContent = "";
          const note = document.createElement("div");
          note.className = "fv-binary";
          note.textContent = "no cached markdown — convert step hasn't run for this URI yet";
          pane.appendChild(note);
          return;
        }
        if (r.status === 503) {
          pane.textContent = "";
          const note = document.createElement("div");
          note.className = "fv-binary";
          note.textContent = "markdown cache not configured on the daemon";
          pane.appendChild(note);
          return;
        }
        if (!r.ok) {
          const text = await r.text().catch(() => `HTTP ${r.status}`);
          pane.textContent = "";
          const err = document.createElement("div");
          err.className = "fv-error";
          err.textContent = `${r.status} — ${text}`;
          pane.appendChild(err);
          return;
        }
        const body = await r.text();
        pane.textContent = body;
      })
      .catch((e) => {
        pane.textContent = "";
        const err = document.createElement("div");
        err.className = "fv-error";
        err.textContent = String(e);
        pane.appendChild(err);
      });
  }

  // Trim a URI for display ("file:///long/abs/path/foo.md" →
  // ".../path/foo.md" when longer than ~60 chars).
  function shortenURI(uri) {
    if (!uri.startsWith("file://")) return uri;
    const path = uri.slice("file://".length);
    if (path.length < 64) return path;
    const parts = path.split("/").filter(Boolean);
    return "…/" + parts.slice(-3).join("/");
  }

  function escapeText(s) {
    return String(s)
      .replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;").replace(/'/g, "&#39;");
  }

  // ─── document-level click delegator ────────────────────────────
  // Any element with [data-uri] anywhere in the DOM triggers the
  // modal. Lets renderers stamp uris into HTML attributes (safe,
  // escaped) without each renderer wiring its own listener.
  document.addEventListener("click", (e) => {
    const el = e.target.closest("[data-uri]");
    if (!el) return;
    e.preventDefault();
    const mode = el.dataset.mode === "compare" ? "compare" : "single";
    openFileViewer(el.dataset.uri, { mode });
  });

  // ─── auto-open from #view= hash on load ───────────────────────
  function checkHash() {
    if (!location.hash.startsWith("#view=")) return;
    const uri = decodeURIComponent(location.hash.slice("#view=".length));
    if (uri) openFileViewer(uri);
  }
  if (document.readyState === "complete" || document.readyState === "interactive") {
    setTimeout(checkHash, 0);
  } else {
    document.addEventListener("DOMContentLoaded", checkHash);
  }
  window.addEventListener("hashchange", checkHash);

  // Public API.
  window.openFileViewer = openFileViewer;
})();
