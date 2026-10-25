#!/bin/sh
# Ordinary command records: no core registration or evaluation of user paths.
set -eu
: "${BASHY_BIN:=bashy}"
if [ "$#" -ne 1 ]; then
    echo 'usage: register.sh ABSOLUTE-PATH-TO-TRANSCRIBE-ADAPTER' >&2
    exit 2
fi
helper=$1
absolute_file() {
    case "$1" in
        /*|[A-Za-z]:/*|[A-Za-z]:\\*|\\\\*) ;;
        *) echo "expected absolute executable path: $1" >&2; return 2 ;;
    esac
    test -f "$1" || { echo "executable does not exist: $1" >&2; return 2; }
}
check_tool() {
    absolute_file "$1"
    case "$2" in
        *[!0-9a-fA-F]*|'') echo 'reviewed executable SHA-256 is required' >&2; return 2 ;;
    esac
    test "${#2}" -eq 64 || { echo 'SHA-256 must contain 64 hexadecimal digits' >&2; return 2; }
}
# Validate all optional deployment arguments before creating any records.
absolute_file "$helper"
if [ -n "${WHISPER_PATH:-}${WHISPER_SHA256:-}" ]; then
    check_tool "${WHISPER_PATH:-}" "${WHISPER_SHA256:-}"
fi
if [ -n "${FFMPEG_PATH:-}${FFMPEG_SHA256:-}" ]; then
    check_tool "${FFMPEG_PATH:-}" "${FFMPEG_SHA256:-}"
fi

# OS/architecture support is a declaration, not native execution evidence.
# Tool/model verification stays in the helper; nothing is downloaded here.
add() {
    name=$1 role=$2 operation=$3
    shift 3
    "$BASHY_BIN" commands add "$name" \
        --set "exec.0=$helper" --set "exec.1=$operation" \
        --set effects.0=read --set effects.1=exec \
        --set os.0=linux --set os.1=darwin --set os.2=windows \
        --set requires=arch=amd64,arm64 \
        --set "provides.0=transcribe.$role" \
        --set accepts.0.path=/version --set 'accepts.0.pattern=^bashy-transcribe-stage-v1$' \
        "$@"
}
add transcribe-local acquire local \
    --set accepts.1.path=/input \
    --set 'accepts.1.pattern=(?s)^([/\\].*|[A-Za-z]:[/\\].*|[^:/\\]+([/\\].*)?)$'
add transcribe-wav decode wav \
    --set effects.2=write --set effects.3=destroy \
    --set accepts.1.path=/artifacts/0/path --set 'accepts.1.pattern=(?i)\.wav$'
add transcribe-export export export \
    --set effects.2=write --set effects.3=destroy \
    --set accepts.1.path=/format --set 'accepts.1.pattern=^(txt|srt|vtt)$'
"$BASHY_BIN" commands add transcribe-model-setup \
    --set "exec.0=$helper" --set exec.1=model \
    --set effects.0=read --set effects.1=write --set effects.2=destroy \
    --set effects.3=exec --set effects.4=net \
    --set os.0=linux --set os.1=darwin --set os.2=windows \
    --set requires=arch=amd64,arm64
if [ -n "${WHISPER_PATH:-}" ]; then
    add transcribe-whisper infer --whisper \
        --set "exec.2=$WHISPER_PATH" --set exec.3=--whisper-sha256 \
        --set "exec.4=$WHISPER_SHA256" --set exec.5=whisper \
        --set effects.2=write --set effects.3=destroy \
        --set accepts.1.path=/artifacts/0/media_type --set 'accepts.1.pattern=^audio/wav$'
else
    add transcribe-whisper infer whisper \
        --set effects.2=write --set effects.3=destroy \
        --set accepts.1.path=/artifacts/0/media_type --set 'accepts.1.pattern=^audio/wav$'
fi
# Opt-in only after independent decoder policy/provenance admission. A digest
# is byte identity, not admission. Installing this record grants no exception.
if [ -n "${FFMPEG_PATH:-}" ]; then
    add transcribe-ffmpeg decode --ffmpeg \
        --set "exec.2=$FFMPEG_PATH" --set exec.3=--ffmpeg-sha256 \
        --set "exec.4=$FFMPEG_SHA256" --set exec.5=ffmpeg \
        --set effects.2=write --set effects.3=destroy \
        --set accepts.1.path=/artifacts/0/path --set 'accepts.1.pattern=(?s)^.+$'
fi
