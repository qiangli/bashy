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

detect_bashy() {
    if [ -n "${BASHY:-}" ] && [ -x "$BASHY" ]; then
        printf '%s\n' "$BASHY"
        return 0
    fi
    if [ -x "$root/bin/bashy" ]; then
        printf '%s\n' "$root/bin/bashy"
        return 0
    fi
    if command -v bashy >/dev/null 2>&1; then
        command -v bashy
        return 0
    fi
    return 1
}

BASHY_EXE=$(detect_bashy || true)

git_head_short() {
    repo=$1
    if [ -n "$BASHY_EXE" ]; then
        (cd "$repo" && "$BASHY_EXE" git rev-parse --short HEAD)
        return
    fi
    git -C "$repo" rev-parse --short HEAD
}

git_clone() {
    url=$1
    target=$2
    if [ -n "$BASHY_EXE" ]; then
        "$BASHY_EXE" git clone --quiet "$url" "$target" >/dev/null
        return
    fi
    git clone --quiet "$url" "$target"
}

git_checkout() {
    repo=$1
    sha=$2
    if [ -n "$BASHY_EXE" ]; then
        (cd "$repo" && "$BASHY_EXE" git checkout "$sha" >/dev/null)
        return
    fi
    git -C "$repo" checkout --quiet "$sha"
}

git_submodule_update() {
    repo=$1
    if command -v git >/dev/null 2>&1; then
        git -C "$repo" submodule update --init --recursive --quiet 2>/dev/null || true
        return
    fi
    if [ -f "$repo/.gitmodules" ]; then
        echo "bootstrap-siblings: host git unavailable; skipping optional submodule update for $repo" >&2
    fi
}

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
    sha=$(printf %s "$sha" | tr -d '[:space:]')  # strip CR (CRLF) / stray whitespace
    if [ -z "$name" ] || [ -z "$sha" ] || [ "$name" = "$sha" ]; then
        echo "bootstrap-siblings: malformed line: $line" >&2
        exit 1
    fi

    target=$root/../$name
    if [ -e "$target/.git" ]; then
        echo "bootstrap-siblings: $name -> $(git_head_short "$target") (already present, leaving alone)"
        continue
    fi

    url=$(repo_url "$name")
    echo "bootstrap-siblings: cloning $url -> $target @ ${sha:0:12}"
    git_clone "$url" "$target"
    git_checkout "$target" "$sha"
    # Siblings may have their own submodules (e.g. coreutils -> ollama/podman forks).
    # They are not needed for the default lean build; host-layer DAG targets can
    # materialize them later when a task actually needs those sources.
    git_submodule_update "$target"
done < "$pins"
