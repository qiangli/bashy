#!/bin/sh
# Contract/composition proof only. Stub inference is not media transcription.
set -eu
export BASHY_HINTS=off
root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
runner=${BASHY_BIN:-bashy}
case "$runner" in /*) BASHY_BIN=$runner ;; *) BASHY_BIN=$(command -v "$runner") ;; esac
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
export BASHY_BIN BASHY_HOME="$work/home" BASHY_COMMANDS_DIR="$work/commands"
export BASHY_SKILLS_DIR="$work/home/skills" STUB_MEDIA="$work/clip [1] ?.wav"
mkdir -p "$work/bin" "$work/skill"
printf 'fixture media\n' > "$STUB_MEDIA"

# Registered adapters are independently replaceable. One body implements the
# four fixture stages only because the fixture has no real decoder or model.
cat > "$work/stub-stage.sh" <<'STAGE'
#!/bin/sh
set -eu
stage=$1 request=$2
case "$stage" in
    acquire)
        "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"acquire",artifacts:[{path:env.STUB_MEDIA}]}' ;;
    decode)
        "$BASHY_BIN" jq '{version:"bashy-transcribe-stage-v1",stage:"decode",artifacts:.artifacts,media:{duration_ms:1200,sample_rate_hz:16000,channels:1}}' "$request" ;;
    infer)
        if [ "${STUB_BAD_INFER:-0}" = 1 ]; then
            "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"infer",segments:[{start_ms:1200,end_ms:0,text:"bad fixture"}]}'
        else
            "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"infer",segments:[{start_ms:0,end_ms:1200,text:"fixture only"}]}'
        fi ;;
    export)
        STUB_STAGE_OUTPUT=$("$BASHY_BIN" jq -r '.output' "$request")
        export STUB_STAGE_OUTPUT
        printf 'fixture only\n' > "$STUB_STAGE_OUTPUT"
        "$BASHY_BIN" jq -n '{version:"bashy-transcribe-stage-v1",stage:"export",artifacts:[{path:env.STUB_STAGE_OUTPUT}]}' ;;
esac
STAGE
chmod +x "$work/stub-stage.sh"
for pair in 'stub-source acquire' 'stub-decoder decode' 'stub-infer infer' 'stub-export export'; do
    set -- $pair
    "$BASHY_BIN" commands add "$1" --set exec.0=/bin/sh \
        --set "exec.1=$work/stub-stage.sh" --set "exec.2=$2" \
        --set effects.0=read --set effects.1=write >/dev/null
done
pipeline="$root/examples/transcribe/pipeline.bsh"
"$BASHY_BIN" commands add transcribe-pipeline \
    --set "exec.0=$BASHY_BIN" --set exec.1=--bashsharp \
    --set "exec.2=$pipeline" --set effects.0=read --set effects.1=write \
    --set effects.2=destroy --set effects.3=exec --set effects.4=net >/dev/null
"$BASHY_BIN" commands show transcribe-pipeline --yaml > "$work/command.txt"
grep -F "$pipeline" "$work/command.txt" >/dev/null

# The default shim supplies only the front door missing from an older installed
# bashy. BASHY_TRANSCRIBE_NATIVE=1 tests the candidate's real S503.1 front door.
if [ "${BASHY_TRANSCRIBE_NATIVE:-0}" = 1 ]; then
    PATH="$(dirname "$BASHY_BIN"):$PATH"
else
cat > "$work/bin/bashy" <<'SHIM'
#!/bin/sh
set -eu
case "${1:-}" in
    transcribe)
        shift
        if [ "${1:-}" = stage ]; then
            shift
            stage=$1
            shift
            [ "$1" = --provider ] || exit 2
            provider=$2
            shift 2
            case "$stage" in
                acquire|decode|infer|export) ;;
                *) exit 2 ;;
            esac
            exec "$BASHY_BIN" "$provider" "$1"
        fi
        exec "$BASHY_BIN" transcribe-pipeline "$@" ;;
    *) exec "$BASHY_BIN" "$@" ;;
esac
SHIM
chmod +x "$work/bin/bashy"
PATH="$work/bin:$PATH"
fi
export PATH

common='--source stub-source --decoder stub-decoder --provider stub-infer --exporter stub-export'
# Intentional word splitting only for the fixed, literal fixture option list.
# shellcheck disable=SC2086
"$BASHY_BIN" --bashsharp "$pipeline" $common --output "$work/direct.txt" "$STUB_MEDIA" > "$work/direct-path.txt"
test "$(cat "$work/direct-path.txt")" = "$work/direct.txt"
test "$(cat "$work/direct.txt")" = 'fixture only'

cp "$root/skills/bashy/commands/transcribe.md" "$work/skill/SKILL.md"
cp "$root/skills/bashy/commands/transcribe.dhnt" "$work/skill/skill.dhnt"
"$BASHY_BIN" skill add "$work/skill" >/dev/null
export BASHY_TRANSCRIBE_INPUT="$STUB_MEDIA" BASHY_TRANSCRIBE_RESULT="$work/skill-path.txt"
export BASHY_TRANSCRIBE_OUTPUT="$work/skill.txt"
export BASHY_TRANSCRIBE_SOURCE=stub-source BASHY_TRANSCRIBE_DECODER=stub-decoder
export BASHY_TRANSCRIBE_PROVIDER=stub-infer BASHY_TRANSCRIBE_EXPORTER=stub-export
"$BASHY_BIN" skill run transcribe --json > "$work/skill-receipt.json"
test "$(cat "$work/skill-path.txt")" = "$work/skill.txt"
cmp "$work/direct.txt" "$work/skill.txt"
"$BASHY_BIN" jq -e '.attest.Valid == true and (.attest.Skill | length > 0) and (.attest.Passed | length > 0)' "$work/skill-receipt.json" >/dev/null
ledger="$BASHY_SKILLS_DIR/attest/transcribe_pipeline.jsonl"
test "$("$BASHY_BIN" wc -l < "$ledger")" -eq 2
"$BASHY_BIN" jq -e '.attest.Valid == true and .status == 0' "$ledger" >/dev/null
receipt_fields='{skill:.attest.Skill,passed:.attest.Passed,failed:.attest.Failed,effects:.attest.Effects,status:.status}'
first=$("$BASHY_BIN" head -n 1 "$ledger" | "$BASHY_BIN" jq -c "$receipt_fields")
second=$("$BASHY_BIN" tail -n 1 "$ledger" | "$BASHY_BIN" jq -c "$receipt_fields")
test "$first" = "$second"

# Editable local composition uses the existing command overlay and src view.
cp "$pipeline" "$work/local-pipeline.bsh"
printf '\n# operator-local edit\n' >> "$work/local-pipeline.bsh"
"$BASHY_BIN" commands set transcribe-pipeline --set "exec.2=$work/local-pipeline.bsh" >/dev/null
"$BASHY_BIN" commands show transcribe-pipeline --yaml > "$work/override-source.txt"
grep -F "$work/local-pipeline.bsh" "$work/override-source.txt" >/dev/null
bashy transcribe $common --output "$work/override.txt" "$STUB_MEDIA" > "$work/override-path.txt"
cmp "$work/direct.txt" "$work/override.txt"
printf 'existing output\n' > "$work/failed.txt"
# shellcheck disable=SC2086
if STUB_BAD_INFER=1 bashy transcribe $common --output "$work/failed.txt" "$STUB_MEDIA" > "$work/failed-path.txt"; then
    echo 'invalid infer result was accepted' >&2
    exit 1
fi
test "$(cat "$work/failed.txt")" = 'existing output'
test ! -s "$work/failed-path.txt"
set -- "$work"/.transcribe.*
test ! -e "$1"
"$BASHY_BIN" tail -n 1 "$ledger" | "$BASHY_BIN" jq -e '.attest.Valid == false' >/dev/null
printf 'stub orientations PASS; same body receipt, skill receipt, editable source override\n'
