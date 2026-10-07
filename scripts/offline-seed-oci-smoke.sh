#!/usr/bin/env bash
# Linux seeded-OCI offline acceptance. Run as root on a disposable host or CI
# runner: it creates a private network namespace and touches only the caches
# and directories named below.
#
#   offline-seed-oci-smoke.sh <dir with the four candidate members> <work dir>
#
# 1. Online: provision the pinned managed podman into a fresh cache and export
#    it as a seed. Host podman is hidden (no /usr/bin or /bin on PATH) so the
#    managed engine, not a distribution package, is what the seed carries.
# 2. Offline (unshare --net): a cold cache must refuse with the missing tool
#    and version; after `self install --seed`, `bashy oci version` must run.
set -euo pipefail
src=$(cd "$1" && pwd)
work=$(mkdir -p "$2" && cd "$2" && pwd)
seed="$work/bashy-seed.tar"
clean_env=(env -i PATH=/usr/local/sbin:/usr/sbin:/sbin HOME=/root LANG=C.UTF-8)

rm -rf "$work/seed-src-cache" "$work/seed-dst-cache" "$work/seed-bin" "$seed"
"${clean_env[@]}" BASHY_BIN_CACHE="$work/seed-src-cache" "$src/bashy" oci version > "$work/oci-online.log" 2>&1 ||
	{ cat "$work/oci-online.log" >&2; exit 1; }
"${clean_env[@]}" BASHY_BIN_CACHE="$work/seed-src-cache" "$src/bashy" self seed export "$seed"

unshare --net "${clean_env[@]}" BASHY_OFFLINE=1 BASHY_BIN_CACHE="$work/seed-dst-cache" \
	/bin/bash -euo pipefail -c '
src=$1 work=$2 seed=$3
ip link set lo up
if BASHY_OFFLINE=0 "$src/bashy" fetch --timeout 5s https://github.com > /dev/null 2>&1; then
	echo "FAIL: network namespace allowed an external fetch" >&2; exit 1
fi
if "$src/bashy" oci version > "$work/oci-cold.log" 2>&1; then
	echo "FAIL: cold offline cache ran podman" >&2; exit 1
fi
# Host /usr/bin is off PATH here, so only bash builtins inspect the logs.
cold=$(< "$work/oci-cold.log")
[[ $cold =~ podman\ v[0-9][^\ ]*\ is\ not\ cached\;\ BASHY_OFFLINE=1 ]] ||
	{ echo "FAIL: cold refusal does not name the tool and version: $cold" >&2; exit 1; }
"$src/bashy" self install --seed "$seed" --dir "$work/seed-bin"
"$work/seed-bin/bashy" oci version > "$work/oci-seeded.log" 2>&1 ||
	{ echo "$(< "$work/oci-seeded.log")" >&2; exit 1; }
' -- "$src" "$work" "$seed"
echo "PASS: seeded managed podman ran offline; cold cache refused with tool and version"
