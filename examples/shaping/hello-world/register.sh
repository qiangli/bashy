#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
if [ "$#" -ne 1 ] || [ -e "$1" ]; then
  echo 'Usage: bash register.sh NEW_SOURCE_DIRECTORY (must not exist)' >&2
  exit 2
fi
mkdir -p "$1"
source_dir=$(cd "$1" && pwd)
cp -R "$here/public" "$here/checks" "$source_dir/"
cp "$here/contract.bsh" "$source_dir/"
git -C "$source_dir" init -q
git -C "$source_dir" add public checks contract.bsh
git -C "$source_dir" -c user.name='Bashy Example' -c user.email='example@example.invalid' commit -qm 'Initialize Hello World'
# No --force: never overwrite an existing app registration.
"$runner" app add hello-world --type shaped --source "$source_dir" --label "Hello World" \
  --tip "A minimal Software Shaping tutorial"
