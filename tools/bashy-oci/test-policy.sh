#!/bin/sh
# Offline structural tests for the reusable image contract. This deliberately
# does not contact a registry or invoke an OCI daemon.
set -eu

repo=$(CDPATH= cd -P "$(dirname "$0")/../.." && pwd)
recipe=$repo/tools/bashy-oci/Containerfile
builder=$repo/tools/bashy-oci/build-smoke.sh
makefile=$repo/Makefile

fail() { echo "test-bashy-oci-policy: $*" >&2; exit 1; }
has() { grep -Fq "$1" "$2" || fail "$2 lacks required policy: $1"; }

[ -f "$recipe" ] || fail "missing Containerfile"
[ -x "$builder" ] || fail "build-smoke.sh is not executable"

[ "$(grep -c '^FROM ' "$recipe")" -eq 2 ] || fail "recipe must have builder and runtime stages"
has 'ubuntu@sha256:' "$recipe"
has 'ARG APT_SNAPSHOT=20260821T190000Z' "$recipe"
has 'apt-get update --snapshot "${APT_SNAPSHOT}"' "$recipe"
has 'make build-bashy' "$recipe"
has 'COPY --from=builder --chown=0:0 /src/bashy/bin/bashy /bashy' "$recipe"
has 'COPY --from=builder --chown=0:0 /src/bashy/bin/bashy.real /bashy.real' "$recipe"
has 'ENTRYPOINT ["/bashy"]' "$recipe"
has 'OTEL_TRACES_EXPORTER=none' "$recipe"
has 'ln -s /bashy /opt/bashy/bin/sh' "$recipe"
has 'test ! /bin/sh -ef /bashy' "$recipe"
has 'USER 10001:10001' "$recipe"

if grep -Eq 'ln .* /bin/sh([ ;]|$)|rm .* /bin/sh([ ;]|$)|COPY .* /bin/sh([ ;]|$)' "$recipe"; then
  fail "generic base must not replace Ubuntu /bin/sh"
fi
if grep -Fq 'vsc-pcts-harness-kit' "$recipe" "$builder"; then
  fail "open-source image tooling must not depend on the proprietary harness"
fi

has 'stage_tree "$repo" "$context/bashy"' "$builder"
has 'stage_tree "$parent/coreutils" "$context/coreutils"' "$builder"
has 'stage_tree "$parent/sh" "$context/sh"' "$builder"
has 'stage_tree "$parent/readline" "$context/readline"' "$builder"
has 'git ls-files --cached --others --exclude-standard -z' "$builder"
has 'localhost/bashy-base:ubuntu24.04' "$builder"
has 'platform=linux/$native_arch' "$builder"
has 'test-bashy-oci-policy:' "$makefile"
has 'build-bashy-oci:' "$makefile"
has 'smoke-bashy-oci:' "$makefile"

sh -n "$builder"
sh -n "$repo/tools/bashy-oci/test-policy.sh"
echo 'test-bashy-oci-policy: PASS'
