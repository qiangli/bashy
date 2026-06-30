#!/usr/bin/env bash
# posix-parity-allshells.sh — run the posix-parity probes for MULTIPLE shells
# (bashy, gosh, dash, zsh) ALL INSIDE one bash:5.3 Linux container, compared to
# `bash 5.3 --posix` as the reference.
#
# Why: posix-parity.sh runs the CANDIDATE on the host (macOS) while the oracle
# runs in a Linux container — that asymmetry makes host/OS-specific probes (e.g.
# `kill -l`, whose signal set differs by OS) read as differences. Running every
# shell INSIDE the same Linux container removes that skew and gives a true
# apples-to-apples cross-shell comparison. See
# docs/shell-conformance-comparison.md for the recorded numbers + analysis.
#
# Usage: scripts/posix-parity-allshells.sh   (needs ../sh sibling + docker or bashy podman)
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)

OCI=${OCI:-}
if [ -z "$OCI" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif [ -n "${BASHY:-}" ]; then OCI="$BASHY podman"
  elif command -v bashy >/dev/null 2>&1; then OCI="bashy podman"
  else echo "error: need docker or bashy podman" >&2; exit 2; fi
fi

ARCH=$($OCI run --rm bash:5.3 uname -m | tr -d '\r')
case "$ARCH" in aarch64|arm64) GOARCH=arm64;; x86_64|amd64) GOARCH=amd64;; *) GOARCH=arm64;; esac

# $HOME is mounted into the podman/docker VM; /tmp generally is not.
WORK="$HOME/.cache/posix-parity-allshells"; rm -rf "$WORK"; mkdir -p "$WORK"

# Reuse posix-parity.sh's probe definitions verbatim (NUM<TAB>script per line).
{ echo 'add(){ printf "%s\t%s\t%s\n" "$1" "$2" ""; }'
  echo 'info(){ printf "%s\t%s\t%s\n" "$1" "$2" "$3"; }'
  grep -E '^(add|info) ' "$ROOT/scripts/posix-parity.sh"; } | bash > "$WORK/probes.tsv"
echo "probes: $(wc -l < "$WORK/probes.tsv")"

echo "building linux/$GOARCH candidates…" >&2
( cd "$ROOT"       && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -o "$WORK/bashy" ./cmd/bash )
( cd "$ROOT/../sh" && GOOS=linux GOARCH=$GOARCH CGO_ENABLED=0 go build -o "$WORK/gosh"  ./cmd/gosh )

cat > "$WORK/runner.sh" <<'RUNNER'
#!/usr/bin/env bash
# Clean PATH MUST include /usr/local/bin — the bash:5.3 image installs bash there.
P=/usr/local/bin:/usr/bin:/bin
norm() { sed -E 's#[^ ]*(bashy|bash|gosh):#SH:#g; s/line [0-9]+/line N/g'; }
declare -A R_OUT R_OK
while IFS=$'\t' read -r num script note; do
  out=$(env -i HOME=/tmp PATH=$P bash --posix -c "$script" 2>/dev/null); rc=$?
  R_OUT[$num]=$(printf '%s' "$out" | norm | tr '\n' '~'); R_OK[$num]=$([ $rc -eq 0 ] && echo ok || echo err)
done < /work/probes.tsv
cmp_shell() { local name=$1; shift; local match=0 diff=0 dl=""
  while IFS=$'\t' read -r num script note; do
    out=$("$@" "$script" 2>/dev/null); rc=$?
    o=$(printf '%s' "$out" | norm | tr '\n' '~'); k=$([ $rc -eq 0 ] && echo ok || echo err)
    if [ "$o" = "${R_OUT[$num]}" ] && [ "$k" = "${R_OK[$num]}" ]; then match=$((match+1)); else diff=$((diff+1)); dl="$dl $num"; fi
  done < /work/probes.tsv
  printf '%-22s %2d / %d match  (%d diff%s)\n' "$name" "$match" "$(wc -l < /work/probes.tsv)" "$diff" "$([ -n "$dl" ] && echo " — #$dl")"; }
echo "== posix-parity, all shells IN-CONTAINER ($(uname -m)), ref = bash 5.3 --posix =="
cmp_shell "bash 5.3 (self)"      env -i HOME=/tmp PATH=$P bash --posix -c
cmp_shell "bashy --posix"        env -i HOME=/tmp PATH=$P /work/bashy --posix -c
cmp_shell "gosh --posix"         env -i HOME=/tmp PATH=$P /work/gosh  --posix -c
cmp_shell "dash"                 env -i HOME=/tmp PATH=$P dash -c
cmp_shell "zsh (sh-emulation)"   env -i HOME=/tmp PATH=$P ARGV0=sh zsh -c
RUNNER

$OCI run --rm -v "$WORK:/work" bash:5.3 \
  sh -c 'apk add --no-cache --quiet zsh dash >/dev/null 2>&1 && bash /work/runner.sh'
