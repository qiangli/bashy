#!/usr/bin/env bash
# flatten-image.sh — export, patch, and re-import the shipped image as ONE layer.
#
# The layered shipped image is FROM the unpatched base, so a vulnerable
# bundled copy survives in a LOWER layer even after /app is patched, and
# `docker save` distributes every layer. This script flattens the merged
# view (where the patched /app already wins) into a single-layer image, so
# the distributed archive physically cannot carry the lower-layer copy:
#
#   1. `docker create` (no code runs) + stream `docker export` into rootfs,
#   2. extract the rootfs and run patch_rootfs.py over the WHOLE tree —
#      gate-first: loud no-op when already clean, otherwise replace every
#      bundled copy below --min-version (including copies OUTSIDE /app such
#      as the Node base's own npm tree) with the pinned fixed release,
#   3. stream the re-tar into `docker import` as the target image, re-applying the
#      source image's runtime config (Entrypoint, Cmd, Env, User,
#      WorkingDir, Labels) as --change flags read from `docker inspect`
#      at build time — never hardcoded here, so a Dockerfile config change
#      cannot silently desync,
#   4. config-parity check: the imported image must carry the same runtime
#      config as the source (HEALTHCHECK excepted — `docker import` has no
#      --change for it; the gateway's /healthz endpoints are unchanged and
#      orchestrators define their own probes).
#
# Required env: SOURCE_IMAGE TARGET_IMAGE WORK_DIR REPLACEMENT_TGZ
#   REPLACEMENT_INTEGRITY SCRIPTS_DIR. Optional: PACKAGE MIN_VERSION.
set -euo pipefail

: "${SOURCE_IMAGE:?need SOURCE_IMAGE (the layered shipped image)}"
: "${TARGET_IMAGE:?need TARGET_IMAGE (the flattened distributable image)}"
: "${WORK_DIR:?need WORK_DIR (scratch dir)}"
: "${REPLACEMENT_TGZ:?need REPLACEMENT_TGZ (pinned fixed-release tarball)}"
: "${REPLACEMENT_INTEGRITY:?need REPLACEMENT_INTEGRITY (pinned sha512)}"
: "${SCRIPTS_DIR:?need SCRIPTS_DIR (bashy-build/scripts)}"
PACKAGE="${PACKAGE:-http-cache-semantics}"
MIN_VERSION="${MIN_VERSION:-4.3.0}"

mkdir -p "$WORK_DIR"
ROOTFS="$WORK_DIR/rootfs"
rm -rf "$ROOTFS"
mkdir -p "$ROOTFS"

echo "flatten: exporting $SOURCE_IMAGE"
CID="$(docker create "$SOURCE_IMAGE")"
trap 'docker rm "$CID" >/dev/null 2>&1 || true' EXIT
docker export "$CID" | tar -xf - -C "$ROOTFS"
docker rm "$CID" >/dev/null
trap - EXIT

echo "flatten: full-rootfs vulnerable-copy gate (patch when needed)"
if python3 "$SCRIPTS_DIR/patch_rootfs.py" --verify-only \
  --rootfs "$ROOTFS" --package "$PACKAGE" --min-version "$MIN_VERSION"; then
  echo "flatten: rootfs already holds no $PACKAGE < $MIN_VERSION — no-op (archive gate still enforced)"
else
  test -f "$REPLACEMENT_TGZ" || { echo "flatten: missing $REPLACEMENT_TGZ" >&2; exit 1; }
  python3 "$SCRIPTS_DIR/patch_rootfs.py" \
    --rootfs "$ROOTFS" --package "$PACKAGE" --min-version "$MIN_VERSION" \
    --replacement "$REPLACEMENT_TGZ" --integrity "$REPLACEMENT_INTEGRITY"
fi

echo "flatten: reading runtime config from $SOURCE_IMAGE"
docker inspect "$SOURCE_IMAGE" --format='{{json .Config}}' >"$WORK_DIR/source-config.json"
# One complete `docker import --change` instruction per record, NUL-separated:
# `docker import` joins --change arguments with newlines and parses them as
# Dockerfile instructions, so a LABEL value with spaces passed unquoted splits
# into bare words and fails the import (can't find = in "gateway"). The
# builder emits each record Dockerfile-quoted (CMD/ENTRYPOINT as JSON arrays,
# ENV/LABEL/WORKDIR/USER values quoted when they hold whitespace or
# shell-special characters); NUL framing means bash never re-splits a value.
python3 "$SCRIPTS_DIR/build_import_changes.py" \
  "$WORK_DIR/source-config.json" "$WORK_DIR/changes.nul"
CHANGES=()
while IFS= read -r -d '' change; do
  CHANGES+=(--change "$change")
done <"$WORK_DIR/changes.nul"
tr '\0' '\n' <"$WORK_DIR/changes.nul"

echo "flatten: importing single-layer $TARGET_IMAGE"
tar -C "$ROOTFS" -cf - . | docker import "${CHANGES[@]}" - "$TARGET_IMAGE"

echo "flatten: config-parity check (source vs flattened)"
docker inspect "$TARGET_IMAGE" --format='{{json .Config}}' >"$WORK_DIR/target-config.json"
python3 - "$WORK_DIR/source-config.json" "$WORK_DIR/target-config.json" <<'EOF'
import json, sys
src = json.load(open(sys.argv[1])); dst = json.load(open(sys.argv[2]))
for key in ("Entrypoint", "Cmd", "Env", "User", "WorkingDir", "Labels"):
    if (src.get(key) or None) != (dst.get(key) or None):
        print(f"flatten: FAIL: Config.{key} drifted:\n  src={src.get(key)}\n  dst={dst.get(key)}",
              file=sys.stderr)
        sys.exit(1)
print(f"flatten: OK — runtime config identical (source Healthcheck={src.get('Healthcheck') is not None}, "
      f"flattened Healthcheck={dst.get('Healthcheck') is not None}; healthcheck metadata is not carried by "
      f"docker import — gateway /healthz endpoints unchanged)")
EOF
docker inspect "$TARGET_IMAGE" --format='flattened image ID: {{.Id}}'
