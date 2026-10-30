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
    var text = btn.getAttribute('data-copy') || '';
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
  // A safe Markdown subset built with DOM APIs (never innerHTML of input):
  // headings, paragraphs, lists, quotes, rules, fenced code, pipe tables,
  // inline code/strong/em/links, and ```chart fences drawn with the kit
  // data palette. It emits semantic tags only; kit.css (.desk-content)
  // styles them. inline mode keeps line breaks and inline marks only.
  function el(tag, cls, text) {
    var n = document.createElement(tag);
    if (cls) n.className = cls;
    if (text != null) n.textContent = text;
    return n;
  }
  function safeHref(u) {
    return /^(https?:\/\/|mailto:|\/|\.\/|#|\?)/i.test(u) ? u : '';
  }
  function inline(parent, text) {
    var re = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*\s][^*]*\*|_[^_\s][^_]*_)|(\[[^\]]+\]\([^)\s]+\))/;
    while (text) {
      var m = re.exec(text);
      if (!m) { parent.appendChild(document.createTextNode(text)); break; }
      if (m.index) parent.appendChild(document.createTextNode(text.slice(0, m.index)));
      var tok = m[0];
      if (m[1]) parent.appendChild(el('code', '', tok.slice(1, -1)));
      else if (m[2]) inline(parent.appendChild(el('strong')), tok.slice(2, -2));
      else if (m[3]) inline(parent.appendChild(el('em')), tok.slice(1, -1));
      else {
        var lm = /^\[([^\]]+)\]\(([^)\s]+)\)$/.exec(tok), href = safeHref(lm[2]);
        if (href) { var a = el('a'); a.href = href; a.rel = 'noopener noreferrer'; inline(a, lm[1]); parent.appendChild(a); }
        else parent.appendChild(document.createTextNode(lm[1]));
      }
      text = text.slice(m.index + tok.length);
    }
  }
  function cells(line) {
    return line.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map(function (c) { return c.trim(); });
  }
  // Chart data: [1,2,3] · {"a":1} · [{"label":"a","value":1}] · CSV "a,1" lines.
  function chartData(src) {
    var rows = [];
    try {
      var v = JSON.parse(src);
      if (Array.isArray(v)) v.forEach(function (x, i) {
        if (typeof x === 'number') rows.push([String(i + 1), x]);
        else if (x && typeof x === 'object') rows.push([String(x.label != null ? x.label : x.name != null ? x.name : i + 1), Number(x.value != null ? x.value : x.y)]);
      });
      else if (v && typeof v === 'object') Object.keys(v).forEach(function (k) { rows.push([k, Number(v[k])]); });
    } catch (e) {
      src.split('\n').forEach(function (line) {
        var p = line.split(','), n = Number(p[p.length - 1]);
        if (p.length >= 2 && line.trim() && !isNaN(n)) rows.push([p.slice(0, -1).join(',').trim(), n]);
      });
    }
    return rows.filter(function (r) { return isFinite(r[1]); });
  }
  function chart(src) {
    var rows = chartData(src), fig = el('figure', 'desk-chart');
    if (!rows.length) { fig.appendChild(el('pre', '', src)); return fig; }
    var NS = 'http://www.w3.org/2000/svg', W = 640, H = 220, pad = 28;
    var max = Math.max.apply(null, rows.map(function (r) { return Math.abs(r[1]); })) || 1;
    var svg = document.createElementNS(NS, 'svg');
    svg.setAttribute('viewBox', '0 0 ' + W + ' ' + H);
    svg.setAttribute('role', 'img');
    var bw = (W - pad * 2) / rows.length;
    rows.forEach(function (r, i) {
      var h = Math.abs(r[1]) / max * (H - pad * 2), x = pad + i * bw;
      var rect = document.createElementNS(NS, 'rect');
      rect.setAttribute('class', 'desk-series-' + (i % 8 + 1));
      rect.setAttribute('x', (x + bw * .1).toFixed(1));
      rect.setAttribute('y', (H - pad - h).toFixed(1));
      rect.setAttribute('width', (bw * .8).toFixed(1));
      rect.setAttribute('height', h.toFixed(1));
      rect.setAttribute('rx', '3');
      var title = document.createElementNS(NS, 'title');
      title.textContent = r[0] + ': ' + r[1];
      rect.appendChild(title);
      svg.appendChild(rect);
      var t = document.createElementNS(NS, 'text');
      t.setAttribute('class', 'desk-chart-label');
      t.setAttribute('x', (x + bw / 2).toFixed(1));
      t.setAttribute('y', String(H - 10));
      t.setAttribute('text-anchor', 'middle');
      t.textContent = r[0];
      svg.appendChild(t);
    });
    svg.setAttribute('aria-label', rows.map(function (r) { return r[0] + ' ' + r[1]; }).join(', '));
    fig.appendChild(svg);
    return fig;
  }
  function blocks(parent, text) {
    var lines = text.replace(/\r\n?/g, '\n').split('\n'), i = 0, para = [];
    function flush() {
      if (!para.length) return;
      inline(parent.appendChild(el('p')), para.join(' '));
      para = [];
    }
    while (i < lines.length) {
      var line = lines[i], m;
      if (!line.trim()) { flush(); i++; continue; }
      if ((m = /^\s*(```|~~~)\s*([\w-]*)/.exec(line))) {
        flush();
        var fence = m[1], lang = m[2], body = [];
        for (i++; i < lines.length && lines[i].trim().indexOf(fence) !== 0; i++) body.push(lines[i]);
        i++;
        if (lang === 'chart') parent.appendChild(chart(body.join('\n')));
        else {
          var pre = el('pre'), code = el('code', '', body.join('\n'));
          if (lang) code.setAttribute('data-lang', lang);
          pre.appendChild(code);
          parent.appendChild(pre);
        }
        continue;
      }
      if ((m = /^(#{1,4})\s+(.*)$/.exec(line))) { flush(); inline(parent.appendChild(el('h' + m[1].length)), m[2]); i++; continue; }
      if (/^\s*([-*_])(\s*\1){2,}\s*$/.test(line)) { flush(); parent.appendChild(el('hr')); i++; continue; }
      if (/^\s*>/.test(line)) {
        flush();
        var q = [];
        for (; i < lines.length && /^\s*>/.test(lines[i]); i++) q.push(lines[i].replace(/^\s*>\s?/, ''));
        blocks(parent.appendChild(el('blockquote')), q.join('\n'));
        continue;
      }
      if (/^\s*([-*+]|\d+[.)])\s+/.test(line)) {
        flush();
        var ordered = /^\s*\d/.test(line), list = parent.appendChild(el(ordered ? 'ol' : 'ul'));
        for (; i < lines.length && /^\s*([-*+]|\d+[.)])\s+/.test(lines[i]); i++)
          inline(list.appendChild(el('li')), lines[i].replace(/^\s*([-*+]|\d+[.)])\s+/, ''));
        continue;
      }
      if (/\|/.test(line) && i + 1 < lines.length && /^\s*\|?\s*:?-{2,}/.test(lines[i + 1])) {
        flush();
        var table = parent.appendChild(el('table')), tr = el('tr');
        cells(line).forEach(function (c) { inline(tr.appendChild(el('th')), c); });
        table.appendChild(el('thead')).appendChild(tr);
        var tb = table.appendChild(el('tbody'));
        for (i += 2; i < lines.length && /\|/.test(lines[i]) && lines[i].trim(); i++) {
          var row = tb.appendChild(el('tr'));
          cells(lines[i]).forEach(function (c) { inline(row.appendChild(el('td')), c); });
        }
        continue;
      }
      para.push(line.trim());
      i++;
    }
    flush();
  }

  // deskRender(target, text, {inline}) renders text into target (replacing
  // its children) and marks it .desk-content. Returns target.
  window.deskRender = function (target, text, opts) {
    text = text == null ? '' : String(text);
    while (target.firstChild) target.removeChild(target.firstChild);
    target.classList.add('desk-content');
    if (opts && opts.inline) {
      target.classList.add('desk-inline');
      text.split('\n').forEach(function (line, i) {
        if (i) target.appendChild(el('br'));
        inline(target, line);
      });
    } else blocks(target, text);
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
