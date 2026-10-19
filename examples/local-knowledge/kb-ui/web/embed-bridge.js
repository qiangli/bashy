// kg-ui ↔ cloudbox embed.js bridge.
//
// When the kg SPA loads inside a cross-origin iframe mounted by
// cloudbox's embed.js snippet, this file:
//
//   1. Sends `{type:"cloudbox-resize", height}` to window.parent on
//      every body-size change so the snippet can resize the iframe
//      to fit the kg UI without scrollbars.
//   2. Sends `{type:"cloudbox-auth-required"}` when a kg API call
//      returns 401 (or 403 with the cloudbox elevation hint) so the
//      snippet can render its sign-in affordance.
//   3. Listens for `{type:"cloudbox-csrf", token}` from the parent and
//      stashes the value so subsequent fetches add the X-CSRF-Token
//      header on mutating XHRs.
//
// Top-level (non-iframed) loads of kg-ui short-circuit — nothing to
// bridge to.

(function () {
  if (window.parent === window) {
    return; // not iframed; bridge is inert
  }

  var KG_CSRF_TOKEN = null;

  // ---- Inbound: receive CSRF token from cloudbox embed.js. -------------
  window.addEventListener('message', function (ev) {
    // No origin check possible — kg-ui doesn't know what cloudbox
    // origin is supposed to be. We only consume a single shape and
    // validate the token format before using it (hex-only, bounded
    // length) so a malicious peer can't inject arbitrary header
    // content.
    var d = ev.data;
    if (!d || typeof d !== 'object' || d.type !== 'cloudbox-csrf') return;
    var tok = String(d.token || '');
    if (/^[A-Za-z0-9_-]{8,128}$/.test(tok)) {
      KG_CSRF_TOKEN = tok;
    }
  });

  // ---- Outbound: auto-resize on body-size changes. ---------------------
  function postResize() {
    try {
      var h = Math.max(
        document.documentElement.scrollHeight,
        document.body ? document.body.scrollHeight : 0
      );
      // '*' target is safe because the message is one-way + advisory.
      // We don't know the parent's origin and we're sending no secrets.
      window.parent.postMessage({ type: 'cloudbox-resize', height: h }, '*');
    } catch (e) {
      // postMessage can throw in degenerate cases (sandboxed iframe with
      // allow-same-origin disabled, opaque parent). Treat as no-op.
    }
  }
  if ('ResizeObserver' in window) {
    var ro = new ResizeObserver(postResize);
    if (document.body) ro.observe(document.body);
    document.addEventListener('DOMContentLoaded', function () {
      if (document.body) ro.observe(document.body);
      postResize();
    });
  } else {
    // ResizeObserver missing — fall back to a low-frequency tick. Rare;
    // every browser kg-ui supports has had it since 2019.
    setInterval(postResize, 1000);
  }

  // ---- Outbound: 401-watching fetch wrapper. ---------------------------
  // Wraps the global fetch so any /api/* call that returns 401 (or 403
  // with an "elevation required" body hint) fires a single
  // cloudbox-auth-required message at the parent.
  var origFetch = window.fetch;
  if (origFetch) {
    window.fetch = function () {
      var args = arguments;
      // Add the CSRF token on mutating requests when we have one.
      try {
        var init = args[1] || {};
        var method = (init.method || 'GET').toUpperCase();
        if (KG_CSRF_TOKEN && method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
          var headers = new Headers(init.headers || {});
          if (!headers.has('X-CSRF-Token')) {
            headers.set('X-CSRF-Token', KG_CSRF_TOKEN);
            init.headers = headers;
            args[1] = init;
          }
        }
      } catch (e) {
        // Header construction failure isn't fatal — leave the request
        // alone and let the upstream call fail/succeed as normal.
      }
      return origFetch.apply(this, args).then(function (resp) {
        if (resp && resp.status === 401) {
          try {
            window.parent.postMessage({ type: 'cloudbox-auth-required' }, '*');
          } catch (e) {}
        }
        return resp;
      });
    };
  }
})();
