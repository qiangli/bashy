#!/bin/bash
set -euo pipefail

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
ok=1

say() { printf '%s\n' "$*"; }

say "repo=$repo"

if [[ -x "$repo/bin/bashy" ]]; then
  say "bashy=$("$repo/bin/bashy" --version | sed -n '1p')"
else
  say "missing: $repo/bin/bashy"
  ok=0
fi

if [[ -n "${GNU_BASH53:-}" && -x "${GNU_BASH53:-}" ]]; then
  gnu_version=$("$GNU_BASH53" --version | sed -n '1p')
  say "gnu_bash53=$GNU_BASH53 :: $gnu_version"
  case "$gnu_version" in
    *"GNU bash, version 5.3"*) ;;
    *)
      say "invalid: GNU_BASH53 must be GNU Bash 5.3"
      ok=0
      ;;
  esac
else
  say "missing: set GNU_BASH53 to a real GNU Bash 5.3 binary for the control arm"
  ok=0
fi

for tool in codex claude agy opencode aider; do
  if command -v "$tool" >/dev/null 2>&1; then
    case "$tool" in
      codex) version=$(codex --version 2>/dev/null || true) ;;
      claude) version=$(claude --version 2>/dev/null || true) ;;
      agy) version=$(agy --version 2>/dev/null || true) ;;
      opencode) version=$(opencode --version 2>/dev/null || true) ;;
      aider) version=$(aider --version 2>/dev/null || true) ;;
    esac
    say "tool:$tool available ${version:-unknown-version}"
  else
    say "tool:$tool missing"
  fi
done

if [[ "$ok" -eq 1 ]]; then
  say "preflight=pass"
else
  say "preflight=fail"
fi

exit "$((1 - ok))"
