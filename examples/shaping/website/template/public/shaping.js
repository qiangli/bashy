// shaping.js — the panel the site-shaping server injects into every page.
// Anonymous visitors get a sign-in hint; account visitors and members get
// the views their tier allows; members also get the facts shared with their
// circle. Plain DOM, no framework, no cookies of its own.
(function () {
  "use strict";
  if (document.getElementById("ss-panel-root")) return;
  var me = document.currentScript || document.querySelector('script[src$="_shaping/shaping.js"]');
  var base = me ? me.src.replace(/shaping\.js(\?.*)?$/, "") : "./_shaping/";
  var api = base + "api/";
  var baseResolved = new URL(base, location.origin + (location.pathname || "/"));
  var mountPath = baseResolved.pathname.replace(/\/_shaping\/?$/, "");
  var mcpURL = location.origin + (mountPath ? mountPath : "") + "/mcp";
  var mcpSeq = 0;

  function el(tag, attrs, text) {
    var e = document.createElement(tag);
    for (var k in attrs || {}) e.setAttribute(k, attrs[k]);
    if (text != null) e.textContent = text;
    return e;
  }

  // A tiny Markdown subset rendered with DOM nodes only (no innerHTML).
  function renderMarkdown(md, into) {
    into.textContent = "";
    var list = null;
    md.split(/\r?\n/).forEach(function (line) {
      var t = line.trim();
      if (!t) { list = null; return; }
      var h = t.match(/^(#{1,4})\s+(.*)$/);
      var li = t.match(/^[-*]\s+(.*)$/);
      if (h) { list = null; into.appendChild(inline(el("h" + Math.min(h[1].length + 2, 6)), h[2])); }
      else if (li) {
        if (!list) { list = el("ul"); into.appendChild(list); }
        list.appendChild(inline(el("li"), li[1]));
      } else { list = null; into.appendChild(inline(el("p"), t)); }
    });
  }
  function inline(node, text) {
    text.split(/(\*\*[^*]+\*\*)/).forEach(function (part) {
      var b = part.match(/^\*\*([^*]+)\*\*$/);
      node.appendChild(b ? el("strong", null, b[1]) : document.createTextNode(part));
    });
    return node;
  }

  // The cloudbox edge checks state-changing requests with a double-submit
  // token: echo its readable csrf_token cookie in X-CSRF-Token.
  function csrfHeaders() {
    var m = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
    return m ? { "X-CSRF-Token": decodeURIComponent(m[1]) } : {};
  }

  // The Software Shaping Flow mark (two opposed S curves), drawn with DOM
  // nodes so the panel never parses markup.
  var FLOW = "M43 85 C65 100 84 86 105 63 C124 42 143 29 169 29 C201 29 225 51 227 85 Q228 92 221 88 C189 65 159 77 137 96 C116 115 96 147 65 148 C35 149 20 122 30 99 Q38 81 43 85 Z";
  function flowMark() {
    var ns = "http://www.w3.org/2000/svg";
    var svg = document.createElementNS(ns, "svg");
    svg.setAttribute("viewBox", "0 0 256 256");
    svg.setAttribute("width", "16");
    svg.setAttribute("height", "16");
    svg.setAttribute("fill", "currentColor");
    svg.setAttribute("aria-hidden", "true");
    svg.setAttribute("class", "ss-flow");
    [null, "rotate(180 128 128)"].forEach(function (t) {
      var path = document.createElementNS(ns, "path");
      path.setAttribute("d", FLOW);
      if (t) path.setAttribute("transform", t);
      svg.appendChild(path);
    });
    return svg;
  }

  // The page's own hooks, honoured on the engine face only:
  //   data-shaping-hide-on-engine  removed (e.g. a link to this very engine)
  //   data-shaping-signin          pointed at the cloudbox sign-in
  //   data-shaping-contact         filled with the reader's contact-* fields
  function shapePage(info, fieldsReady) {
    Array.prototype.forEach.call(document.querySelectorAll("[data-shaping-hide-on-engine]"), function (n) { n.remove(); });
    var signIn = signInHref(info);
    Array.prototype.forEach.call(document.querySelectorAll("a[data-shaping-signin]"), function (a) {
      a.setAttribute("href", signIn);
      a.removeAttribute("target");
    });
    var card = document.querySelector("[data-shaping-contact]");
    if (!card || info.tier === "anonymous") return;
    fieldsReady.then(function (fields) {
      var rows = fields.filter(function (x) { return x.name.indexOf("contact-") === 0; });
      if (!rows.length) return;
      Array.prototype.forEach.call(card.querySelectorAll("[data-shaping-signin]"), function (a) {
        var row = a.closest("p") || a;
        row.remove();
      });
      rows.forEach(function (x) {
        var p = el("p", { class: "ss-contact-row" });
        var v = x.body.trim();
        var href = /^https?:\/\//.test(v) ? v : (/^[^\s@]+@[^\s@]+$/.test(v) ? "mailto:" + v : null);
        p.appendChild(el("strong", null, x.label + ": "));
        p.appendChild(href ? el("a", { href: href, target: "_blank", rel: "noopener" }, v) : document.createTextNode(v));
        card.appendChild(p);
      });
    }).catch(function () {});
  }

  // Sign in, then come back to this very page: cloudbox's /cloud/signin
  // remembers ?next= (same-origin path only) across the provider round trip.
  function signInHref(info) {
    var u = new URL(info.sign_in || "/cloud/signin", location.origin);
    u.searchParams.set("next", location.pathname + location.search);
    return u.href;
  }

  function get(path) {
    return fetch(api + path, { credentials: "same-origin" }).then(function (r) {
      if (!r.ok) throw new Error("HTTP " + r.status);
      return r.json();
    });
  }

  function currentSitePage() {
    var p = location.pathname || "/";
    if (mountPath && (p === mountPath || p.indexOf(mountPath + "/") === 0)) {
      p = p.slice(mountPath.length) || "/";
    }
    return p.charAt(0) === "/" ? p : "/" + p;
  }

  function safeHTTPURL(raw) {
    if (typeof raw !== "string" || !raw.trim()) return null;
    try {
      var u = new URL(raw, location.origin);
      if (u.protocol === "http:" || u.protocol === "https:") return u.href;
    } catch (_) {}
    return null;
  }

  function firstText(content) {
    if (!Array.isArray(content)) return "";
    for (var i = 0; i < content.length; i++) {
      if (content[i] && typeof content[i].text === "string") return content[i].text;
    }
    return "";
  }

  function parseMCPEnvelope(raw, ctype) {
    var text = String(raw || "").trim();
    if ((ctype && ctype.indexOf("text/event-stream") !== -1) || /^event:|^data:/m.test(text)) {
      var lines = text.split(/\r?\n/);
      var last = null;
      for (var i = 0; i < lines.length; i++) {
        if (lines[i].indexOf("data:") === 0) {
          var chunk = lines[i].slice(5).trim();
          if (!chunk) continue;
          try {
            var parsed = JSON.parse(chunk);
            if (parsed && (parsed.result !== undefined || parsed.error !== undefined)) last = parsed;
          } catch (_) {}
        }
      }
      if (last) return last;
    }
    return JSON.parse(text);
  }

  // Call one tool on the app's same-origin site MCP (<mount>/mcp) through
  // outpost: the caller's cloudbox session cookie identifies them — no token,
  // no Authorization header, no OS password.
  function callMCP(tool, args) {
    var headers = Object.assign({
      "Content-Type": "application/json",
      "Accept": "application/json, text/event-stream",
      "MCP-Protocol-Version": "2025-06-18"
    }, csrfHeaders());
    var body = JSON.stringify({
      jsonrpc: "2.0",
      id: ++mcpSeq,
      method: "tools/call",
      params: { name: tool, arguments: args || {} }
    });
    return fetch(mcpURL, {
      method: "POST",
      credentials: "same-origin",
      headers: headers,
      body: body
    }).then(function (r) {
      var ctype = (r.headers && r.headers.get && r.headers.get("Content-Type")) || "";
      return r.text().then(function (raw) {
        if (!r.ok) {
          throw new Error(r.status === 401 ? "Sign in required." : ("HTTP " + r.status));
        }
        var env = parseMCPEnvelope(raw, ctype);
        if (env.error) {
          throw new Error(env.error.message || "MCP call failed.");
        }
        var res = env.result || {};
        if (res.isError) {
          throw new Error(firstText(res.content) || "Not authorized.");
        }
        if (res.structuredContent && typeof res.structuredContent === "object") {
          return res.structuredContent;
        }
        var txt = firstText(res.content);
        if (txt) {
          try { return JSON.parse(txt); } catch (_) {}
        }
        return {};
      });
    });
  }

  function renderShapeBox(panel, info) {
    if (info.scope !== "shape") return;
    var isOwner = info.tier === "owner";
    var box = el("div", { class: "ss-shape-box" });
    box.appendChild(el("p", { class: "ss-wishes-label" }, "Shape this site"));
    var form = el("div", { class: "ss-wish-form" });
    var input = el("textarea", {
      class: "ss-wish-input",
      rows: "2",
      placeholder: "Describe a change to this site…"
    });
    var submit = el("button", { type: "button", class: "ss-wish-submit" }, "Send wish");
    form.appendChild(input);
    form.appendChild(submit);
    box.appendChild(form);

    var status = el("div", { class: "ss-shape-status", role: "status", "aria-live": "polite" });
    var actions = el("div", { class: "ss-shape-actions" });
    box.appendChild(status);
    box.appendChild(actions);

    submit.addEventListener("click", function () {
      var request = (input.value || "").trim();
      if (!request || submit.disabled) return;
      submit.disabled = true;
      actions.textContent = "";
      status.textContent = "Sending wish…";

      var cid = "";
      callMCP("shape_wish", { request: request, page: currentSitePage() })
        .then(function (wishOut) {
          var cand = (wishOut && wishOut.result) || wishOut || {};
          cid = typeof cand.id === "string" ? cand.id : "";
          if (cand.kind === "request" || cand.status === "requested") {
            status.textContent = "Queued as a request for the owner" + (cid ? " (" + cid + ")." : ".");
            return null;
          }
          if (!cid) throw new Error("No candidate returned.");
          status.textContent = "Verifying candidate…";
          return callMCP("shape_verify", { id: cid }).then(function (verOut) {
            var ver = (verOut && verOut.result) || verOut || {};
            if (ver.verdict && ver.verdict.pass === false) {
              throw new Error(ver.verdict.detail || "Verification failed.");
            }
            status.textContent = "Verified. Preparing preview…";
            return callMCP("shape_preview", { id: cid })
              .then(function (prevOut) {
                var prev = (prevOut && prevOut.result) || prevOut || {};
                return { verified: ver, preview: prev, previewErr: null };
              })
              .catch(function (perr) {
                return { verified: ver, preview: null, previewErr: perr };
              });
          });
        })
        .then(function (outcome) {
          if (!outcome) return;
          var summary = "Verified (" + cid + ")";
          if (outcome.previewErr) {
            summary += " · Preview unavailable";
          } else {
            summary += " · Preview ready";
          }
          if (!isOwner) {
            summary += " — awaiting owner approval.";
          }
          status.textContent = summary;

          var href = outcome.preview && safeHTTPURL(outcome.preview.url);
          if (href) {
            actions.appendChild(el("a", {
              class: "ss-shape-preview",
              href: href,
              target: "_blank",
              rel: "noopener"
            }, "Preview"));
          }

          if (isOwner) {
            var approve = el("button", { type: "button", class: "ss-shape-approve" }, "Approve");
            approve.addEventListener("click", function () {
              if (approve.disabled) return;
              approve.disabled = true;
              submit.disabled = true;
              status.textContent = "Approving…";
              callMCP("shape_approve", { id: cid })
                .then(function () {
                  input.value = "";
                  approve.remove();
                  status.textContent = "Approved (" + cid + ").";
                })
                .catch(function (aerr) {
                  approve.disabled = false;
                  status.textContent = (aerr && aerr.message) || "Approval failed.";
                })
                .finally(function () {
                  submit.disabled = false;
                });
            });
            actions.appendChild(approve);
          }
        })
        .catch(function (err) {
          status.textContent = (err && err.message) || "Wish could not be completed.";
        })
        .finally(function () {
          submit.disabled = false;
        });
    });

    panel.appendChild(box);
  }

  // Personalization wishes map strictly to declared properties and are kept
  // locally in localStorage — no model call, no server state.
  var storageKey = "ss-prefs:" + new URL(base, location.origin).pathname;

  function isAllowedPatch(prop, val, props) {
    if (!props || !Object.prototype.hasOwnProperty.call(props, prop)) return false;
    var spec = props[prop];
    return !!(spec && Array.isArray(spec.values) && spec.values.indexOf(val) !== -1);
  }

  function loadPrefs(props) {
    var clean = {};
    try {
      var raw = localStorage.getItem(storageKey);
      if (!raw) return clean;
      var parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object") return clean;
      Object.keys(parsed).forEach(function (k) {
        if (isAllowedPatch(k, parsed[k], props)) clean[k] = parsed[k];
      });
    } catch (_) {}
    return clean;
  }

  function savePrefs(prefs) {
    try {
      if (Object.keys(prefs).length === 0) localStorage.removeItem(storageKey);
      else localStorage.setItem(storageKey, JSON.stringify(prefs));
    } catch (_) {}
  }

  function applyPrefs(prefs, props) {
    var rootEl = document.documentElement;
    Object.keys(props || {}).forEach(function (prop) {
      var spec = props[prop] || {};
      var val = Object.prototype.hasOwnProperty.call(prefs, prop) ? prefs[prop] : null;
      if (spec.css_var || prop.indexOf("--") === 0) {
        var varName = spec.css_var || prop;
        if (val !== null) rootEl.style.setProperty(varName, val);
        else rootEl.style.removeProperty(varName);
      }
      if (spec.class_prefix || (!spec.css_var && prop.indexOf("--") !== 0)) {
        var prefix = spec.class_prefix || ("ss-" + prop.replace(/_/g, "-") + "-");
        (spec.values || []).forEach(function (v) { rootEl.classList.remove(prefix + v); });
        if (val !== null) rootEl.classList.add(prefix + val);
      }
    });
  }

  function wishActive(wish, prefs) {
    var keys = Object.keys(wish.patch || {});
    if (!keys.length) return false;
    return keys.every(function (k) { return prefs[k] === wish.patch[k]; });
  }

  function renderWishes(panel, info) {
    var props = info.properties || {};
    var wishes = (info.wishes || []).filter(function (w) {
      var keys = Object.keys((w && w.patch) || {});
      return keys.length > 0 && keys.every(function (k) { return isAllowedPatch(k, w.patch[k], props); });
    });
    if (!wishes.length) return;

    var prefs = loadPrefs(props);
    applyPrefs(prefs, props);

    var box = el("div", { class: "ss-wishes" });
    box.appendChild(el("p", { class: "ss-wishes-label" }, "Personalize"));
    var list = el("div", { class: "ss-wish-list" });
    var buttons = [];

    function syncButtons() {
      buttons.forEach(function (entry) {
        var active = wishActive(entry.wish, prefs);
        entry.btn.classList.toggle("is-active", active);
        entry.btn.setAttribute("aria-pressed", String(active));
      });
    }

    wishes.forEach(function (w) {
      var wb = el("button", { type: "button", class: "ss-wish", "data-wish-id": w.id, "aria-pressed": "false" }, w.label);
      wb.addEventListener("click", function () {
        var active = wishActive(w, prefs);
        Object.keys(w.patch).forEach(function (k) {
          if (active) delete prefs[k];
          else if (isAllowedPatch(k, w.patch[k], props)) prefs[k] = w.patch[k];
        });
        savePrefs(prefs);
        applyPrefs(prefs, props);
        syncButtons();
      });
      buttons.push({ wish: w, btn: wb });
      list.appendChild(wb);
    });

    var reset = el("button", { type: "button", class: "ss-wish ss-wish-reset" }, "Reset");
    reset.addEventListener("click", function () {
      prefs = {};
      savePrefs(prefs);
      applyPrefs(prefs, props);
      syncButtons();
    });
    list.appendChild(reset);
    syncButtons();
    box.appendChild(list);
    panel.appendChild(box);
  }

  var tierText = {
    anonymous: "You are browsing anonymously.",
    account: "Signed in.",
    "signed-in": "Signed in.",
    member: "Signed in as a member.",
    owner: "Signed in as the owner."
  };

  var VERSION_POLL_MS = 4000;

  // Poll /_shaping/api/version every few seconds so open pages pick up an
  // approved revision without SSE or a second server. Reloads immediately
  // unless the viewer is editing, in which case a one-line notice appears.
  function startVersionPoll(initialVersion, root) {
    var known = typeof initialVersion === "string" ? initialVersion : "";
    var inFlight = false;
    var stopped = false;
    var timer = null;

    function isEditing() {
      var a = document.activeElement;
      if (a) {
        var tag = (a.tagName || "").toUpperCase();
        if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || a.isContentEditable) {
          return true;
        }
      }
      if (root && root.querySelectorAll) {
        var fields = root.querySelectorAll("input, textarea");
        for (var i = 0; i < fields.length; i++) {
          if (fields[i].value && fields[i].value.trim() !== "") return true;
        }
      }
      return false;
    }

    function stop() {
      stopped = true;
      if (timer !== null) {
        clearInterval(timer);
        timer = null;
      }
    }

    function showNotice() {
      if (stopped) return;
      stop();
      var notice = el("div", { class: "ss-update-notice", role: "status", "aria-live": "polite" });
      notice.appendChild(el("span", null, "A new version is live."));
      var btn = el("button", { type: "button", class: "ss-reload" }, "Reload");
      btn.addEventListener("click", function () { location.reload(); });
      notice.appendChild(btn);
      if (root.firstChild) root.insertBefore(notice, root.firstChild);
      else root.appendChild(notice);
    }

    function check() {
      if (inFlight || stopped || document.hidden) return;
      inFlight = true;
      get("version").then(function (res) {
        var next = res && typeof res.version === "string" ? res.version : "";
        if (!next) return;
        if (!known) { known = next; return; }
        if (next === known) return;
        if (isEditing()) { showNotice(); return; }
        stop();
        location.reload();
      }).catch(function () { /* server restarting or offline: retry next tick */ })
        .finally(function () { inFlight = false; });
    }

    timer = setInterval(check, VERSION_POLL_MS);
    if (document.addEventListener) {
      document.addEventListener("visibilitychange", function () {
        if (!document.hidden) check();
      });
    }
  }

  get("me").then(function (info) {
    // Inside the console (the owner's shaping copy) visitors cannot be
    // identified, so the visitor panel would only ask to sign in; the
    // console's own wish button is the tool there.
    if (info.public === false) return;
    var root = el("div", { id: "ss-panel-root" });
    var button = el("button", { type: "button", class: "ss-toggle", "aria-expanded": "false" });
    button.appendChild(flowMark());
    button.appendChild(document.createTextNode(" For you"));
    var panel = el("section", { class: "ss-panel", hidden: "" });
    panel.appendChild(el("h2", null, "Shaped for you"));
    panel.appendChild(el("p", { class: "ss-tier" }, tierText[info.tier] || ""));

    if (info.shape) {
      var shape = el("a", { class: "ss-shape", href: new URL(info.shape, location.origin).href, target: "_blank", rel: "noopener" });
      shape.appendChild(flowMark());
      shape.appendChild(document.createTextNode(" Shape this site"));
      panel.appendChild(shape);
    }

    renderShapeBox(panel, info);

    renderWishes(panel, info);

    if (info.tier !== "anonymous") {
      var out_p = el("p", { class: "ss-signout" });
      var so = el("a", { href: "#" }, "Sign out");
      so.addEventListener("click", function (ev) {
        ev.preventDefault();
        // Ends the session on THIS origin (the satellite); then the page is
        // public again. Signing out of the master does not do this yet.
        fetch(new URL("/cloud/logout", location.origin).href, { method: "POST", credentials: "same-origin", headers: csrfHeaders() })
          .catch(function () {})
          .finally(function () { location.reload(); });
      });
      out_p.appendChild(so);
      panel.appendChild(out_p);
    }

    if (info.tier === "anonymous") {
      var p = el("p");
      var a = el("a", { href: signInHref(info) }, "Sign in");
      p.appendChild(a);
      p.appendChild(document.createTextNode(" to get a summary made for you; you will come back to this page."));
      panel.appendChild(p);
    }

    var out = el("div", { class: "ss-output", "aria-live": "polite" });
    (info.views || []).forEach(function (v) {
      var vb = el("button", { type: "button", class: "ss-view" }, v.title);
      vb.addEventListener("click", function () {
        vb.disabled = true;
        out.textContent = "Preparing " + v.title + "…";
        fetch(api + "view?name=" + encodeURIComponent(v.name), { method: "POST", credentials: "same-origin", headers: csrfHeaders() })
          .then(function (r) { return r.json().then(function (j) { return { ok: r.ok, j: j }; }); })
          .then(function (res) {
            if (!res.ok) { out.textContent = res.j.error || "Not available."; return; }
            renderMarkdown(res.j.markdown || "", out);
          })
          .catch(function () { out.textContent = "The view could not be prepared."; })
          .finally(function () { vb.disabled = false; });
      });
      panel.appendChild(vb);
    });

    var facts = el("div", { class: "ss-facts" });
    panel.appendChild(facts);
    var fieldsReady = info.fields > 0 ? get("fields").then(function (f) { return f.fields || []; }) : Promise.resolve([]);
    fieldsReady.then(function (fields) {
      fields.forEach(function (x) {
        if (x.name.indexOf("contact-") === 0) return; // shown in the page's contact card
        var d = el("div", { class: "ss-fact" });
        d.appendChild(el("strong", null, x.label));
        d.appendChild(el("p", null, x.body));
        facts.appendChild(d);
      });
    }).catch(function () {});
    shapePage(info, fieldsReady);
    panel.appendChild(out);

    if (info.more && info.tier !== "anonymous" && info.contact) {
      var req = el("p", { class: "ss-more" });
      req.appendChild(document.createTextNode("Need contact details or certificates? "));
      req.appendChild(el("a", { href: info.contact }, "Request access"));
      panel.appendChild(req);
    }

    button.addEventListener("click", function () {
      var open = panel.hasAttribute("hidden");
      if (open) panel.removeAttribute("hidden"); else panel.setAttribute("hidden", "");
      button.setAttribute("aria-expanded", String(open));
    });
    root.appendChild(button);
    root.appendChild(panel);
    document.body.appendChild(root);
    startVersionPoll(info.version || "", root);
  }).catch(function () { /* not behind the shaping server: stay invisible */ });
})();
