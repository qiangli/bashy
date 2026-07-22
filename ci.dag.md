---
name: ci
description: Shared CI graph for dhnt Go projects — include it, override what differs
vars:
  REPO ?=
  BINARY ?=
  VERSION ?=
  GO_TEST_FLAGS ?= -short
---

# ci — the shared build graph

Targets every dhnt Go project needs, defined once. A project's `dag.md` pulls
them in by pinned reference and overrides only what differs:

    ---
    include:
      - gh:qiangli/bashy@v0.19.0/ci.dag.md
    vars:
      REPO = qiangli/outpost
      BINARY = outpost
    ---

    ## Tasks

    ### test
    Project-specific: this repo's shell tests need a TTY.
    (fenced bash body here)

A local target of the same name **wins** — that is the extension point. Nothing
here is mandatory to adopt; include the file and override any target whose body
does not fit.

Two rules the bodies follow, because they run on QA hosts and CI runners as
well as laptops:

- **Pure bashy.** No `/tmp` (not guaranteed on Windows), no `grep -o`, no
  `sort -V` — bashy's pure-Go coreutils have none of them, and bashy *is* the
  target userland on a QA host. Bodies must behave identically on macOS, Linux
  and Windows.
- **Fail closed.** A missing checksum, an unset required var, or an
  unverifiable download is an error, never a skipped step. A green run that
  silently checked nothing is worse than a red one.

## Tasks

### tidy
Format, vet, and tidy the module. Identical across every Go project here.
Effects: write, net

```bash
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go mod tidy
"$BASHY_EXE" go fmt ./...
"$BASHY_EXE" go vet ./...
```

### test
Run the Go test suite. `GO_TEST_FLAGS` defaults to `-short`; override the whole
target when a repo must exclude packages (e.g. PTY-driven tests in a headless
run).
Effects: read, net

```bash
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go test ${GO_TEST_FLAGS} ./...
```

### build
Build `$BINARY` into ./bin for this host. A repo with sibling-path replaces in
go.mod should override this to bootstrap them first.
Requires: tidy-check
Effects: write, net

```bash
BASHY_EXE="${BASHY:-bashy}"
: "${BINARY:?set BINARY in your dag.md vars, e.g. BINARY = outpost}"
"$BASHY_EXE" mkdir -p bin
"$BASHY_EXE" go build -o "bin/${BINARY}" ./cmd/"${BINARY}"
echo ">> built bin/${BINARY}"
```

### tidy-check
Cheap gate that the module is consistent before a build. Separate from `tidy`
so a build does not rewrite files as a side effect.
Effects: read

```bash
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" go vet ./... >/dev/null
```

### qa
Verify a **published** release build — no source build, no Go toolchain, only
bashy. Downloads the asset for THIS host's OS/arch, verifies its checksum, and
runs the smoke defined by `qa-smoke`.

This is the target the standing QA poller runs; on pass it authors
`refs/qa/<version>/<os>`, which is the gate `promote.yml` reads. Keeping it here
means every project's release gate has the same shape and the same failure
modes.

`VERSION` is the tag under test (e.g. `v1.2.3-dev`). Assets are NAMED with the
base version even when downloaded from a `-dev` tag, because promotion moves
those exact bytes to the bare tag.
Requires: qa-smoke
Effects: read, net

```bash
echo ">> QA PASS ${VERSION} — $(cat .qa/.qa-platform 2>/dev/null)"
```

### qa-fetch
Download and checksum-verify the published asset into cwd-local `.qa/`.
Overridable for a repo whose assets are archives rather than raw binaries.
Effects: write, net

```bash
set -e
: "${REPO:?set REPO in your dag.md vars, e.g. REPO = qiangli/outpost}"
: "${BINARY:?set BINARY in your dag.md vars, e.g. BINARY = outpost}"
: "${VERSION:?set VERSION to the tag under test, e.g. VERSION=v1.2.3-dev}"
BASEV="${VERSION%%-*}"
os=$(bashy uname -s | tr 'A-Z' 'a-z'); case "$os" in *darwin*) os=darwin;; *linux*) os=linux;; *) os=windows;; esac
arch=$(bashy uname -m); case "$arch" in arm64|aarch64) arch=arm64;; x86_64|amd64) arch=amd64;; esac
ext=""; [ "$os" = windows ] && ext=.exe
base="https://github.com/${REPO}/releases/download/${VERSION}"
asset="${BINARY}-${BASEV}-${os}-${arch}${ext}"

d=".qa"; bashy mkdir -p "$d"        # cwd-local: /tmp is not guaranteed on Windows
echo ">> QA ${VERSION} on ${os}/${arch} — ${asset}"
bashy curl -fsSL -o "$d/${asset}" "${base}/${asset}"

# The .sha256 sidecar is "<sha>  <filename>"; extract with awk (no grep -o).
# Fail closed: an absent or empty checksum is a hard failure. Never smoke
# unverified bytes — this gate is what a fleet rollout trusts.
if bashy curl -fsSL -o "$d/out.sha256" "${base}/${BINARY}-${BASEV}-${os}-${arch}.sha256" 2>/dev/null; then
  want=$(awk '{print $1}' "$d/out.sha256" | head -1)
  got=$(bashy sha256sum "$d/${asset}" | awk '{print $1}' | head -1)
  { [ -n "$want" ] && [ "$want" = "$got" ]; } || { echo "FAIL sha256 (want=$want got=$got)"; exit 1; }
  echo ">> sha256 verified"
else
  echo "FAIL no checksum published for ${asset}"; exit 1
fi
chmod +x "$d/${asset}" 2>/dev/null || true
echo "${os}/${arch}" > "$d/.qa-platform"
echo "$d/${asset}" > "$d/.qa-binary"
```

### qa-smoke
Minimal proof the published bytes are safe to ship: the binary executes and
self-reports the version under test. A project that ships something other than
a self-versioning CLI should override this.

Deliberately minimal — this gate runs before a fleet rollout, so it answers
"will these exact bytes brick a host on swap and re-exec?", not "is the feature
correct". Deeper coverage belongs in `test`.
Requires: qa-fetch
Effects: read

```bash
set -e
BIN=$(cat .qa/.qa-binary)
BASEV="${VERSION%%-*}"
out=$("$BIN" --version 2>&1 | head -1)
echo "   $out"
case "$out" in
  *"${BASEV#v}"*) ;;
  *) echo "FAIL version probe: expected ${BASEV}, got: $out"; exit 1 ;;
esac
```

### release
Cut a release with GoReleaser. Fires from CI on a `vX.Y.Z-dev` tag; a bare tag
is a byte-promotion of already-tested assets and must NOT rebuild.
Effects: write, net

```bash
BASHY_EXE="${BASHY:-bashy}"
"$BASHY_EXE" goreleaser release --clean
```

### clean
Remove build and QA artifacts.
Effects: write

```bash
bashy rm -rf bin .qa
```
