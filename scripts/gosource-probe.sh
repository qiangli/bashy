#!/usr/bin/env bash
# Sprint 118, W1 (Bashy half): end-to-end probes for `--source=go`, driven
# through the DEFAULT-BUILT launcher (bin/bashy, i.e. the C launcher over
# bin/bashy.real) rather than through `go run` or an in-process test hook.
#
# The three modes are the ones /tmp/s118-integration/bashpp-tests/tools/corpus
# specifies:
#
#   baseline     go build -o program original.go   ; ./program
#   interpreted  bashy --bashpp --source=go original.go
#   compiled     bashy transpile --bashpp --source=go original.go \
#                    -o generated.go --map generated.go.map
#                go build -o program generated.go ; rm generated.go ; ./program
#
# The compiled program is run with an EMPTY PATH and with its generated source
# deleted, so a pass cannot come from a lingering source file or from a command
# found on the host.
#
# This is a probe harness for this repository's own review, NOT the corpus
# runner and NOT evidence of corpus certification: it does not hash a candidate,
# does not build a provenance record and adjudicates only what it prints.
#
# Usage: scripts/gosource-probe.sh [-k]   (-k: keep the scratch tree)

set -u

repo=$(cd "$(dirname "$0")/.." && pwd)
BASHY=${BASHY:-$repo/bin/bashy}
GO=${GO:-$(command -v go)}
keep=0
[ "${1:-}" = "-k" ] && keep=1

[ -x "$BASHY" ] || { echo "no launcher at $BASHY (run: make build)" >&2; exit 2; }
[ -x "$GO" ] || { echo "no go toolchain" >&2; exit 2; }

work=$(mktemp -d)
cleanup() { [ "$keep" = 1 ] && return; chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"; }
trap cleanup EXIT

# The host toolchain IS the pinned one; never let a probe download another, and
# never let a probe with its own HOME create a second module cache.
export GOTOOLCHAIN=${GOTOOLCHAIN:-go1.27.0}
# Documented master switch for proactive hints. Without it a probe's stderr
# depends on which agent is driving the terminal, which is not a property of
# the shell under test.
export BASHY_HINTS=off
export GOPATH=${GOPATH:-$("$GO" env GOPATH)}
export GOMODCACHE=${GOMODCACHE:-$("$GO" env GOMODCACHE)}
export GOCACHE=${GOCACHE:-$("$GO" env GOCACHE)}

pass=0 fail=0
ok()   { pass=$((pass+1)); printf 'PASS  %s\n' "$1"; }
bad()  { fail=$((fail+1)); printf 'FAIL  %s\n      %s\n' "$1" "$2"; }
# check NAME EXPECTED ACTUAL
check() { [ "$2" = "$3" ] && ok "$1" || bad "$1" "want [$2] got [$3]"; }

echo "launcher : $BASHY"
echo "payload  : $BASHY.real"
echo "go       : $($GO version)"
echo "goroot   : $($GO env GOROOT)"
echo

# --------------------------------------------------------------------------
# P1  unchanged hello: all three modes, byte-identical stdout
# --------------------------------------------------------------------------
p1=$work/p1; mkdir -p "$p1/src" "$p1/base" "$p1/comp" "$p1/run"
cat > "$p1/src/hello.go" <<'EOF'
package main

import "fmt"

func main() {
	fmt.Println("Hello, 世界")
	fmt.Printf("%d %t %q\n", 42, true, "x")
}
EOF
before=$(shasum -a 256 < "$p1/src/hello.go")

(cd "$p1/base" && "$GO" build -p 2 -o program "$p1/src/hello.go") 2>"$p1/base.err"
baseline=$(cd "$p1/run" && "$p1/base/program" 2>&1); baseline_exit=$?

interpreted=$(cd "$p1/run" && "$BASHY" --bashpp --source=go "$p1/src/hello.go" 2>&1); interp_exit=$?

"$BASHY" transpile --bashpp --source=go "$p1/src/hello.go" \
	-o "$p1/comp/generated.go" --map "$p1/comp/generated.go.map" 2>"$p1/transpile.err"
transpile_exit=$?
if [ $transpile_exit -eq 0 ]; then
	(cd "$p1/comp" && "$GO" build -p 2 -o program generated.go) 2>"$p1/comp.err"
	build_exit=$?
	rm -f "$p1/comp/generated.go"
	compiled=$(cd "$p1/run" && PATH= "$p1/comp/program" 2>&1); comp_exit=$?
else
	build_exit=1; compiled="(transpile failed: $(cat "$p1/transpile.err"))"; comp_exit=1
fi
after=$(shasum -a 256 < "$p1/src/hello.go")

check "P1.baseline hello"                "0" "$baseline_exit"
check "P1.interpreted hello == baseline" "$baseline" "$interpreted"
check "P1.interpreted exit"              "$baseline_exit" "$interp_exit"
check "P1.transpile exit"                "0" "$transpile_exit"
check "P1.native build of generated"     "0" "$build_exit"
check "P1.compiled hello == baseline"    "$baseline" "$compiled"
check "P1.compiled exit (empty PATH, source deleted)" "$baseline_exit" "$comp_exit"
check "P1.generated source really gone"  "gone" "$([ -e "$p1/comp/generated.go" ] && echo present || echo gone)"
check "P1.original bytes unchanged"      "$before" "$after"
check "P1.source map present"            "yes" "$([ -s "$p1/comp/generated.go.map" ] && echo yes || echo no)"
check "P1.map names the original file"   "yes" \
	"$(grep -q "\"source_file\": \"$p1/src/hello.go\"" "$p1/comp/generated.go.map" && echo yes || echo no)"

# --------------------------------------------------------------------------
# P2  globals and init ordering
# --------------------------------------------------------------------------
p2=$work/p2; mkdir -p "$p2/src" "$p2/base" "$p2/comp" "$p2/run"
cat > "$p2/src/order.go" <<'EOF'
package main

import "fmt"

var a = f()
var b = 3
var c string

func f() int { return b + 1 }

func init() { fmt.Println("init1", a, b, c == "") }
func init() { fmt.Println("init2") }

func main() { fmt.Println("main", a, b) }
EOF
(cd "$p2/base" && "$GO" build -p 2 -o program "$p2/src/order.go") 2>/dev/null
p2base=$(cd "$p2/run" && "$p2/base/program" 2>&1)
p2interp=$(cd "$p2/run" && "$BASHY" --bashpp --source=go "$p2/src/order.go" 2>&1)
"$BASHY" transpile --bashpp --source=go "$p2/src/order.go" \
	-o "$p2/comp/generated.go" --map "$p2/comp/generated.go.map" 2>"$p2/transpile.err"
(cd "$p2/comp" && "$GO" build -p 2 -o program generated.go) 2>"$p2/comp.err"
rm -f "$p2/comp/generated.go"
p2comp=$(cd "$p2/run" && PATH= "$p2/comp/program" 2>&1)
check "P2.interpreted init order" "$p2base" "$p2interp"
check "P2.compiled init order"    "$p2base" "$p2comp"

# --------------------------------------------------------------------------
# P3  build-only, non-main package: --check executes nothing; transpile works
# --------------------------------------------------------------------------
p3=$work/p3; mkdir -p "$p3/pkg" "$p3/out" "$p3/run"
cat > "$p3/pkg/p.go" <<'EOF'
package p

import "fmt"

var X = 7

func init() { fmt.Println("SHOULD-NOT-RUN") }

func F() int { return X + 1 }
EOF
p3check=$("$BASHY" --bashpp --source=go --check "$p3/pkg/p.go" 2>&1); p3check_exit=$?
check "P3.--check on non-main exits 0"       "0" "$p3check_exit"
check "P3.--check printed nothing (ran no init)" "" "$p3check"
p3dircheck=$("$BASHY" --bashpp --source=go --check "$p3/pkg" 2>&1); p3dircheck_exit=$?
check "P3.--check on a non-main directory"   "0" "$p3dircheck_exit"
"$BASHY" transpile --bashpp --source=go "$p3/pkg" \
	-o "$p3/out/generated.go" --map "$p3/out/generated.go.map" 2>"$p3/transpile.err"
check "P3.transpile of a non-main package"   "0" "$?"
(cd "$p3/out" && "$GO" build -p 2 -o program generated.go) 2>"$p3/comp.err"
check "P3.native build of the non-main artifact" "0" "$?"
p3run=$(cd "$p3/run" && PATH= "$p3/out/program" 2>&1)
check "P3.artifact runs no main body"        "" "$p3run"
# A non-main package cannot be RUN: that refusal must survive.
p3run_refusal=$("$BASHY" --bashpp --source=go "$p3/pkg/p.go" 2>&1); p3run_exit=$?
check "P3.running a non-main package is refused" "2" "$p3run_exit"
check "P3.refusal names the Go requirement"  "yes" \
	"$(printf '%s' "$p3run_refusal" | grep -q "package main" && echo yes || echo no)"

# --------------------------------------------------------------------------
# P4  module import through lower.NewModuleImporter
# --------------------------------------------------------------------------
p4=$work/p4; mkdir -p "$p4/helper" "$p4/app" "$p4/out" "$p4/run"
cat > "$p4/helper/go.mod" <<'EOF'
module example.com/helper

go 1.26
EOF
cat > "$p4/helper/greet.go" <<'EOF'
package helper

func Greet() string { return "hi from the helper module" }
EOF
cat > "$p4/app/go.mod" <<'EOF'
module example.com/app

go 1.26

require example.com/helper v0.0.0

replace example.com/helper => ../helper
EOF
cat > "$p4/app/main.go" <<'EOF'
package main

import (
	"fmt"

	"example.com/helper"
)

func main() { fmt.Println(helper.Greet()) }
EOF
(cd "$p4/app" && "$GO" build -p 2 -o "$p4/out/base" .) 2>"$p4/base.err"
p4base=$(cd "$p4/run" && "$p4/out/base" 2>&1)
# --check and transpile type-check through the module importer, from a cwd
# that is NOT the module: this is the review's "assets-only cwd" convention.
p4check=$(cd "$p4/run" && "$BASHY" --bashpp --source=go --check "$p4/app/main.go" 2>&1); p4check_exit=$?
check "P4.--check resolves a helper module from a foreign cwd" "0" "$p4check_exit"
check "P4.--check printed nothing"           "" "$p4check"
(cd "$p4/run" && "$BASHY" transpile --bashpp --source=go "$p4/app/main.go" \
	-o "$p4/app/generated.go" --map "$p4/app/generated.go.map") 2>"$p4/transpile.err"
check "P4.transpile resolves the helper module" "0" "$?"
(cd "$p4/app" && "$GO" build -p 2 -o "$p4/out/program" generated.go) 2>"$p4/comp.err"
check "P4.native build of the module artifact"  "0" "$?"
rm -f "$p4/app/generated.go"
p4comp=$(cd "$p4/run" && PATH= "$p4/out/program" 2>&1)
check "P4.compiled module program == baseline" "$p4base" "$p4comp"
# Interpreted mode: the interpreter used to resolve bash++ imports in the
# RUNNER's working directory (interp/bashpp_import.go, Dir: r.Dir), so this was
# reported from both cwds rather than asserted. sh de4ff069 added
# interp.GoSourceModuleDir, wired from internal/agentos onto
# cli.GoSourceModuleDir, and the module context now travels with the SOURCE —
# so the foreign-cwd case is an ASSERTION, not an INFO line.
p4i_module=$(cd "$p4/app" && "$BASHY" --bashpp --source=go "$p4/app/main.go" 2>&1); p4i_module_exit=$?
p4i_foreign=$(cd "$p4/run" && "$BASHY" --bashpp --source=go "$p4/app/main.go" 2>&1); p4i_foreign_exit=$?
check "P4.interpreted from the module cwd resolves the import" "resolved" \
	"$(printf '%s' "$p4i_module" | grep -q "could not be resolved" && echo unresolved || echo resolved)"
check "P4.interpreted from an assets-only cwd resolves the import" "$p4base" "$p4i_foreign"
check "P4.interpreted from an assets-only cwd exits 0" "0" "$p4i_foreign_exit"
printf 'INFO  P4.interpreted from the module cwd: exit=%s out=[%s]\n' \
	"$p4i_module_exit" "$p4i_module"

# P4b  the module context must NOT drag the working directory with it: a
# relative asset path still resolves against the runtime cwd, exactly as the
# natively-built baseline does. This is the pair the story is about — a module
# dependency AND assets-only cwd, both correct in one run.
p4b=$work/p4b; mkdir -p "$p4b/run"
cat > "$p4/app/asset.go" <<'EOF'
package main

import (
	"fmt"
	"os"
)

func reportAsset() {
	cwd, _ := os.Getwd()
	_, err := os.Stat("asset.txt")
	fmt.Println("cwd", cwd, "asset", err == nil)
}
EOF
echo "ASSET-OK" > "$p4b/run/asset.txt"
cat > "$p4/app/main.go" <<'EOF'
package main

import (
	"fmt"

	"example.com/helper"
)

func main() {
	fmt.Println(helper.Greet())
	reportAsset()
}
EOF
(cd "$p4/app" && "$GO" build -p 2 -o "$p4/out/base2" .) 2>"$p4/base2.err"
p4b_base=$(cd "$p4b/run" && "$p4/out/base2" 2>&1)
p4b_interp=$(cd "$p4b/run" && "$BASHY" --bashpp --source=go "$p4/app" 2>&1); p4b_exit=$?
check "P4b.module dep + assets-only cwd == native baseline" "$p4b_base" "$p4b_interp"
check "P4b.exit"                                            "0" "$p4b_exit"
check "P4b.the asset really was read from the runtime cwd"  "yes" \
	"$(printf '%s' "$p4b_interp" | grep -q "asset true" && echo yes || echo no)"

# --------------------------------------------------------------------------
# P5  diagnostics keep the ORIGINAL filename, and never reach the shell parser
# --------------------------------------------------------------------------
p5=$work/p5; mkdir -p "$p5/src" "$p5/run"
cat > "$p5/src/syntaxerr.go" <<'EOF'
package main

if true; then echo shell-ran; fi
EOF
cat > "$p5/src/typeerr.go" <<'EOF'
package main

import "fmt"

func main() { fmt.Println(undefinedName) }
EOF
for kind in syntaxerr typeerr; do
	src=$p5/src/$kind.go
	gobase=$(cd "$p5/run" && "$GO" build -p 2 -o /dev/null "$src" 2>&1); gobase_exit=$?
	out=$(cd "$p5/run" && "$BASHY" --bashpp --source=go "$src" 2>&1); out_exit=$?
	chk=$(cd "$p5/run" && "$BASHY" --bashpp --source=go --check "$src" 2>&1); chk_exit=$?
	check "P5.$kind run exits 2"    "2" "$out_exit"
	check "P5.$kind --check exits 2" "2" "$chk_exit"
	check "P5.$kind names the original file" "yes" \
		"$(printf '%s' "$out" | grep -q "^$src:" && echo yes || echo no)"
	check "P5.$kind no shell dispatch" "clean" \
		"$(printf '%s' "$out" | grep -qE "shell-ran|command not found|unexpected|encountered" && echo leaked || echo clean)"
	printf 'INFO  P5.%s go: [%s]\n      P5.%s bashy: [%s]\n' \
		"$kind" "$(printf '%s' "$gobase" | head -1)" "$kind" "$(printf '%s' "$out" | head -1)"
done

# --------------------------------------------------------------------------
# P6  CLI routing: flag order, shell-only modes, POSIX, the drop-in
# --------------------------------------------------------------------------
p6=$work/p6; mkdir -p "$p6"
cp "$p1/src/hello.go" "$p6/hello.go"
out=$("$BASHY" --source go --bashpp "$p6/hello.go" 2>&1);            check "P6.--source go --bashpp (separated, source first)" "Hello, 世界" "$(printf '%s' "$out" | head -1)"
out=$("$BASHY" --bashpp --source go "$p6/hello.go" 2>&1);            check "P6.--bashpp --source go (separated, bashpp first)" "Hello, 世界" "$(printf '%s' "$out" | head -1)"
out=$("$BASHY" --source=go --bashpp "$p6/hello.go" 2>&1);            check "P6.--source=go --bashpp (joined)" "Hello, 世界" "$(printf '%s' "$out" | head -1)"
out=$("$BASHY" --bashpp --source=go --pretty-print "$p6/hello.go" 2>&1); pp_exit=$?
check "P6.--pretty-print with Go input is refused" "2" "$pp_exit"
check "P6.--pretty-print refusal is not a shell parse error" "clean" \
	"$(printf '%s' "$out" | grep -qE "words and redirects|syntax error near" && echo leaked || echo clean)"
check "P6.--pretty-print refusal names the flag" "yes" \
	"$(printf '%s' "$out" | grep -q -- "--pretty-print" && echo yes || echo no)"
out=$("$BASHY" --bashpp --source=go --dump-strings "$p6/hello.go" 2>&1); check "P6.--dump-strings is refused" "2" "$?"
out=$("$BASHY" --posix --bashpp --source=go "$p6/hello.go" 2>&1);    check "P6.--posix is refused" "2" "$?"
out=$("$BASHY" --no-bashpp --source=go "$p6/hello.go" 2>&1);         check "P6.--source=go with Bash++ off is refused" "2" "$?"
check "P6.that refusal names --bashpp" "yes" \
	"$(printf '%s' "$out" | grep -q -- "requires --bashpp" && echo yes || echo no)"
out=$("$BASHY" --bashpp --source=c "$p6/hello.go" 2>&1);             check "P6.unknown language is refused" "2" "$?"
out=$("$repo/bin/bash" --bashpp --source=go "$p6/hello.go" 2>&1);    check "P6.the bash drop-in refuses --source=go" "2" "$?"
out=$("$BASHY" --bashpp --check "$p6/hello.go" 2>&1);                check "P6.--check without --source=go is refused" "2" "$?"
# $@ is preserved and the selectors never become the script's arguments.
cat > "$p6/args.go" <<'EOF'
package main

import (
	"fmt"
	"os"
)

func main() { fmt.Println(len(os.Args), os.Args[1], os.Args[2]) }
EOF
out=$("$BASHY" --bashpp --source=go "$p6/args.go" one two 2>&1); args_exit=$?
# The CLI's half of this (the selectors never become $@, the operand is $0)
# is asserted in internal/cli; evaluating os.Args is an sh-side gap today, so
# this is recorded rather than adjudicated.
check "P6.arguments are not mistaken for selectors" "clean" \
	"$(printf '%s' "$out" | grep -qE "flag provided but not defined|--source|--bashpp" && echo leaked || echo clean)"
printf 'INFO  P6.os.Args under the interpreter: exit=%s out=[%s]\n' "$args_exit" "$out"

# --------------------------------------------------------------------------
# P7  transpile must never write over a collected original
# --------------------------------------------------------------------------
p7=$work/p7; mkdir -p "$p7/pkg" "$p7/alias"
cat > "$p7/pkg/main.go" <<'EOF'
package main

import "fmt"

func main() { fmt.Println(helper()) }
EOF
cat > "$p7/pkg/helper.go" <<'EOF'
package main

func helper() string { return "helper" }
EOF
ln -s "$p7/pkg/helper.go" "$p7/alias/symlink.go"
ln "$p7/pkg/helper.go" "$p7/alias/hardlink.go"
sum_main=$(shasum -a 256 < "$p7/pkg/main.go")
sum_helper=$(shasum -a 256 < "$p7/pkg/helper.go")

collide() { # NAME  ARGS...
	local name=$1; shift
	local out exit
	out=$("$BASHY" transpile --bashpp --source=go "$@" 2>&1); exit=$?
	check "P7.$name refused" "2" "$exit"
	check "P7.$name says so" "yes" \
		"$(printf '%s' "$out" | grep -q "cannot be the same file" && echo yes || echo no)"
}
collide "directory recipe -o over a collected original" "$p7/pkg" -o "$p7/pkg/helper.go" --map "$p7/pkg/x.map"
collide "directory recipe --map over a collected original" "$p7/pkg" -o "$p7/out.go" --map "$p7/pkg/helper.go"
collide "directory recipe -o through a symlink alias" "$p7/pkg" -o "$p7/alias/symlink.go" --map "$p7/x.map"
collide "directory recipe -o through a hardlink alias" "$p7/pkg" -o "$p7/alias/hardlink.go" --map "$p7/x.map"
collide "explicit --go-file -o over that file" --go-file "$p7/pkg/helper.go" -o "$p7/pkg/helper.go" --map "$p7/x.map"
check "P7.main.go bytes unchanged"   "$sum_main"   "$(shasum -a 256 < "$p7/pkg/main.go")"
check "P7.helper.go bytes unchanged" "$sum_helper" "$(shasum -a 256 < "$p7/pkg/helper.go")"
# The legitimate destination still works.
"$BASHY" transpile --bashpp --source=go "$p7/pkg" -o "$p7/out/generated.go" --map "$p7/out/generated.go.map" 2>"$p7/ok.err"
check "P7.a destination outside the package still transpiles" "0" "$?"

# --------------------------------------------------------------------------
# P8  directory selection follows the Go build constraints
# --------------------------------------------------------------------------
p8=$work/p8; mkdir -p "$p8/pkg" "$p8/run"
host_os=$("$GO" env GOOS)
other_os=linux; [ "$host_os" = linux ] && other_os=darwin
cat > "$p8/pkg/main_$host_os.go" <<EOF
package main

import "fmt"

func main() { fmt.Println("selected $host_os") }
EOF
cat > "$p8/pkg/main_$other_os.go" <<EOF
package main

import "fmt"

func main() { fmt.Println("selected $other_os") }
EOF
cat > "$p8/pkg/go.mod" <<'EOF'
module example.com/p8

go 1.26
EOF
cat > "$p8/pkg/excluded.go" <<'EOF'
//go:build ignore

package main

func neverSelected() {}
EOF
p8base=$(cd "$p8/pkg" && "$GO" build -p 2 -o "$p8/run/base" . && cd "$p8/run" && ./base 2>&1)
p8interp=$(cd "$p8/run" && "$BASHY" --bashpp --source=go "$p8/pkg" 2>&1); p8_exit=$?
check "P8.constrained directory runs"       "0" "$p8_exit"
check "P8.selection matches go build"       "$p8base" "$p8interp"

# --------------------------------------------------------------------------
# P9  a Go program must not pick up shell startup or logout files
# --------------------------------------------------------------------------
p9=$work/p9; mkdir -p "$p9/home" "$p9/run"
printf 'printf LOGOUT-RAN > %s/marker\n' "$p9" > "$p9/home/.bash_logout"
printf 'printf RC-RAN > %s/rcmarker\n' "$p9" > "$p9/home/.bashrc"
out=$(cd "$p9/run" && HOME=$p9/home GOPATH=$GOPATH GOMODCACHE=$GOMODCACHE GOCACHE=$GOCACHE \
	"$BASHY" --login --bashpp --source=go "$p1/src/hello.go" 2>&1)
check "P9.--login Go program still runs"    "Hello, 世界" "$(printf '%s' "$out" | head -1)"
check "P9.~/.bash_logout did not run"       "absent" "$([ -e "$p9/marker" ] && echo present || echo absent)"
check "P9.~/.bashrc did not run"            "absent" "$([ -e "$p9/rcmarker" ] && echo present || echo absent)"

echo
printf 'probes: %d passed, %d failed\n' "$pass" "$fail"
[ "$keep" = 1 ] && echo "scratch tree: $work"
[ "$fail" -eq 0 ]
