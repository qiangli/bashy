#!/usr/bin/env bashy
# Check the selected Python executable before starting the YouTube adapter.
# This is a local byte check; release signing and host installation are separate.
set -euo pipefail

fail() { printf 'Python launcher rejected: %s\n' "$1" >&2; exit 1; }
[ "$#" -eq 1 ] || fail 'expected one acquire request path'
[ -n "${BASHY_TRANSCRIBE_PYTHON:-}" ] || fail 'BASHY_TRANSCRIBE_PYTHON is required'

script_source=${BASH_SOURCE[0]}
case "$(uname -s)" in
    Windows*|MINGW*|MSYS*)
        # Bashy receives a native drive path on Windows. POSIX dirname treats
        # its backslashes as literal characters and would resolve the cwd.
        script_source=${script_source//\\//}
        script_dir=$(dirname -- "$script_source") ;;
    *) script_dir=$(cd -- "$(dirname -- "$script_source")" && pwd -P) ;;
esac
policy=$script_dir/dependencies/external-python-runtime.json
policy_sha256=bcd13fd6844b079f99f400b54bbe1c844d52ebb66579a6baeb35df5d42044a75
actual_policy_hash=$(sha256sum -- "$policy") || fail 'cannot hash Python policy'
[ "${actual_policy_hash%% *}" = "$policy_sha256" ] || fail 'Python policy bytes changed'

case "$(uname -s)/$(uname -m)" in
    Linux/x86_64|Linux/amd64) target=linux/amd64 ;;
    Linux/aarch64|Linux/arm64) target=linux/arm64 ;;
    Darwin/x86_64|Darwin/amd64) target=darwin/amd64 ;;
    Darwin/arm64|Darwin/aarch64) target=darwin/arm64 ;;
    Windows*/x86_64|Windows*/amd64|MINGW*/x86_64|MSYS*/x86_64) target=windows/amd64 ;;
    Windows*/aarch64|Windows*/arm64|MINGW*/aarch64|MSYS*/aarch64) target=windows/arm64 ;;
    *) fail 'no reviewed OS/architecture target' ;;
esac

jq -e '.schema == "s503-external-python-runtime-v1" and
    (.targets | keys == ["darwin/amd64", "darwin/arm64", "linux/amd64",
                        "linux/arm64", "windows/amd64", "windows/arm64"])' \
    "$policy" >/dev/null || fail 'Python policy lacks six reviewed targets'

python=$(realpath -- "$BASHY_TRANSCRIBE_PYTHON") || fail 'selected Python path is absent'
[ -f "$python" ] || fail 'selected Python is not a regular file'
case "$python" in
    /*|[A-Za-z]:/*|[A-Za-z]:\\*) ;;
    *) fail 'selected Python path is not absolute' ;;
esac

if [ -n "${BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX:-}" ]; then
    [ "$target" = linux/amd64 ] || [ "$target" = linux/arm64 ] ||
        fail 'source receipt is supported only on Linux'
    prefix=$(realpath -- "$BASHY_TRANSCRIBE_PYTHON_SOURCE_PREFIX") ||
        fail 'source prefix is absent'
    receipt=$prefix/source-build-receipt.json
    [ -f "$receipt" ] || fail 'source receipt is absent'
    jq -e '
        .schema == "s503-linux-python-source-setup-v1" and
        .status == "local_source_build_unadmitted" and
        .source.url == "https://www.python.org/ftp/python/3.12.15/Python-3.12.15.tar.xz" and
        .source.sha256 == "c2c4321961fab0fb999d66e0cecf521c2ab3994c7992873ea99e306c1094fd5a" and
        .source.signature_sha256 == "e80124b35dabf82264f254dbb291697df96768f6e4ee0e43d7e8f110a9ea50b6" and
        .source.release_key_fingerprint == "7169605F62C751356D054A26A821E680E5FA6305"' \
        "$receipt" >/dev/null || fail 'source receipt does not match reviewed setup'
    [ "$(jq -er '.target' "$receipt")" = "$target" ] || fail 'source target differs from host'
    [ "$(jq -er '.prefix' "$receipt")" = "$prefix" ] || fail 'source prefix differs from receipt'
    [ "$(jq -er '.executable' "$receipt")" = "$python" ] || fail 'source executable differs from receipt'
    expected=$(jq -er '.installed_files["bin/python3.12"].sha256' "$receipt") ||
        fail 'source receipt lacks executable hash'
    [ "$python" = "$prefix/bin/python3.12" ] || fail 'source executable is outside prefix'
else
    expected=$(jq -er ".targets[\"$target\"].python_sha256" "$policy") ||
        fail 'target Python hash is absent'
fi

case "$expected" in
    *[!0123456789abcdef]*|'') fail 'expected Python hash is invalid' ;;
esac
[ "${#expected}" -eq 64 ] || fail 'expected Python hash is invalid'
actual=$(sha256sum -- "$python") || fail 'cannot hash selected Python'
[ "${actual%% *}" = "$expected" ] ||
    fail "selected Python SHA-256 differs from reviewed bytes (actual=${actual%% *}; expected=$expected; path=$python)"

exec "$python" "$script_dir/source_youtube.py" "$1"
