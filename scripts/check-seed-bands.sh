#!/bin/sh
set -eu

usage() { echo "usage: $0 TAG [--yoke DIR] [--reseed]" >&2; exit 2; }
[ "$#" -gt 0 ] || usage
tag=$1
shift
yoke_dir=
reseed=${BASHY_RESEED:-0}
while [ "$#" -gt 0 ]; do
  case $1 in
    --yoke) [ "$#" -ge 2 ] || usage; yoke_dir=$2; shift 2 ;;
    --reseed) reseed=1; shift ;;
    *) usage ;;
  esac
done
[ -n "$yoke_dir" ] || yoke_dir=../yoke
models=$yoke_dir/pkg/fleet/baseline/models
[ -d "$models" ] || { echo "seed models directory not found: $models" >&2; exit 1; }

major_release=0
base_tag=${tag%%-*}
base_version=${base_tag#v}
major=${base_version%%.*}
minor_patch=${base_version#*.}
minor=${minor_patch%%.*}
patch=${minor_patch#*.}
case $base_tag in v*.*.*) ;; *) echo "invalid release tag: $tag" >&2; exit 2;; esac
case $major:$minor:$patch in *[!0-9:]*|::*|:*:|*::|*.*) echo "invalid release tag: $tag" >&2; exit 2;; esac
[ "$minor" = 0 ] && [ "$patch" = 0 ] && major_release=1

dates_file=${TMPDIR:-/tmp}/check-seed-bands.$$.dates
trap 'rm -f "$dates_file"' 0 HUP INT TERM
: > "$dates_file"
find "$models" -type f -name '*.yaml' -print | while IFS= read -r file; do
  awk '
    /^band_source:[[:space:]]*seeded([[:space:]]|$)/ { seeded=1 }
    seeded && /seeded[[:space:]][0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]/ {
      line=$0; sub(/^.*seeded[[:space:]]+/, "", line); sub(/[^0-9-].*$/, "", line); if (line > best) best=line
    }
    END { if (seeded) { if (best == "") { print "seeded model has no seeded YYYY-MM-DD date: " FILENAME > "/dev/stderr"; exit 1 }; print best } }
  ' "$file" >> "$dates_file" || exit 1
done
count=$(wc -l < "$dates_file" | tr -d ' ')
newest=$(sort "$dates_file" | tail -n 1)
if [ "$count" -eq 0 ]; then
  echo "major release needs regenerated seed bands (no seeded models)" >&2
  exit 1
fi

previous=
previous_date=
if [ "$major_release" -eq 1 ] || [ "$reseed" = 1 ]; then
  for candidate in $(git tag -l 'v*.0.0' --sort=version:refname); do
    candidate_major=${candidate#v}; candidate_major=${candidate_major%.0.0}
    [ "$candidate" = "$base_tag" ] && continue
    if [ "$candidate_major" -lt "$major" ] || { [ "$major_release" -eq 0 ] && [ "$candidate_major" -eq "$major" ]; }; then previous=$candidate; fi
  done
  if [ -n "$previous" ]; then previous_date=$(git for-each-ref --format='%(creatordate:short)' "refs/tags/$previous" 2>/dev/null || true); fi
fi

fresh=1
if [ -n "$previous_date" ]; then
  earliest=$(printf '%s\n%s\n' "$newest" "$previous_date" | LC_ALL=C sort | head -n 1)
  if [ "$earliest" = "$newest" ] && [ "$newest" != "$previous_date" ]; then fresh=0; fi
fi
if [ "$major_release" -eq 1 ] || [ "$reseed" = 1 ]; then
  if [ "$fresh" -ne 1 ]; then
    echo "major release needs regenerated seed bands (newest seed $newest older than $previous of $previous_date) — run yoke-seedfit (docs/research/fleet-seed-bands-*.md) and re-pin yoke" >&2
    exit 1
  fi
  if [ "$major_release" -eq 1 ]; then echo "seed regenerated on major release (newest seed $newest)"
  else echo "seed regenerated on owner instruction (newest seed $newest)"; fi
else
  echo "seed bands kept from $newest"
fi
