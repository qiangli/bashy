#!/usr/bin/env node
// write-npm-lock.mts — generate installed/package/package-lock.json with
// upstream's own generator.
//
// Run from the upstream source checkout (src/):
//   node --import ./scripts/tsx.mjs ../fork/bashy-build/scripts/write-npm-lock.mts \
//     --package-dir <extracted packed tree> [--out <lock>] [--tarball <tgz>]
//
// WHY THE PACKED TREE (not src/): the packager rewrites the manifest before
// packing (@openclaw/ai workspace:* -> exact staged version plus hoisted
// aligned deps, workspace: devDeps stripped, platform optionals added), and
// generateNpmPackageLock() DROPS workspace: specs from the manifest it locks.
// A lock generated for src/ therefore omits @openclaw/ai while the SHIPPED
// (packed) manifest pins it, so npm ci fails closed on the mismatch. The
// packed manifest's deps are all plain registry specs, so generation runs
// against the extracted tree while the generator's pnpm cross-check still
// anchors every resolved version to src/pnpm-lock.yaml.
//
// WHY THE vendor-openclaw-ai FILE LINK: @openclaw/ai@<V> appears nowhere in
// pnpm-lock.yaml (workspace link: only), so the generator's pnpm validation
// throws for the plain version spec. The packed tarball already bundles the
// ai runtime — built from the pinned source in this same run — at
// node_modules/@openclaw/ai. This script copies it to vendor-openclaw-ai/ and
// points the packed manifest at file:./vendor-openclaw-ai. The generator
// records a link entry for the file: dep (link entries skip its pnpm check,
// as for any link) while still validating the subtree's transitive deps.
// The shipped tree keeps the file: spec plus the vendor dir, so manifest,
// lock and tree agree and npm ci stays fail-closed.

import { execFileSync } from "node:child_process";
import {
  cpSync,
  existsSync,
  mkdirSync,
  readFileSync,
  realpathSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

export const AI_PACKAGE = "@openclaw/ai";
export const VENDOR_DIR = "vendor-openclaw-ai";
const GENERATOR_RELATIVE = path.join("scripts", "generate-npm-package-lock.mts");

export interface WriteLockArgs {
  packageDir: string;
  out: string | undefined;
  tarball: string | undefined;
}

export function usage(): string {
  return [
    "Usage: write-npm-lock.mts --package-dir <extracted packed tree> [--out <lock>] [--tarball <tgz>]",
    "  Run from the upstream source checkout (src/). Writes package-lock.json",
    "  (default: <package-dir>/package-lock.json) with upstream's generator.",
    "  --tarball names the packager tarball; it is only listed on failure,",
    "  so the next log shows what was packed.",
  ].join("\n");
}

export function parseArgs(argv: string[]): WriteLockArgs {
  const args: WriteLockArgs = { packageDir: "", out: undefined, tarball: undefined };
  for (let index = 0; index < argv.length; index += 1) {
    const flag = argv[index];
    if (flag === "--help" || flag === "-h") {
      throw new Error("__USAGE__");
    }
    if (flag === "--package-dir" || flag === "--out" || flag === "--tarball") {
      const value = argv[index + 1];
      if (value === undefined || value.startsWith("--")) {
        throw new Error(`${flag} needs a value`);
      }
      if (flag === "--package-dir") {
        args.packageDir = value;
      } else if (flag === "--out") {
        args.out = value;
      } else {
        args.tarball = value;
      }
      index += 1;
      continue;
    }
    throw new Error(`unknown argument: ${flag}`);
  }
  if (!args.packageDir) {
    throw new Error("--package-dir is required");
  }
  return args;
}

type JsonRecord = Record<string, unknown>;

function isRecord(value: unknown): value is JsonRecord {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function readManifest(packageDir: string): JsonRecord {
  const manifestPath = path.join(packageDir, "package.json");
  return JSON.parse(readFileSync(manifestPath, "utf8")) as JsonRecord;
}

const DEP_FIELDS = ["dependencies", "optionalDependencies", "peerDependencies"] as const;

export function dependencyKeys(manifest: JsonRecord): Record<string, string[]> {
  const keys: Record<string, string[]> = {};
  for (const field of [...DEP_FIELDS, "bundleDependencies", "bundledDependencies"]) {
    const value = manifest[field];
    if (Array.isArray(value)) {
      keys[field] = value.map((entry) => String(entry));
    } else {
      keys[field] = isRecord(value) ? Object.keys(value) : [];
    }
  }
  return keys;
}

export function summarizeManifest(manifest: JsonRecord): string {
  const name = typeof manifest["name"] === "string" ? manifest["name"] : "<missing>";
  const version = typeof manifest["version"] === "string" ? manifest["version"] : "<missing>";
  const lines = [`package: ${name}@${version}`];
  const keys = dependencyKeys(manifest);
  for (const field of Object.keys(keys)) {
    const list = keys[field] ?? [];
    lines.push(`${field} (${list.length}): ${list.join(", ") || "<none>"}`);
  }
  const aiSpec = isRecord(manifest["dependencies"])
    ? manifest["dependencies"][AI_PACKAGE]
    : undefined;
  lines.push(`${AI_PACKAGE} spec: ${typeof aiSpec === "string" ? aiSpec : "<absent>"}`);
  return lines.join("\n");
}

export function topLevelEntries(names: string[]): string[] {
  const top = new Set<string>();
  for (const name of names) {
    const first = name.split("/")[0];
    if (first) {
      top.add(first);
    }
  }
  return [...top].sort();
}

export function tarTopLevelEntries(tarball: string): string[] | undefined {
  try {
    const output = execFileSync("tar", ["-tzf", tarball], {
      encoding: "utf8",
      maxBuffer: 64 * 1024 * 1024,
    }) as string;
    return topLevelEntries(output.split("\n").filter((line) => line.length > 0));
  } catch {
    return undefined;
  }
}

export interface FailureContext {
  packageDir: string;
  tarball: string | undefined;
  manifest: JsonRecord | undefined;
  vendorPresent: boolean;
}

export function renderFailureDiagnostics(error: unknown, ctx: FailureContext): string {
  const message = error instanceof Error ? error.message : String(error);
  const lines = ["write-npm-lock: FAILED: " + message, `package dir: ${ctx.packageDir}`];
  if (ctx.manifest !== undefined) {
    lines.push(summarizeManifest(ctx.manifest));
  } else {
    lines.push("package.json: unreadable");
  }
  lines.push(`vendor dir (${VENDOR_DIR}): ${ctx.vendorPresent ? "present" : "absent"}`);
  if (ctx.tarball !== undefined) {
    const entries = tarTopLevelEntries(ctx.tarball);
    lines.push(
      entries === undefined
        ? `tarball ${ctx.tarball}: could not list entries`
        : `tarball top-level entries (${entries.length}): ${entries.join(", ")}`,
    );
  }
  return lines.join("\n");
}

function specOf(manifest: JsonRecord, name: string): string | undefined {
  const deps = manifest["dependencies"];
  if (!isRecord(deps)) {
    return undefined;
  }
  const spec = deps[name];
  return typeof spec === "string" ? spec : undefined;
}

function normalizeFileSpec(spec: string): string {
  return spec.startsWith("file:./") ? `file:${spec.slice("file:./".length)}` : spec;
}

async function main(): Promise<void> {
  let args: WriteLockArgs;
  try {
    args = parseArgs(process.argv.slice(2));
  } catch (error) {
    if (error instanceof Error && error.message === "__USAGE__") {
      process.stdout.write(`${usage()}\n`);
      return;
    }
    throw error;
  }
  const repoRoot = process.cwd();
  const generatorPath = path.join(repoRoot, GENERATOR_RELATIVE);
  const pnpmLockPath = path.join(repoRoot, "pnpm-lock.yaml");
  if (!existsSync(generatorPath) || !existsSync(pnpmLockPath)) {
    throw new Error(
      `must run from the upstream source checkout (src/): missing ${GENERATOR_RELATIVE} or pnpm-lock.yaml`,
    );
  }
  const packageDir = path.resolve(args.packageDir);
  const manifestPath = path.join(packageDir, "package.json");
  if (!existsSync(manifestPath)) {
    throw new Error(`no package.json in ${packageDir}`);
  }
  const originalText = readFileSync(manifestPath, "utf8");
  let manifest = JSON.parse(originalText) as JsonRecord;
  const outPath = args.out !== undefined ? path.resolve(args.out) : path.join(packageDir, "package-lock.json");
  const vendorDir = path.join(packageDir, VENDOR_DIR);
  let manifestRewritten = false;

  const aiSpec = specOf(manifest, AI_PACKAGE);
  if (aiSpec !== undefined && aiSpec.startsWith("workspace:")) {
    throw new Error(
      renderFailureDiagnostics(
        `refusing a source-style manifest: ${AI_PACKAGE} is ${aiSpec}; pass the extracted packager tarball dir, not the source checkout`,
        { packageDir, tarball: args.tarball, manifest, vendorPresent: existsSync(vendorDir) },
      ),
    );
  }
  if (aiSpec !== undefined && !aiSpec.startsWith("file:")) {
    const bundledManifestPath = path.join(packageDir, "node_modules", AI_PACKAGE, "package.json");
    if (!existsSync(bundledManifestPath)) {
      throw new Error(
        renderFailureDiagnostics(
          `packed ${AI_PACKAGE}@${aiSpec} has no bundled copy at node_modules/${AI_PACKAGE}; cannot vendor it for the lock`,
          { packageDir, tarball: args.tarball, manifest, vendorPresent: existsSync(vendorDir) },
        ),
      );
    }
    const bundled = JSON.parse(readFileSync(bundledManifestPath, "utf8")) as JsonRecord;
    if (bundled["version"] !== aiSpec) {
      throw new Error(
        renderFailureDiagnostics(
          `bundled ${AI_PACKAGE} version ${String(bundled["version"])} does not match packed spec ${aiSpec}; refusing to guess`,
          { packageDir, tarball: args.tarball, manifest, vendorPresent: existsSync(vendorDir) },
        ),
      );
    }
    rmSync(vendorDir, { recursive: true, force: true });
    mkdirSync(path.dirname(vendorDir), { recursive: true });
    cpSync(path.dirname(bundledManifestPath), vendorDir, { recursive: true });
    manifest = JSON.parse(originalText) as JsonRecord;
    (manifest["dependencies"] as JsonRecord)[AI_PACKAGE] = `file:./${VENDOR_DIR}`;
    writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
    manifestRewritten = true;
  }

  const generatorUrl = pathToFileURL(generatorPath).href;
  const generator = (await import(generatorUrl)) as {
    generateNpmPackageLock?: (packageDir: string) => string;
  };
  if (typeof generator.generateNpmPackageLock !== "function") {
    throw new Error(`no generateNpmPackageLock export in ${GENERATOR_RELATIVE}`);
  }
  let lockText: string;
  try {
    lockText = generator.generateNpmPackageLock(packageDir);
  } catch (error) {
    if (manifestRewritten) {
      writeFileSync(manifestPath, originalText);
    }
    throw new Error(
      renderFailureDiagnostics(error, {
        packageDir,
        tarball: args.tarball,
        manifest,
        vendorPresent: existsSync(vendorDir),
      }),
    );
  }
  const lock = JSON.parse(lockText) as JsonRecord;
  const lockRoot = isRecord(lock["packages"]) ? lock["packages"][""] : undefined;
  const lockRootDeps = isRecord(lockRoot) && isRecord(lockRoot["dependencies"])
    ? (lockRoot["dependencies"] as JsonRecord)
    : undefined;
  const manifestAi = specOf(manifest, AI_PACKAGE);
  const lockAi = lockRootDeps !== undefined ? lockRootDeps[AI_PACKAGE] : undefined;
  const aiMatches =
    (manifestAi === undefined && lockAi === undefined) ||
    (typeof manifestAi === "string" &&
      typeof lockAi === "string" &&
      (lockAi === manifestAi ||
        (manifestAi.startsWith("file:") &&
          lockAi.startsWith("file:") &&
          normalizeFileSpec(lockAi) === normalizeFileSpec(manifestAi))));
  if (!isRecord(lockRoot) || !aiMatches) {
    if (manifestRewritten) {
      writeFileSync(manifestPath, originalText);
    }
    throw new Error(
      renderFailureDiagnostics(`generated lock root does not match the packed ${AI_PACKAGE} spec`, {
        packageDir,
        tarball: args.tarball,
        manifest,
        vendorPresent: existsSync(vendorDir),
      }),
    );
  }
  if (!lockText.endsWith("\n")) {
    lockText += "\n";
  }
  writeFileSync(outPath, lockText);
  const rootDepCount =
    lockRootDeps === undefined
      ? 0
      : Object.keys(lockRootDeps).length;
  process.stdout.write(
    [
      `write-npm-lock: wrote ${outPath} (${Buffer.byteLength(lockText)} bytes)`,
      `write-npm-lock: root deps ${rootDepCount}; ${AI_PACKAGE} via ${manifestAi ?? "<absent>"}`,
    ].join("\n") + "\n",
  );
}

let invokedAsMain = false;
try {
  invokedAsMain =
    process.argv[1] !== undefined &&
    realpathSync(process.argv[1]) === realpathSync(fileURLToPath(import.meta.url));
} catch {
  invokedAsMain = false;
}
if (invokedAsMain) {
  await main().catch((error: unknown) => {
    if (error instanceof Error && error.message.startsWith("write-npm-lock: FAILED")) {
      process.stderr.write(`${error.message}\n`);
    } else {
      const message = error instanceof Error ? error.message : String(error);
      process.stderr.write(`write-npm-lock: FAILED: ${message}\n`);
    }
    if (error instanceof Error && error.message === "__USAGE__") {
      process.stderr.write(`${usage()}\n`);
    } else if (
      error instanceof Error &&
      (error.message.includes("--package-dir") || error.message.includes("unknown argument"))
    ) {
      process.stderr.write(`${usage()}\n`);
    }
    process.exit(1);
  });
}
