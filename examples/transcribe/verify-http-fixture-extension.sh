#!/bin/sh
# Test-only extension proof. It never contacts an external website or runs a
# model: a loopback HTTP fixture supplies a small FPCM media representation.
#
# This exercises the public command and skill registration surfaces. The source
# adapter fetches a fixture-site HTML page and its referenced media; the decoder
# validates and turns FPCM into raw PCM. The inference fixture refuses any
# other PCM, so the pipeline cannot reach export by returning only a canned
# transcript.
set -eu

export BASHY_HINTS=off
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
runner=${BASHY_BIN:-bashy}
case "$runner" in /*) BASHY_BIN=$runner ;; *) BASHY_BIN=$(command -v "$runner") ;; esac
command -v python3 >/dev/null || { echo 'fixture needs python3 for its loopback HTTP server' >&2; exit 77; }
command -v curl >/dev/null || { echo 'fixture needs curl for real HTTP acquisition' >&2; exit 77; }

work=$(mktemp -d)
server_pid=
cleanup() {
    test -z "$server_pid" || kill "$server_pid" 2>/dev/null || :
    test -z "$server_pid" || wait "$server_pid" 2>/dev/null || :
    rm -rf "$work"
}
trap cleanup EXIT HUP INT TERM
export BASHY_BIN BASHY_HOME="$work/home" BASHY_COMMANDS_DIR="$work/commands"
export BASHY_SKILLS_DIR="$work/home/skills"
mkdir -p "$work/site" "$work/bin" "$work/skill"

# FPCM is deliberately not a WAV file: six-byte magic followed by PCM16LE.
# The expected payload is four samples (eight bytes).
printf 'FPCM1\n\000\000\377\177\000\200\000\000' > "$work/site/episode.fpcm"
printf '<!doctype html><meta name="fixture-media" content="episode.fpcm">\n' > "$work/site/index.html"
python3 - "$work/site" "$work/port" <<'PY' >"$work/server.log" 2>&1 &
import functools, http.server, pathlib, socketserver, sys
root, port_file = sys.argv[1:]
class Server(socketserver.TCPServer):
    allow_reuse_address = True
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=root)
with Server(("127.0.0.1", 0), handler) as server:
    pathlib.Path(port_file).write_text(str(server.server_address[1]))
    server.serve_forever()
PY
server_pid=$!
for _ in 1 2 3 4 5 6 7 8 9 10; do test -s "$work/port" && break; sleep 1; done
test -s "$work/port" || { cat "$work/server.log" >&2; exit 1; }
fixture_url="http://127.0.0.1:$(cat "$work/port")/index.html"

# All four providers are external, user-registerable commands. Only source and
# decoder are the extensions under test; infer/export are narrow witnesses for
# real decoded bytes moving through the unchanged pipeline.
cat > "$work/fixture-stage.sh" <<'STAGE'
#!/bin/sh
set -eu
stage=$1 request=$2
work_dir=$($BASHY_BIN jq -r '.work_dir // empty' "$request")
case "$stage" in
  fixture-site)
    input=$($BASHY_BIN jq -r '.input' "$request")
    case "$input" in http://127.0.0.1:*/*) ;; *) echo 'fixture-site accepts loopback fixture URLs only' >&2; exit 2 ;; esac
    mkdir -p "$work_dir"
    page="$work_dir/fixture-site.html" media="$work_dir/fixture-site.fpcm"
    curl --fail --silent --show-error --location "$input" > "$page"
    name=$(sed -n 's/.*name="fixture-media" content="\([A-Za-z0-9._-]*\)".*/\1/p' "$page")
    case "$name" in episode.fpcm) ;; *) echo 'fixture-site response has no safe media reference' >&2; exit 1 ;; esac
    base=${input%/*}
    curl --fail --silent --show-error --location "$base/$name" > "$media"
    test -s "$media"
    FIXTURE_MEDIA=$media; export FIXTURE_MEDIA
    "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"acquire",artifacts:[{path:env.FIXTURE_MEDIA}]}'
    ;;
  fixture-fpcm)
    input=$($BASHY_BIN jq -r '.artifacts[0].path' "$request")
    mkdir -p "$work_dir"
    output="$work_dir/fixture-site.pcm"
    python3 - "$input" "$output" <<'PY'
import pathlib, sys
raw = pathlib.Path(sys.argv[1]).read_bytes()
if not raw.startswith(b"FPCM1\n") or len(raw) != 14:
    raise SystemExit("fixture FPCM header or payload is invalid")
pathlib.Path(sys.argv[2]).write_bytes(raw[6:])
PY
    test "$(wc -c < "$output" | tr -d ' ')" = 8
    FIXTURE_PCM=$output; export FIXTURE_PCM
    "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"decode",artifacts:[{path:env.FIXTURE_PCM}],media:{duration_ms:1,sample_rate_hz:16000,channels:1}}'
    ;;
  fixture-infer)
    input=$($BASHY_BIN jq -r '.artifacts[0].path' "$request")
    python3 - "$input" <<'PY'
import pathlib, sys
if pathlib.Path(sys.argv[1]).read_bytes() != b"\x00\x00\xff\x7f\x00\x80\x00\x00":
    raise SystemExit("inference fixture did not receive the decoded FPCM PCM16 payload")
PY
    "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"infer",segments:[{start_ms:0,end_ms:1,text:"fixture media decoded"}]}'
    ;;
  fixture-export)
    output=$($BASHY_BIN jq -r '.output' "$request")
    text=$($BASHY_BIN jq -r '.segments[0].text' "$request")
    printf '%s\n' "$text" > "$output"
    FIXTURE_OUTPUT=$output; export FIXTURE_OUTPUT
    "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"export",artifacts:[{path:env.FIXTURE_OUTPUT}]}'
    ;;
  *) echo "unknown fixture stage: $stage" >&2; exit 2 ;;
esac
STAGE
chmod +x "$work/fixture-stage.sh"
for pair in 'fixture-site-source fixture-site' 'fixture-fpcm-decoder fixture-fpcm' 'fixture-infer fixture-infer' 'fixture-export fixture-export'; do
    set -- $pair
    "$BASHY_BIN" commands add "$1" --set exec.0=/bin/sh --set "exec.1=$work/fixture-stage.sh" --set "exec.2=$2" --set effects.0=read --set effects.1=write --set effects.2=net >/dev/null
done

pipeline="$root/examples/transcribe/pipeline.bsh"
test -f "$pipeline" || { echo "missing S503.3 pipeline: $pipeline" >&2; exit 77; }
"$BASHY_BIN" commands add transcribe-pipeline --set "exec.0=$BASHY_BIN" --set exec.1=--bashsharp --set "exec.2=$pipeline" --set effects.0=read --set effects.1=write --set effects.2=destroy --set effects.3=exec --set effects.4=net >/dev/null
"$BASHY_BIN" commands show transcribe-pipeline --yaml > "$work/script-command.yaml"
grep -F "$pipeline" "$work/script-command.yaml" >/dev/null

# Older installed Bashy lacks only S503.1's front door. The shim preserves the
# real registered-command dispatch and makes the fixture executable in review
# checkouts; BASHY_TRANSCRIBE_NATIVE=1 uses the native candidate unchanged.
if [ "${BASHY_TRANSCRIBE_NATIVE:-0}" = 1 ]; then
    PATH="$(dirname "$BASHY_BIN"):$PATH"
else
cat > "$work/bin/bashy" <<'SHIM'
#!/bin/sh
set -eu
if [ "${1:-}" = transcribe ]; then
  shift
  if [ "${1:-}" = stage ]; then
    shift; stage=$1; shift; test "$1" = --provider; provider=$2; shift 2
    if [ "$provider" = unavailable-fpcm ]; then
      echo "bashy transcribe: provider \"$provider\" is unavailable; register it with \`bashy commands add $provider\`" >&2
      exit 127
    fi
    exec "$BASHY_BIN" "$provider" "$1"
  fi
  if [ "${1:-}" = select ]; then
    shift; stage=$1; request=$2
    FIXTURE_STAGE=$stage; export FIXTURE_STAGE
    "$BASHY_BIN" jq -e '.stage == env.FIXTURE_STAGE' "$request" >/dev/null
    if [ "${FIXTURE_SELECTOR_AMBIGUOUS:-0}" = 1 ]; then
      echo "ambiguous fixture $stage adapters" >&2; exit 3
    fi
    case "$stage" in
      acquire) echo fixture-site-source ;; decode) echo fixture-fpcm-decoder ;;
      infer) echo fixture-infer ;; export) echo fixture-export ;;
      *) exit 2 ;;
    esac
    exit 0
  fi
  exec "$BASHY_BIN" transcribe-pipeline "$@"
fi
exec "$BASHY_BIN" "$@"
SHIM
chmod +x "$work/bin/bashy"
PATH="$work/bin:$PATH"
fi
export PATH

common='--source fixture-site-source --decoder fixture-fpcm-decoder --provider fixture-infer --exporter fixture-export'
# shellcheck disable=SC2086
"$BASHY_BIN" --bashsharp "$pipeline" $common --output "$work/script.txt" "$fixture_url" > "$work/script-path.txt"
test "$(cat "$work/script-path.txt")" = "$work/script.txt"
test "$(cat "$work/script.txt")" = 'fixture media decoded'
grep -F 'GET /index.html' "$work/server.log" >/dev/null
grep -F 'GET /episode.fpcm' "$work/server.log" >/dev/null

if [ "${BASHY_TRANSCRIBE_NATIVE:-0}" != 1 ]; then
    # The shim only fills the pre-S503.2 selection front door; it still selects
    # the registered providers and the pipeline still does all four stage calls.
    bashy transcribe --output "$work/automatic.txt" "$fixture_url" > "$work/automatic-path.txt"
    cmp "$work/script.txt" "$work/automatic.txt"
fi

cp "$root/skills/bashy/commands/transcribe.md" "$work/skill/SKILL.md"
cp "$root/skills/bashy/commands/transcribe.dhnt" "$work/skill/skill.dhnt"
"$BASHY_BIN" skill add "$work/skill" >/dev/null
"$BASHY_BIN" skill show transcribe --json > "$work/skill-record.json"
"$BASHY_BIN" jq -e '.identity != "" and .capability != "" and .contract.effects == ["read","write","destroy","net"]' "$work/skill-record.json" >/dev/null
export BASHY_TRANSCRIBE_INPUT="$fixture_url" BASHY_TRANSCRIBE_RESULT="$work/skill-path.txt"
export BASHY_TRANSCRIBE_OUTPUT="$work/skill.txt" BASHY_TRANSCRIBE_SOURCE=fixture-site-source
export BASHY_TRANSCRIBE_DECODER=fixture-fpcm-decoder BASHY_TRANSCRIBE_PROVIDER=fixture-infer BASHY_TRANSCRIBE_EXPORTER=fixture-export
"$BASHY_BIN" skill run transcribe --json > "$work/skill-receipt.json"
cmp "$work/script.txt" "$work/skill.txt"
test "$(cat "$work/skill-path.txt")" = "$work/skill.txt"
"$BASHY_BIN" jq -e '.attest.Valid == true and (.attest.Skill | length > 0) and (.attest.Passed | length > 0)' "$work/skill-receipt.json" >/dev/null
test "$("$BASHY_BIN" jq -r '.identity' "$work/skill-record.json")" = "$("$BASHY_BIN" jq -r '.attest.Skill' "$work/skill-receipt.json")"
test "$("$BASHY_BIN" jq -r '.capability' "$work/skill-record.json")" = "$("$BASHY_BIN" jq -r '.capability' "$work/skill-receipt.json")"

if [ "${BASHY_TRANSCRIBE_NATIVE:-0}" != 1 ]; then
    if FIXTURE_SELECTOR_AMBIGUOUS=1 bashy transcribe --output "$work/ambiguous.txt" "$fixture_url" >"$work/ambiguous.out" 2>"$work/ambiguous.err"; then
        echo 'ambiguous fixture adapters were accepted' >&2; exit 1
    fi
    grep -F 'ambiguous fixture acquire adapters' "$work/ambiguous.err" >/dev/null
    test ! -e "$work/ambiguous.txt"
    # Explicitly composed providers remain deterministic despite ambiguity.
    # shellcheck disable=SC2086
    FIXTURE_SELECTOR_AMBIGUOUS=1 bashy transcribe $common --output "$work/explicit.txt" "$fixture_url" > "$work/explicit-path.txt"
    cmp "$work/script.txt" "$work/explicit.txt"
fi

# Composition stays local: change only the public command record to a copied
# Bash# body. No core dispatcher, original source adapter, or binary rebuild is
# involved, and the resulting output remains byte-for-byte identical.
cp "$pipeline" "$work/local-pipeline.bsh"
printf '\n# local test-only composition overlay\n' >> "$work/local-pipeline.bsh"
"$BASHY_BIN" commands set transcribe-pipeline --set "exec.2=$work/local-pipeline.bsh" >/dev/null
"$BASHY_BIN" commands show transcribe-pipeline --yaml | grep -F "$work/local-pipeline.bsh" >/dev/null
# shellcheck disable=SC2086
bashy transcribe $common --output "$work/override.txt" "$fixture_url" > "$work/override-path.txt"
cmp "$work/script.txt" "$work/override.txt"

# Unavailable adapters fail at the existing public dispatcher and cannot leave
# a success output. S503.2's selector-specific ambiguity case belongs to its
# real applicability tests; this fixture keeps explicit registration honest.
if bashy transcribe --source fixture-site-source --decoder unavailable-fpcm --provider fixture-infer --exporter fixture-export --output "$work/unavailable.txt" "$fixture_url" >"$work/unavailable.out" 2>"$work/unavailable.err"; then
    echo 'unavailable decoder was accepted' >&2; exit 1
fi
grep -F 'provider "unavailable-fpcm" is unavailable' "$work/unavailable.err" >/dev/null
test ! -e "$work/unavailable.txt"
printf 'http fixture extension PASS; acquired FPCM -> decoded PCM -> infer/export, script/skill parity, local override, unavailable adapter refusal\n'
