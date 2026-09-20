#!/bin/sh
# Sprint 216, Story 540 — the AUTHORITATIVE three-mode FROM-scratch gate.
#
# Builds and RUNS three real `FROM scratch` images from
# examples/quickstart/Containerfile:
#
#   (a) mode-a  bashy binary + .bsh source, interpreted
#   (b) mode-b  transpile --standalone native binary, no bashy at runtime
#   (c) mode-c  Python island, interpreter prepared before the final stage
#
# Every image is RUN with --network=none, so a run that reaches for a download
# fails the gate — that is the point for (c).
#
# Contract:
#   - No container engine  -> SKIP, exit 0 (local dev without podman/docker).
#   - Engine present        -> every build/run MUST pass, or exit 1.
#     A local pass here is NOT authoritative on its own; the required Linux CI
#     job (.github/workflows/quickstart-scratch.yml) is the release gate and is
#     where the published sizes and SBOM line come from.
#
# It also records, for each image: uncompressed and gzip-compressed size, plus
# one `go version -m` SBOM line for the standalone binary.
#
# Usage:
#   scripts/quickstart-container-smoke.sh
#   BASHY_OCI=podman scripts/quickstart-container-smoke.sh   # force an engine
#   QUICKSTART_KEEP=1 scripts/quickstart-container-smoke.sh  # keep the images for inspection
set -eu

repo=$(CDPATH= cd -P "$(dirname "$0")/.." && pwd)
parent=$(dirname "$repo")
cf=bashy/examples/quickstart/Containerfile
tag_prefix=${QUICKSTART_TAG_PREFIX:-localhost/bashy-quickstart}

say()  { echo "quickstart-container: $*"; }
fail() { echo "quickstart-container: FAIL $*" >&2; exit 1; }

# ── engine selection: no engine is a SKIP, not a failure ─────────────────────
if [ -n "${BASHY_OCI:-}" ]; then
  oci=$BASHY_OCI
elif command -v podman >/dev/null 2>&1; then
  oci=podman
elif command -v docker >/dev/null 2>&1; then
  oci=docker
else
  say "SKIP no podman/docker engine — local dev may skip; the Linux CI job is the gate"
  exit 0
fi
# An installed-but-unusable engine (no running machine/daemon) is also a SKIP.
if ! "$oci" info >/dev/null 2>&1; then
  say "SKIP $oci is installed but not usable (no running machine/daemon) — the Linux CI job is the gate"
  exit 0
fi
say "engine=$oci"

# ── stage a clean context: bashy + the open-source sibling checkouts ─────────
stage_tree() {
  src=$1; dest=$2
  [ -d "$src" ] || fail "required sibling checkout is missing: $src"
  git -C "$src" rev-parse --is-inside-work-tree >/dev/null 2>&1 ||
    fail "required sibling is not a git checkout: $src"
  mkdir -p "$dest"
  (cd "$src" && git ls-files --cached --others --exclude-standard -z |
    tar --null -T - -cf -) | (cd "$dest" && tar -xf -)
}

context=$(mktemp -d "${TMPDIR:-/tmp}/quickstart-scratch.XXXXXX")
cleanup() {
  rm -rf "$context"
  [ -n "${QUICKSTART_KEEP:-}" ] && return 0
  for t in mode-a mode-b mode-c standalone-builder; do
    "$oci" rmi -f "$tag_prefix-$t" >/dev/null 2>&1 || true
  done
}
trap cleanup EXIT HUP INT TERM

say "staging build context under $context"
stage_tree "$repo"               "$context/bashy"
stage_tree "$parent/coreutils"   "$context/coreutils"
stage_tree "$parent/sh"          "$context/sh"
stage_tree "$parent/readline"    "$context/readline"
stage_tree "$parent/filebrowser" "$context/filebrowser"
stage_tree "$parent/bashsharp"   "$context/bashsharp"
stage_tree "$parent/yoke"        "$context/yoke"

build_target() {
  target=$1; tag=$2
  say "build $target -> $tag"
  "$oci" build --target "$target" -f "$context/$cf" -t "$tag" "$context" \
    || fail "build $target"
}

# Run a scratch image with NO network and a read-only root; --tmpfs gives the
# island a writable /tmp. stdout is the result; stderr goes to a file so a
# warning can never masquerade as (or hide) the greeting.
run_image() {
  tag=$1
  "$oci" run --rm --pull=never --network=none --read-only \
    --tmpfs /tmp:rw,nosuid,nodev,size=32m,mode=1777 \
    "$tag" 2>"$context/stderr"
}

report_size() {
  label=$1; tag=$2
  unc=$("$oci" image inspect --format '{{.Size}}' "$tag" 2>/dev/null || echo 0)
  comp=$("$oci" save "$tag" 2>/dev/null | gzip -9 | wc -c | tr -d ' ')
  say "SIZE $label image uncompressed=${unc} bytes compressed(gzip -9)=${comp} bytes"
}

# The run must exit 0 AND print exactly the greeting: a non-zero exit is not
# masked by a pipe, and extra output is a failure, not "close enough".
expect_hello() {
  label=$1; tag=$2
  if ! got=$(run_image "$tag"); then
    say "stderr:"; cat "$context/stderr" >&2
    fail "$label exited non-zero (stdout: '$got')"
  fi
  if [ "$got" != "hello, world!" ]; then
    say "stderr:"; cat "$context/stderr" >&2
    fail "$label ran but printed '$got' (want 'hello, world!')"
  fi
  say "PASS $label — ran FROM scratch, --network=none, --read-only, printed 'hello, world!'"
}

# ── (a) bashy + .bsh ─────────────────────────────────────────────────────────
build_target mode-a "$tag_prefix-mode-a"
expect_hello "(a) bashy + .bsh" "$tag_prefix-mode-a"
report_size  "(a)" "$tag_prefix-mode-a"

# ── (b) transpile --standalone ───────────────────────────────────────────────
build_target mode-b "$tag_prefix-mode-b"
expect_hello "(b) transpile --standalone" "$tag_prefix-mode-b"
report_size  "(b)" "$tag_prefix-mode-b"

# Standalone binary size + one go version -m SBOM line, read from the builder
# stage where `go` and the binary both exist.
build_target build-standalone "$tag_prefix-standalone-builder"
"$oci" run --rm --network=none --entrypoint /bin/sh "$tag_prefix-standalone-builder" -c '
  set -eu
  unc=$(wc -c < /out/hello | tr -d " ")
  comp=$(gzip -9 -c /out/hello | wc -c | tr -d " ")
  printf "quickstart-container: SIZE (b) standalone-binary uncompressed=%s bytes compressed(gzip -9)=%s bytes\n" "$unc" "$comp"
  line=$(go version -m /out/hello | grep -E "dep[[:space:]]+mvdan.cc/sh" | head -1 | tr -s "[:space:]" " ")
  printf "quickstart-container: SBOM (b) %s\n" "${line# }"
' || fail "(b) size/SBOM probe"

# ── (c) Python island ────────────────────────────────────────────────────────
build_target mode-c "$tag_prefix-mode-c"
expect_hello "(c) Python island (prepared, no runtime download)" "$tag_prefix-mode-c"
report_size  "(c)" "$tag_prefix-mode-c"

say "OK all three FROM-scratch images built and ran"
