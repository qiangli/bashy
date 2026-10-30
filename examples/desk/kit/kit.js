/* bashy desk UI kit — shared page behaviour for every desk tool.
   Copy buttons flip to a "Copied" state; badges render from status;
   theme toggle overrides prefers-color-scheme (remembered per browser);
   toasts, keyboard shortcuts and the one content renderer (deskRender)
   every tool uses for text. No dependencies. */
(function () {
  'use strict';
  var root = document.documentElement;

  // Theme: the manual choice survives reloads and is shared by every tool.
  try {
    var saved = localStorage.getItem('desk-theme');
    if (saved === 'dark' || saved === 'light') root.setAttribute('data-theme', saved);
  } catch (e) {}

  function effectiveTheme() {
    var t = root.getAttribute('data-theme');
    if (t === 'dark' || t === 'light') return t;
    return window.matchMedia && matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
  }
  function toggleTheme() {
    var next = effectiveTheme() === 'dark' ? 'light' : 'dark';
    root.setAttribute('data-theme', next);
    try { localStorage.setItem('desk-theme', next); } catch (e) {}
  }

  function flashCopied(btn) {
    var prev = btn.textContent;
    btn.setAttribute('data-copied', 'true');
    btn.textContent = 'Copied';
    setTimeout(function () {
      btn.setAttribute('data-copied', 'false');
      btn.textContent = prev;
    }, 1600);
  }

  async function copyText(btn) {
    var text = btn.getAttribute('data-copy') || location.href;
    try {
      await navigator.clipboard.writeText(text);
      flashCopied(btn);
    } catch (e) {
      var msg = document.querySelector('[data-desk-alert]');
      if (msg) msg.textContent = 'Copy this instead: ' + text;
    }
  }

  document.addEventListener('click', function (ev) {
    var btn = ev.target.closest('[data-desk-copy]');
    if (btn) copyText(btn);
    if (ev.target.closest('[data-desk-theme]')) toggleTheme();
  });

  // Badge helper for tools that render status client-side.
  window.deskBadge = function (status, label) {
    var s = document.createElement('span');
    s.className = 'desk-badge';
    s.setAttribute('data-status', status);
    s.textContent = label || status;
    return s;
  };

  // Toasts: transient confirmations; sticky problems stay in [data-desk-alert].
  window.deskToast = function (msg, kind) {
    var box = document.querySelector('.desk-toasts');
    if (!box) {
      box = document.createElement('div');
      box.className = 'desk-toasts';
      box.setAttribute('role', 'status');
      box.setAttribute('aria-live', 'polite');
      document.body.appendChild(box);
    }
    var t = document.createElement('div');
    t.className = 'desk-toast';
    t.setAttribute('data-kind', kind || 'info');
    t.textContent = msg;
    box.appendChild(t);
    setTimeout(function () { t.remove(); }, kind === 'error' ? 6000 : 3200);
    return t;
  };

  // Shortcuts, identical in every tool:
  //   Mod+S       [data-desk-shortcut=save]     Mod+Enter  [data-desk-shortcut=submit]
  //   t           toggle theme                  y          copy the item link
  //   ?           list shortcuts
  // Single keys only fire outside text entry.
  var SHORTCUTS = 'Shortcuts: ⌘/Ctrl+S save · ⌘/Ctrl+Enter submit · t theme · y copy link · ? help';
  function editable(el) {
    return el && (el.isContentEditable || /^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName));
  }
  function press(name) {
    var el = document.querySelector('[data-desk-shortcut="' + name + '"]');
    if (el && !el.disabled && !el.hidden) { el.click(); return true; }
    return false;
  }
  document.addEventListener('keydown', function (ev) {
    var mod = ev.metaKey || ev.ctrlKey;
    if (mod && !ev.altKey && (ev.key === 's' || ev.key === 'S')) {
      if (press('save')) ev.preventDefault();
      return;
    }
    if (mod && ev.key === 'Enter') {
      if (press('submit')) ev.preventDefault();
      return;
    }
    if (mod || ev.altKey || editable(ev.target)) return;
    if (ev.key === 't') toggleTheme();
    else if (ev.key === 'y') {
      var link = document.querySelector('.desk-appbar [data-desk-copy]');
      if (link) link.click();
    } else if (ev.key === '?') window.deskToast(SHORTCUTS);
  });

  // ---- deskRender: the one text renderer. ----
  // A thin client for the tool's POST api/render endpoint (Markdown in,
  // sanitized server HTML out). There is no client-side Markdown subset
  // anymore: the endpoint runs the shared server renderer, so previews and
  // pages share one look and one sanitizer. The server answer is sanitized
  // HTML, safe to swap in. Same name and signature as before:
  // deskRender(target, text, {inline}) fills target asynchronously and
  // returns it; {inline} only adds the .desk-inline styling hint. A slow
  // or failed render keeps the previous content and toasts once.
  var renderFailed = false;
  window.deskRender = function (target, text, opts) {
    text = text == null ? '' : String(text);
    target.classList.add('desk-content');
    if (opts && opts.inline) target.classList.add('desk-inline');
    var seq = (target._deskSeq = (target._deskSeq || 0) + 1);
    fetch('api/render', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({text: text})
    }).then(function (res) {
      if (!res.ok) throw new Error('render HTTP ' + res.status);
      return res.json();
    }).then(function (data) {
      if (target._deskSeq !== seq) return;
      if (data && typeof data.html === 'string') target.innerHTML = data.html;
    }).catch(function () {
      if (!renderFailed) {
        renderFailed = true;
        if (window.deskToast) window.deskToast('Preview unavailable; showing source.', 'error');
      }
    });
    return target;
  };
  // Server pages mark text for the renderer: data-desk-render="block|inline";
  // the element's text content is the source.
  window.deskRender.all = function (scope) {
    (scope || document).querySelectorAll('[data-desk-render]').forEach(function (n) {
      window.deskRender(n, n.textContent, {inline: n.getAttribute('data-desk-render') === 'inline'});
    });
  };
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { window.deskRender.all(); });
  } else window.deskRender.all();
})();
