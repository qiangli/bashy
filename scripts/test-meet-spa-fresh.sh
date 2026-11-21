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
#   4. bashy tier — an explicit bashy binary supplies pnpm without system Node.
#   5. standalone checkout — with no ../yoke, `check` resolves the yoke module
#                               pinned by go.mod and compares its artifact.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
script=$root/scripts/build-meet-spa.sh
self_container=$root/scripts/test-self-container.sh

tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-meet-spa-fresh.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "test-meet-spa-fresh: FAIL: $*" >&2
	exit 1
}

# The warm run must use a shared store online; the test run must consume it
# offline. pnpm 11 reads pnpm_config_* while older releases read npm_config_*.
for config in pnpm npm; do
	[ "$(grep -c -- "-e ${config}_config_store_dir=/cache/pnpm-store" "$self_container")" -eq 2 ] ||
		fail "both container runs must export ${config}_config_store_dir"
	grep -q -- "-e ${config}_config_offline=false" "$self_container" ||
		fail "warm run must export ${config}_config_offline=false"
	grep -q -- "-e ${config}_config_offline=true" "$self_container" ||
		fail "test run must export ${config}_config_offline=true"
done

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
echo "test-meet-spa-fresh: [1/7] fresh matching artifact accepted"

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
echo "test-meet-spa-fresh: [2/7] stale (byte) artifact rejected and left unchanged"

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
echo "test-meet-spa-fresh: [3/7] stale (file-set) artifact rejected and left unchanged"

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
# BASHY_BIN is empty so no explicit pnpm provider is available. Run from the
# empty fixture directory so the script's repo-relative bin/bashy fallback
# cannot find a lane-built binary (make test builds bin/bashy first).
if (cd "$nobin" && PATH="$nobin" BASHY_BIN= MEET_SPA_WEB_DIR="$fakeweb" \
	/bin/sh "$script" check) >"$tmp/3.out" 2>&1; then
	cat "$tmp/3.out" >&2
	fail "check passed with no toolchain available (must fail closed)"
fi
grep -q 'requires node and pnpm' "$tmp/3.out" || {
	cat "$tmp/3.out" >&2
	fail "toolchain-missing check did not fail on the toolchain (env not scrubbed?)"
}
echo "test-meet-spa-fresh: [4/7] check with no toolchain failed closed"

# A fake bashy implements only pnpm. With no node, pnpm or corepack on PATH,
# check must select the provisioned tier and still reject a stale artifact.
cat >"$tmp/fake-bashy" <<EOF
#!/bin/sh
[ "\$1" = pnpm ] || exit 2
shift
case "\$1" in
--version) echo 11.17.0 ;;
install) exit 0 ;;
build) mkdir -p dist; cp -R '$dist/.' dist/ ;;
*) exit 2 ;;
esac
EOF
chmod +x "$tmp/fake-bashy"
rm -rf "$art"
mkdir -p "$art"
cp -R "$dist/." "$fakeweb/../artifact/"
if ! PATH="/bin:/usr/bin" BASHY_BIN="$tmp/fake-bashy" MEET_SPA_WEB_DIR="$fakeweb" \
	/bin/sh "$script" check >"$tmp/bashy.out" 2>&1; then
	cat "$tmp/bashy.out" >&2
	fail "bashy-provisioned tier did not pass"
fi
grep -q 'using bashy-provisioned pnpm' "$tmp/bashy.out" || fail "bashy tier was not selected"
echo "test-meet-spa-fresh: [5/7] bashy-provisioned pnpm selected without system Node"

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
echo "test-meet-spa-fresh: [6/7] standalone check resolves the pinned yoke module"

# ---- 5. optional skips when content-fresh, rebuilds on edit; required never skips ----
opt_root=$tmp/opt-case
opt_web=$opt_root/web
opt_art=$opt_root/artifact
opt_bin=$tmp/opt-bin
opt_count=$tmp/opt-pnpm-count
mkdir -p "$opt_web/src" "$opt_bin"
printf '{"name":"meet-web","packageManager":"pnpm@11.17.0"}\n' >"$opt_web/package.json"
printf 'lockfileVersion: 9.0\n' >"$opt_web/pnpm-lock.yaml"
printf 'export const v = 1;\n' >"$opt_web/src/app.ts"
cat >"$opt_bin/node" <<'EOF'
#!/bin/sh
exit 0
EOF
cat >"$opt_bin/pnpm" <<EOF
#!/bin/sh
case "\$1" in
install) exit 0 ;;
build)
	printf 'build\n' >>'$opt_count'
	mkdir -p dist
	cp -R '$dist/.' dist/
	;;
esac
EOF
chmod +x "$opt_bin/node" "$opt_bin/pnpm"
: >"$opt_count"

# First optional run builds (no stamp yet) and promotes artifact.
PATH="$opt_bin:/bin:/usr/bin" BASHY_BIN= MEET_SPA_WEB_DIR="$opt_web" /bin/sh "$script" optional >"$tmp/5a.out" 2>&1 ||
	fail "first optional build failed"
[ "$(wc -l <"$opt_count" | tr -d ' ')" = 1 ] || fail "first optional run should build once"

# Second optional run with unchanged inputs skips pnpm+vite.
PATH="$opt_bin:/bin:/usr/bin" BASHY_BIN= MEET_SPA_WEB_DIR="$opt_web" /bin/sh "$script" optional >"$tmp/5b.out" 2>&1 ||
	fail "warm optional build failed"
grep -q 'content-fresh; skipping optional rebuild' "$tmp/5b.out" || fail "warm optional run did not skip"
[ "$(wc -l <"$opt_count" | tr -d ' ')" = 1 ] || fail "warm optional run unexpectedly rebuilt"

# Editing a web input invalidates freshness and triggers a rebuild.
printf 'export const v = 2;\n' >"$opt_web/src/app.ts"
PATH="$opt_bin:/bin:/usr/bin" BASHY_BIN= MEET_SPA_WEB_DIR="$opt_web" /bin/sh "$script" optional >"$tmp/5c.out" 2>&1 ||
	fail "post-edit optional build failed"
[ "$(wc -l <"$opt_count" | tr -d ' ')" = 2 ] || fail "post-edit optional run did not rebuild"

# required mode always rebuilds even when content-fresh.
PATH="$opt_bin:/bin:/usr/bin" BASHY_BIN= MEET_SPA_WEB_DIR="$opt_web" /bin/sh "$script" required >"$tmp/5d.out" 2>&1 ||
	fail "required build failed"
[ "$(wc -l <"$opt_count" | tr -d ' ')" = 3 ] || fail "required run unexpectedly skipped"
echo "test-meet-spa-fresh: [7/7] optional skips when content-fresh, rebuilds on edit, required always builds"

echo "test-meet-spa-fresh: PASS"
