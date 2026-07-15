#!/usr/bin/env bashy
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "$0")" && pwd)"

cat >&2 <<'EOF'
ci-failure-fixer: this entrypoint is deprecated.
Delegating to ci-failure-router.sh, which now performs deterministic claiming
and launches the on-shift fixer.
EOF

exec "$script_dir/ci-failure-router.sh" "$@"
