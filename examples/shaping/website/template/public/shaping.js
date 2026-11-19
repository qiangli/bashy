// shaping.js — the panel the site-shaping server injects into every page.
// Anonymous visitors get a sign-in hint; signed-in visitors and members get
// the views their tier allows; members also get the facts shared with their
// circle. Plain DOM, no framework, no cookies of its own.
(function () {
  "use strict";
  if (document.getElementById("ss-panel-root")) return;
  var me = document.currentScript || document.querySelector('script[src$="_shaping/shaping.js"]');
  var base = me ? me.src.replace(/shaping\.js(\?.*)?$/, "") : "./_shaping/";
  var api = base + "api/";

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

  var tierText = {
    anonymous: "You are browsing anonymously.",
    "signed-in": "Signed in.",
    member: "Signed in as a member.",
    owner: "Signed in as the owner."
  };

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
  }).catch(function () { /* not behind the shaping server: stay invisible */ });
})();
