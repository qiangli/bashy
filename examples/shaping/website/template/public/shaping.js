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
    var root = el("div", { id: "ss-panel-root" });
    var button = el("button", { type: "button", class: "ss-toggle", "aria-expanded": "false" }, "✦ For you");
    var panel = el("section", { class: "ss-panel", hidden: "" });
    panel.appendChild(el("h2", null, "Shaped for you"));
    panel.appendChild(el("p", { class: "ss-tier" }, tierText[info.tier] || ""));

    if (info.tier === "anonymous") {
      var p = el("p");
      var a = el("a", { href: new URL(info.sign_in, location.origin).href, target: "_blank", rel: "noopener" }, "Sign in");
      p.appendChild(a);
      p.appendChild(document.createTextNode(" to get a summary made for you, then reload this page."));
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

    if (info.fields > 0) {
      var facts = el("div", { class: "ss-facts" });
      panel.appendChild(facts);
      get("fields").then(function (f) {
        f.fields.forEach(function (x) {
          var d = el("div", { class: "ss-fact" });
          d.appendChild(el("strong", null, x.label));
          d.appendChild(el("p", null, x.body));
          facts.appendChild(d);
        });
      });
    }
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
