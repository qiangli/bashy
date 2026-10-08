#!/bin/bash
# Verify that the bashy closure contains only permissive dependencies:
# 1. No yamux (MPL-2.0 tunnel dependency removed in Sprint 368)
# 2. No outpost in the binary's dependency closure
# 3. No non-permissive tree-sitter grammars (dropped in Sprint 369: caddy, disassembly, jq, ebnf, nim)
# 4. License gate passes on the compiled binary via bashy release sbom
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

echo "==> Verifying dependency closure for cmd/bashy, cmd/bash, cmd/sh..."

for target in ./cmd/bashy ./cmd/bash ./cmd/sh; do
  echo "Checking $target dependencies..."
  deps=$(go list -deps "$target")

  # 1. No yamux
  if echo "$deps" | grep -E "yamux" >/dev/null; then
    echo "ERROR: $target closure contains prohibited MPL tunnel dependency 'yamux':" >&2
    echo "$deps" | grep -E "yamux" >&2
    exit 1
  fi

  # 2. No outpost
  if echo "$deps" | grep -E "^github\.com/qiangli/outpost(/.*)?$" >/dev/null; then
    echo "ERROR: $target closure contains prohibited 'outpost' dependency:" >&2
    echo "$deps" | grep -E "^github\.com/qiangli/outpost(/.*)?$" >&2
    exit 1
  fi

  # 3. No non-permissive grammars dropped in Sprint 369
  for grammar in caddy disassembly tree_sitter_jq tree_sitter_ebnf tree_sitter_nim; do
    if echo "$deps" | grep -E "(^|/)$grammar(/|$)" >/dev/null; then
      echo "ERROR: $target closure contains non-permissive grammar '$grammar':" >&2
      echo "$deps" | grep -E "(^|/)$grammar(/|$)" >&2
      exit 1
    fi
  done
done

echo "==> Dependency closure is clean: no yamux, no outpost, no non-permissive grammars."

# 4. Verify SBOM license gate on a built binary
echo "==> Verifying SBOM license classification and gate..."
bin="${1:-}"
if [[ -z "$bin" ]]; then
  if [[ -x bin/bashy ]]; then
    bin=bin/bashy
  elif [[ -x bin/bashy.exe ]]; then
    bin=bin/bashy.exe
  else
    tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/bashy-closure-check.XXXXXX")
    trap 'rm -rf "$tmpdir"' EXIT
    bin="$tmpdir/bashy"
    go build -o "$bin" ./cmd/bashy
  fi
fi

go run ./cmd/bashy release sbom --check-only --gate "$bin"
echo "==> Permissive closure verification PASSED."
