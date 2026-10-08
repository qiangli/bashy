#!/bin/sh
# Regression for the meet-SPA freshness gate (scripts/build-meet-spa.sh).
#
# Proves the four behaviours the stale-artifact bug demands, hermetically —
# no network, no real SPA build, and NEVER touching the tracked sibling
# artifact:
#   1. fresh accepted        — a dist identical to the artifact passes.
#   2. stale rejected + kept  — a divergent artifact is refused (both a byte
#                               mismatch and a file-set mismatch), and the
#                               comparison leaves the artifact byte-for-byte
#                               unchanged (a gate that repaired what it checks
#                               would hide the drift).
#   3. missing toolchain fails closed — `check` with no node/pnpm/corepack/bashy
#                               exits non-zero instead of green-lighting an
#                               unverifiable bundle.
#   4. standalone checkout — with no ../yoke, `check` resolves the yoke module
#                               pinned by go.mod and compares its artifact.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
script=$root/scripts/build-meet-spa.sh

tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-meet-spa-fresh.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "test-meet-spa-fresh: FAIL: $*" >&2
	exit 1
}

# cksum of every file in a tree, path-qualified and stable-sorted — a fingerprint
# that changes if any byte, any file name, or the file set changes. Avoids
# `find -exec`, which the pure-Go coreutils userland deliberately does not
# implement, so this runs identically under bashy and GNU find.
fingerprint() {
	( cd "$1" && find . -type f | LC_ALL=C sort | while IFS= read -r f; do
		cksum "$f"
	done )
}

# ---- fixture: a fresh dist and a tracked artifact that match exactly ----
dist=$tmp/dist
art=$tmp/artifact
mkdir -p "$dist/assets" "$art"
printf '<!doctype html><html><head></head><body>meet</body></html>\n' >"$dist/index.html"
printf 'console.log("app");\n' >"$dist/assets/app-DEADBEEF.js"
printf 'body{color:#000}\n' >"$dist/assets/app-DEADBEEF.css"
cp -R "$dist/." "$art/"

# ---- 1. fresh accepted ----
if ! sh "$script" compare "$dist" "$art" >"$tmp/1.out" 2>&1; then
	cat "$tmp/1.out" >&2
	fail "a fresh, matching artifact was rejected"
fi
echo "test-meet-spa-fresh: [1/4] fresh matching artifact accepted"

# ---- 2a. stale rejected (byte mismatch) + artifact unchanged ----
printf 'console.log("STALE — old build");\n' >"$art/assets/app-DEADBEEF.js"
before=$(fingerprint "$art")
[ -n "$before" ] || fail "artifact fingerprint was empty before the byte-mismatch check"
if sh "$script" compare "$dist" "$art" >"$tmp/2a.out" 2>&1; then
	fail "a byte-divergent (stale) artifact was accepted"
fi
grep -q 'STALE' "$tmp/2a.out" || fail "stale rejection lacked an actionable STALE diagnostic"
after=$(fingerprint "$art")
[ -n "$after" ] || fail "artifact fingerprint was empty after the byte-mismatch check"
[ "$before" = "$after" ] || fail "the non-mutating check changed the artifact tree"
echo "test-meet-spa-fresh: [2/4] stale (byte) artifact rejected and left unchanged"

# restore parity, then diverge the FILE SET
cp -R "$dist/." "$art/"
printf 'orphan\n' >"$art/assets/orphan.js"
before=$(fingerprint "$art")
[ -n "$before" ] || fail "artifact fingerprint was empty before the file-set check"
if sh "$script" compare "$dist" "$art" >"$tmp/2b.out" 2>&1; then
	fail "a file-set-divergent artifact was accepted"
fi
after=$(fingerprint "$art")
[ -n "$after" ] || fail "artifact fingerprint was empty after the file-set check"
[ "$before" = "$after" ] || fail "the non-mutating check changed the artifact tree (file-set case)"
echo "test-meet-spa-fresh: [3/4] stale (file-set) artifact rejected and left unchanged"

# ---- 3. missing toolchain fails closed ----
# A fake web_dir with a lockfile so the script reaches its toolchain probe, and a
# PATH scrubbed of every pnpm provider (node, pnpm, corepack, bashy). Invoke the
# shell by absolute path, because the checked script reaches the expected
# toolchain failure using shell builtins only; PATH can therefore contain just
# the empty fixture directory on every supported host, including Linux.
fakeweb=$tmp/web
mkdir -p "$fakeweb"
printf '{"name":"meet-web","packageManager":"pnpm@11.17.0"}\n' >"$fakeweb/package.json"
printf 'lockfileVersion: 9.0\n' >"$fakeweb/pnpm-lock.yaml"

nobin=$tmp/nobin
mkdir -p "$nobin"
# Prefix-assignment (not `env -i`, which the pure-Go coreutils userland refuses
# to use to run a command): PATH contains only the empty fixture directory, and
# BASHY_BIN is empty so no explicit pnpm provider is available.
if PATH="$nobin" BASHY_BIN= MEET_SPA_WEB_DIR="$fakeweb" \
	/bin/sh "$script" check >"$tmp/3.out" 2>&1; then
	cat "$tmp/3.out" >&2
	fail "check passed with no toolchain available (must fail closed)"
fi
grep -q 'requires node and pnpm' "$tmp/3.out" || {
	cat "$tmp/3.out" >&2
	fail "toolchain-missing check did not fail on the toolchain (env not scrubbed?)"
}
echo "test-meet-spa-fresh: [4/4] check with no toolchain failed closed"

# ---- 4. a standalone clone resolves the yoke module pin, not ../yoke ----
# A tiny fake Go tool returns a fixture module directory. Fake node/pnpm build a
# matching dist, so this verifies the resolver without network or a real SPA.
pinned=$tmp/pinned-yoke
pinned_web=$pinned/pkg/meet/web
pinned_art=$pinned/pkg/meet/artifact
mkdir -p "$pinned_web" "$pinned_art/assets" "$tmp/bin"
printf '{"name":"meet-web","packageManager":"pnpm@11.17.0"}\n' >"$pinned_web/package.json"
printf 'lockfileVersion: 9.0\n' >"$pinned_web/pnpm-lock.yaml"
printf '<!doctype html><html><head></head><body>pinned</body></html>\n' >"$pinned_art/index.html"
printf 'console.log("pinned");\n' >"$pinned_art/assets/app.js"
cat >"$tmp/bin/go" <<EOF
#!/bin/sh
printf '{"Dir":"%s"}\\n' '$pinned'
EOF
cat >"$tmp/bin/node" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$tmp/bin/pnpm" <<EOF
#!/bin/sh
case "\$1" in
build) mkdir -p dist; cp -R '$pinned_art/.' dist/ ;;
esac
EOF
chmod +x "$tmp/bin/go" "$tmp/bin/node" "$tmp/bin/pnpm"
# Run from a directory with no ../yoke sibling: the umbrella checkout has one,
# so running in place would never exercise the standalone fallback.
mkdir -p "$tmp/standalone/bashy"
if ! (cd "$tmp/standalone/bashy" && PATH="$tmp/bin:/bin:/usr/bin" BASHY_BIN= /bin/sh "$script" check) >"$tmp/4.out" 2>&1; then
	cat "$tmp/4.out" >&2
	fail "standalone check did not resolve yoke from the go.mod pin"
fi
grep -q 'using pinned yoke module' "$tmp/4.out" || fail "standalone check did not report its pinned-yoke fallback"
echo "test-meet-spa-fresh: [5/5] standalone check resolves the pinned yoke module"

echo "test-meet-spa-fresh: PASS"
