#!/usr/bin/env bash
# genie-preview-workflow — the one documented genie preview workflow, as a
# re-runnable script (Sprint 379 Y7; user doc: docs/first-party-harness.md,
# "Genie preview workflow").
#
# Two legs, local first, each a one-turn task through the host's model door:
#   local  a small Ollama model pulled by bashy, served by `bashy llm up`
#   cloud  a door model (door-*) or a registry API model whose credential is
#          already configured on this host
# Each leg asks genie to create a file, run a command, and show the result,
# then checks the file on disk (the answer alone proves nothing).
#
# Portable: bash 3.2+ or bashy itself (Windows: `bashy.exe scripts/genie-preview-workflow.sh`).
# Only bash builtins and `bashy` verbs are used (no curl/jq/grep/timeout).
#
# Usage: scripts/genie-preview-workflow.sh [--local-model M] [--cloud-model M]
#                                          [--legs local,cloud] [--keep-model]
#                                          [--isolate]
# Env:   BASHY (path to bashy), GENIE_PREVIEW_LOCAL_MODEL (default qwen3:1.7b),
#        GENIE_PREVIEW_CLOUD_MODEL (default: auto-pick), GENIE_PREVIEW_LEGS,
#        GENIE_PREVIEW_PULL_TIMEOUT (s, 1800), GENIE_PREVIEW_TURN_TIMEOUT (s, 600),
#        GENIE_PREVIEW_UP_TIMEOUT (s, 120), GENIE_PREVIEW_ISOLATE=1.
#
# --isolate runs against a throwaway BASHY_HOME and door port, so a host that
# already has a door, a genie bundle and sessions is left untouched.
#
# Exit: 0 every requested leg passed
#       1 a leg failed (including a timeout)
#       2 usage or environment error (no bashy, bad flag)
#       3 local passed but the cloud leg was SKIPPED: no cloud model or
#         credential is configured on this host (not green for the preview bar)
# Cleanup (always): stop the door only if this run started it, remove the
# model only if this run pulled it, delete scratch directories.
set -u

LOCAL_MODEL="${GENIE_PREVIEW_LOCAL_MODEL:-qwen3:1.7b}"
CLOUD_MODEL="${GENIE_PREVIEW_CLOUD_MODEL:-}"
LEGS="${GENIE_PREVIEW_LEGS:-local,cloud}"
PULL_TIMEOUT="${GENIE_PREVIEW_PULL_TIMEOUT:-1800}"
TURN_TIMEOUT="${GENIE_PREVIEW_TURN_TIMEOUT:-600}"
UP_TIMEOUT="${GENIE_PREVIEW_UP_TIMEOUT:-120}"
ISOLATE="${GENIE_PREVIEW_ISOLATE:-0}"
KEEP_MODEL=0

while [ $# -gt 0 ]; do
	case "$1" in
		--local-model) LOCAL_MODEL="${2:-}"; shift 2 ;;
		--cloud-model) CLOUD_MODEL="${2:-}"; shift 2 ;;
		--legs) LEGS="${2:-}"; shift 2 ;;
		--keep-model) KEEP_MODEL=1; shift ;;
		--isolate) ISOLATE=1; shift ;;
		-h | --help) sed -n '2,33p' "$0"; exit 0 ;;
		*) echo "genie-preview: unknown flag $1" >&2; exit 2 ;;
	esac
done

here="$(cd "$(dirname "$0")" && pwd)"
if [ -z "${BASHY:-}" ]; then
	if command -v bashy >/dev/null 2>&1; then
		BASHY="$(command -v bashy)"
	elif [ -x "$here/../bin/bashy" ]; then
		BASHY="$here/../bin/bashy"
	else
		echo "genie-preview: no bashy (set \$BASHY or put bashy on PATH)" >&2
		exit 2
	fi
fi
case ",$LEGS," in *,local,* | *,cloud,*) ;; *) echo "genie-preview: --legs needs local and/or cloud" >&2; exit 2 ;; esac

TOKEN="genie-preview-ok"
PROMPT="Create a file named hello.txt in the current directory whose only content is the single line $TOKEN. Then run the shell command: cat hello.txt. Finally show me the exact output of that command."

SCRATCH="${TMPDIR:-.}"
[ -d "$SCRATCH" ] || SCRATCH="."
SCRATCH="$SCRATCH/genie-preview.$$"
mkdir -p "$SCRATCH" || { echo "genie-preview: cannot create $SCRATCH" >&2; exit 2; }
# Absolute: genie and the dag recipes run in other directories.
SCRATCH="$(cd "$SCRATCH" && pwd)"

# native_path: child programs read env paths natively. A shell that shows
# Windows drives as /c/Users/... (bashy on Windows) must hand them c:/Users/...
native_path() {
	case "$1" in
		/[a-zA-Z]/*) printf '%s:%s' "${1:1:1}" "${1:2}" ;;
		*) printf '%s' "$1" ;;
	esac
}

if [ "$ISOLATE" = 1 ]; then
	mkdir -p "$SCRATCH/home"
	export BASHY_HOME="$(native_path "$SCRATCH/home")"
	export BASHY_LLM_PORT="${BASHY_LLM_PORT:-24571}"
fi
DOOR_PORT="${BASHY_LLM_PORT:-24556}"

started_door=0
rc_local=-1
rc_cloud=-1

say() { printf 'genie-preview: %s\n' "$*"; }

# run_timed SECONDS CMD...: rc 124 on timeout (coreutils timeout semantics).
run_timed() {
	secs="$1"; shift
	"$BASHY" timeout "$secs" "$@"
}

door_listening() {
	"$BASHY" fetch --timeout 3s "http://127.0.0.1:$DOOR_PORT/health" >/dev/null 2>&1
}

# stop_door: `llm down`, then wait for the port to close. Where `llm down`
# cannot signal the door (Windows), fall back to the pid the door logged in
# this run's own isolated home; a shared door is never killed by pid.
stop_door() {
	run_timed 60 "$BASHY" llm down >/dev/null 2>&1
	drc=$?
	n=0
	while door_listening && [ "$n" -lt 15 ]; do sleep 1; n=$((n + 1)); done
	if door_listening && [ "$drc" != 0 ] && [ "$ISOLATE" = 1 ] && [ -f "$SCRATCH/home/broker/door.log" ]; then
		pid=""
		while IFS= read -r l || [ -n "$l" ]; do
			case "$l" in *"(pid "*) pid="${l##*(pid }"; pid="${pid%%)*}" ;; esac
		done <"$SCRATCH/home/broker/door.log"
		if [ -n "$pid" ]; then
			say "cleanup: llm down rc=$drc; killing the door pid $pid from this run's log"
			kill "$pid" 2>/dev/null
			n=0
			while door_listening && [ "$n" -lt 15 ]; do sleep 1; n=$((n + 1)); done
		fi
	fi
	if door_listening; then
		say "cleanup: the door on port $DOOR_PORT is still answering" >&2
	else
		say "cleanup: stopped the door on port $DOOR_PORT (started by this run)"
	fi
}

cleanup() {
	status=$?
	trap - EXIT
	if [ "$KEEP_MODEL" = 0 ] && [ -f "$SCRATCH/models.before" ] && door_listening; then
		remove_new_models
	fi
	if [ "$started_door" = 1 ]; then
		stop_door
	fi
	cd / 2>/dev/null || true
	rm -rf "$SCRATCH" 2>/dev/null
	exit "$status"
}
trap cleanup EXIT

# model_names: the NAME column of `ollama list`, one per line.
model_names() {
	"$BASHY" ollama list 2>/dev/null | { read -r _header; while read -r name _rest; do [ -n "$name" ] && printf '%s\n' "$name"; done; }
}

# remove_new_models: remove every model that was not there when the run
# began — the pulled model and the context variants the recipe derives from it.
remove_new_models() {
	model_names >"$SCRATCH/models.after"
	while IFS= read -r m; do
		if ! contains_line "$SCRATCH/models.before" "$m"; then
			if run_timed 120 "$BASHY" ollama rm "$m" >/dev/null 2>&1; then
				say "cleanup: removed model $m (added by this run)"
			else
				say "cleanup: could not remove model $m" >&2
			fi
		fi
	done <"$SCRATCH/models.after"
}

contains_line() { # contains_line FILE LINE (exact)
	while IFS= read -r l || [ -n "$l" ]; do
		[ "$l" = "$2" ] && return 0
	done <"$1"
	return 1
}

# first_line_of FILE: first non-empty line, pure bash.
first_line_of() {
	while IFS= read -r l || [ -n "$l" ]; do
		if [ -n "$l" ]; then printf '%s' "$l"; return 0; fi
	done <"$1"
	return 1
}

contains() { # contains FILE NEEDLE
	while IFS= read -r l || [ -n "$l" ]; do
		case "$l" in *"$2"*) return 0 ;; esac
	done <"$1"
	return 1
}

# turn LEG MODEL: one genie turn in a fresh directory; the check is the file.
turn() {
	leg="$1"; model="$2"
	dir="$SCRATCH/$leg"
	mkdir -p "$dir"
	out="$SCRATCH/$leg.out"
	err="$SCRATCH/$leg.err"
	say "[$leg] one turn on $model (timeout ${TURN_TIMEOUT}s)"
	(cd "$dir" && run_timed "$TURN_TIMEOUT" "$BASHY" genie -m "$model" "$PROMPT") >"$out" 2>"$err"
	trc=$?
	if [ "$trc" = 124 ]; then
		say "[$leg] FAIL: the turn timed out after ${TURN_TIMEOUT}s" >&2
		return 1
	fi
	if [ "$trc" != 0 ]; then
		say "[$leg] FAIL: genie exited $trc" >&2
		tail_of "$err"
		return 1
	fi
	if [ ! -f "$dir/hello.txt" ]; then
		say "[$leg] FAIL: genie did not create hello.txt" >&2
		tail_of "$out"
		return 1
	fi
	if ! contains "$dir/hello.txt" "$TOKEN"; then
		say "[$leg] FAIL: hello.txt does not contain $TOKEN" >&2
		return 1
	fi
	if ! contains "$out" "$TOKEN"; then
		say "[$leg] FAIL: the answer does not show the command output" >&2
		tail_of "$out"
		return 1
	fi
	say "[$leg] PASS: hello.txt created, command run, result shown"
	return 0
}

tail_of() { # last 15 lines of FILE to stderr, pure bash
	n=0; buf=()
	while IFS= read -r l || [ -n "$l" ]; do
		buf+=("$l")
		[ "${#buf[@]}" -gt 15 ] && buf=("${buf[@]:1}")
	done <"$1"
	for l in "${buf[@]+"${buf[@]}"}"; do printf '  | %s\n' "$l" >&2; done
}

ensure_door() {
	if door_listening; then
		say "door already up on port $DOOR_PORT (left running)"
		return 0
	fi
	run_timed "$UP_TIMEOUT" "$BASHY" llm up >"$SCRATCH/up.out" 2>&1
	urc=$?
	if [ "$urc" = 0 ]; then
		started_door=1
		say "door started on port $DOOR_PORT"
		return 0
	fi
	say "FAIL: bashy llm up (rc=$urc)" >&2
	tail_of "$SCRATCH/up.out"
	return 1
}

say "bashy=$("$BASHY" --version 2>&1 | { read -r l; printf '%s' "$l"; }) os=$(uname -s 2>/dev/null || echo unknown) isolate=$ISOLATE"

leg_local() {
	say "[local] model $LOCAL_MODEL"
	ensure_door || return 1
	model_names >"$SCRATCH/models.before"
	if "$BASHY" ollama show "$LOCAL_MODEL" >/dev/null 2>&1; then
		say "[local] model already present (kept after the run)"
	else
		say "[local] pulling $LOCAL_MODEL (timeout ${PULL_TIMEOUT}s)"
		run_timed "$PULL_TIMEOUT" "$BASHY" ollama pull "$LOCAL_MODEL" >"$SCRATCH/pull.out" 2>&1
		prc=$?
		if [ "$prc" != 0 ]; then
			say "[local] FAIL: ollama pull (rc=$prc)" >&2
			tail_of "$SCRATCH/pull.out"
			return 1
		fi
	fi
	turn local "$LOCAL_MODEL"
}

# pick_cloud: print the cloud model to use, else nothing. An API model needs
# its api_key_ref present in the environment or the vault (names only, never
# values); a door-* model rides the host's door and its CLI seat.
pick_cloud() {
	list="$SCRATCH/models.out"
	"$BASHY" model list --custom >"$list" 2>/dev/null || return 1
	names_door=""; names_api=""
	while IFS= read -r l; do
		set -- $l
		[ $# -ge 3 ] || continue
		[ "$3" = api ] || continue
		case "$1" in
			door-*) names_door="$names_door $1" ;;
			*) names_api="$names_api $1" ;;
		esac
	done <"$list"
	vault="$SCRATCH/secrets.out"
	"$BASHY" secret ls >"$vault" 2>/dev/null || : >"$vault"
	for m in $names_api; do
		ref=""
		while IFS= read -r l; do
			case "$l" in api_key_ref:*) ref="${l#api_key_ref:}"; ref="${ref# }" ;; esac
		done < <("$BASHY" model show "$m" 2>/dev/null)
		[ -n "$ref" ] || continue
		if [ -n "${!ref:-}" ] || contains "$vault" "$ref"; then
			printf '%s' "$m"
			return 0
		fi
	done
	# A door model needs a live sticky binding (llm sticky create), which goes
	# stale when the CLI seat upgrades; so it is the fallback, not the first pick.
	for m in $names_door; do printf '%s' "$m"; return 0; done
	return 1
}

leg_cloud() {
	model="$CLOUD_MODEL"
	if [ -z "$model" ]; then
		model="$(pick_cloud)" || model=""
	fi
	if [ -z "$model" ]; then
		say "[cloud] SKIP: no door model or credentialed API model configured on this host"
		return 3
	fi
	say "[cloud] model $model"
	ensure_door || return 1
	turn cloud "$model"
}

case ",$LEGS," in
	*,local,*)
		leg_local; rc_local=$? ;;
esac
case ",$LEGS," in
	*,cloud,*)
		leg_cloud; rc_cloud=$? ;;
esac

verdict=0
[ "$rc_local" = 0 ] || [ "$rc_local" = -1 ] || verdict=1
if [ "$rc_cloud" = 3 ]; then
	[ "$verdict" = 0 ] && verdict=3
elif [ "$rc_cloud" != 0 ] && [ "$rc_cloud" != -1 ]; then
	verdict=1
fi
say "result: local=$rc_local cloud=$rc_cloud exit=$verdict (0 pass, 1 fail, 3 cloud skipped)"
exit "$verdict"
