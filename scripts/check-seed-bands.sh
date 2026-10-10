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
# Seed lint (plan §E): every non-retired agent seed is L3+ and binds a
# curated (built-in, non-retired, non-hidden) tool and model. The rule lives
# in yoke as a Go test so there is one implementation; BASHY_SEED_LINT=0
# skips it (this script's own fixture test has no Go module).
if [ "${BASHY_SEED_LINT:-1}" != 0 ]; then
  if ! (cd "$yoke_dir" && go test ./pkg/fleet -run '^TestSeedLint$' -count=1 >&2); then
    echo "release seed check failed: seed lint (go test ./pkg/fleet -run TestSeedLint in $yoke_dir)" >&2
    exit 1
  fi
fi
stamp=$yoke_dir/pkg/fleet/baseline/seed-bands.txt
[ -f "$stamp" ] || { echo "seed bands stamp not found: $stamp" >&2; exit 1; }
seed_date=$(awk -F: '/^[[:space:]]*date[[:space:]]*:/ { sub(/^[[:space:]]*/, "", $2); sub(/[[:space:]]*$/, "", $2); print $2; exit }' "$stamp")
case $seed_date in [0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]) ;; *) echo "seed bands stamp has missing or invalid date" >&2; exit 1;; esac
if ! awk -v d="$seed_date" 'BEGIN { split(d,a,"-"); y=a[1]+0; m=a[2]+0; day=a[3]+0; leap=(y%4==0 && (y%100!=0 || y%400==0)); max=(m==2 ? 28+leap : (m==4 || m==6 || m==9 || m==11 ? 30 : 31)); exit !(y>0 && m>=1 && m<=12 && day>=1 && day<=max) }'; then
  echo "seed bands stamp has missing or invalid date" >&2
  exit 1
fi
stamp_models=$(awk -F: '/^[[:space:]]*models[[:space:]]*:/ { gsub(/[[:space:]]/, "", $2); print $2; exit }' "$stamp")
case $stamp_models in ''|*[!0-9]*) echo "seed bands stamp has invalid models count" >&2; exit 1;; esac

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

count=$(find "$models" -type f -name '*.yaml' -exec awk '/^band_source:[[:space:]]*seeded([[:space:]]|$)/ { found=1 } END { if (found) print FILENAME }' {} \; | wc -l | tr -d ' ')
if [ "$count" -eq 0 ]; then
  echo "release seed check failed: no seeded models found" >&2
  exit 1
fi
if [ "$count" -ne "$stamp_models" ]; then
  echo "warning: stamp says $stamp_models models but found $count seeded model(s)" >&2
fi
newest=$seed_date

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
    if [ "$major_release" -eq 1 ]; then kind="major release"; else kind="release with --reseed"; fi
    echo "$kind needs regenerated seed bands (stamp date $newest older than $previous of $previous_date) — run yoke-seedfit (docs/research/fleet-seed-bands-*.md) and re-pin yoke" >&2
    exit 1
  fi
  if [ "$major_release" -eq 1 ]; then echo "seed regenerated on major release (newest seed $newest)"
  else echo "seed regenerated on owner instruction (newest seed $newest)"; fi
else
  echo "seed bands kept from $newest"
fi
