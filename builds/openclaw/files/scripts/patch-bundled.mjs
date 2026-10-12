#!/usr/bin/env node
// patch-bundled.mjs — post-pack security patch for GHSA-ch52-4w7c-c8xp.
//
// After upstream's scripts/package-openclaw-for-docker.mjs writes the
// gateway tarball and BEFORE any signing, this script replaces EVERY bundled
// copy of http-cache-semantics below the fixed version (at least
// node_modules/npm/node_modules/http-cache-semantics — npm 12.x bundles its
// own 4.2.0 and bundled deps ignore overrides, so the workspace override in
// apply-override.sh cannot move it) with the registry tarball of the fixed
// release, then repacks deterministically and updates the CycloneDX SBOM.
//
// node only, no dependencies (tar/gzip via builtins). Fail-closed throughout:
// a mismatching registry integrity, an unreadable version, or an unsupported
// tar entry aborts without touching the tarball.
//
// Usage (patch):
//   node patch-bundled.mjs --tarball <in-out.tgz> --replacement <pkg.tgz> \
//     --integrity <sha512-BASE64 of pkg.tgz> [--sbom <cyclonedx.json>]
//     [--package http-cache-semantics] [--min-version 4.3.0]
//     [--fixed-mtime 0]
// Usage (gate — run after the patch, before signing):
//   node patch-bundled.mjs --verify-only --tarball <in-out.tgz>
//     [--package http-cache-semantics] [--min-version 4.3.0]
//
// The gate FAILS (exit 1) if any copy of the package below --min-version
// remains anywhere in the tarball, including copies with a missing or
// unparseable version.

import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import zlib from "node:zlib";

const DEFAULT_PACKAGE = "http-cache-semantics";
const DEFAULT_MIN_VERSION = "4.3.0";
const ADVISORY = "GHSA-ch52-4w7c-c8xp";
const SBOM_NOTE = `bashy security patch ${ADVISORY}`;

function usageError(message) {
  console.error(`patch-bundled: ${message}`);
  console.error("usage: node patch-bundled.mjs --tarball <tgz> --replacement <tgz> --integrity <sha512-...> [--sbom <json>] [--verify-only --tarball <tgz>]");
  process.exit(2);
}

function parseArgs(argv) {
  const opt = {};
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i];
    if (!arg.startsWith("--")) usageError(`unexpected argument: ${arg}`);
    const key = arg.slice(2);
    if (key === "verify-only") {
      opt.verifyOnly = true;
      continue;
    }
    const value = argv[i + 1];
    if (value === undefined || value.startsWith("--")) usageError(`missing value for ${arg}`);
    opt[key.replace(/-([a-z])/g, (_, c) => c.toUpperCase())] = value;
    i++;
  }
  opt.package = opt.package ?? DEFAULT_PACKAGE;
  opt.minVersion = opt.minVersion ?? DEFAULT_MIN_VERSION;
  opt.fixedMtime = opt.fixedMtime ?? "0";
  if (!opt.tarball) usageError("missing --tarball");
  if (!/^\d+$/.test(opt.fixedMtime)) usageError("--fixed-mtime must be epoch seconds");
  if (!opt.verifyOnly && (!opt.replacement || !opt.integrity)) {
    usageError("patch mode needs --replacement and --integrity");
  }
  const min = parseVersion(opt.minVersion);
  if (!min) usageError(`unparseable --min-version: ${opt.minVersion}`);
  opt.minParts = min;
  opt.fixedMtimeNum = Number(opt.fixedMtime);
  return opt;
}

// --- minimal semver (numeric x.y.z; prerelease/build metadata ignored) ---

function parseVersion(text) {
  const m = /^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$/.exec(String(text ?? "").trim());
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
}

function compareVersions(a, b) {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return 0;
}

function satisfiesMin(versionText, minParts) {
  const parts = parseVersion(versionText);
  return parts ? compareVersions(parts, minParts) >= 0 : null; // null = unreadable
}

// --- tar (ustar) reader/writer: regular files, dirs, symlinks, pax path ---

function parseOctal(buf, off, len) {
  if (buf[off] & 0x80) {
    // base-256 binary encoding: clear the top bit, read big-endian.
    let value = buf[off] & 0x7f;
    for (let i = 1; i < len; i++) value = value * 256 + buf[i];
    return value;
  }
  const text = buf.subarray(off, off + len).toString("ascii").replace(/\0.*$/, "").trim();
  if (!/^[0-7]*$/.test(text)) throw new Error("patch-bundled: corrupt tar header (bad octal)");
  return text === "" ? 0 : parseInt(text, 8);
}

function parseCString(buf, off, len) {
  const end = buf.indexOf(0, off);
  const stop = end === -1 || end > off + len ? off + len : end;
  return buf.subarray(off, stop).toString("utf8");
}

function verifyHeaderChecksum(header) {
  const stored = header.subarray(148, 156).toString("ascii").replace(/[\0 ]/g, "");
  let sum = 0;
  for (let i = 0; i < 512; i++) sum += i >= 148 && i < 156 ? 32 : header[i];
  const expected = stored === "" ? 0 : parseInt(stored, 8);
  if (!Number.isSafeInteger(expected) || sum !== expected) {
    throw new Error("patch-bundled: corrupt tar header (checksum mismatch); refusing to repack");
  }
}

function parsePaxRecords(buf) {
  // One 'x' block: lines "<len> <key>=<value>\n"; we only need `path`.
  const out = {};
  for (const line of buf.toString("utf8").split("\n")) {
    if (!line) continue;
    const m = /^(\d+) ([^=]+)=(.*)$/.exec(line);
    if (!m) throw new Error("patch-bundled: corrupt pax extended header");
    out[m[2]] = m[3];
  }
  return out;
}

function parseTarArchive(data) {
  const entries = [];
  let off = 0;
  let paxPath = null;
  const block = Buffer.alloc(512);
  while (off + 512 <= data.length) {
    const header = data.subarray(off, off + 512);
    if (header.equals(block)) break; // first zero block ends the archive
    if (!/^ustar/.test(header.subarray(257, 262).toString("ascii"))) {
      throw new Error("patch-bundled: unsupported tar header (not ustar)");
    }
    const size = parseOctal(header, 124, 12);
    verifyHeaderChecksum(header);
    const typeflag = String.fromCharCode(header[156]);
    const name = parseCString(header, 0, 100);
    const prefix = parseCString(header, 345, 155);
    const fullName = prefix ? `${prefix}/${name}` : name;
    const entry = {
      name: fullName,
      mode: parseOctal(header, 100, 8),
      uid: parseOctal(header, 108, 8),
      gid: parseOctal(header, 116, 8),
      mtime: parseOctal(header, 136, 12),
      type: typeflag,
      linkname: parseCString(header, 157, 100),
      uname: parseCString(header, 265, 32),
      gname: parseCString(header, 297, 32),
      data: Buffer.alloc(0),
    };
    off += 512;
    const payload = data.subarray(off, off + size);
    if (payload.length < size) throw new Error("patch-bundled: truncated tar archive");
    off += Math.ceil(size / 512) * 512;
    if (typeflag === "x") {
      const records = parsePaxRecords(payload);
      if (records.path) paxPath = records.path;
      continue; // pax headers never become output entries themselves
    }
    if (typeflag === "g" || typeflag === "L" || typeflag === "K") {
      throw new Error(`patch-bundled: unsupported tar entry type '${typeflag}' for ${fullName}; refusing to repack blindly`);
    }
    if (typeflag !== "0" && typeflag !== "\0" && typeflag !== "5" && typeflag !== "2") {
      throw new Error(`patch-bundled: unsupported tar entry type '${typeflag}' for ${fullName}; refusing to repack blindly`);
    }
    if (paxPath) {
      entry.name = paxPath;
      paxPath = null;
    }
    if ((typeflag === "0" || typeflag === "\0") && entry.name.endsWith("/")) {
      throw new Error(`patch-bundled: regular file with trailing slash: ${entry.name}`);
    }
    entry.data = Buffer.from(payload);
    entries.push(entry);
  }
  return entries;
}

function formatPaxRecord(key, value) {
  const body = `${key}=${value}\n`;
  let len = Buffer.byteLength(body, "utf8") + 2; // guess: digits + space
  for (;;) {
    const digits = String(len).length;
    const next = Buffer.byteLength(body, "utf8") + digits + 1;
    if (next === len) return `${len} ${body}`;
    len = next;
  }
}

function writeHeader(entry, fixedMtime) {
  // Returns 1-2 512-byte blocks: optional ././@PaxHeader for long names.
  const nameBytes = Buffer.byteLength(entry.name, "utf8");
  let name = entry.name;
  let prefix = "";
  let paxBlock = null;
  if (nameBytes <= 100) {
    // fits the name field as-is
  } else {
    const idx = entry.name.lastIndexOf("/", Math.min(entry.name.length, 156));
    const candidatePrefix = idx > 0 ? entry.name.slice(0, idx) : "";
    const candidateName = idx > 0 ? entry.name.slice(idx + 1) : entry.name;
    if (
      idx > 0 &&
      Buffer.byteLength(candidatePrefix, "utf8") <= 155 &&
      Buffer.byteLength(candidateName, "utf8") <= 100 &&
      Buffer.byteLength(candidateName, "utf8") > 0
    ) {
      prefix = candidatePrefix;
      name = candidateName;
    } else {
      // GNU-style pax long path: ././@PaxHeader + truncated name.
      const record = formatPaxRecord("path", entry.name);
      const payload = Buffer.from(record, "utf8");
      const paxEntry = {
        name: "././@PaxHeader",
        mode: 0o644,
        uid: 0,
        gid: 0,
        type: "x",
        linkname: "",
        uname: "",
        gname: "",
        data: payload,
      };
      paxBlock = Buffer.concat([writeHeader(paxEntry, fixedMtime), padToBlocks(payload)]);
      name = entry.name.slice(0, 100);
    }
  }
  const header = Buffer.alloc(512);
  header.write(name, 0, "utf8");
  header.write(octal(entry.mode & 0o7777, 8), 100, "ascii");
  header.write(octal(entry.uid, 8), 108, "ascii");
  header.write(octal(entry.gid, 8), 116, "ascii");
  header.write(octal(entry.data.length, 12), 124, "ascii");
  header.write(octal(fixedMtime, 12), 136, "ascii");
  header.write("        ", 148, "ascii"); // checksum placeholder (spaces)
  header[156] = entry.type.charCodeAt(0);
  header.write(entry.linkname, 157, "utf8");
  header.write("ustar", 257, "ascii");
  header.write("00", 263, "ascii");
  header.write(entry.uname, 265, "utf8");
  header.write(entry.gname, 297, "utf8");
  if (prefix) header.write(prefix, 345, "utf8");
  let sum = 0;
  for (const byte of header) sum += byte;
  // POSIX chksum: 6 octal digits, NUL, space (NOT the generic octal() form).
  const sumText = sum.toString(8);
  if (sumText.length > 6) throw new Error("patch-bundled: tar checksum overflow");
  header.write(sumText.padStart(6, "0") + "\0 ", 148, "ascii");
  const out = paxBlock ? [paxBlock, header] : [header];
  return Buffer.concat(out);
}

function octal(value, len) {
  // len includes the trailing NUL: 7 digits + NUL for the 8-wide fields.
  const text = Math.floor(value).toString(8);
  if (text.length > len - 1) throw new Error("patch-bundled: tar field overflow");
  return text.padStart(len - 1, "0") + "\0";
}

function padToBlocks(data) {
  if (data.length === 0) return data; // empty files occupy zero blocks
  const remainder = data.length % 512;
  if (remainder === 0) return data;
  return Buffer.concat([data, Buffer.alloc(512 - remainder)]);
}

function createTarArchive(entries, fixedMtime) {
  const sorted = [...entries].sort((a, b) =>
    a.name < b.name ? -1 : a.name > b.name ? 1 : 0,
  );
  const parts = [];
  for (const entry of sorted) {
    parts.push(writeHeader(entry, fixedMtime));
    if (entry.type === "0" || entry.type === "\0") parts.push(padToBlocks(entry.data));
  }
  parts.push(Buffer.alloc(1024)); // two zero blocks end the archive
  return Buffer.concat(parts);
}

function gzipDeterministic(data) {
  const gzipped = zlib.gzipSync(data);
  // Zero the MTIME field (bytes 4-7) so identical inputs give identical
  // bytes on every run, independent of the clock. OS/XFL bytes are constant
  // for a given Node build (CI pins NODE_VERSION), so they need no edit.
  gzipped[4] = 0;
  gzipped[5] = 0;
  gzipped[6] = 0;
  gzipped[7] = 0;
  return gzipped;
}

function readTarball(tarballPath) {
  return parseTarArchive(zlib.gunzipSync(fs.readFileSync(tarballPath)));
}

function writeTarballAtomic(tarballPath, data) {
  const mode = fs.statSync(tarballPath).mode;
  const tmp = `${tarballPath}.patched.${process.pid}`;
  fs.writeFileSync(tmp, data);
  fs.chmodSync(tmp, mode);
  fs.renameSync(tmp, tarballPath);
}

// --- locate bundled copies: <anything>/node_modules/<pkg>/package.json ---

function findPackageCopies(entries, packageName) {
  const suffix = `/node_modules/${packageName}/package.json`;
  const copies = new Map(); // prefix -> { versionText, versionOk }
  for (const entry of entries) {
    if (entry.type !== "0" && entry.type !== "\0") continue;
    if (!entry.name.endsWith(suffix)) continue;
    const prefix = entry.name.slice(0, entry.name.length - "/package.json".length);
    let versionText = null;
    try {
      versionText = JSON.parse(entry.data.toString("utf8")).version ?? null;
    } catch {
      versionText = null; // unreadable manifest: handled as vulnerable below
    }
    copies.set(prefix, { versionText });
  }
  return copies;
}

// --- patch mode ---

function verifyReplacementIntegrity(replacementBytes, integrity) {
  const m = /^sha512-([A-Za-z0-9+/]+={0,2})$/.exec(String(integrity ?? "").trim());
  if (!m) {
    throw new Error(
      "patch-bundled: --integrity must be the registry dist.integrity value (sha512-BASE64); refusing to proceed",
    );
  }
  const actual = crypto.createHash("sha512").update(replacementBytes).digest("base64");
  if (actual !== m[1]) {
    throw new Error("patch-bundled: replacement tarball sha512 does NOT match dist.integrity; refusing to proceed");
  }
}

function readReplacementEntries(replacementBytes, packageName, minParts) {
  const entries = parseTarArchive(zlib.gunzipSync(Buffer.from(replacementBytes)));
  const root = entries.find((e) => e.name === "package/package.json");
  if (!root) throw new Error("patch-bundled: replacement tarball has no package/package.json");
  let manifest;
  try {
    manifest = JSON.parse(root.data.toString("utf8"));
  } catch {
    throw new Error("patch-bundled: replacement package.json is not valid JSON");
  }
  if (manifest.name !== packageName) {
    throw new Error(`patch-bundled: replacement is ${manifest.name}, not ${packageName}; refusing to proceed`);
  }
  const ok = satisfiesMin(manifest.version, minParts);
  if (ok !== true) {
    throw new Error(`patch-bundled: replacement version ${manifest.version} is not >= the fixed release; refusing to proceed`);
  }
  const stripped = [];
  for (const entry of entries) {
    if (!entry.name.startsWith("package/")) {
      throw new Error(`patch-bundled: replacement entry outside package/: ${entry.name}`);
    }
    stripped.push({ ...entry, name: entry.name.slice("package/".length) });
  }
  if (stripped.length === 0) throw new Error("patch-bundled: replacement tarball is empty");
  return { entries: stripped, version: String(manifest.version) };
}

function applyPatch(entries, replacement, packageName, minParts) {
  const copies = findPackageCopies(entries, packageName);
  const patched = []; // { prefix, from }
  const kept = []; // { prefix, version }
  for (const [prefix, { versionText }] of copies) {
    if (satisfiesMin(versionText, minParts) === true) {
      kept.push({ prefix, version: versionText });
    } else {
      patched.push({ prefix, from: versionText });
    }
  }
  if (patched.length === 0) return { entries, patched, kept };
  const next = entries.filter(
    (entry) => !patched.some(({ prefix }) => entry.name === prefix || entry.name.startsWith(prefix + "/")),
  );
  for (const { prefix } of patched) {
    for (const file of replacement.entries) {
      next.push({ ...file, name: `${prefix}/${file.name}`, data: Buffer.from(file.data) });
    }
  }
  return { entries: next, patched, kept };
}

// --- gate mode: fail if any copy below --min-version remains ---

function verifyTarball(entries, packageName, minParts) {
  const copies = findPackageCopies(entries, packageName);
  const bad = [];
  for (const [prefix, { versionText }] of copies) {
    const ok = satisfiesMin(versionText, minParts);
    if (ok !== true) bad.push(`${prefix}: ${versionText ?? "<unreadable version>"}`);
  }
  return { copies: copies.size, bad };
}

// --- CycloneDX SBOM in-place update ---

function updateSbom(sbomPath, packageName, newVersion) {
  let doc;
  try {
    doc = JSON.parse(fs.readFileSync(sbomPath, "utf8"));
  } catch (error) {
    throw new Error(`patch-bundled: cannot read SBOM ${sbomPath}: ${error.message}`);
  }
  if (!doc || typeof doc !== "object" || !Array.isArray(doc.components)) {
    throw new Error(`patch-bundled: SBOM ${sbomPath} has no components[]; refusing to guess its shape`);
  }
  const newParts = parseVersion(newVersion);
  if (!newParts) throw new Error(`patch-bundled: replacement version ${newVersion} is unparseable`);
  let updated = 0;
  const renames = []; // [oldBomRef, newBomRef] for dependency-graph fixups
  for (const component of doc.components) {
    if (!component || typeof component !== "object") continue;
    if (component.name !== packageName) continue;
    const oldVersion = component.version;
    const oldParts = parseVersion(oldVersion);
    if (!oldParts) continue; // no usable version: leave for a human, do not guess
    if (compareVersions(oldParts, newParts) >= 0) continue; // already at or above the patch: never downgrade
    const before = component["bom-ref"];
    component.version = newVersion;
    if (typeof component.purl === "string" && component.purl.startsWith(`pkg:npm/${packageName}@`)) {
      const qualifier = component.purl.includes("?") ? component.purl.slice(component.purl.indexOf("?")) : "";
      component.purl = `pkg:npm/${packageName}@${newVersion}${qualifier}`;
    } else if (component.purl === undefined) {
      component.purl = `pkg:npm/${packageName}@${newVersion}`;
    }
    if (typeof before === "string" && before.includes(String(oldVersion))) {
      component["bom-ref"] = before.split(String(oldVersion)).join(newVersion);
      renames.push([before, component["bom-ref"]]);
    }
    component.properties = Array.isArray(component.properties) ? component.properties : [];
    const note = `${SBOM_NOTE}: bundled ${packageName}@${oldVersion} replaced with registry ${packageName}@${newVersion}`;
    if (!component.properties.some((p) => p && p.value === note)) {
      component.properties.push({ name: "bashy:note", value: note });
    }
    updated++;
  }
  if (renames.length > 0 && Array.isArray(doc.dependencies)) {
    for (const dep of doc.dependencies) {
      if (!dep || !Array.isArray(dep.dependsOn)) continue;
      dep.dependsOn = dep.dependsOn.map((ref) => {
        for (const [oldRef, newRef] of renames) if (ref === oldRef) return newRef;
        return ref;
      });
    }
  }
  if (updated > 0) {
    fs.writeFileSync(sbomPath, JSON.stringify(doc, null, 2) + "\n");
  }
  return updated;
}

// --- entry point ---

function main() {
  const opt = parseArgs(process.argv.slice(2));
  if (opt.verifyOnly) {
    const entries = readTarball(opt.tarball);
    const { copies, bad } = verifyTarball(entries, opt.package, opt.minParts);
    if (bad.length > 0) {
      console.error(`patch-bundled: VERIFY FAIL — ${bad.length} vulnerable ${opt.package} cop(ies) in ${opt.tarball}:`);
      for (const line of bad) console.error(`  - ${line}`);
      process.exit(1);
    }
    console.log(`patch-bundled: VERIFY OK — ${copies} ${opt.package} cop(ies), all >= ${opt.minVersion}`);
    return;
  }
  const replacementBytes = fs.readFileSync(opt.replacement);
  verifyReplacementIntegrity(replacementBytes, opt.integrity);
  console.log("patch-bundled: replacement integrity OK (sha512 matches dist.integrity)");
  const replacement = readReplacementEntries(replacementBytes, opt.package, opt.minParts);
  console.log(`patch-bundled: replacement is ${opt.package}@${replacement.version}`);
  const entries = readTarball(opt.tarball);
  const { entries: next, patched, kept } = applyPatch(entries, replacement, opt.package, opt.minParts);
  for (const { prefix, version } of kept) console.log(`patch-bundled: keep ${prefix} (already ${version})`);
  if (patched.length === 0) {
    console.log(`patch-bundled: nothing to patch — no ${opt.package} < ${opt.minVersion} in ${opt.tarball}`);
  } else {
    for (const { prefix, from } of patched) {
      console.log(`patch-bundled: patch ${prefix} (was ${from ?? "<unreadable>"}) -> ${replacement.version}`);
    }
    writeTarballAtomic(opt.tarball, gzipDeterministic(createTarArchive(next, opt.fixedMtimeNum)));
    console.log(`patch-bundled: repacked ${opt.tarball} (${patched.length} cop(ies), deterministic: sorted entries, fixed mtime ${opt.fixedMtimeNum})`);
  }
  // The tarball gate is the enforcement; re-verify the bytes just written.
  const { bad } = verifyTarball(readTarball(opt.tarball), opt.package, opt.minParts);
  if (bad.length > 0) {
    console.error("patch-bundled: FAIL — vulnerable copies remain after patch:");
    for (const line of bad) console.error(`  - ${line}`);
    process.exit(1);
  }
  if (opt.sbom) {
    if (patched.length === 0) {
      console.log(`patch-bundled: SBOM ${opt.sbom} untouched (nothing patched)`);
    } else {
      const updated = updateSbom(opt.sbom, opt.package, replacement.version);
      console.log(`patch-bundled: SBOM ${opt.sbom}: updated ${updated} component(s) to ${replacement.version} with "${SBOM_NOTE}" note`);
    }
  }
  console.log("patch-bundled: done");
}

export {
  parseVersion,
  compareVersions,
  satisfiesMin,
  parseTarArchive,
  createTarArchive,
  gzipDeterministic,
  findPackageCopies,
  verifyTarball,
  applyPatch,
  updateSbom,
  verifyReplacementIntegrity,
};

const invokedAsScript =
  process.argv[1] !== undefined &&
  path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);

if (invokedAsScript) {
  try {
    main();
  } catch (error) {
    console.error(`patch-bundled: ERROR — ${error instanceof Error ? error.message : String(error)}`);
    process.exit(1);
  }
}
