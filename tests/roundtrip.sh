#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
WRAPPER=$ROOT/examples/collab-save/collab-save.bsh
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
export GIT_AUTHOR_NAME='Roundtrip Test' GIT_AUTHOR_EMAIL='roundtrip@example.invalid'
export GIT_COMMITTER_NAME=$GIT_AUTHOR_NAME GIT_COMMITTER_EMAIL=$GIT_AUTHOR_EMAIL

git init --bare "$TMP/remote.git" >/dev/null
git init -b main "$TMP/repo" >/dev/null
git -C "$TMP/repo" config user.name "$GIT_AUTHOR_NAME"
git -C "$TMP/repo" config user.email "$GIT_AUTHOR_EMAIL"
git -C "$TMP/repo" remote add origin "$TMP/remote.git"
printf '# exported note\nSaved through collab-save.\n' > "$TMP/note.md"
python3 - "$TMP/media.wav" <<'PY'
import sys
with open(sys.argv[1], "wb") as f:
    f.write(b"RIFF" + bytes(range(256)) * 8)
PY

bashy --bashsharp "$WRAPPER" save "$TMP/note.md" --repo "$TMP/repo" --dry-run --json > "$TMP/save-dry.json"
jq -e '.dry_run == true and (.paths | length == 1)' "$TMP/save-dry.json" >/dev/null
[[ ! -e $TMP/repo/note.md ]]
bashy --bashsharp "$WRAPPER" publish --repo "$TMP/repo" --remote origin --dry-run --json > "$TMP/publish-dry.json"
jq -e '.dry_run == true and .remote == "origin"' "$TMP/publish-dry.json" >/dev/null

if command -v git-lfs >/dev/null 2>&1; then
  LFS=1
else
  LFS=0
  echo 'SKIP: git-lfs unavailable; validating ordinary Git media round trip only'
fi
bashy --bashsharp "$WRAPPER" save "$TMP/note.md" "$TMP/media.wav" --repo "$TMP/repo" --message 'roundtrip' --json > "$TMP/save.json"
jq -e '.committed == true and (.paths | length == 2)' "$TMP/save.json" >/dev/null
bashy --bashsharp "$WRAPPER" publish --repo "$TMP/repo" --remote origin --json > "$TMP/publish.json"
jq -e '.published == true and .remote == "origin"' "$TMP/publish.json" >/dev/null
if (( LFS )); then git clone "$TMP/remote.git" "$TMP/clone" >/dev/null 2>&1
else git clone "$TMP/remote.git" "$TMP/clone" >/dev/null 2>&1; fi
cmp "$TMP/note.md" "$TMP/clone/note.md"
if (( LFS )); then
  git -C "$TMP/repo" check-attr filter -- media.wav | grep -qE 'filter: lfs'
  grep -qE '^version https://git-lfs.github.com/spec/v1$' "$TMP/clone/media.wav"
  git -C "$TMP/clone" lfs install --local >/dev/null
  git -C "$TMP/clone" lfs pull >/dev/null
  cmp "$TMP/media.wav" "$TMP/clone/media.wav"
else
  cmp "$TMP/media.wav" "$TMP/clone/media.wav"
fi

mkdir -p "$TMP/private"
printf 'do not save\n' > "$TMP/private/api-token.txt"
if bashy --bashsharp "$WRAPPER" save "$TMP/private/api-token.txt" --repo "$TMP/repo" >"$TMP/refuse.out" 2>"$TMP/refuse.err"; then
  echo 'expected secret-looking path to be refused' >&2
  exit 1
fi
grep -qE 'refusing sensitive-looking path' "$TMP/refuse.err"

mkdir -p "$TMP/outside"
printf 'outside\n' > "$TMP/outside/escape.md"
if (cd "$TMP/repo" && bashy --bashsharp "$WRAPPER" save ../outside/escape.md --repo . >"$TMP/escape.out" 2>"$TMP/escape.err"); then
  echo 'expected ../ target to be refused' >&2
  exit 1
fi
grep -qE 'target escapes repository' "$TMP/escape.err"

echo 'PASS: save, explicit publish, fresh clone, media, secret-path and ../ escape refusal'
