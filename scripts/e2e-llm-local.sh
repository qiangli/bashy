#!/usr/bin/env bash
# e2e-llm-local — bashy local models through the llm door (Sprint 379 Y2).
#
# Brings the model door up (`llm up`, port 24556), pulls a tiny model into
# bashy's own Ollama engine, then chats once through the OpenAI-compatible
# path and once through the Anthropic-compatible path, asserting a
# non-empty answer each time.
#
# Ordering note: `llm up` runs BEFORE the pull because `bashy ollama pull`
# is a client verb — it POSTs /api/pull to the door, so the door must
# answer first. `up` is idempotent (a running door is left alone), so this
# reads the same as "pull, up, chat" on a host where the door is up.
#
# Portable across macOS / Linux / Windows (Git Bash): only bash + curl
# (or `bashy fetch` as fallback) plus python3/jq/grep for one JSON field.
#
# Usage: scripts/e2e-llm-local.sh [--model NAME] [--no-pull]
# Env:   BASHY (path to the bashy binary), E2E_MODEL, E2E_NO_PULL=1.
# Exit 0 on PASS, 1 on FAIL. Prints one line per stage + a verdict line.
set -u

MODEL="${E2E_MODEL:-llama3.2:1b}"
PROMPT="Reply with exactly: e2e-ok"
NO_PULL="${E2E_NO_PULL:-0}"

while [ $# -gt 0 ]; do
	case "$1" in
		--model) MODEL="$2"; shift 2 ;;
		--no-pull) NO_PULL=1; shift ;;
		*) echo "e2e-llm-local: unknown flag $1" >&2; exit 2 ;;
	esac
done

here="$(cd "$(dirname "$0")" && pwd)"
if [ -z "${BASHY:-}" ]; then
	if command -v bashy >/dev/null 2>&1; then
		BASHY="$(command -v bashy)"
	elif [ -x "$here/../bin/bashy" ]; then
		BASHY="$here/../bin/bashy"
	else
		echo "e2e-llm-local: FAIL (no bashy: set \$BASHY or put bashy on PATH)" >&2
		exit 1
	fi
fi

pass=0
fail=0

# Scratch files live next to TMPDIR when usable, else in the cwd (Windows
# hosts have no /tmp; the ssh cwd is writable).
E2E_TMP="${TMPDIR:-/tmp}"
[ -d "$E2E_TMP" ] || E2E_TMP="."
E2E_OUT="$E2E_TMP/e2e-llm-local-out.txt"

# Tool probes — a PATH hit is not proof (Windows Store stubs resolve but
# do not run headless). Probe once, cache, and pick the first tier that
# actually executes. stdin is /dev/null here so probes never eat input.
HAVE_CURL=0
HAVE_PY=0
HAVE_JQ=0
HAVE_GREP=0
if command -v curl >/dev/null 2>&1 && curl --version </dev/null >/dev/null 2>&1; then HAVE_CURL=1; fi
if command -v python3 >/dev/null 2>&1 && python3 -c 'pass' </dev/null >/dev/null 2>&1; then HAVE_PY=1; fi
if [ "$HAVE_PY" = 0 ] && command -v jq >/dev/null 2>&1 && printf '{}' | jq -e . </dev/null >/dev/null 2>&1; then HAVE_JQ=1; fi
if [ "$HAVE_CURL" = 0 ] && [ -z "${BASHY:-}" ]; then
	echo "e2e-llm-local: FAIL (no working http client: need curl or bashy fetch)" >&2
	exit 1
fi
# Answer extraction needs no external tool: python3/jq when they run,
# else a pure-bash witness below (works even on Windows bashy, which
# ships no python/jq/grep/sed).

# failhead: first 20 lines of a file to stderr, pure bash (head is not
# guaranteed — Windows bashy ships no text userland).
failhead() {
	n=0
	while IFS= read -r line || [ -n "$line" ]; do
		printf '%s\n' "$line" >&2
		n=$((n + 1))
		[ "$n" -ge 20 ] && break
	done < "$1"
}

stage() { # stage <name> <command...>: runs it, prints ok/FAIL, counts.
	name="$1"; shift
	if "$@" >"$E2E_OUT" 2>&1; then
		echo "e2e-llm-local: stage $name ... ok"
		pass=$((pass + 1))
	else
		echo "e2e-llm-local: stage $name ... FAIL"
		failhead "$E2E_OUT"
		fail=$((fail + 1))
	fi
}

stage_contains() { # stage_contains <name> <needle> <cmd...>: ok iff rc=0 and output contains needle.
	name="$1"; needle="$2"; shift 2
	if "$@" >"$E2E_OUT" 2>&1; then
		found=0
		while IFS= read -r line || [ -n "$line" ]; do
			case "$line" in
				*"$needle"*) found=1; break ;;
			esac
		done <"$E2E_OUT"
		if [ "$found" = 1 ]; then
			echo "e2e-llm-local: stage $name ... ok"
			pass=$((pass + 1))
			return 0
		fi
	fi
	echo "e2e-llm-local: stage $name ... FAIL"
	failhead "$E2E_OUT"
	fail=$((fail + 1))
	return 1
}

# http_post <url> <data> <header>...: body on stdout, rc!=0 on HTTP >= 400.
# header argv entries keep their spaces (arrays, never word-split).
http_post() {
	url="$1"; data="$2"; shift 2
	if [ "$HAVE_CURL" = 1 ]; then
		args=(-fsS --max-time 300 -X POST)
		for h in "$@"; do args+=(-H "$h"); done
		curl "${args[@]}" --data "$data" "$url"
	elif [ -n "${BASHY:-}" ]; then
		args=(-f --timeout 300s -X POST)
		for h in "$@"; do args+=(-H "$h"); done
		"$BASHY" fetch "${args[@]}" -d "$data" "$url"
	else
		echo "no http client (need curl or bashy fetch)" >&2
		return 1
	fi
}

# json_witness <body> <key>: first content char of the first "<key>":"..."
# value, pure bash (no python/jq/grep/sed — the Windows-bashy tier).
# Prints the char (proving non-empty), rc 0; rc 1 when absent or empty.
json_witness() {
	body="$1"
	prefix="\"$2\":\""
	case "$body" in
		*"$prefix"*)
			rest="${body#*"$prefix"}"
			ch="${rest:0:1}"
			case "$ch" in
				"" | '"') return 1 ;;
				*) printf '%s' "$ch"; return 0 ;;
			esac
			;;
		*) return 1 ;;
	esac
}

# openai_answer: stdin (chat/completions JSON) -> message content on stdout.
openai_answer() {
	if [ "$HAVE_PY" = 1 ]; then
		python3 -c 'import json,sys
d = json.load(sys.stdin)
cs = d.get("choices") or []
m = (cs[0].get("message") if cs else {}) or {}
print(m.get("content", ""))'
	elif [ "$HAVE_JQ" = 1 ]; then
		jq -r '.choices[0].message.content // ""'
	else
		# Slurp stdin without cat (absent on Windows bashy).
		slurped=""
		while IFS= read -r line || [ -n "$line" ]; do slurped="${slurped}${line}"; done
		json_witness "$slurped" "content"
	fi
}

# anthropic_answer: stdin (messages JSON) -> first text block on stdout.
anthropic_answer() {
	if [ "$HAVE_PY" = 1 ]; then
		python3 -c 'import json,sys
d = json.load(sys.stdin)
ts = [b.get("text", "") for b in (d.get("content") or []) if isinstance(b, dict) and b.get("type") == "text"]
print(ts[0] if ts else "")'
	elif [ "$HAVE_JQ" = 1 ]; then
		jq -r '[.content[] | select(.type=="text") | .text][0] // ""'
	else
		slurped=""
		while IFS= read -r line || [ -n "$line" ]; do slurped="${slurped}${line}"; done
		json_witness "$slurped" "text"
	fi
}

echo "e2e-llm-local: model=$MODEL host=$(hostname 2>/dev/null || uname -n 2>/dev/null || echo unknown) os=$(uname -s 2>/dev/null || echo unknown)"
echo "e2e-llm-local: bashy=$("$BASHY" --version 2>&1)"

stage "llm-up" "$BASHY" llm up
if [ "$NO_PULL" = "0" ]; then
	stage "ollama-pull" "$BASHY" ollama pull "$MODEL"
else
	echo "e2e-llm-local: stage ollama-pull ... SKIP (E2E_NO_PULL=1)"
fi
stage_contains "ollama-list" "$MODEL" "$BASHY" ollama list

# shellcheck disable=SC1090
if ! eval "$("$BASHY" llm env)"; then
	echo "e2e-llm-local: stage llm-env ... FAIL" >&2
	exit 1
fi
echo "e2e-llm-local: stage llm-env ... ok"
pass=$((pass + 1))
[ -n "${OPENAI_BASE_URL:-}" ] || { echo "e2e-llm-local: OPENAI_BASE_URL empty" >&2; exit 1; }
[ -n "${ANTHROPIC_BASE_URL:-}" ] || { echo "e2e-llm-local: ANTHROPIC_BASE_URL empty" >&2; exit 1; }

OPENAI_BODY="{\"model\":\"$MODEL\",\"stream\":false,\"temperature\":0,\"max_tokens\":64,\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT\"}]}"
if OPENAI_RESP="$(http_post "$OPENAI_BASE_URL/chat/completions" "$OPENAI_BODY" "Authorization: Bearer $OPENAI_API_KEY" "Content-Type: application/json")"; then
	OPENAI_ANSWER="$(printf '%s' "$OPENAI_RESP" | openai_answer)"
	if [ -n "$OPENAI_ANSWER" ]; then
		echo "e2e-llm-local: stage openai-chat ... ok (${#OPENAI_ANSWER} chars: ${OPENAI_ANSWER:0:80})"
		pass=$((pass + 1))
	else
		echo "e2e-llm-local: stage openai-chat ... FAIL (empty answer)"
		printf '%s\n' "${OPENAI_RESP:0:2000}" >&2
		fail=$((fail + 1))
	fi
else
	echo "e2e-llm-local: stage openai-chat ... FAIL (http error)"
	fail=$((fail + 1))
fi

ANTHROPIC_BODY="{\"model\":\"$MODEL\",\"max_tokens\":64,\"temperature\":0,\"messages\":[{\"role\":\"user\",\"content\":\"$PROMPT\"}]}"
# anthropic_post <url>: body on stdout; rc 0 = HTTP 200, 1 = route
# absent (404), 2 = any other failure.
anthropic_post() {
	if [ "$HAVE_CURL" = 1 ]; then
		# Code rides on stdout (last 3 bytes) — no temp file: native
		# Windows curl.exe cannot see bashy's virtual /tmp.
		resp="$(curl -s -w '%{http_code}' --max-time 300 -X POST \
			-H "Authorization: Bearer $OPENAI_API_KEY" -H "x-api-key: $ANTHROPIC_API_KEY" \
			-H "anthropic-version: 2023-06-01" -H "Content-Type: application/json" \
			--data "$ANTHROPIC_BODY" "$1")"
		[ "${#resp}" -ge 3 ] || { echo "no http response" >&2; return 2; }
		rest="${resp%???}"
		code="${resp#"$rest"}"
		body="$rest"
		case "$code" in
			200) printf '%s' "$body"; return 0 ;;
			404) return 1 ;;
			*) echo "http $code" >&2; return 2 ;;
		esac
	fi
	# No curl (bashy-fetch fallback): no status codes, so any failure
	# is fatal — the compat fallback below only triggers on curl's 404.
	if body="$(http_post "$1" "$ANTHROPIC_BODY" "Authorization: Bearer $OPENAI_API_KEY" "x-api-key: $ANTHROPIC_API_KEY" "anthropic-version: 2023-06-01" "Content-Type: application/json")"; then
		printf '%s' "$body"
		return 0
	fi
	return 2
}
# The canonical Anthropic route is $ANTHROPIC_BASE_URL/v1/messages
# (i.e. /anthropic/v1/messages on the door). Doors that predate that
# route still serve the same Anthropic wire format at
# $OPENAI_BASE_URL/messages (/v1/messages): on a 404 there — and only
# there — fall back to it and say so. Any other failure stays fatal.
ANTHROPIC_RESP=""
ANTHROPIC_RESP="$(anthropic_post "$ANTHROPIC_BASE_URL/v1/messages")"
ANTHROPIC_RC="$?"
# Fall back to the compat route ONLY when the canonical route is absent
# (404 = door predates it). Any other failure stays fatal so a broken
# canonical route on a new door cannot hide behind the compat one.
if [ "$ANTHROPIC_RC" = "1" ]; then
	echo "e2e-llm-local: note: canonical $ANTHROPIC_BASE_URL/v1/messages is 404 (door predates the route); trying compat $OPENAI_BASE_URL/messages"
	ANTHROPIC_RESP="$(anthropic_post "$OPENAI_BASE_URL/messages")"
	ANTHROPIC_RC="$?"
fi
if [ "$ANTHROPIC_RC" = "0" ]; then
	ANTHROPIC_ANSWER="$(printf '%s' "$ANTHROPIC_RESP" | anthropic_answer)"
	if [ -n "$ANTHROPIC_ANSWER" ]; then
		echo "e2e-llm-local: stage anthropic-chat ... ok (${#ANTHROPIC_ANSWER} chars: ${ANTHROPIC_ANSWER:0:80})"
		pass=$((pass + 1))
	else
		echo "e2e-llm-local: stage anthropic-chat ... FAIL (empty answer)"
		printf '%s\n' "${ANTHROPIC_RESP:0:2000}" >&2
		fail=$((fail + 1))
	fi
else
	echo "e2e-llm-local: stage anthropic-chat ... FAIL (rc=$ANTHROPIC_RC)"
	fail=$((fail + 1))
fi

if [ "$fail" = "0" ]; then
	echo "e2e-llm-local: PASS (pass=$pass fail=0 model=$MODEL)"
	exit 0
else
	echo "e2e-llm-local: FAIL (pass=$pass fail=$fail model=$MODEL)" >&2
	exit 1
fi
