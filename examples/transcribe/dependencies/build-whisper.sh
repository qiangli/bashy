#!/usr/bin/env bash
# Platform-aware whisper.cpp v1.9.5 source build + cache recipe for POSIX runners
# Supports: ubuntu-24.04 (linux/amd64), ubuntu-24.04-arm (linux/arm64),
#           macos-15-intel (darwin/amd64), macos-15 (darwin/arm64)
#
# Sprint #503 / Story #2742 / Story-ID 74ed924ba244

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PYTHON="${PYTHON:-python3}"

if ! command -v "$PYTHON" >/dev/null 2>&1; then
    echo "ERROR: python3 required on runner" >&2
    exit 1
fi

exec "$PYTHON" "$SCRIPT_DIR/build_whisper.py" "$@"
