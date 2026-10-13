#!/usr/bin/env bash
# Deliberate release hold. No environment flag may turn an unmeasured gate green.
set -euo pipefail
echo 'BLOCKED: five native health/load/completion/embedding/cancellation/shutdown and no-egress proofs are not implemented or measured.' >&2
exit 1
