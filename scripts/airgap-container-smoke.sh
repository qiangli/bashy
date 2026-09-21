#!/bin/sh
# Sprint 227 — the airgap gate: what works inside the offline bashy image.
#
# Builds the image (`bashy dag build-image` → localhost/bashy:<ver>-linux-<arch>,
# or probes AIRGAP_IMAGE when set) and runs ONE probe per row of
# docs/airgap-image.md under --network=none --read-only --cap-drop=ALL:
#
#   shell      --version, -c, --posix, --bashsharp (examples/quickstart/*.bsh
#              against their .expected files), Stage 0, no network tools
#   builtins   every name `bashy commands` lists as a bash builtin: `type -t`
#   coreutils  every name it lists as coreutils: `<name> --version`/`--help`
#              in-process — works / present / bin-managed (offline by design)
#   verbs      every yoke verb it lists: `/bashy <verb> --help`; engines,
#              remote-by-design verbs and externals are offline BY DESIGN
#
# The row source is `bashy commands --all --json` from the image itself — never a
# hand-copied list. The doc's table is REGENERATED from this script's output
# (AIRGAP_WRITE_DOC=1); without it the script diffs its output against the doc,
# so the doc can never claim a row the gate did not just prove.
#
# Contract:
#   - No usable engine  -> SKIP, exit 0.
#   - Engine present    -> no FAIL row and the doc must match, or exit 1. The
#                          Linux CI job (airgap-image.yml, amd64 + arm64) is
#                          the release gate.
#
# Usage:
#   scripts/airgap-container-smoke.sh                        # build + probe + diff doc
#   AIRGAP_WRITE_DOC=1 scripts/airgap-container-smoke.sh     # regenerate the doc table
#   AIRGAP_IMAGE=localhost/bashy:0.25.0-linux-arm64 scripts/airgap-container-smoke.sh
#   BASHY_OCI="bashy podman" …                               # engine (default: bin/bashy podman)
set -eu
repo=$(CDPATH= cd -P "$(dirname "$0")/.." && pwd)
doc=$repo/docs/airgap-image.md
say()  { echo "airgap: $*"; }
fail() { echo "airgap: FAIL $*" >&2; exit 1; }

bashy=${BASHY:-$repo/bin/bashy}
[ -x "$bashy" ] || bashy=$(command -v bashy || true)
[ -n "$bashy" ] || fail "no bashy binary (build with make build, or set BASHY)"

# ── engine: no usable engine is a SKIP, not a failure ────────────────────────
oci=${BASHY_OCI:-"$bashy podman"}
if ! $oci info >/dev/null 2>&1; then
  say "SKIP engine '$oci' is not usable here (no machine/daemon) — the Linux CI job is the gate"
  exit 0
fi
say "engine=$oci"

# ── the image ────────────────────────────────────────────────────────────────
if [ -n "${AIRGAP_IMAGE:-}" ]; then
  image=$AIRGAP_IMAGE
else
  arch=${BASHY_IMAGE_ARCH:-$("$bashy" go env GOARCH 2>/dev/null || uname -m)}
  case "$arch" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; esac
  say "building the image for linux/$arch through the engine"
  image=$(cd "$repo" && BASHY_IMAGE_ARCH=$arch BASHY_OCI="$oci" "$bashy" dag build-image 2>&1 | grep '^localhost/bashy:' | tail -1)
  [ -n "$image" ] || fail "dag build-image printed no image tag"
fi
say "image=$image"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
out=$work/rows.tsv
: >"$out"
T=$(printf '\t')
bound=; command -v timeout >/dev/null 2>&1 && bound="timeout 900"

# run <args…>: the user contract, exactly — offline, read-only, no capabilities.
run() { $bound $oci run --rm --network=none --read-only --cap-drop=ALL --tmpfs /tmp -v "$work:/work:ro" -w /work "$image" "$@"; }
row() { printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "$4" >>"$out"; }   # section  name  status  note
first() { printf '%s' "$1" | head -1 | tr '\t' ' ' | tr -cd '\040-\176' | cut -c1-70; }
status=0

# ── shell rows ───────────────────────────────────────────────────────────────
probe() { # name cmd…  (expected: works)
  name=$1; shift
  if got=$(run "$@" 2>&1); then row shell "$name" works "$(first "$got")"
  else row shell "$name" FAIL "rc=$? $(first "$got")"; status=1; fi
}
probe '--version' --version
probe '-c (arithmetic, printf)' -c 'printf "%s\n" $((6*7))'
probe '--posix' --posix -c 'echo ${#HOME}'
for bsh in hello kwargs enums; do
  cp "$repo/examples/quickstart/$bsh.bsh" "$work/"
  want=$(cat "$repo/examples/quickstart/$bsh.expected")
  if got=$(run --bashsharp "./$bsh.bsh" 2>&1) && [ "$got" = "$want" ]; then
    row shell "--bashsharp examples/quickstart/$bsh.bsh" works "output matches $bsh.expected"
  else
    row shell "--bashsharp examples/quickstart/$bsh.bsh" FAIL "$(first "$got")"; status=1
  fi
done
# Stage 0: the home prefix is canonicalized on a non-tty sink — stated, not hidden.
got=$(run -c 'echo "$HOME"' 2>&1 || true)
if [ "$got" = '$HOME' ]; then row shell 'echo $HOME (non-tty stdout)' stated 'prints $HOME: Stage 0 output canonicalization (output_reduce.go); a tty passes through'
else row shell 'echo $HOME (non-tty stdout)' stated "prints $got"; fi
got=$(run -c 'curl -s http://example.com' 2>&1 || true)
case "$got" in
  *"curl: command not found"*) row shell 'network (curl)' absent 'no network tools in the image; --network=none besides' ;;
  *) row shell 'network (curl)' FAIL "$(first "$got")"; status=1 ;;
esac

# ── inventory rows: the image's own `bashy commands --all --json` ────────────
run commands --all --json >"$work/commands.json" 2>/dev/null || fail "bashy commands --all --json failed inside the image"
# pull one JSON string array out (names are plain words; no nested brackets)
names() { tr -d '\n' <"$work/commands.json" | sed -n "s/.*\"$1\":\[\([^]]*\)\].*/\1/p" | tr -d '"' | tr ',' '\n' | sed '/^$/d'; }
names builtins  >"$work/builtins.txt"
names verbs     >"$work/verbs.txt"
names coreutils | grep -vxF -f "$work/builtins.txt" >"$work/coreutils.txt"
[ -s "$work/coreutils.txt" ] || fail "no coreutils names parsed from the image's commands --json"

# verbs whose offline failure is by design: engines, remote-by-design verbs
# (philosophy.md §3), bin-managed externals, and the ones that talk to a host.
by_design='podman docker oci sandbox ollama otel dks kubectl helm aws azure gcloud doctl login tessaro sphere peer git gh act act-runner rclone loom zot seaweedfs kopia mirror go cmake clang node npm npx pnpm yarn python pip uv mise cargo rustc rustup rust git-scm curl fetch browser web ask chat delegate coach foreman supervise meet app upgrade bootstrap self release rg'

cat >"$work/probe.sh" <<'EOF'
# runs INSIDE the image. $1 = list file; verbs.txt is probed as `/bashy <name>`
# (the hidden front-door aliases have no bare shim), everything else bare.
# One line per name: name<TAB>class<TAB>rc<TAB>first-line. No `timeout`
# wrapper here: it would exec an external process, and the coreutils are
# in-process — the host bounds the whole run instead.
probe_one() {
  n=$1; mode=$2
  t=$(type -t "$n" 2>/dev/null || echo none)
  if [ "$t" = builtin ]; then printf '%s\tbuiltin\t0\t\n' "$n"; return; fi
  if [ "$mode" = front ]; then set -- /bashy "$n"; else set -- "$n"; fi
  o=$( { "$@" --version </dev/null; } 2>&1 ); rc=$?
  case "$o" in *"$n: command not found"*|*"unknown command \"$n\""*|*"unknown verb"*)
    printf '%s\tabsent\t%s\t%s\n' "$n" "$rc" "$(printf '%s' "$o" | head -1 | tr -cd '\040-\176' | cut -c1-70)"; return ;; esac
  if [ $rc -ne 0 ]; then
    o2=$( { "$@" --help </dev/null; } 2>&1 ); rc2=$?
    case "$o2" in *"$n: command not found"*|*"unknown command \"$n\""*|*"unknown verb"*)
      printf '%s\tabsent\t%s\t\n' "$n" "$rc2"; return ;; esac
    [ $rc2 -eq 0 ] && rc=0
    [ -n "$o2" ] && o=$o2
  fi
  # a bin-managed external answers offline with its provisioning error
  case "$o" in *"posix provider"*|*binmgr*|*"no system git"*|*"not pre-seeded"*)
    printf '%s\texternal\t%s\t%s\n' "$n" "$rc" "$(printf '%s' "$o" | head -1 | tr '\t' ' ' | tr -cd '\040-\176' | cut -c1-70)"; return ;; esac
  printf '%s\tpresent\t%s\t%s\n' "$n" "$rc" "$(printf '%s' "$o" | head -1 | tr '\t' ' ' | tr -cd '\040-\176' | cut -c1-70)"
}
mode=; [ "$1" = verbs.txt ] && mode=front
for n in $(cat "$1"); do probe_one "$n" "$mode"; done
EOF

for list in builtins coreutils verbs; do
  run -c '. ./probe.sh' probe "$list.txt" >"$work/$list.out" 2>/dev/null || true
  [ -s "$work/$list.out" ] || fail "probe produced no rows for $list"
  while IFS="$T" read -r n class rc line; do
    case " $by_design " in *" $n "*) design=1 ;; *) design=0 ;; esac
    case "$list:$class" in
      *:builtin)          row "$list" "$n" works "bash builtin" ;;
      builtins:*)         row builtins "$n" FAIL "type -t says $class"; status=1 ;;
      *:external)         row "$list" "$n" 'not usable offline' "bin-managed external: would download — $line" ;;
      *:present)
        if [ "$rc" -eq 0 ]; then row "$list" "$n" works "$line"
        elif [ $design = 1 ]; then row "$list" "$n" 'not usable offline' "engine / remote-by-design (by design): $line"
        else row "$list" "$n" present "runs in-process; --version/--help rc=$rc: $line"; fi ;;
      *:absent)
        if [ $design = 1 ]; then row "$list" "$n" absent "bin-managed external: not in the image, would download"
        else row "$list" "$n" FAIL "command not found in the image"; status=1; fi ;;
      *) row "$list" "$n" FAIL "unclassified probe line: $class"; status=1 ;;
    esac
  done <"$work/$list.out"
done

# ── the table (doc = one source) ─────────────────────────────────────────────
table=$work/table.md
{
  echo '| section | name | in the image | note |'
  echo '|---|---|---|---|'
  # notes are informational: arch / version / commit tokens are normalized so
  # the same table is measured on amd64 and arm64, dev and release builds
  sort -t "$T" -k1,1 -k2,2 "$out" | awk -F'\t' '{ gsub(/\|/, "\\|"); printf "| %s | `%s` | %s | %s |\n", $1, $2, $3, $4 }' |
    sed -E 's/(amd64|arm64|x86_64|aarch64)/<arch>/g; s/v?[0-9]+\.[0-9]+(\.[0-9]+)?(-[A-Za-z0-9.]+)?/<ver>/g; s/\([0-9a-f]{7,12}\)/(<sha>)/g; s/ dev( |$)/ <ver>\1/g; s/-dev([ )]|$)/-<ver>\1/g'
} >"$table"
count() { grep -c "${T}$1${T}" "$out" || true; }
say "rows=$(wc -l <"$out" | tr -d ' ') works=$(count works) present=$(count present) offline-by-design=$(( $(count 'not usable offline') + $(count absent) )) fail=$(count FAIL)"
grep "${T}FAIL${T}" "$out" | sed 's/^/airgap:   FAIL /' >&2 || true

begin='<!-- airgap-table:begin (generated by scripts/airgap-container-smoke.sh — do not edit) -->'
end='<!-- airgap-table:end -->'
if [ "${AIRGAP_WRITE_DOC:-0}" = 1 ]; then
  [ -f "$doc" ] || fail "$doc missing — write the prose first, the script only owns the table"
  awk -v b="$begin" -v e="$end" -v t="$table" '
    $0==b { print; while ((getline l < t) > 0) print l; skip=1; next }
    $0==e { skip=0 }
    !skip { print }' "$doc" >"$work/doc.new" && mv "$work/doc.new" "$doc"
  say "doc table regenerated: $doc"
else
  awk -v b="$begin" -v e="$end" '$0==b{f=1;next} $0==e{f=0} f' "$doc" >"$work/doc.table" 2>/dev/null || true
  if ! diff -u "$work/doc.table" "$table" >"$work/doc.diff"; then
    say "FAIL docs/airgap-image.md table differs from what the gate measured (AIRGAP_WRITE_DOC=1 to regenerate):" >&2
    head -40 "$work/doc.diff" >&2; status=1
  else
    say "doc table matches the measured rows"
  fi
fi
[ $status -eq 0 ] && say "PASS" || fail "see rows above"
