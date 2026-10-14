#!/bin/sh
# Hermetic regression for scripts/fetch-signed-runner.sh: proves fail-closed
# wiring (arg validation, pin extraction, tamper refusal, verifier refusal)
# without network. Uses a fixture yoke dir, file:// fixtures and a stub
# cosign (COSIGN_BIN + ALLOW_UNPINNED_COSIGN=1 test seam) that records its
# exact argv. Real Sigstore verification is proven by a live release run,
# not here.
set -eu
script_dir=$(dirname -- "$0")
root=$(cd -- "${script_dir}/.." && pwd)
fetch=$root/scripts/fetch-signed-runner.sh

pass=0 fail=0
ok() { pass=$((pass + 1)); echo "ok: $1"; }
bad() { fail=$((fail + 1)); echo "FAIL: $1"; }

# make_fixture DIR: fixture yoke checkout + fixture release files; prints
# the fixture exe sha256. Globals set: YOKE, SRV, STUB, STUBLOG, EXE_SHA.
make_fixture() {
  dir=$1
  YOKE=$dir/yoke
  SRV=$dir/srv
  mkdir -p "$YOKE/external/ollama/runner_embed" "$YOKE/pkg/binmgr/sigstore" "$SRV"
  printf 'test-runner-executable-bytes' >"$dir/exe"
  EXE_SHA=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$dir/exe")
  python3 - "$dir/exe" "$SRV/runner-darwin-arm64.gz" <<'EOF'
import gzip, sys
with open(sys.argv[1], 'rb') as f, gzip.GzipFile(sys.argv[2], 'wb', mtime=0) as g:
    g.write(f.read())
EOF
  printf 'fixture-gz-bundle' >"$SRV/runner-darwin-arm64.gz.sigstore.json"
  printf 'fixture-exe-bundle' >"$SRV/runner-darwin-arm64.sigstore.json"
  cat >"$YOKE/external/ollama/runner_embed/extract.go" <<EOF
package runner_embed
const (
  releaseVersion  = "bashy-v0.21.2-15"
  releaseIdentity = "https://github.com/qiangli/ollama/.github/workflows/bashy-build.yml@refs/tags/bashy-v0.21.2-15"
)
var releaseSHA256 = map[string]string{
"linux/amd64":   "",
"linux/arm64":   "",
"darwin/amd64":  "",
"darwin/arm64":  "$EXE_SHA",
"windows/amd64": "",
}
EOF
  printf '{"fixture":"trusted-root"}' >"$YOKE/pkg/binmgr/sigstore/trusted_root.json"
  ROOT_SHA=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$YOKE/pkg/binmgr/sigstore/trusted_root.json")
  cat >"$YOKE/pkg/binmgr/sigstore.go" <<EOF
package binmgr
const trustedRootSHA256 = "$ROOT_SHA"
EOF
  printf 'package binmgr\n' >"$YOKE/pkg/binmgr/pins.go"
  STUB=$dir/cosign-stub
  STUBLOG=$dir/cosign-argv.log
  cat >"$STUB" <<EOF
#!/bin/sh
printf '%s\n' "\$*" >>"$STUBLOG"
[ "\${STUB_FAIL:-0}" = "1" ] && exit 1
exit 0
EOF
  chmod +x "$STUB"
}

run_fetch() { # $1 = case dir; $@ = script args
  casedir=$1; shift
  export YOKE_DIR="$YOKE" SIGNED_RUNNER_CACHE="$casedir/cache"
  export RUNNER_BASE_URL="file://$SRV"
  export COSIGN_BIN="$STUB" ALLOW_UNPINNED_COSIGN=1 TMPDIR="$casedir/tmp"
  mkdir -p "$casedir/cache" "$casedir/tmp"
  "$fetch" "$@"
}

# 1. Happy path (two-arg form): verifies, installs, calls cosign correctly.
t=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t/fx"
if run_fetch "$t/c1" darwin arm64 >"$t/c1.log" 2>&1; then
  cmp -s "$t/fx/srv/runner-darwin-arm64.gz" "$YOKE/external/ollama/runner_embed/ycode-runner.gz" \
    && cmp -s "$t/fx/srv/runner-darwin-arm64.sigstore.json" "$YOKE/external/ollama/runner_embed/ycode-runner.sigstore.json" \
    && ok "happy path installs verified pair" || bad "happy path output mismatch"
  grep -q "certificate-identity https://github.com/qiangli/ollama/.github/workflows/bashy-build.yml@refs/tags/bashy-v0.21.2-15" "$STUBLOG" \
    && grep -q "certificate-oidc-issuer https://token.actions.githubusercontent.com" "$STUBLOG" \
    && grep -q "\-\-offline" "$STUBLOG" \
    && [ "$(grep -c verify-blob "$STUBLOG")" -eq 2 ] \
    && ok "stub cosign got exact identity/issuer/offline twice" || bad "stub cosign argv wrong"
else
  bad "happy path failed: $(cat "$t/c1.log")"
fi

# 1b. Single-arg form accepted.
t2=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t2/fx"
if run_fetch "$t2/c1" darwin_arm64 >"$t2/c1.log" 2>&1; then ok "single-arg form accepted"; else bad "single-arg form failed"; fi

# 2a. Tampered payload (valid gzip of different bytes): refused, nothing installed.
t3=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t3/fx"
printf 'tampered-runner-bytes' >"$t3/other"
python3 - "$t3/other" "$t3/fx/srv/runner-darwin-arm64.gz" <<'EOF'
import gzip, sys
with open(sys.argv[1], 'rb') as f, gzip.GzipFile(sys.argv[2], 'wb', mtime=0) as g:
    g.write(f.read())
EOF
rm -f "$YOKE/external/ollama/runner_embed/ycode-runner.gz"
if run_fetch "$t3/c" darwin arm64 >"$t3/c.log" 2>&1; then
  bad "tampered payload accepted"
else
  [ ! -e "$YOKE/external/ollama/runner_embed/ycode-runner.gz" ] \
    && ok "tampered payload refused, nothing installed" || bad "tampered payload left output"
fi

# 2b. Corrupt gzip (truncated): refused.
t3b=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t3b/fx"
python3 - "$t3b/fx/srv/runner-darwin-arm64.gz" <<'EOF'
import sys
data = open(sys.argv[1], 'rb').read()
open(sys.argv[1], 'wb').write(data[:len(data) // 2])
EOF
if run_fetch "$t3b/c" darwin arm64 >"$t3b/c.log" 2>&1; then
  bad "corrupt gzip accepted"
else
  ok "corrupt gzip refused"
fi

# 3. Pinned-sha mismatch (fixture pin is zeros): refused.
t4=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t4/fx"
sed -i.bak "s/$EXE_SHA/0000000000000000000000000000000000000000000000000000000000000000/" "$YOKE/external/ollama/runner_embed/extract.go"
if run_fetch "$t4/c" darwin arm64 >"$t4/c.log" 2>&1; then
  bad "pin mismatch accepted"
else
  ok "pin mismatch refused"
fi

# 4. Empty pin (unfilled yoke): refused before any download.
t5=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t5/fx"
sed -i.bak "s/\"darwin\/arm64\":  \"$EXE_SHA\"/\"darwin\/arm64\":  \"\"/" "$YOKE/external/ollama/runner_embed/extract.go"
if run_fetch "$t5/c" darwin arm64 >"$t5/c.log" 2>&1; then
  bad "empty pin accepted"
else
  ok "empty pin refused"
fi

# 5. Unsupported target: refused.
t6=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t6/fx"
if run_fetch "$t6/c" plan9 arm64 >"$t6/c.log" 2>&1; then
  bad "unsupported target accepted"
else
  ok "unsupported target refused"
fi

# 6. Verifier rejection fails closed.
t7=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t7/fx"
export STUB_FAIL=1
if run_fetch "$t7/c" darwin arm64 >"$t7/c.log" 2>&1; then
  bad "verifier rejection accepted"
else
  [ ! -e "$YOKE/external/ollama/runner_embed/ycode-runner.gz" ] \
    && ok "verifier rejection refused, nothing installed" || bad "verifier rejection left output"
fi
unset STUB_FAIL

# 7. Wrong yoke identity: refused.
t8=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t8/fx"
sed -i.bak 's#qiangli/ollama#attacker/ollama#' "$YOKE/external/ollama/runner_embed/extract.go"
if run_fetch "$t8/c" darwin arm64 >"$t8/c.log" 2>&1; then
  bad "wrong identity accepted"
else
  ok "wrong identity refused"
fi

# 8. Tampered trusted root: refused.
t9=$(mktemp -d "${TMPDIR:-/tmp}/fr-test.XXXXXX")
make_fixture "$t9/fx"
printf 'tampered' >>"$YOKE/pkg/binmgr/sigstore/trusted_root.json"
if run_fetch "$t9/c" darwin arm64 >"$t9/c.log" 2>&1; then
  bad "tampered trusted root accepted"
else
  ok "tampered trusted root refused"
fi

echo "---"
echo "pass=$pass fail=$fail"
[ "$fail" -eq 0 ]
