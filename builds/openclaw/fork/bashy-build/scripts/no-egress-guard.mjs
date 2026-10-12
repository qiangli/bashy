// no-egress-guard.mjs — NODE_OPTIONS --import hook for the no-egress test.
//
// Intercepts outbound attempts to the guarded hosts
// (registry.npmjs.org, telemetry.openclaw.ai) from the tested process by
// wrapping global fetch and http/https.request. Every attempt is appended to
// $BASHY_EGRESS_LOG as JSON and then aborted. All other hosts pass through
// untouched: this guard asserts silence toward the update/telemetry hosts,
// not airgap.
//
// Second layer only: the strace analysis in no-egress-test.sh asserts
// loopback-only traffic. This hook additionally names the exact guarded
// hosts (the in-place update check and telemetry endpoints), so a failure
// message says WHICH kill switch failed.

import fs from "node:fs";
import http from "node:http";
import https from "node:https";

const LOG = process.env.BASHY_EGRESS_LOG;
const GUARDED = new Set(["registry.npmjs.org", "telemetry.openclaw.ai"]);

function record(entry) {
  if (!LOG) return;
  fs.appendFileSync(LOG, JSON.stringify(entry) + "\n");
}

function isGuarded(input, options) {
  let host = "";
  try {
    if (typeof input === "string" || input instanceof URL) {
      host = new URL(input).hostname.toLowerCase();
    } else if (input && typeof input === "object") {
      host =
        String(input.hostname ?? input.host ?? options?.hostname ?? options?.host ?? "")
          .split(":")[0]
          .toLowerCase();
    }
    if (!host && options) {
      host = String(options.hostname ?? options.host ?? "").split(":")[0].toLowerCase();
    }
  } catch {
    return false;
  }
  return GUARDED.has(host);
}

const origFetch = globalThis.fetch;
if (typeof origFetch === "function") {
  globalThis.fetch = (input, init) => {
    if (isGuarded(input, init)) {
      record({ host: "guard", api: "fetch", url: String(input), at: Date.now() });
      return Promise.reject(new Error("no-egress-guard: blocked fetch to guarded host"));
    }
    return origFetch(input, init);
  };
}

for (const mod of [http, https]) {
  const orig = mod.request;
  mod.request = function (input, options, ...rest) {
    if (isGuarded(input, typeof options === "object" ? options : undefined)) {
      record({ host: "guard", api: `${mod === https ? "https" : "http"}.request`, url: String(input?.href ?? input), at: Date.now() });
      const req = orig.call(this, "http://127.0.0.1:9/", ...rest);
      process.nextTick(() =>
        req.destroy(new Error("no-egress-guard: blocked request to guarded host")),
      );
      return req;
    }
    return orig.call(this, input, options, ...rest);
  };
}
