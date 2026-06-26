#!/usr/bin/env bash
# verify-builtins.sh — conformance check for the builtin-vs-external command
# table. For a curated set of commands (every POSIX special + regular built-in,
# the bash 5.3 extensions, and a sample of real externals), it compares
# `type -t <cmd>` between bin/bash (the pure drop-in) and real bash 5.3 in the
# same container PATH, and reports any disagreement.
#
# The classification a shell gives a name (builtin / file / keyword / alias /
# function) IS shell behavior — POSIX even gives "special built-ins" distinct
# semantics (a redirection/assignment error is fatal; assignments persist). So a
# faithful bash drop-in must agree with bash on which names are built in.
#
# Expected: every name agrees EXCEPT the two additive fork builtins (nohup,
# setsid), which bin/bash implements as builtins (so `nohup foo &` survives a
# closed SSH session) where stock bash leaves them external. That delta is
# intentional and additive — documented in docs/builtin-vs-external-conformance.md.
#
# Usage: scripts/verify-builtins.sh   (needs bin/bash built + an OCI runtime)
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
BIN="$HERE/bin/bash"
[ -x "$BIN" ] || { echo "build bin/bash first (make build)" >&2; exit 2; }
OCI=${OCI:-$(command -v podman || command -v docker || echo "ycode podman")}
IMAGE=${IMAGE:-bash:5.3}

# Cross-compile a linux ours for the container.
OURS=$(mktemp -d)/ours
( cd "$HERE" && GOOS=linux GOARCH="${GOARCH:-arm64}" go build -o "$OURS" ./cmd/bash ) || exit 2

CMDS=": . break continue eval exec exit export readonly return set shift times trap unset \
cd getopts read pwd umask wait kill jobs fg bg alias unalias command false true type hash fc \
ulimit declare local printf echo test [ let mapfile shopt source enable bind caller disown \
compgen complete compopt history help suspend dirs popd pushd readarray typeset logout \
nohup setsid \
ls grep sed cat awk env which sort head tail"

TC=$(mktemp); printf 'for c in %s; do printf "%%s\\t" "$c"; type -t "$c" 2>/dev/null || echo "(none)"; done\n' "$CMDS" > "$TC"

$OCI run --rm -i -v "$OURS:/ours:ro" -v "$TC:/tc.sh:ro" "$IMAGE" sh -c '
  bash /tc.sh > /tmp/b.txt 2>&1
  /ours /tc.sh > /tmp/o.txt 2>&1
  n=$(wc -l < /tmp/b.txt)
  echo "=== type -t disagreements (bin/bash vs bash 5.3, same PATH) ==="
  d=$(paste /tmp/b.txt /tmp/o.txt | awk -F"\t" "{ if (\$2!=\$4) print \"  \" \$1 \": bash=\" \$2 \" / ours=\" \$4 }")
  printf "%s\n" "${d:-  (none)}"
  c=$(printf "%s\n" "$d" | grep -c .)
  echo "=== $n names checked; $c disagreement(s) (expected: 2 — nohup, setsid additive fork builtins) ==="
'
