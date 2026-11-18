#!/bin/sh
# The previous release, in the locked image, builds/packages the next one.
# DIST_SOURCE_ROOT contains immutable bashy/bash/outpost/yoke checkouts and
# dist-inputs.json. This command never creates tags or calls GitHub.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_root=${DIST_SOURCE_ROOT:?set DIST_SOURCE_ROOT to the prepared immutable source root}
[ -f "$source_root/dist-inputs.json" ] || { echo 'dist-inputs.json missing' >&2; exit 2; }
if [ "${BASHY_BUILD_RUN:-}" = 1 ]; then
 case ${DIST_STAGE:-all} in
  all) ;;
  build) set -- --build-only "$@" ;;
  package) set -- --package-only "$@" ;;
  *) echo "DIST_STAGE must be all, build, or package" >&2; exit 2 ;;
 esac
 /usr/bin/python3 "$root/scripts/dist.py" --inputs "$source_root/dist-inputs.json" "$@"
else
 [ "${DIST_STAGE:-all}" = all ] || { echo "staged signing handoff must run inside the locked image" >&2; exit 2; }
 [ "$#" = 0 ] || { echo "run staged build/package options inside the locked image" >&2; exit 2; }
 export BASHY_BUILD_CPUS=8
 "${BASHY:-bashy}" "$root/tools/bashy-build/run.sh" "$source_root" -- 'cd /work/bashy && DIST_SOURCE_ROOT=/work /usr/bin/dash scripts/dist.sh'
fi
