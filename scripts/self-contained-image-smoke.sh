#!/bin/sh
# Sprint 227 — "Bashy is all you need": the self-contained gate.
#
# From a PUBLISHED bashy release archive, on a host whose PATH has no git, go,
# cc, podman or docker, with an EMPTY BASHY_BIN_CACHE, run the two lines:
#
#   bashy self image
#   bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh
#
# and print the provision inventory — every file bashy fetched to make that
# work, with digests. That inventory IS docs/airgap-image.md §"What the host
# needs". Nothing is built here, nothing comes from a checkout: the release
# tag's archive is downloaded (that one download is the user's; everything
# after it is bashy's own doing).
#
# Usage:
#   scripts/self-contained-image-smoke.sh v0.25.0-dev        # the candidate tag
#   SELF_KEEP=1 … keeps the work dir; SELF_ARCHIVE=/path/to/bashy-<os>-<arch>.tar.gz uses local bytes
#   SELF_PATH=/some/dir:/other overrides the scrubbed PATH (default: a mirror of
#   the system dirs minus git/go/cc/podman/docker/…)
#
# Exit 0 = PASS (both lines ran, the script's output matched). Prints
# `self-contained: PASS` / `FAIL`. Never SKIPs: a missing engine is the failure
# this gate exists to catch.
set -eu
tag=${1:-${SELF_TAG:-}}
[ -n "$tag" ] || { echo "usage: $0 vX.Y.Z[-dev]" >&2; exit 2; }
repo=${SELF_REPO:-qiangli/bashy}
say()  { echo "self-contained: $*"; }
fail() { echo "self-contained: FAIL $*" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]'); arch=$(uname -m)
case "$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
case "$os" in darwin|linux) ;; *) fail "run this on linux or macOS (Windows: see docs/airgap-image.md)";; esac

work=${SELF_WORK:-$(mktemp -d)}
[ "${SELF_KEEP:-0}" = 1 ] || trap 'rm -rf "$work"' EXIT
cache=$work/cache; mkdir -p "$cache" "$work/bin" "$work/proj"
say "host=$os/$arch tag=$tag work=$work"

# ── PATH without the tools the claim says you do not need ────────────────────
if [ -n "${SELF_PATH:-}" ]; then
  scrub=$SELF_PATH
else
  scrub=$work/scrub; mkdir -p "$scrub"
  for d in /usr/local/sbin /usr/local/bin /usr/sbin /usr/bin /sbin /bin; do
    [ -d "$d" ] || continue
    for f in "$d"/*; do
      b=$(basename "$f")
      case "$b" in git|go|gofmt|cc|gcc|clang|clang-*|podman|podman-remote|docker|buildah|skopeo|nerdctl|conmon|crun|runc|newuidmap|newgidmap) continue ;; esac
      # newuidmap/newgidmap are the ONE stated linux host fact — keep them
      [ -e "$scrub/$b" ] || ln -s "$f" "$scrub/$b"
    done
  done
  case "$os" in linux) for h in newuidmap newgidmap; do [ -x /usr/bin/$h ] && ln -sf /usr/bin/$h "$scrub/$h"; done ;; esac
  scrub=$scrub
fi
for t in git go cc podman docker; do
  if p=$(PATH=$scrub command -v $t 2>/dev/null); then fail "scrubbed PATH still has $t ($p)"; fi
done
say "PATH scrubbed (no git/go/cc/podman/docker)"

# ── the one download that is the user's: the release archive ─────────────────
if [ -n "${SELF_ARCHIVE:-}" ]; then
  archive=$SELF_ARCHIVE
else
  asset=bashy-$os-$arch.tar.gz
  url=https://github.com/$repo/releases/download/$tag/$asset
  archive=$work/$asset
  say "downloading $url"
  curl -fsSL -o "$archive" "$url" || fail "download failed: $url"
  curl -fsSL -o "$work/checksums.txt" "https://github.com/$repo/releases/download/$tag/checksums.txt" || fail "no checksums.txt on $tag"
  want=$(grep " $asset\$" "$work/checksums.txt" | awk '{print $1}')
  got=$(shasum -a 256 "$archive" 2>/dev/null | awk '{print $1}' || sha256sum "$archive" | awk '{print $1}')
  [ -n "$want" ] && [ "$want" = "$got" ] || fail "checksum mismatch for $asset (want $want got $got)"
  say "archive sha256 verified: $got"
fi
tar -xzf "$archive" -C "$work/bin"
bashy=$work/bin/bashy
[ -x "$bashy" ] || fail "no bashy in the archive"
E="env -i HOME=$HOME PATH=$scrub BASHY_BIN_CACHE=$cache BASHY_TELEMETRY_QUIET=1 OTEL_TRACES_EXPORTER=none"
say "bashy: $($E "$bashy" --version 2>/dev/null | head -1)"

# ── macOS / Windows: the engine runs in bashy's own podman machine ───────────
if [ "$os" = darwin ]; then
  if ! $E "$bashy" podman machine list --format '{{.Name}}' 2>/dev/null | grep -q .; then
    t0=$(date +%s)
    say "podman machine init (first time: downloads the machine OS image — podman's own fetch)"
    $E "$bashy" podman machine init 2>&1 | grep -vE '^\s*$' | tail -3 | sed 's/^/self-contained:   /' || fail "podman machine init"
    say "machine init took $(( $(date +%s) - t0 )) s"
  fi
  if ! $E "$bashy" podman info >/dev/null 2>&1; then
    t0=$(date +%s)
    $E "$bashy" podman machine start 2>&1 | tail -2 | sed 's/^/self-contained:   /' || fail "podman machine start"
    say "machine start took $(( $(date +%s) - t0 )) s"
  fi
fi

# ── line 1: bashy self image ─────────────────────────────────────────────────
t0=$(date +%s)
image=$($E "$bashy" self image --version "$tag" 2>"$work/self-image.err" | tail -1) || { cat "$work/self-image.err" >&2; fail "bashy self image"; }
say "line 1: bashy self image → $image ($(( $(date +%s) - t0 )) s)"
grep -E 'fetching|binmgr' "$work/self-image.err" | sed 's/^/self-contained:   /' || true

# ── line 2: run a Bash# script offline ───────────────────────────────────────
cat >"$work/proj/script.bsh" <<'EOF'
func greet(name string) string {
    return "hello, " + name + "!"
}
printf '%s\n' greet("air-gap")
printf 'x\ny\nz\n' | sort -r | head -1
EOF
got=$(cd "$work/proj" && $E "$bashy" podman run --rm --network=none -v "$PWD:/work" -w /work "$image" --bashsharp ./script.bsh 2>&1) || fail "line 2 failed: $got"
[ "$got" = "hello, air-gap!
z" ] || fail "unexpected output: $got"
say "line 2: bashy podman run --network=none → $(printf '%s' "$got" | tr '\n' ' ')"

# ── the provision inventory: what bashy fetched, with digests ────────────────
say "provision inventory (BASHY_BIN_CACHE=$cache):"
find "$cache" -type f \( -perm -u+x -o -name '*.conf' -o -name '*.json' \) 2>/dev/null | sort | while read -r f; do
  d=$(shasum -a 256 "$f" 2>/dev/null | cut -c1-16 || sha256sum "$f" | cut -c1-16)
  printf 'self-contained:   %-60s %s  %s\n' "${f#$cache/}" "$(du -k "$f" | cut -f1)K" "$d"
done
say "cache total: $(du -sk "$cache" | cut -f1)K"
case "$os" in
  darwin) say "machine: $($E "$bashy" podman machine list --format '{{.Name}} {{.VMType}} {{.DiskSize}}' 2>/dev/null | head -2 | tr '\n' ';')" ;;
  linux)  say "rootless: $($E "$bashy" podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null) newuidmap=$(PATH=$scrub command -v newuidmap || echo none)" ;;
esac
say "PASS"
