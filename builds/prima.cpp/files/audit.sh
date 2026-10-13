#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
src=${1:?source}
mkdir -p out
python3 "$here/merge-inputs.py" native-inputs audit-inputs out/prima-inputs.json
python3 -m venv audit-venv
audit-venv/bin/pip install 'scancode-toolkit==32.5.0'
audit-venv/bin/scancode --license --license-text --copyright --package --processes 2 \
  --json-pp out/prima-licenses.json audit-inputs
python3 "$here/audit.py"
# Pinned audit tool, built from source; it is not a shipped runtime dependency.
GOTOOLCHAIN=go1.27.2 GOBIN="$PWD/audit-bin" go install github.com/anchore/grype/cmd/grype@v0.120.1
"$PWD/audit-bin/grype" sbom:out/prima-sbom.cdx.json -o json --fail-on high >out/prima-vulnerabilities.json

{
  cat "$src/LICENSE" "$src/deps/nng/LICENSE.txt"
  head -n 22 "$src/common/json.hpp"
} >out/prima-NOTICES.txt
