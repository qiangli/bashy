#!/usr/bin/env bash
# k3s-overlay.sh SRC OUTDIR
#
# Writes OUTDIR/overlay.json, the `go build -overlay` map the bashy k3s build
# compiles with, and OUTDIR/apply.tsv (TARGET<TAB>BACKING, BACKING empty =
# delete), which the audit job applies to its own tree because govulncheck
# ignores -overlay. The checked-out tree is never edited, so the go.mod/go.sum
# and `git status` gates still see the pinned source.
#
# Every replaced file is identified by the SHA-256 of its exact upstream text
# at the pinned tag (and, for the one module file, at the module version k3s's
# go.mod pins). A tag or module bump that changes ANY of these files fails here,
# loudly, before anything is built — the edits are then re-reviewed against the
# new text and the hashes updated. builds/k3s/README.md §"Removed and changed vs
# upstream k3s — regression map" has one row per edit below.
#
#   cgo        pkg/containerd/builtins_linux.go   minus the btrfs snapshotter
#              import (cgo-only; `no_btrfs` cannot drop an unconditional import)
#   cgo        pkg/proctitle/proctile.go          SetProcTitle is a no-op, like
#              upstream's own windows file (erikdubbelboer/gspt is cgo-only)
#   docker     pkg/containerd/builtins_linux.go   minus the nix snapshotter
#              plugin import, and
#              pkg/agent/containerd/config_linux.go and
#              pkg/containerd/utility_linux.go    NixSupported refuses; the
#              kernel check no longer comes from github.com/docker/docker
#              (pkg/agent/containerd/zz_bashy_kernel_linux.go, added). With the
#              `no_cri_dockerd` tag these were the last importers of
#              github.com/docker/docker (GO-2026-4883, GO-2026-4887; no fix).
#   spegel     pkg/spegel/*.go                    one stub file: the embedded
#              registry mirror refuses to start. Drops spegel and
#              go-libp2p-kad-dht (GO-2024-3218; no fix).
#   openpgp    <containerd module>/internal/cri/server/images/image_pull.go
#              no imgcrypt decrypt-on-unpack option: CRI encrypted-image
#              pulls are not supported. Drops containerd/imgcrypt ->
#              containers/ocicrypt -> golang.org/x/crypto/openpgp
#              (GO-2026-5932; no fix).
set -euo pipefail

usage='usage: k3s-overlay.sh SRC OUTDIR'
src=$(cd "${1:?$usage}" && pwd)
out=${2:?$usage}
mkdir -p "$out"
out=$(cd "$out" && pwd)
die() { echo "k3s-overlay: $*" >&2; exit 1; }
sha256() {
	if command -v sha256sum >/dev/null; then sha256sum "$1"; else shasum -a 256 "$1"; fi | awk '{print $1}'
}

# The containerd module k3s pins (go.mod replace -> github.com/k3s-io/containerd/v2),
# from the build's vendor/: go refuses an overlay of a file under GOMODCACHE, so
# the build runs `go mod vendor` (go.mod/go.sum untouched) and -mod=vendor.
ctrd=$src/vendor/github.com/containerd/containerd/v2
[[ -f $src/vendor/modules.txt && -d $ctrd ]] || die "run \`go mod vendor\` in $src first (the containerd module is overlaid in vendor/)"

: >"$out/apply.tsv"
n=0
lines=() # the file being edited, one element per line (no trailing newline)

# begin TARGET SHA256: load the upstream file, refusing any other text.
begin() {
	[[ -f $1 ]] || die "$1: missing (the pinned upstream file moved)"
	local got
	got=$(sha256 "$1")
	[[ $got == "$2" ]] || die "$1: sha256 $got, expected $2 — not the upstream text this overlay was reviewed against"
	mapfile -t lines <"$1"
}
# drop N TEXT: delete line N, which must read exactly TEXT.
drop() {
	[[ ${lines[$1 - 1]-} == "$2" ]] || die "line $1 is not: $2"
	lines[$1 - 1]=$'\x01'
}
# swap N OLD NEW: replace line N, which must read exactly OLD.
swap() {
	[[ ${lines[$1 - 1]-} == "$2" ]] || die "line $1 is not: $2"
	lines[$1 - 1]=$3
}
# commit TARGET: write the edited file into OUTDIR and map it.
commit() {
	n=$((n + 1))
	local backing=$out/$n-${1##*/} line
	for line in "${lines[@]}"; do
		[[ $line == $'\x01' ]] || printf '%s\n' "$line"
	done >"$backing"
	printf '%s\t%s\n' "$1" "$backing" >>"$out/apply.tsv"
}
# write TARGET (stdin): map TARGET (new or replaced) to stdin's text.
write() {
	n=$((n + 1))
	local backing=$out/$n-${1##*/}
	cat >"$backing"
	printf '%s\t%s\n' "$1" "$backing" >>"$out/apply.tsv"
}
# delete TARGET SHA256: the file is removed from the build.
delete() {
	begin "$1" "$2"
	printf '%s\t\n' "$1" >>"$out/apply.tsv"
}

# --- cgo + docker: pkg/containerd/builtins_linux.go -------------------------
f=$src/pkg/containerd/builtins_linux.go
begin "$f" 725c4a279e34ed1abf84b91d74687145732d9779f731c27fcf636a9f47abbee5
drop 29 '	_ "github.com/containerd/containerd/v2/plugins/snapshots/btrfs/plugin"'
drop 37 '	_ "github.com/pdtpartners/nix-snapshotter/pkg/plugin"'
commit "$f"

# --- cgo: pkg/proctitle/proctile.go ------------------------------------------
f=$src/pkg/proctitle/proctile.go
begin "$f" 58662b43033394c79a862135096309ca85a1367bffe16c4c74c11fc7e687ced5
write "$f" <<'EOF'
//go:build linux

package proctitle

// SetProcTitle is a no-op in the bashy build: erikdubbelboer/gspt sets the
// process title through cgo only, and this build is CGO_ENABLED=0.
func SetProcTitle(cmd string) {}
EOF

# --- docker: pkg/agent/containerd/config_linux.go ---------------------------
f=$src/pkg/agent/containerd/config_linux.go
begin "$f" b093a48dd1b44ef9624eeec1c49956b96b46f41569f0781245c77442b6b1ea18
drop 15 '	"github.com/docker/docker/pkg/parsers/kernel"'
drop 21 '	"github.com/pdtpartners/nix-snapshotter/pkg/nix"'
swap 91 '		EnableUnprivileged:    kernel.CheckKernelVersion(4, 11, 0),' \
	'		EnableUnprivileged:    bashyCheckKernelVersion(4, 11, 0),'
swap 142 '	return nix.Supported(root)' \
	'	return errors.New("the nix snapshotter is not compiled into this k3s build (bashy: it links github.com/docker/docker)")'
commit "$f"
f=$src/pkg/containerd/utility_linux.go
begin "$f" fd5c335904bd29a96df57916fe393a357bc5b5804ea35ebaebc9d1a14ada8525
drop 12 '	"github.com/pdtpartners/nix-snapshotter/pkg/nix"'
swap 31 '	return nix.Supported(root)' \
	'	return errors.New("the nix snapshotter is not compiled into this k3s build (bashy: it links github.com/docker/docker)")'
commit "$f"
write "$src/pkg/agent/containerd/zz_bashy_kernel_linux.go" <<'EOF'
//go:build linux

package containerd

// Added by the bashy k3s build (builds/k3s/files/k3s-overlay.sh) in place of
// github.com/docker/docker/pkg/parsers/kernel.CheckKernelVersion, with the same
// behavior: true unless uname's release parses as older than k.major.minor
// (an unreadable or unparseable release also yields true, as upstream's does).

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

func bashyCheckKernelVersion(k, major, minor int) bool {
	var uts unix.Utsname
	if err := unix.Uname(&uts); err != nil {
		logrus.Warnf("error getting kernel version: %s", err)
		return true
	}
	release := unix.ByteSliceToString(uts.Release[:])
	var gotK, gotMajor, gotMinor int
	var partial, flavor string
	if parsed, _ := fmt.Sscanf(release, "%d.%d%s", &gotK, &gotMajor, &partial); parsed < 2 {
		logrus.Warnf("error getting kernel version: Can't parse kernel version %s", release)
		return true
	}
	if parsed, _ := fmt.Sscanf(partial, ".%d%s", &gotMinor, &flavor); parsed < 1 {
		gotMinor = 0
	}
	for _, d := range [][2]int{{gotK, k}, {gotMajor, major}, {gotMinor, minor}} {
		if d[0] != d[1] {
			return d[0] > d[1]
		}
	}
	return true
}
EOF

# --- spegel: pkg/spegel ------------------------------------------------------
delete "$src/pkg/spegel/bootstrap.go" f683aa4e1cdeb9f53dee233819f575bb626b669f92d01f04ef2e0caa71a8b165
delete "$src/pkg/spegel/registry.go" 10c891addfd51a5a8d981af4655e90192b8729b71cdbd3a3f8974903e9f2a042
delete "$src/pkg/spegel/store.go" 09ac6564cfea0938ce17dbe067e21118de83a3dd960d03723d9349f536131b5f
f=$src/pkg/spegel/spegel.go
begin "$f" 21d0ce4bc062345dd0208104c0d0879cd6cc4499804a87a457f4f9b5f3c3d0d8
write "$f" <<'EOF'
// Package spegel is a stub in the bashy k3s build (builds/k3s/files/k3s-overlay.sh):
// the embedded distributed registry mirror (spegel over libp2p, which links
// go-libp2p-kad-dht, GO-2024-3218, no fixed version) is not compiled in. It
// keeps the API k3s's callers use; starting the mirror fails.
package spegel

import (
	"context"
	"errors"

	"github.com/k3s-io/k3s/pkg/agent/https"
	"github.com/k3s-io/k3s/pkg/daemons/config"
)

var errNotCompiled = errors.New("the embedded registry mirror (--embedded-registry, spegel) is not compiled into this k3s build")

// Bootstrapper stands in for spegel's routing.Bootstrapper; nothing runs it.
type Bootstrapper interface{}

type bootstrapper struct{}

func NewSelfBootstrapper() Bootstrapper                                { return bootstrapper{} }
func NewAgentBootstrapper(server, token, dataDir string) Bootstrapper  { return bootstrapper{} }
func NewServerBootstrapper(controlConfig *config.Control) Bootstrapper { return bootstrapper{} }
func NewChainingBootstrapper(bootstrappers ...Bootstrapper) Bootstrapper {
	return bootstrapper{}
}

// Config keeps the upstream fields k3s's callers set.
type Config struct {
	ClientCAFile    string
	ClientCertFile  string
	ClientKeyFile   string
	ServerCAFile    string
	ServerCertFile  string
	ServerKeyFile   string
	ExternalAddress string
	InternalAddress string
	RegistryPort    string
	PSK             []byte
	Bootstrapper    Bootstrapper
	Router          https.RouterFunc
}

// DefaultRegistry is the instance k3s configures; it never starts.
var DefaultRegistry = &Config{Bootstrapper: NewSelfBootstrapper()}

// InjectMirror adds no mirror endpoint.
func (c *Config) InjectMirror(nodeConfig *config.Node) error { return errNotCompiled }

// Start refuses: k3s reports "failed to start embedded registry: ..." and the
// agent exits, so --embedded-registry is never silently ignored.
func (c *Config) Start(ctx context.Context, nodeConfig *config.Node, criReadyChan <-chan struct{}) error {
	return errNotCompiled
}

func (c *Config) Ready(ctx context.Context) (bool, error) { return false, errNotCompiled }
EOF

# --- openpgp: containerd's CRI image pull ------------------------------------
f=$ctrd/internal/cri/server/images/image_pull.go
begin "$f" a8c276e0618aaba2dec28cdba20ce125b7700ab067e357feed04f97a9857c77e
drop 35 '	"github.com/containerd/imgcrypt/v2"'
drop 36 '	"github.com/containerd/imgcrypt/v2/images/encryption"'
drop 687 '	if c.config.ImageDecryption.KeyModel == criconfig.KeyModelNode {'
drop 688 '		ltdd := imgcrypt.Payload{}'
drop 689 '		decUnpackOpt := encryption.WithUnpackConfigApplyOpts(encryption.WithDecryptedUnpack(&ltdd))'
drop 690 '		opt := containerd.WithUnpackOpts([]containerd.UnpackOpt{decUnpackOpt})'
drop 691 '		return []containerd.RemoteOpt{opt}'
drop 692 '	}'
swap 693 '	return nil' '	return nil // bashy k3s build: no imgcrypt (ocicrypt -> x/crypto/openpgp), encrypted images are not decrypted'
commit "$f"

# The map, in go's -overlay JSON shape. Paths are absolute runner paths with no
# characters JSON would need escaped; refuse rather than mis-encode one.
{
	printf '{"Replace":{'
	sep=
	while IFS=$'\t' read -r target backing; do
		[[ $target$backing != *[\"\\]* ]] || die "path needs JSON escaping: $target"
		printf '%s"%s":"%s"' "$sep" "$target" "$backing"
		sep=,
	done <"$out/apply.tsv"
	printf '}}\n'
} >"$out/overlay.json"
echo "k3s overlay: $out/overlay.json ($(wc -l <"$out/apply.tsv" | tr -d ' ') files)"
