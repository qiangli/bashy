#!/usr/bin/env bash
# cgo-free-overlay.sh SRC OUTDIR
#
# Writes OUTDIR/overlay.json, a `go build -overlay` map that makes k3s build
# with CGO_ENABLED=0 WITHOUT editing the checked-out tree (so the go.mod/go.sum
# and `git status` gates still see the pinned source). Exactly two upstream
# files are replaced, and each replacement is checked to be exactly the
# intended edit, so a new k3s tag that moves either file fails here, loudly:
#
#   pkg/containerd/builtins_linux.go  minus the btrfs snapshotter import. That
#     package is cgo-only (`linux && !no_btrfs && cgo`), and k3s imports it
#     unconditionally, so the `no_btrfs` tag alone leaves an import with no
#     buildable files. The overlay is the one line the tag cannot remove.
#   pkg/proctitle/proctile.go         a no-op SetProcTitle, as upstream's own
#     windows file is: erikdubbelboer/gspt sets the process title via cgo only.
#     Cosmetic (the name `ps` shows), nothing functional.
set -euo pipefail

src=$(cd "${1:?usage: cgo-free-overlay.sh SRC OUTDIR}" && pwd)
out=${2:?usage: cgo-free-overlay.sh SRC OUTDIR}
mkdir -p "$out"
out=$(cd "$out" && pwd)
die() { echo "cgo-free-overlay: $*" >&2; exit 1; }

btrfs='	_ "github.com/containerd/containerd/v2/plugins/snapshots/btrfs/plugin"'
f=pkg/containerd/builtins_linux.go
[[ $(grep -cxF "$btrfs" "$src/$f") == 1 ]] || die "$f: expected exactly one btrfs plugin import"
grep -vxF "$btrfs" "$src/$f" >"$out/builtins_linux.go"
[[ $(($(wc -l <"$src/$f") - $(wc -l <"$out/builtins_linux.go"))) == 1 ]] || die "$f: edit removed more than one line"

f2=pkg/proctitle/proctile.go
want='//go:build linux

package proctitle

import (
	"github.com/erikdubbelboer/gspt"
)

func SetProcTitle(cmd string) {
	gspt.SetProcTitle(cmd)
}'
[[ $(<"$src/$f2") == "$want" ]] || die "$f2 is not the file this overlay replaces"
cat >"$out/proctile.go" <<'EOF'
//go:build linux

package proctitle

// SetProcTitle is a no-op: gspt sets the process title through cgo only.
func SetProcTitle(cmd string) {}
EOF

printf '{"Replace":{"%s/%s":"%s/builtins_linux.go","%s/%s":"%s/proctile.go"}}\n' \
	"$src" "$f" "$out" "$src" "$f2" "$out" >"$out/overlay.json"
echo "cgo-free overlay: $out/overlay.json"
