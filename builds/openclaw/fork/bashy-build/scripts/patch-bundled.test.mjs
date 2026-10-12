// patch-bundled.test.mjs — fixture tests for patch-bundled.mjs.
// Run: node --test builds/openclaw/files/scripts/patch-bundled.test.mjs
// Self-contained: synthetic tarballs, a locally built fake registry tarball,
// and a locally computed sha512 (no network, no registry).
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import crypto from "node:crypto";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { describe, it } from "node:test";
import { fileURLToPath } from "node:url";
import zlib from "node:zlib";
import {
  createTarArchive,
  gzipDeterministic,
  parseTarArchive,
} from "./patch-bundled.mjs";

const SCRIPT = fileURLToPath(new URL("./patch-bundled.mjs", import.meta.url));
const PKG = "http-cache-semantics";

function fileEntry(name, text, mode = 0o644) {
  return { name, mode, uid: 0, gid: 0, type: "0", linkname: "", uname: "", gname: "", data: Buffer.from(text) };
}

function dirEntry(name) {
  return { name: name.endsWith("/") ? name : name + "/", mode: 0o755, uid: 0, gid: 0, type: "5", linkname: "", uname: "", gname: "", data: Buffer.alloc(0) };
}

// Fake registry tarball: package/{package.json,index.js} for PKG@version.
function fakeRegistryTarball(version, bodyMarker) {
  const entries = [
    dirEntry("package"),
    fileEntry("package/package.json", JSON.stringify({ name: PKG, version })),
    fileEntry("package/index.js", `// ${bodyMarker}\n`),
  ];
  return gzipDeterministic(createTarArchive(entries, 0));
}

// Fake shipped gateway tarball with TWO vulnerable 4.2.0 copies (one under
// npm's bundled tree, one dev-path nested copy), one already-fixed 4.3.0
// copy, plus tar edge cases: an empty file, a symlink, a >100-char path.
function fakeGatewayTarball() {
  const longDir = "package/node_modules/npm/node_modules/" + "a".repeat(90);
  const entries = [
    dirEntry("package"),
    fileEntry("package/package.json", JSON.stringify({ name: "openclaw-gateway-test", version: "0.0.0" })),
    dirEntry("package/node_modules/npm"),
    fileEntry("package/node_modules/npm/package.json", JSON.stringify({ name: "npm", version: "12.1.0" })),
    dirEntry("package/node_modules/npm/node_modules/http-cache-semantics"),
    fileEntry(
      "package/node_modules/npm/node_modules/http-cache-semantics/package.json",
      JSON.stringify({ name: PKG, version: "4.2.0" }),
    ),
    fileEntry("package/node_modules/npm/node_modules/http-cache-semantics/index.js", "// vulnerable\n"),
    fileEntry("package/node_modules/npm/node_modules/http-cache-semantics/empty.js", ""),
    {
      name: "package/node_modules/npm/node_modules/http-cache-semantics/link.js",
      mode: 0o777, uid: 0, gid: 0, type: "2", linkname: "index.js", uname: "", gname: "", data: Buffer.alloc(0),
    },
    fileEntry(`${longDir}/package.json`, JSON.stringify({ name: PKG, version: "4.2.0" })),
    fileEntry("package/node_modules/other/node_modules/http-cache-semantics/package.json", JSON.stringify({ name: PKG, version: "4.3.0" })),
  ];
  return gzipDeterministic(createTarArchive(entries, 0));
}

function fakeSbom() {
  return {
    bomFormat: "CycloneDX",
    specVersion: "1.4",
    components: [
      {
        "bom-ref": `pkg:npm/${PKG}@4.2.0`,
        name: PKG,
        version: "4.2.0",
        purl: `pkg:npm/${PKG}@4.2.0`,
        type: "library",
      },
      { "bom-ref": "pkg:npm/npm@12.1.0", name: "npm", version: "12.1.0", purl: "pkg:npm/npm@12.1.0", type: "library" },
    ],
    dependencies: [
      { ref: `pkg:npm/${PKG}@4.2.0`, dependsOn: [] },
      { ref: "pkg:npm/npm@12.1.0", dependsOn: [`pkg:npm/${PKG}@4.2.0`] },
    ],
  };
}

function setup() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "patch-bundled-test-"));
  const tarball = path.join(dir, "gateway.tgz");
  fs.writeFileSync(tarball, fakeGatewayTarball());
  const replacement = path.join(dir, "replacement.tgz");
  fs.writeFileSync(replacement, fakeRegistryTarball("4.3.0", "fixed"));
  const integrity = "sha512-" + crypto.createHash("sha512").update(fs.readFileSync(replacement)).digest("base64");
  const sbom = path.join(dir, "sbom.json");
  fs.writeFileSync(sbom, JSON.stringify(fakeSbom(), null, 2));
  return { dir, tarball, replacement, integrity, sbom };
}

function runCli(args) {
  try {
    const out = execFileSync(process.execPath, [SCRIPT, ...args], { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] });
    return { status: 0, out };
  } catch (error) {
    return { status: error.status ?? 1, out: (error.stdout ?? "") + (error.stderr ?? "") };
  }
}

function readEntries(tarball) {
  return parseTarArchive(zlib.gunzipSync(fs.readFileSync(tarball)));
}

function manifestOf(entries, prefix) {
  const found = entries.find((e) => e.name === `${prefix}/package.json`);
  return found ? JSON.parse(found.data.toString("utf8")).version : undefined;
}

describe("patch-bundled fixtures", () => {
  it("tar round-trips entries incl. empty files, symlinks, long paths, deterministically", () => {
    const before = readEntries(path.join(setup().dir, "gateway.tgz"));
    const again = parseTarArchive(zlib.gunzipSync(gzipDeterministic(createTarArchive(before, 0))));
    assert.equal(again.length, before.length);
    const empty = again.find((e) => e.name.endsWith("/empty.js"));
    assert.ok(empty && empty.data.length === 0, "empty file survives with zero data blocks");
    const link = again.find((e) => e.name.endsWith("/link.js"));
    assert.ok(link && link.type === "2" && link.linkname === "index.js", "symlink survives");
    const long = again.find((e) => e.name.includes("a".repeat(90)));
    assert.ok(long, "long path survives via pax header");
    const a = gzipDeterministic(createTarArchive(before, 0));
    const b = gzipDeterministic(createTarArchive([...before].reverse(), 0));
    assert.ok(a.equals(b), "repack is deterministic regardless of input order");
  });

  it("verify-only FAILS on the vulnerable fixture and names the copies", () => {
    const { tarball } = setup();
    const result = runCli(["--verify-only", "--tarball", tarball]);
    assert.equal(result.status, 1);
    assert.match(result.out, /node_modules\/npm\/node_modules\/http-cache-semantics/);
  });

  it("patch replaces every vulnerable copy, keeps fixed ones, updates the SBOM, then verifies clean", () => {
    const { tarball, replacement, integrity, sbom } = setup();
    const result = runCli(["--tarball", tarball, "--replacement", replacement, "--integrity", integrity, "--sbom", sbom]);
    assert.equal(result.status, 0, result.out);
    const entries = readEntries(tarball);
    assert.equal(manifestOf(entries, "package/node_modules/npm/node_modules/http-cache-semantics"), "4.3.0");
    const fixed = entries.find((e) => e.name === "package/node_modules/npm/node_modules/http-cache-semantics/index.js");
    assert.equal(fixed.data.toString("utf8"), "// fixed\n");
    // The second bundled copy (dev-path nested location) is patched too.
    const nested = entries.filter((e) => e.name.includes("node_modules/http-cache-semantics/package.json"));
    assert.ok(nested.length >= 2, "every bundled copy is visited");
    for (const entry of nested) {
      assert.equal(JSON.parse(entry.data.toString("utf8")).version, "4.3.0");
    }
    // SBOM: version + purl bumped, note property present, graph ref fixed.
    const doc = JSON.parse(fs.readFileSync(sbom, "utf8"));
    const component = doc.components.find((c) => c.name === PKG);
    assert.equal(component.version, "4.3.0");
    assert.equal(component.purl, `pkg:npm/${PKG}@4.3.0`);
    assert.ok(
      component.properties.some((p) => String(p.value).includes(`bashy security patch ${"GHSA-ch52-4w7c-c8xp"}`)),
      "SBOM carries the security-patch note",
    );
    const npmDep = doc.dependencies.find((d) => d.ref === "pkg:npm/npm@12.1.0");
    assert.ok(npmDep.dependsOn.includes(`pkg:npm/${PKG}@4.3.0`), "dependency graph follows the bom-ref rename");
    const npmComponent = doc.components.find((c) => c.name === "npm");
    assert.equal(npmComponent.version, "12.1.0", "unrelated components untouched");
    // Gate passes on the patched tarball, and re-patching is byte-identical.
    assert.equal(runCli(["--verify-only", "--tarball", tarball]).status, 0);
    const bytesAfterFirstPatch = fs.readFileSync(tarball);
    assert.equal(runCli(["--tarball", tarball, "--replacement", replacement, "--integrity", integrity, "--sbom", sbom]).status, 0);
    assert.ok(fs.readFileSync(tarball).equals(bytesAfterFirstPatch), "patch is idempotent and deterministic");
  });

  it("patch FAILS CLOSED on integrity mismatch and leaves the tarball untouched", () => {
    const { tarball, replacement, sbom } = setup();
    const before = fs.readFileSync(tarball);
    const bad = "sha512-" + "A".repeat(86);
    const result = runCli(["--tarball", tarball, "--replacement", replacement, "--integrity", bad, "--sbom", sbom]);
    assert.notEqual(result.status, 0);
    assert.match(result.out, /does NOT match/);
    assert.ok(fs.readFileSync(tarball).equals(before), "tarball bytes unchanged");
    assert.equal(runCli(["--verify-only", "--tarball", tarball]).status, 1, "still vulnerable");
  });
});
