#!/usr/bin/env bash
# Ensure the sibling-path replace target in go.mod (../sh) exists on disk, by
# cloning the pinned commit from upstream if the sibling is missing.
#
# Why: bashy lives in two contexts.
#   1. Inside the dhnt umbrella, ../sh is already mounted as a submodule
#      (dhnt/sh). The script detects it and leaves it alone — one shared
#      copy across every consumer in the umbrella.
#   2. As a standalone clone (CI, contributor checkout), the sibling doesn't
#      exist. The script clones it into ../sh at the SHA in .sibling-pins.
#
# Idempotent. Safe to re-run.
set -euo pipefail

cd "$(dirname "$0")/.."
root=$(pwd)
pins=$root/.sibling-pins

if [ ! -f "$pins" ]; then
    echo "bootstrap-siblings: missing $pins" >&2
    exit 1
fi

# repo URL per dep name; if you add a new sibling, append here.
repo_url() {
    case "$1" in
        sh) echo "https://github.com/qiangli/sh.git" ;;
        coreutils) echo "https://github.com/qiangli/coreutils.git" ;;
        readline) echo "https://github.com/qiangli/readline.git" ;;
        *) echo "bootstrap-siblings: no repo URL for '$1'" >&2; return 1 ;;
    esac
}

while IFS= read -r line; do
    case "$line" in
        ''|'#'*) continue ;;
    esac
    name=${line%%=*}
    sha=${line#*=}
    if [ -z "$name" ] || [ -z "$sha" ] || [ "$name" = "$sha" ]; then
        echo "bootstrap-siblings: malformed line: $line" >&2
        exit 1
    fi

    target=$root/../$name
    if [ -e "$target/.git" ]; then
        echo "bootstrap-siblings: $name -> $(cd "$target" && git rev-parse --short HEAD) (already present, leaving alone)"
        continue
    fi

    url=$(repo_url "$name")
    echo "bootstrap-siblings: cloning $url -> $target @ ${sha:0:12}"
    git clone --quiet "$url" "$target"
    git -C "$target" checkout --quiet "$sha"
done < "$pins"
