# The official build image

Every build and test of bashy, yoke, outpost and cloudbox runs in one image,
whether it runs on the dev box, the gate host or GitHub. When everything runs
in the same image, a failure is either in the code (the same everywhere) or in
the setup or platform (only one place differs). Builds never depend on what a
host shell happens to have installed.

- `Containerfile.base`: `bashy-base`, the bashy shell and Go userland on a
  pinned Ubuntu runtime, packaging a bashy that was compiled inside the
  previous build image.
- `Containerfile`: the build image on top of that base: Go from the official
  tarball (checksum-locked), `build-essential`, `make`, `git` and the cgo
  libraries the build lanes need, all from the pinned apt snapshot.
- `image.lock`: the image's identity (bashy version and commits, Go version
  and checksums, apt snapshot, base and image digests, and the image it was
  built with).
- `run.sh SRC_DIR -- COMMAND`: the single entry point. It runs the command
  through `bashy oci` in the locked image, with Go caches in a named volume on
  the runner, so nothing is downloaded twice.
- `build.sh`: builds the chain (on a gate runner, never the dev box).

## Rolling chain

The previous release builds the next one. `build.sh roll vN SOURCES`
compiles vN's bashy inside the locked vN-1 image (one static file, the release
shape), packages it as `bashy-base:vN`, builds `bashy-build:vN` on top, smoke
tests it and prints the lock values to commit. It refuses to roll unless the
lock names an image digest, so no version is skipped and nothing is built with
an unrelated toolchain.

If vN-1 cannot build vN, only the bugs that block that build are fixed, in
vN's sources, never in the old release; other bugs follow the normal process.
Then vN is rolled again inside its own image. That self-built vN is the real
result and is what the lock names for vN's gates.

## Self-hosting is a hard release gate

A release that cannot build itself is not released: a user who downloads vN
must be able to rebuild vN with it.

1. vN-1 builds vN (`roll vN`, lock at vN-1).
2. vN builds vN (`roll vN` again, lock at the vN from step 1). This is the gate.
3. If step 2 fails, even when vN does not produce a working bashy at all,
   vN-1 stays the builder: fix the build-blocking bug in vN's sources, let
   vN-1 build the patched vN, then let the patched vN try to build vN.
   Repeat until vN builds itself. vN-1 is never modified; only vN changes.
   Use a candidate lock (`BASHY_BUILD_LOCK=...`) for these iterations and
   commit the lock only for the final self-built vN.
4. Fixed point: the vN built by vN builds vN once more, and the two
   vN-built binaries must be byte-identical (`bashy_sha256` equal). A
   difference is a reproducibility bug and blocks the release.

The chain starts at the released v1.0.0-rc.35 binary, packaged as it shipped
with `build.sh seed v1.0.0-rc.35 BINARY` (its SHA-256 is in the lock). Earlier
releases are not rebuilt.

## Rules

- If bashy itself fails inside this image, that is a P0: fix bashy first.
- A failure off-image (a bare host shell, a different runner setup) is a venue
  issue: fix the venue, not the code (docs in the umbrella's
  `docs/release-failure-ledger.md`).
- Gate scripts refuse to run without `BASHY_BUILD_IMAGE` (set by the image),
  except with an explicit, logged override for diagnosis.
