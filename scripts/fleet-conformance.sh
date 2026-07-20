#!/usr/bin/env bash
# Distributed GNU Bash 5.3 conformance across a container fleet — CONFIG-DRIVEN.
#
# No host details live in this script. The fleet is described in a config file
# (--config PATH, or $FLEET_CONFIG, or ./fleet-conformance.conf, or
# $XDG_CONFIG_HOME/bashy/fleet-conformance.conf). Each non-comment line is a
# space-separated key=val record describing one host:
#
#   name=<label> transport=<local|ssh|winssh> [target=<ssh target>] \
#     workdir=<path on that host> arch=<arm64|amd64> \
#     [bashy=<bashy path on that host>] [weight=<int>] [excl=<c1,c2,...>]
#
# transport:  local  = this machine (uses ./bin/bashy)
#             ssh    = unix remote over ssh (scp ships the testee)
#             winssh = Windows remote over ssh (testee shipped over the LAN via
#                      HTTP + curl.exe, since tunnels mangle large binaries;
#                      run inline — never a remote script, which breaks podman)
# weight:     omit to auto-probe the host's podman VM (min(cpus, mem/CHUNK_MEM_GB)).
# excl:       chunks this host must NOT run (e.g. a WSL2 host that diverges on a
#             few fixtures). The chunk COUNT itself is a corpus property pinned in
#             chunks.json and is NEVER derived from fleet capacity.
#
# See fleet-conformance.conf.example. The authoritative gate stays single-host +
# native (`make test-bash-parallel` = 86/86); this is the speed lane.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/.." && pwd)"
BASHY="$REPO/bin/bashy"
CONFIG=""; DRYRUN=0; ONLY=""; SKIP=""
: "${CHUNK_MEM_GB:=4}" "${MAX_PER_HOST:=4}" "${HTTP_PORT:=8099}" "${EXPECT:=86}" "${CONNECT_TIMEOUT:=8}"
LANIP="${LANIP:-}"
# fail-fast ssh so a host that's off the current network (office/home/library) is
# skipped in seconds, not hung on. No BatchMode — some transports need auth/elevation.
SSH=(ssh -o "ConnectTimeout=$CONNECT_TIMEOUT" -o ServerAliveInterval=5 -o ServerAliveCountMax=2)

usage(){ sed -n '2,40p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }
while [ $# -gt 0 ]; do case "$1" in
  --config) CONFIG="$2"; shift 2;;
  --config=*) CONFIG="${1#*=}"; shift;;
  --only) ONLY="$2"; shift 2;;            # run only these hosts (space/comma list)
  --skip) SKIP="$2"; shift 2;;            # skip these hosts (e.g. reserved for other work)
  --dry-run) DRYRUN=1; shift;;
  --expect) EXPECT="$2"; shift 2;;
  -h|--help) usage 0;;
  *) echo "unknown flag: $1" >&2; usage 2;;
esac; done
ONLY="${ONLY//,/ }"; SKIP="${SKIP//,/ }"
in_list(){ local n="$1" l="$2" x; for x in $l; do [ "$x" = "$n" ] && return 0; done; return 1; }

if [ -z "$CONFIG" ]; then
  for c in "${FLEET_CONFIG:-}" "./fleet-conformance.conf" \
           "${XDG_CONFIG_HOME:-$HOME/.config}/bashy/fleet-conformance.conf"; do
    [ -n "$c" ] && [ -f "$c" ] && { CONFIG="$c"; break; }
  done
fi
[ -n "$CONFIG" ] && [ -f "$CONFIG" ] || { echo "no fleet config (use --config PATH; see fleet-conformance.conf.example)" >&2; exit 2; }
log(){ printf '>> %s\n' "$*" >&2; }
log "config: $CONFIG"
cd "$REPO"

# --- parse config into per-host associative fields -----------------------------
declare -a HOSTS AVAIL
declare -A H_TRANSPORT H_TARGET H_WORKDIR H_ARCH H_BASHY H_WEIGHT H_EXCL H_LOAD H_ENABLED
while IFS= read -r line; do
  line="${line%%#*}"; [ -n "${line// /}" ] || continue
  name=""; declare -A kv=()
  for tok in $line; do case "$tok" in *=*) kv["${tok%%=*}"]="${tok#*=}";; esac; done
  name="${kv[name]:-}"; [ -n "$name" ] || { echo "config line missing name=: $line" >&2; exit 2; }
  HOSTS+=("$name")
  H_TRANSPORT[$name]="${kv[transport]:-local}"
  H_TARGET[$name]="${kv[target]:-}"
  H_WORKDIR[$name]="${kv[workdir]:-.}"
  H_ARCH[$name]="${kv[arch]:-arm64}"
  H_BASHY[$name]="${kv[bashy]:-bashy}"
  H_WEIGHT[$name]="${kv[weight]:-}"
  H_EXCL[$name]="${kv[excl]:-}"
  H_ENABLED[$name]="${kv[enabled]:-1}"
  H_LOAD[$name]=0
done < "$CONFIG"
[ "${#HOSTS[@]}" -gt 0 ] || { echo "config has no hosts" >&2; exit 2; }

# remote runner helpers (transport-aware) --------------------------------------
on_host(){ # $1=host $2=shell-snippet ; runs snippet on that host, returns output
  local h="$1" snip="$2"
  case "${H_TRANSPORT[$h]}" in
    local)  ( cd "$REPO" && eval "$snip" );;
    ssh|winssh) "${SSH[@]}" "${H_TARGET[$h]}" "$snip";;  # caller keeps winssh inline
  esac
}
host_bashy(){ [ "${H_TRANSPORT[$1]}" = local ] && echo "$BASHY" || echo "${H_BASHY[$1]}"; }

# --- discover which hosts are actually up NOW (dynamic fleet) -------------------
# The config lists POTENTIAL hosts; at office/home/library a different subset is
# reachable, and some may be reserved for other work. Probe each, skip the ones
# that are disabled / excluded / unreachable, and run only on what's up — the
# assignment redistributes over the survivors.
log "probing fleet (config lists ${#HOSTS[@]}; using whatever is up now)…"
for h in "${HOSTS[@]}"; do
  if [ "${H_ENABLED[$h]}" = 0 ]; then log "  $h: disabled (enabled=0) — skip"; continue; fi
  if [ -n "$ONLY" ] && ! in_list "$h" "$ONLY"; then log "  $h: not in --only — skip"; continue; fi
  if [ -n "$SKIP" ] && in_list "$h" "$SKIP"; then log "  $h: --skip (reserved) — skip"; continue; fi
  b="$(host_bashy "$h")"
  info="$(on_host "$h" "$b podman info --format '{{.Host.CPUs}} {{.Host.MemTotal}}'" 2>/dev/null | tr -d '\r\000' | tail -1 || true)"
  cpus="$(printf '%s' "$info" | awk '{print $1}')"; memb="$(printf '%s' "$info" | awk '{print $2}')"
  if [ -z "$cpus" ]; then log "  $h: unreachable or podman not up — skip"; continue; fi
  if [ -z "${H_WEIGHT[$h]}" ]; then
    memgb=$(( memb/1024/1024/1024 )); w=$cpus
    memcap=$(( memgb/CHUNK_MEM_GB )); [ "$memcap" -lt "$w" ] && w=$memcap
    [ "$w" -gt "$MAX_PER_HOST" ] && w=$MAX_PER_HOST; [ "$w" -lt 1 ] && w=1
    H_WEIGHT[$h]="$w"
    log "  $h: up — ${cpus} cpu, ${memgb} GiB → weight $w${H_EXCL[$h]:+  (excl ${H_EXCL[$h]})}"
  else
    log "  $h: up — weight ${H_WEIGHT[$h]} (config)${H_EXCL[$h]:+  (excl ${H_EXCL[$h]})}"
  fi
  AVAIL+=("$h")
done
[ "${#AVAIL[@]}" -gt 0 ] || { echo "no hosts available right now" >&2; exit 2; }
log "available this run: ${AVAIL[*]}"

# --- weighted least-load assignment of the pinned chunks -----------------------
CHUNKS="$("$BASHY" go run ./tools/bash53suite -chunk-count 2>/dev/null || echo 8)"
: "${CHUNK_ORDER:=}"; [ -n "$CHUNK_ORDER" ] || CHUNK_ORDER="$(seq 1 "$CHUNKS")"
declare -A ASSIGN
for ch in $CHUNK_ORDER; do
  best=""; bestscore=""
  for h in "${AVAIL[@]}"; do
    case ",${H_EXCL[$h]}," in *",$ch,"*) continue;; esac
    score=$(( (H_LOAD[$h]+1)*1000 / H_WEIGHT[$h] ))
    if [ -z "$bestscore" ] || [ "$score" -lt "$bestscore" ]; then best="$h"; bestscore="$score"; fi
  done
  [ -n "$best" ] || { echo "no eligible host for chunk $ch" >&2; exit 2; }
  H_LOAD[$best]=$(( H_LOAD[$best]+1 )); ASSIGN[$best]="${ASSIGN[$best]:+${ASSIGN[$best]},}$ch"
done
log "assignment (resource-weighted):"
for h in "${AVAIL[@]}"; do [ -n "${ASSIGN[$h]:-}" ] && log "  $h ← ${ASSIGN[$h]}"; done
[ "$DRYRUN" = 1 ] && { log "dry-run: not executing"; exit 0; }

STAGE=/tmp/fleet-conf; OUT="$STAGE/out"; rm -rf "$OUT"; mkdir -p "$OUT" "$STAGE"

# --- build the testees the configured arches need ------------------------------
declare -A NEED_ARCH; for h in "${AVAIL[@]}"; do NEED_ARCH[${H_ARCH[$h]}]=1; done
for arch in "${!NEED_ARCH[@]}"; do
  log "building linux/$arch testee + harness…"
  mkdir -p "$STAGE/bin/bash-linux-$arch"
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -o "$STAGE/bin/bash-linux-$arch/bash" ./cmd/bash
  GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -o "$STAGE/bin/bash53suite-linux-$arch" ./tools/bash53suite
done
FX="$(readlink external/bash-5.3)"; rm -rf "$STAGE/fx"; cp -R "$FX" "$STAGE/fx"
rm -f "$STAGE/fx/tests/recho" "$STAGE/fx/tests/zecho" "$STAGE/fx/tests/xcase" "$STAGE/fx/tests/printenv"
tar -czf "$STAGE/fixtures.tgz" -C "$STAGE/fx" .

# LAN http server only if a winssh host exists
HTTPD=0
for h in "${AVAIL[@]}"; do [ "${H_TRANSPORT[$h]}" = winssh ] && HTTPD=1; done
if [ "$HTTPD" = 1 ]; then
  [ -n "$LANIP" ] || LANIP="$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')"
  pkill -f "http.server $HTTP_PORT" 2>/dev/null || true
  ( cd "$STAGE" && nohup python3 -m http.server "$HTTP_PORT" --bind 0.0.0.0 >/tmp/fleet-httpd.log 2>&1 & )
  sleep 1; log "serving testees at http://$LANIP:$HTTP_PORT (for winssh hosts)"
fi

# --- ship the fresh testee to each host ----------------------------------------
ship(){ # $1=host  — fleet vars expand locally; ~ / $USERPROFILE expand on the host
  local h="$1" a="${H_ARCH[$h]}" wd="${H_WORKDIR[$h]}" url="http://$LANIP:$HTTP_PORT" rc
  case "${H_TRANSPORT[$h]}" in
    local) install -m755 "$STAGE/bin/bash-linux-$a/bash" "$REPO/bin/bash-linux-$a/bash"
           install -m755 "$STAGE/bin/bash53suite-linux-$a" "$REPO/bin/bash53suite-linux-$a";;
    ssh)   "${SSH[@]}" "${H_TARGET[$h]}" "mkdir -p $wd/bin/bash-linux-$a"
           scp -o "ConnectTimeout=$CONNECT_TIMEOUT" -q "$STAGE/bin/bash-linux-$a/bash" "${H_TARGET[$h]}:$wd/bin/bash-linux-$a/bash"
           scp -o "ConnectTimeout=$CONNECT_TIMEOUT" -q "$STAGE/bin/bash53suite-linux-$a" "${H_TARGET[$h]}:$wd/bin/bash53suite-linux-$a";;
    winssh) rc="curl.exe -s -o \"$wd/bin/bash-linux-$a/bash\" $url/bin/bash-linux-$a/bash; curl.exe -s -o \"$wd/bin/bash53suite-linux-$a\" $url/bin/bash53suite-linux-$a"
            "${SSH[@]}" "${H_TARGET[$h]}" "$rc";;
  esac
}
log "shipping testees…"; for h in "${AVAIL[@]}"; do [ -n "${ASSIGN[$h]:-}" ] && ship "$h"; done

# --- run one chunk on a host ---------------------------------------------------
run_chunk(){ # $1=host $2=chunk -> writes $OUT/$host-$chunk.{result,log}
  local h="$1" ch="$2" a="${H_ARCH[$h]}" wd="${H_WORKDIR[$h]}" b r rc img="localhost/bash53-conformance:latest"
  b="$(host_bashy "$h")"
  case "${H_TRANSPORT[$h]}" in
    local)  local tr; tr="$(cd "$REPO/external/bash-5.3" && pwd -P)"
            r="$("$BASHY" podman run --rm --user 1000:1000 -v "$REPO:/work" -v "$tr:/bash53" -w /work -e CHUNK="$ch/$CHUNKS" -e BASH53_RUNNER="fleet-$h-$ch" "$img" ./bin/bash53suite-linux-$a -tests-dir /bash53/tests -bash ./bin/bash-linux-$a/bash 2>&1)";;
    ssh)    rc="cd $wd && tr=\$(cd external/bash-5.3 && pwd -P) && $b podman run --rm --user 1000:1000 -v \"\$PWD:/work\" -v \"\$tr:/bash53\" -w /work -e CHUNK=$ch/$CHUNKS -e BASH53_RUNNER=fleet-$h-$ch $img ./bin/bash53suite-linux-$a -tests-dir /bash53/tests -bash ./bin/bash-linux-$a/bash 2>&1"
            r="$("${SSH[@]}" "${H_TARGET[$h]}" "$rc")";;
    winssh) rc="cd \"$wd\" && \"$b\" podman run --rm --user 1000:1000 -v \"\$PWD:/work\" -w /work -e CHUNK=$ch/$CHUNKS -e BASH53_RUNNER=fleet-$h-$ch $img ./bin/bash53suite-linux-$a -tests-dir /work/external/bash-5.3/tests -bash ./bin/bash-linux-$a/bash 2>&1"
            r="$("${SSH[@]}" "${H_TARGET[$h]}" "$rc")";;
  esac
  printf '%s\n' "$r" | grep -i 'Results:' | tail -1 > "$OUT/$h-$ch.result" || true
  printf '%s' "$r" > "$OUT/$h-$ch.log"
}

# run an explicit chunk list on a host, bounded concurrency = weight
run_host_chunks(){ local h="$1" chunks="$2" ch running=0 maxc="${H_WEIGHT[$h]:-1}"; IFS=',' read -ra list <<<"$chunks"
  for ch in "${list[@]}"; do [ -n "$ch" ] || continue; run_chunk "$h" "$ch" &
    running=$((running+1)); [ "$running" -ge "$maxc" ] && { wait -n 2>/dev/null || wait; running=$((running-1)); }
  done; wait; }

# A RUN succeeded iff it emitted a `Results:` line. No line = an INFRASTRUCTURE
# failure (host dropped off the network, container/OOM/oci error, ssh timeout) —
# NOT a test failure (which is a real result and is kept). Only run failures retry.
valid_result(){ local ch="$1" f; for f in "$OUT"/*-"$ch".result; do [ -s "$f" ] && grep -qi 'Results:' "$f" && return 0; done; return 1; }
declare -A FAILHOST LASTHOST
# pick_host: an available host eligible for this chunk, preferring one OTHER than
# the host that just failed it (so a bad/dropped host doesn't get the retry too).
pick_host(){ local ch="$1" avoid="${FAILHOST[$ch]:-}" h fb=""
  for h in "${AVAIL[@]}"; do case ",${H_EXCL[$h]}," in *",$ch,"*) continue;; esac
    if [ "$h" = "$avoid" ]; then fb="${fb:-$h}"; else echo "$h"; return 0; fi; done
  [ -n "$fb" ] && { echo "$fb"; return 0; }; return 1; }   # only the failed host is eligible

: "${MAX_ROUNDS:=3}"
log "fanning out…"; START=$(date +%s)
# round 1: the resource-weighted assignment
for h in "${AVAIL[@]}"; do
  [ -n "${ASSIGN[$h]:-}" ] || continue
  IFS=',' read -ra _cs <<<"${ASSIGN[$h]}"; for _c in "${_cs[@]}"; do LASTHOST[$_c]="$h"; done
  run_host_chunks "$h" "${ASSIGN[$h]}" &
done
wait
# rounds 2..N: re-dispatch any chunk whose RUN failed onto an alternate host
round=2
while [ "$round" -le "$MAX_ROUNDS" ]; do
  declare -A RA=(); pend=0
  for ch in $CHUNK_ORDER; do
    valid_result "$ch" && continue
    FAILHOST[$ch]="${LASTHOST[$ch]:-}"
    alt="$(pick_host "$ch")" || { log "chunk $ch: run failed and no other eligible host is up — leaving it"; continue; }
    RA[$alt]="${RA[$alt]:+${RA[$alt]},}$ch"; LASTHOST[$ch]="$alt"; pend=$((pend+1))
  done
  [ "$pend" = 0 ] && break
  log "retry round $((round-1)): $pend run-failure(s) re-dispatched onto an alternate host"
  for h in "${AVAIL[@]}"; do [ -n "${RA[$h]:-}" ] && run_host_chunks "$h" "${RA[$h]}" & done
  wait; unset RA; round=$((round+1))
done
END=$(date +%s)
[ "$HTTPD" = 1 ] && pkill -f "http.server $HTTP_PORT" 2>/dev/null || true

# --- aggregate (one valid result per chunk; a chunk with no run is flagged) -----
P=0; F=0; N=0; runfail=0
for ch in $CHUNK_ORDER; do
  line=""; who=""
  for f in "$OUT"/*-"$ch".result; do
    [ -s "$f" ] && grep -qi 'Results:' "$f" && { line="$(cat "$f")"; who="$(basename "$f" .result)"; break; }
  done
  if [ -z "$line" ]; then log "chunk $ch: RUN FAILED on all attempts (infrastructure, not a test failure)"; runfail=$((runfail+1)); continue; fi
  p="$(echo "$line" | awk '{print $2}')"; f="$(echo "$line" | awk '{print $4}')"
  P=$((P+${p:-0})); F=$((F+${f:-0})); N=$((N+1))
  printf '  chunk %-2s [%-16s] %s\n' "$ch" "$who" "$line"
done
echo "--------------------------------------------------------"
printf 'FLEET AGGREGATE: %d passed, %d failed  (%d/%s chunks ran, %ds wall)\n' "$P" "$F" "$N" "$CHUNKS" "$((END-START))"
[ "$runfail" = 0 ] || echo "WARNING: $runfail chunk(s) never ran — infrastructure, retried $((MAX_ROUNDS-1))x"
[ "$runfail" = 0 ] && [ "$P" = "$EXPECT" ] && [ "$F" = 0 ] && { echo "PASS: fleet reproduces $EXPECT/0"; exit 0; }
[ "$runfail" != 0 ] && { echo "INCOMPLETE: infrastructure failures prevented a full run"; exit 3; }
echo "MISMATCH: expected $EXPECT/0"; exit 1
