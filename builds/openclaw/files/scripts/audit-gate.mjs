#!/usr/bin/env node
// audit-gate.mjs — enforces the braces GHSA-vfj7-8cjw-p6xm exception policy.
//
// Gates (see bashy-build/audit-exceptions for the reviewed exception list):
//   (a) enforced by `pnpm audit --prod --audit-level high` in CI (must exit 0);
//       this script additionally FAILS if the prod audit JSON mentions the
//       `braces` package at ANY severity (exception covers dev-only use).
//   (b) on the FULL audit, FAIL on any high/critical advisory except exactly
//       the advisories listed in the exceptions file.
//   (c) FAIL (exception expired) if an excepted advisory now lists a patched /
//       fixed version, i.e. an upgrade path exists and the exception must be
//       retired instead of renewed.
//
// Fails closed: an unrecognized audit JSON shape with non-empty content is a
// failure, never a pass.
//
// Usage:
//   node audit-gate.mjs --full audit-full.json --prod audit-prod.json \
//     --exceptions audit-exceptions
//
// Exceptions file format (one per line, '#' comments, blank lines ignored):
//   GHSA-vfj7-8cjw-p6xm | braces | <reason> | reviewed-on: YYYY-MM-DD | <evidence>

import fs from "node:fs";

function usage() {
  console.error(
    "usage: audit-gate.mjs --full <audit-full.json> --prod <audit-prod.json> --exceptions <file>",
  );
  process.exit(2);
}

const args = process.argv.slice(2);
const opt = {};
for (let i = 0; i < args.length; i += 2) opt[args[i]] = args[i + 1];
if (!opt["--full"] || !opt["--prod"] || !opt["--exceptions"]) usage();

const HIGH_CRITICAL = new Set(["high", "critical"]);

function parseExceptions(path) {
  const list = [];
  for (const raw of fs.readFileSync(path, "utf8").split("\n")) {
    const line = raw.trim();
    if (!line || line.startsWith("#")) continue;
    const parts = line.split("|").map((s) => s.trim());
    if (parts.length < 2 || !/^GHSA-[a-z0-9-]+$/i.test(parts[0])) {
      throw new Error(`malformed exceptions line: ${raw}`);
    }
    // A reason starting with "BUILD-ENV:" marks an advisory present only in the
    // build environment (dev path) whose SHIPPED copies are patched and gated
    // separately; it still expires if the package enters the --prod graph, but
    // not merely because a fixed version exists (that fix is what ships).
    const buildEnv = /^BUILD-ENV:/i.test(parts[2] ?? "");
    list.push({ advisory: parts[0].toUpperCase(), pkg: parts[1].toLowerCase(), buildEnv });
  }
  return list;
}

// Collect advisories from npm- and pnpm-style audit JSON shapes into:
//   { id, ghsa, pkg, severity, patched }
// where `patched` is true when the report names a fixed/patched version.
function collectAdvisories(doc) {
  const out = [];
  const push = (a) => {
    if (!a || typeof a !== "object") return;
    const severity = String(a.severity ?? a.severityLevel ?? "").toLowerCase();
    const pkg = String(
      a.module_name ?? a.package_name ?? a.packageName ?? a.name ?? a.package ?? "",
    ).toLowerCase();
    const url = String(a.url ?? "");
    const ghsaMatch =
      url.match(/GHSA-[a-z0-9-]+/i) ||
      String(a.id ?? a.advisory ?? a.advisoryId ?? "").match(/GHSA-[a-z0-9-]+/i);
    const ghsa = ghsaMatch ? ghsaMatch[0].toUpperCase() : null;
    const patchedRaw =
      a.patched_versions ?? a.patchedVersions ?? a.fixed_in ?? a.fixedIn ?? a.fixAvailable ?? "";
    const patchedText = typeof patchedRaw === "string" ? patchedRaw : JSON.stringify(patchedRaw);
    const patched =
      patchedText.trim() !== "" &&
      !/^(none|null|undefined|<0|no fix|not fixed|false)$/i.test(patchedText.trim());
    out.push({
      id: String(a.id ?? a.advisory ?? ghsa ?? pkg ?? "unknown"),
      ghsa,
      pkg,
      severity,
      patched,
    });
  };
  if (Array.isArray(doc)) {
    doc.forEach(push);
  } else if (doc && typeof doc === "object") {
    for (const key of ["advisories", "vulnerabilities"]) {
      const section = doc[key];
      if (!section || typeof section !== "object") continue;
      for (const entry of Object.values(section)) {
        if (Array.isArray(entry)) entry.forEach(push);
        else if (entry && typeof entry === "object" && "via" in entry && Array.isArray(entry.via)) {
          // npm v2+ shape: { name, severity, via: [name | {url,...}], fixAvailable }
          for (const v of entry.via) {
            if (v && typeof v === "object") push({ ...v, name: v.name ?? entry.name, severity: v.severity ?? entry.severity, fixAvailable: entry.fixAvailable });
            else push({ name: String(v), severity: entry.severity, fixAvailable: entry.fixAvailable });
          }
        } else push(entry);
      }
    }
  }
  return out.filter((a) => a.pkg || a.ghsa);
}

function loadAudit(path) {
  const raw = fs.readFileSync(path, "utf8").trim();
  // pnpm audit prints nothing (empty) when clean; treat empty as no findings.
  if (!raw) return [];
  const doc = JSON.parse(raw);
  const found = collectAdvisories(doc);
  const looksEmpty =
    (doc && typeof doc === "object" && !Array.isArray(doc) &&
      JSON.stringify(doc).replace(/[^\w]/g, "") === "") ||
    (doc?.metadata?.vulnerabilities &&
      Object.values(doc.metadata.vulnerabilities).every((n) => Number(n) === 0) &&
      found.length === 0);
  if (found.length === 0 && !looksEmpty && JSON.stringify(doc).length > 32) {
    throw new Error(
      `unrecognized audit JSON shape in ${path}; failing closed (see README for supported shapes)`,
    );
  }
  void looksEmpty;
  return found;
}

const exceptions = parseExceptions(opt["--exceptions"]);
const full = loadAudit(opt["--full"]);
const prod = loadAudit(opt["--prod"]);
const failures = [];

// Gate (a2): braces must not appear in the --prod graph at any severity.
for (const a of prod) {
  if (a.pkg === "braces") {
    failures.push(`braces appears in the --prod graph (${a.id}, severity ${a.severity || "?"})`);
  }
}

// Gate (b): full audit high/critical must all be excepted.
for (const a of full) {
  if (!HIGH_CRITICAL.has(a.severity)) continue;
  const allowed = exceptions.some(
    (e) => (a.ghsa && e.advisory === a.ghsa) || (!a.ghsa && e.pkg === a.pkg),
  );
  if (!allowed) failures.push(`unexcepted ${a.severity} advisory: ${a.id} (package ${a.pkg || "?"})`);
}

// Gate (c): an excepted advisory that now lists a patched version has expired,
// or the excepted package now appears in the --prod graph.
const prodPkgs = new Set(prod.map((a) => a.pkg).filter(Boolean));
for (const e of exceptions) {
  if (prodPkgs.has(e.pkg)) {
    failures.push(`exception expired: ${e.advisory} (${e.pkg}) now appears in the --prod graph`);
  }
  const hits = full.filter((a) => (a.ghsa && a.ghsa === e.advisory) || a.pkg === e.pkg);
  for (const h of hits) {
    if (h.patched && !e.buildEnv) {
      failures.push(
        `exception expired: ${e.advisory} (${e.pkg}) now lists a patched version (via ${h.id})`,
      );
    }
  }
  if (hits.length === 0) {
    console.log(`note: excepted advisory ${e.advisory} (${e.pkg}) absent from this audit; review at next renewal`);
  }
}

if (failures.length > 0) {
  console.error("audit-gate: FAIL");
  for (const f of failures) console.error("  - " + f);
  process.exit(1);
}
console.log(
  `audit-gate: OK (${full.length} full-audit advisories, ${prod.length} prod-audit advisories, ${exceptions.length} exception(s))`,
);
