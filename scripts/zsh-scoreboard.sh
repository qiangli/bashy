#!/usr/bin/env bash
# zsh scoreboard — Tier 0 of the zsh-compatibility ladder (see the umbrella
# doc bashy-zsh-compatibility-estimate.md): score bashy on zsh's OWN
# regression suite (Test/*.ztst at the zsh-5.9 tag — the version macOS ships
# and the version the engine's LangZsh tracks).
#
# Both arms — bashy AND real zsh — run through the same Go ztst runner
# (tools/ztst), so runner approximations cancel out. Real zsh defines
# validity: a case real zsh fails under this runner is runner/environment
# noise and is EXCLUDED from the denominator. Headline metric:
#
#   bashy passes N of the M cases real zsh passes  (per class + total)
#
# Scope: classes A B C D E W Z (non-interactive). Excluded: P (needs root),
# V (zmodload modules), X (zle), Y (completion) — the interactive surface,
# out of Tier-0 scope by design.
#
# INFO metric, not a gate. Never quote a bare "N% zsh compatible" from this —
# the claim is always "N% of zsh's own suite, non-interactive scope".
#
# Clean-room: clones zsh at runtime into a gitignored cache (.zsh-tests/),
# never vendors it — same posture as the yash suite.
set -u
cd "$(dirname "$0")/.."
ROOT=$PWD
OUT=${1:-/tmp/zsh-scoreboard}
mkdir -p "$OUT"

CLASSES=${ZSH_CLASSES:-"A B C D E W Z"}
ZSH_TAG=${ZSH_TAG:-zsh-5.9}
export ZSH_TAG

ZT="$ROOT/.zsh-tests" # gitignored clone cache
if [ ! -d "$ZT/Test" ]; then
  echo "cloning zsh @ $ZSH_TAG…" >&2
  rm -rf "$ZT"
  git clone --depth 1 --branch "$ZSH_TAG" https://github.com/zsh-users/zsh.git "$ZT" >&2 ||
    { echo "clone failed" >&2; exit 2; }
fi

ZSH_BIN=${ZSH_BIN:-$(command -v zsh || true)}
[ -n "$ZSH_BIN" ] || { echo "need a real zsh for the reference arm" >&2; exit 2; }
echo "reference: $ZSH_BIN ($("$ZSH_BIN" --version))" >&2

echo "building bin/bash…" >&2
go build -o bin/bash ./cmd/bash || exit 2

# shellcheck disable=SC2086
run_arm() { # $1=label $2=shell-cmd
  echo "running $1 arm…" >&2
  go run ./tools/ztst -shell "$2" -zsh "$ZSH_BIN" -testdir "$ZT/Test" $CLASSES \
    > "$OUT/$1.tsv" 2> "$OUT/$1.log" || { echo "$1 arm runner failed" >&2; exit 2; }
}
run_arm zsh "$ZSH_BIN -f"
run_arm bashy "$ROOT/bin/bash"

# tally: denominator = cases the reference arm passes (valid cases)
awk -F'\t' '
  FNR==1 && NR!=1 { arm="bashy" } NR==FNR { arm="zsh" }
  { k=$1"|"$2; v[arm"|"k]=$4; msg[k]=$5; file[k]=$1; ln[k]=$3 }
  END{
    for (ak in v) if (substr(ak,1,4)=="zsh|") {
      k=substr(ak,5)
      if (v[ak]!="OK") { noise++; continue }
      valid++; cls=substr(file[k],1,1); vcls[cls]++
      if (v["bashy|"k]=="OK") { pass++; pcls[cls]++ }
      else printf "%s:%s [%s] %s\n", file[k], ln[k], v["bashy|"k], msg[k] | "sort"
    }
    close("sort")
    printf "=== zsh-own-suite scoreboard (tag %s, classes: non-interactive) ===\n", ENVIRON["ZSH_TAG"] > "/dev/stderr"
    n=split("A B C D E W Z", cl, " ")
    for (i=1; i<=n; i++) { c=cl[i]
      if (vcls[c]) printf "  %s: %d/%d (%d%%)\n", c, pcls[c], vcls[c], pcls[c]*100/vcls[c] > "/dev/stderr" }
    printf "bashy: %d pass / %d valid = %d%%  (runner-noise excluded: %d)\n",
      pass, valid, (valid?pass*100/valid:0), noise > "/dev/stderr"
  }
' "$OUT/zsh.tsv" "$OUT/bashy.tsv" > "$OUT/failures.txt"

echo "--- failures by file (top) ---" >&2
awk -F: '{c[$1]++} END{for(f in c) print c[f], f}' "$OUT/failures.txt" | sort -rn | head -15 >&2
echo "full list: $OUT/failures.txt ; per-case verdicts: $OUT/{zsh,bashy}.tsv" >&2
