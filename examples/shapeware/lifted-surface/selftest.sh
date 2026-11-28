#!/usr/bin/env bash
# Self-test of the reusable lifted-surface checks/candidate.bsh and the
# launcher's gotest verb (shapeware spec §4.3b, §6, Appendix D).
#
# It builds a tiny Go repository with one lifted surface, shapeware/calc/,
# whose checks/candidate.bsh and scripts/launcher.sh are this folder's files,
# then grades work copies (git clones) of it:
#   allowed       a change inside metadata.candidate-files passes
#   reproducible  the same change gives the same VARIANT
#   outside       a change outside the globs fails
#   untracked     a new untracked file outside the globs fails
#   contract      a changed CONTRACT.md fails
#   weakened      a changed features.yaml line fails
#   broken        an allowed change that breaks the pinned go test fails
#   ignored       a new git-ignored Go file outside the globs fails
#   symlink       a tracked symlink re-pointed to another directory fails
#   mode          an exec-bit change outside the globs fails
#   outputs       untracked build outputs (metadata.build-outputs) are left out
#   protected-link  a protected file replaced by a symlink to the same bytes fails
#   quotes        single-quoted metadata values are read like the lint reads them
#   spec-change   with --spec-change, a fix that changes a pinned expectation
#                 (features.yaml, CONTRACT.md, Known issues) passes against the
#                 copy's spec and lists the change; without the flag it fails
#   spec-checks   with --spec-change, a changed checks/ file still fails
#   spec-skill    with --spec-change, SKILL.md above Known issues still fails
#   spec-skill-after  with --spec-change, a section after Known issues still fails
#   spec-link     with --spec-change, a checks/ file replaced by a symlink to the
#                 same bytes still fails
#   spec-file-link  with --spec-change, a spec file that becomes a symlink fails
#   deleted       a deleted candidate file is hashed as "deleted" (one side
#                 lacks the file)
#   fail-fast     after a protected-file or file-set FAIL the copy is never
#                 built: the build step's marker is not written
# and runs the launcher's gotest verb directly:
#   gotest-root, gotest-nested, gotest-nested-deep  a ./path in the root
#                 module and in a nested module (its own go.mod) passes
#   gotest-dep    a dependency import path required in go.mod passes; the
#                 dependency is a local module (replace => ../dep), so offline
#   testmain-fake-*-fail  a TestMain that prints a fake "--- PASS" (and a
#                 fake -list line) without running the test fails
#   residual-testmain-forgery-passes  a TestMain written to forge both -list
#                 and -json still passes: the residual spec §6 states
#   env-no-launcher-impl  LAUNCHER_IMPL does not reach the test
#   old-*         a go 1.16 module: a required dependency passes; a module
#                 only in the build list (and in a replace line) fails
#   writes-*-fail a test that writes go.mod or go.sum (root or nested
#                 module) fails, and the write is not restored
#   gotest-*-fail no matching test, "..", "./...", a missing directory, a
#                 symlink out of the implementation, an import path not
#                 required in go.mod, one in the implementation's own module
#                 (cite it as ./path), and a NAME that is not one top-level
#                 test (Test.*, A|B, a subtest, bare Test) each print FAIL NAME
# Needs bashy, git and go. Everything is written under one temporary folder.
# With CONFORM=/path/to/conform it also lints the fake surface.
# Usage: bash selftest.sh    Exit 1 if any case does not behave as stated.
set -u
export BASHY_HINTS=off
src=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
export GOWORK=off GOPROXY=off GOFLAGS=-mod=readonly
export GIT_AUTHOR_NAME=selftest GIT_AUTHOR_EMAIL=selftest@example.invalid
export GIT_COMMITTER_NAME=selftest GIT_COMMITTER_EMAIL=selftest@example.invalid

base=$work/base
s=$base/shapeware/calc
mkdir -p "$base/internal/calc" "$base/cmd/calc" "$s/checks" "$s/scripts"
cp "$src/checks/candidate.bsh" "$s/checks/candidate.bsh"
cp "$src/scripts/launcher.sh" "$s/scripts/launcher.sh"
printf '.shapeware/\nbin/\nnode_modules/\nzz_*.go\n' >"$base/.gitignore"
mkdir -p "$base/docs/v1" "$base/docs/v2"
echo one >"$base/docs/v1/page.md"
echo two >"$base/docs/v2/page.md"
ln -s v1 "$base/docs/current"
printf 'module example.invalid/calc\n\ngo 1.21\n\nrequire example.invalid/dep v0.0.0\n\nreplace example.invalid/dep => ../dep\n' >"$base/go.mod"
# A dependency module, outside the implementation, and a nested module inside it.
mkdir -p "$work/dep/greet" "$base/tools/sub/inner"
printf 'module example.invalid/dep\n\ngo 1.21\n' >"$work/dep/go.mod"
printf 'package greet\n\nimport "testing"\n\nfunc TestGreet(t *testing.T) {}\n' >"$work/dep/greet/greet_test.go"
printf 'module example.invalid/calc/tools/sub\n\ngo 1.21\n' >"$base/tools/sub/go.mod"
printf 'package sub\n\nimport "testing"\n\nfunc TestSub(t *testing.T) {}\n' >"$base/tools/sub/sub_test.go"
printf 'package inner\n\nimport "testing"\n\nfunc TestInner(t *testing.T) {}\n' >"$base/tools/sub/inner/inner_test.go"
printf 'package calc\n\n// Extra is deleted by the deleted case.\nconst Extra = 1\n' >"$base/internal/calc/extra.go"
cat >"$base/internal/calc/calc.go" <<'EOF'
package calc

// Add returns a + b.
func Add(a, b int) int { return a + b }
EOF
cat >"$base/internal/calc/calc_test.go" <<'EOF'
package calc

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("Add(2, 3) != 5")
	}
}
EOF
cat >"$base/cmd/calc/main.go" <<'EOF'
package main

import (
	"fmt"
	"os"
	"strconv"

	"example.invalid/calc/internal/calc"
)

func main() {
	a, _ := strconv.Atoi(os.Args[1])
	b, _ := strconv.Atoi(os.Args[2])
	fmt.Println(calc.Add(a, b))
}
EOF
cat >"$s/SKILL.md" <<'EOF'
---
name: calc
description: Specify the add surface of a fake calculator. Use only in the lifted-surface self-test.
metadata:
  type: shapeware
  shapeware: "0.1"
  version: "0.1.0"
  implementation: "https://example.invalid/calc.git"
  candidate-files: "internal/calc/*.go"
  build-outputs: "bin/* node_modules"
---
# calc

Type: shapeware (v0.1)

## Known issues

## Notes
Plain notes, outside the Known issues section.
EOF
cat >"$s/CONTRACT.md" <<'EOF'
# calc contract

Plain text is compared exactly. Objects are closed; there are none.

### C-01
The pinned go test passes.
```console
$ calc gotest ./internal/calc TestAdd
PASS TestAdd
```
exit: 0
EOF
cat >"$s/features.yaml" <<'EOF'
- id: add
  status: verified
  intent: Two numbers add up.
  acceptance:
    - calc gotest ./internal/calc TestAdd => PASS TestAdd
    - calc-app 2 3 => 5
- id: modules
  status: verified
  intent: Pinned go tests in a nested module and in a dependency.
  acceptance:
    - calc gotest ./tools/sub/inner TestInner => PASS TestInner
    - calc gotest example.invalid/dep/greet TestGreet => PASS TestGreet
EOF
cat >"$s/dag.md" <<'EOF'
---
name: calc
description: Lifecycle of the calc self-test surface.
type: dag
default: test
---
# calc: lifecycle

## Tasks

### source
The implementation is the repository around this folder.
Effects: read, write
Band: none

```bash
mkdir -p .shapeware && git rev-parse --show-toplevel > .shapeware/impl
```

### build
Compile the implementation (or IMPL) into SHAPEWARE_BIN and write the calc launcher.
Effects: read, write, exec
Band: unmeasured

```bash
set -eu
impl=$(cd "${IMPL:-$(cat .shapeware/impl)}" && pwd -P)
bin=${SHAPEWARE_BIN:-$PWD/.shapeware/bin}
quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }
mkdir -p "$bin"
(cd "$impl" && go build -o "$bin/calc-app" ./cmd/calc)
printf '#!/bin/sh\nLAUNCHER_IMPL=%s exec sh %s "$@"\n' "$(quote "$impl")" "$(quote "$PWD/scripts/launcher.sh")" >"$bin/calc"
chmod +x "$bin/calc"
[ -z "${BUILD_MARK:-}" ] || echo "$impl" >>"$BUILD_MARK"
echo "build: ok"
```

### test
Run the acceptance checks.
Effects: read, exec
Band: none

```bash
"$BASHY" --bashsharp ./checks/acceptance.bsh
```

### install
Nothing to install in the self-test.
Effects: read
Band: none

```bash
echo "install: nothing to do"
```

### run
Run calc with ARGS.
Effects: read, exec
Band: none

```bash
mapfile -d '' -t argv < <(printf '%s' "${ARGS:-2 3}" | xargs printf '%s\0')
"${SHAPEWARE_BIN:-.shapeware/bin}/calc-app" "${argv[@]}"
```
EOF
cat >"$s/checks/acceptance.bsh" <<'EOF'
# Acceptance for the calc self-test surface: PASS|FAIL|TBD <id>#<n> <example>.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
export PATH=${SHAPEWARE_BIN:-$here/.shapeware/bin}:$PATH
fail=0 id='' status=tbd n=0
while IFS= read -r line; do
  case $line in
    '- id: '*) id=${line#- id: }; status=tbd; n=0 ;;
    '  status: '*) status=${line#  status: } ;;
    '    - '*' => '*)
      ex=${line#    - }; n=$((n+1))
      cmd=${ex%% => *}; want=${ex#* => }
      mapfile -d '' -t argv < <(printf '%s' "$cmd" | xargs printf '%s\0')
      got=$("${argv[@]}" 2>&1); rc=$?
      if [[ $rc -eq 0 && $got == "$want" ]]; then echo "PASS $id#$n $ex"
      elif [[ $status == tbd ]]; then echo "TBD $id#$n $ex"
      else echo "FAIL $id#$n $ex (exit $rc, got: ${got:-<none>})"; fail=1; fi ;;
  esac
done < "$here/features.yaml"
exit $fail
EOF
git -C "$base" init -q -b main && git -C "$base" add -A && git -C "$base" commit -qm base
# Ignored build outputs in the installed checkout only, as after a real build.
mkdir -p "$base/bin" "$base/node_modules/pkg/lib"
echo binary >"$base/bin/calc"
echo 'module.exports = 1' >"$base/node_modules/pkg/lib/index.js"

failed=0
n=0
copy() { n=$((n + 1)); c=$work/copy$n; git clone -q "$base" "$c"; }
export BUILD_MARK=$work/built
built() { grep -qxF "$(cd "$1" && pwd -P)" "$BUILD_MARK" 2>/dev/null; } # COPY: did a build run on it
grade() { (cd "$s" && IMPL=$base bashy --bashsharp checks/candidate.bsh "$@") >"$work/out" 2>"$work/err"; }
expect() { # NAME WANT-EXIT GREP-PATTERN
  local rc=$1 name=$2 want=$3 pat=$4
  if [[ $rc -eq $want ]] && grep -q -- "$pat" "$work/out"; then
    echo "ok   $name (exit $rc)"
  else
    echo "FAIL $name: exit $rc, want $want and a line matching '$pat'"
    sed 's/^/     | /' "$work/out" "$work/err"
    failed=1
  fi
}

copy; printf '\n// Sub is not used yet.\nfunc Sub(a, b int) int { return a - b }\n' >>"$c/internal/calc/calc.go"
printf '    - calc-app 4 5 => 9\n' >>"$c/shapeware/calc/features.yaml"
grade "$c"; expect $? allowed 0 '^VARIANT 0.1.0+shaped.[0-9a-f]\{7\}$'
grep -q '^FAIL' "$work/out" && { echo "FAIL allowed: a FAIL line was printed"; failed=1; }
v1=$(grep '^VARIANT' "$work/out")
grade "$c"; expect $? reproducible 0 "^$v1\$"

copy; printf '// changed\n' >>"$c/cmd/calc/main.go"
grade "$c"; expect $? outside 1 '^FAIL change within metadata.candidate-files (cmd/calc/main.go)'

copy; echo note >"$c/notes.txt"
grade "$c"; expect $? untracked 1 '^FAIL change within metadata.candidate-files (notes.txt)'

copy; printf '\nOne more line.\n' >>"$c/shapeware/calc/CONTRACT.md"
grade "$c"; expect $? contract 1 '^FAIL protected shapeware/calc/CONTRACT.md unchanged'

copy; sed 's/=> 5$/=> 6/' "$base/shapeware/calc/features.yaml" >"$c/shapeware/calc/features.yaml"
grade "$c"; expect $? weakened 1 '^FAIL features.yaml only adds lines'

copy; sed 's/return a + b/return a - b/' "$base/internal/calc/calc.go" >"$c/internal/calc/calc.go"
grade "$c"; expect $? broken 1 '^FAIL add#1 calc gotest ./internal/calc TestAdd => PASS TestAdd'

copy; printf 'package main\n\nfunc init() {}\n' >"$c/cmd/calc/zz_hook.go"
grade "$c"; expect $? ignored 1 '^FAIL change within metadata.candidate-files (cmd/calc/zz_hook.go)'

copy; rm "$c/docs/current" && ln -s v2 "$c/docs/current"
grade "$c"; expect $? symlink 1 '^FAIL change within metadata.candidate-files (docs/current)'

copy; chmod +x "$c/cmd/calc/main.go"
grade "$c"; expect $? mode 1 '^FAIL change within metadata.candidate-files (cmd/calc/main.go)'

copy; printf '// changed\n' >>"$c/internal/calc/calc.go"
grade "$c"; expect $? outputs 0 '^PASS changes within metadata.candidate-files (1 files)'

copy; rm "$c/shapeware/calc/checks/acceptance.bsh" && ln -s "$s/checks/acceptance.bsh" "$c/shapeware/calc/checks/acceptance.bsh"
grade "$c"; expect $? protected-link 1 '^FAIL protected shapeware/calc/checks/acceptance.bsh unchanged'

sed "s|candidate-files: \"internal/calc/\*.go\"|candidate-files: 'internal/calc/*.go'|" "$s/SKILL.md" >"$work/skill" && mv "$work/skill" "$s/SKILL.md"
grep -q "candidate-files: 'internal" "$s/SKILL.md" || { echo "FAIL quotes: SKILL.md was not rewritten"; failed=1; }
git -C "$base" commit -qam 'single quotes'
copy; printf '// changed\n' >>"$c/internal/calc/calc.go"
grade "$c"; expect $? quotes 0 '^PASS changes within metadata.candidate-files (1 files)'
# A fix whose pinned expectation changes: 2 + 3 now reads 6.
spec_fix() {
  sed 's/return a + b/return a + b + 1/' "$base/internal/calc/calc.go" >"$c/internal/calc/calc.go"
  sed 's/Add(2, 3) != 5/Add(2, 3) != 6/; s/!= 5"/!= 6"/' "$base/internal/calc/calc_test.go" >"$c/internal/calc/calc_test.go"
  sed 's/=> 5$/=> 6/' "$base/shapeware/calc/features.yaml" >"$c/shapeware/calc/features.yaml"
  sed 's/The pinned go test passes./The pinned go test passes (2 + 3 reads 6)./' "$base/shapeware/calc/CONTRACT.md" >"$c/shapeware/calc/CONTRACT.md"
  awk '{ print } /^## Known issues$/ { print "- 2 + 3 read 5; the fix reads 6." }' "$base/shapeware/calc/SKILL.md" >"$c/shapeware/calc/SKILL.md"
}
copy; spec_fix
grade "$c"; expect $? spec-change-without-flag 1 '^FAIL features.yaml only adds lines'
grade --spec-change "$c"; expect $? spec-change 0 '^VARIANT 0.1.0+shaped.[0-9a-f]\{7\}$'
grep -q '^FAIL' "$work/out" && { echo "FAIL spec-change: a FAIL line was printed"; sed 's/^/     | /' "$work/out"; failed=1; }
for pat in '^SPEC-CHANGE files: CONTRACT.md SKILL.md features.yaml$' '^SPEC-CHANGE features.yaml - .*calc-app 2 3 => 5$' \
  '^SPEC-CHANGE features.yaml + .*calc-app 2 3 => 6$' '^SPEC-CHANGE assertion C-01 changed$' '^PASS add#2 calc-app 2 3 => 6$'; do
  grep -q -- "$pat" "$work/out" || { echo "FAIL spec-change: no line matching '$pat'"; failed=1; }
done

copy; spec_fix; printf '# weakened\nexit 0\n' >"$c/shapeware/calc/checks/acceptance.bsh"
grade --spec-change "$c"; expect $? spec-checks 1 '^FAIL protected shapeware/calc/checks/acceptance.bsh unchanged'

copy; spec_fix; sed 's/version: "0.1.0"/version: "0.2.0"/' "$c/shapeware/calc/SKILL.md" >"$c/skill" && mv "$c/skill" "$c/shapeware/calc/SKILL.md"
grade --spec-change "$c"; expect $? spec-skill 1 '^FAIL SKILL.md changes only its Known issues'

copy; spec_fix; printf 'One more note.\n' >>"$c/shapeware/calc/SKILL.md"
grade --spec-change "$c"; expect $? spec-skill-after 1 '^FAIL SKILL.md changes only its Known issues'

copy; spec_fix; rm "$c/shapeware/calc/checks/acceptance.bsh" && ln -s "$s/checks/acceptance.bsh" "$c/shapeware/calc/checks/acceptance.bsh"
grade --spec-change "$c"; expect $? spec-link 1 '^FAIL protected shapeware/calc/checks/acceptance.bsh unchanged'

copy; spec_fix; mv "$c/shapeware/calc/CONTRACT.md" "$work/contract.$n" && ln -s "$work/contract.$n" "$c/shapeware/calc/CONTRACT.md"
grade --spec-change "$c"; expect $? spec-file-link 1 '^FAIL spec file shapeware/calc/CONTRACT.md is a regular file'

# One side lacks a file: a deleted candidate file is hashed as "deleted".
copy; rm "$c/internal/calc/extra.go"
want=$(printf 'deleted  internal/calc/extra.go\n' | { if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi; } | cut -c1-7)
grade "$c"; expect $? deleted 0 "^VARIANT 0.1.0+shaped.$want\$"

# Fail fast: no build after a comparison FAIL; a passing copy is built.
copy; printf '\nOne more line.\n' >>"$c/shapeware/calc/CONTRACT.md"
grade "$c"; expect $? fail-fast-protected 1 '^VARIANT '
built "$c" && { echo "FAIL fail-fast-protected: the copy was built after a protected-file FAIL"; failed=1; }
copy; printf 'package main\n\nfunc init() { panic("ran") }\n' >"$c/cmd/calc/zz_hook.go"
grade "$c"; expect $? fail-fast-files 1 '^FAIL copy builds (dag.md build) (not run'
built "$c" && { echo "FAIL fail-fast-files: the copy was built after a file-set FAIL"; failed=1; }
copy; printf '// changed\n' >>"$c/internal/calc/calc.go"
grade "$c"; expect $? fail-fast-passing 0 '^PASS copy builds'
built "$c" || { echo "FAIL fail-fast-passing: the build marker was not written"; failed=1; }

# The launcher's gotest verb, run directly against the base implementation.
gotest() { # NAME WANT-LINE PKG TEST
  local name=$1 want=$2 got rc
  got=$(LAUNCHER_IMPL=$base sh "$src/scripts/launcher.sh" gotest "$3" "$4" 2>&1); rc=$?
  local wrc=1; [[ $want == PASS* ]] && wrc=0
  if [[ $got == "$want" && $rc -eq $wrc ]]; then echo "ok   $name (exit $rc)"
  else echo "FAIL $name: got '$got' exit $rc, want '$want'"; failed=1; fi
}
gotest gotest-root 'PASS TestAdd' ./internal/calc TestAdd
gotest gotest-nested 'PASS TestSub' ./tools/sub TestSub
gotest gotest-nested-deep 'PASS TestInner' ./tools/sub/inner TestInner
gotest gotest-dep 'PASS TestGreet' example.invalid/dep/greet TestGreet
gotest gotest-nested-nomatch-fail 'FAIL TestNoSuch' ./tools/sub TestNoSuch
gotest gotest-nested-wrong-module-fail 'FAIL TestInner' ./tools/sub TestInner
gotest gotest-dotdot-fail 'FAIL TestSub' ./tools/../tools/sub TestSub
gotest gotest-pattern-fail 'FAIL TestSub' ./tools/... TestSub
gotest gotest-missing-fail 'FAIL TestSub' ./tools/nope TestSub
gotest gotest-dep-nomatch-fail 'FAIL TestNoSuch' example.invalid/dep/greet TestNoSuch
gotest gotest-not-required-fail 'FAIL TestGreet' example.invalid/other/greet TestGreet
# Operands the launcher refuses before any go test runs (it never widens).
gotest gotest-name-pattern-fail 'FAIL Test.*' ./internal/calc 'Test.*'
gotest gotest-name-alternation-fail 'FAIL TestAdd|TestSub' ./internal/calc 'TestAdd|TestSub'
gotest gotest-name-subtest-fail 'FAIL TestAdd/x' ./internal/calc TestAdd/x
gotest gotest-name-bare-fail 'FAIL Test' ./internal/calc Test
gotest gotest-name-not-test-fail 'FAIL Add' ./internal/calc Add
gotest gotest-root-pattern-fail 'FAIL TestAdd' ./... TestAdd
gotest gotest-bare-dotslash-fail 'FAIL TestAdd' ./ TestAdd
gotest gotest-parent-fail 'FAIL TestAdd' ../base/internal/calc TestAdd
gotest gotest-absolute-fail 'FAIL TestAdd' "$base/internal/calc" TestAdd
gotest gotest-flag-fail 'FAIL TestAdd' -race TestAdd
gotest gotest-stdlib-fail 'FAIL TestAdd' strings TestAdd
gotest gotest-dep-pattern-fail 'FAIL TestGreet' example.invalid/dep/... TestGreet
gotest gotest-dep-dotdot-fail 'FAIL TestGreet' example.invalid/x/../dep/greet TestGreet
gotest gotest-own-module-path-fail 'FAIL TestAdd' example.invalid/calc/internal/calc TestAdd
ln -s "$work/dep" "$base/escape"
gotest gotest-symlink-out-fail 'FAIL TestGreet' ./escape/greet TestGreet
rm "$base/escape"

# Review fixes (sprint 742 M6), in implementations of their own so the base
# stays untouched: a TestMain that fakes a PASS, a test that writes go.mod or
# go.sum, the launcher's environment, and a module in the build list that
# go.mod does not require.
m6=$work/m6
mkdir -p "$m6/ghost" "$m6/ghostlist" "$m6/writer" "$m6/n" "$m6/env"
printf 'module example.invalid/m6\n\ngo 1.21\n' >"$m6/go.mod"
cat >"$m6/ghost/ghost_test.go" <<'EOF'
package ghost

import (
	"fmt"
	"os"
	"testing"
)

// TestMain never runs a test; it prints the line a passing test prints.
func TestMain(m *testing.M) {
	fmt.Println("=== RUN   TestGhost")
	fmt.Println("--- PASS: TestGhost (0.00s)")
	fmt.Println("PASS")
	os.Exit(0)
}
EOF
cat >"$m6/ghostlist/ghost_test.go" <<'EOF'
package ghostlist

import (
	"fmt"
	"os"
	"testing"
)

// TestMain also fakes the -list output.
func TestMain(m *testing.M) {
	fmt.Println("TestGhost")
	fmt.Println("--- PASS: TestGhost (0.00s)")
	os.Exit(0)
}
EOF
cat >"$m6/writer/writer_test.go" <<'EOF'
package writer

import (
	"os"
	"testing"
)

// TestWriteRoot appends to the implementation's go.mod (the module root).
func TestWriteRoot(t *testing.T) {
	f, err := os.OpenFile("../go.mod", os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("// written by a test\n")
}
EOF
printf 'module example.invalid/m6/n\n\ngo 1.21\n' >"$m6/n/go.mod"
cat >"$m6/n/n_test.go" <<'EOF'
package n

import (
	"os"
	"testing"
)

// TestWriteNested creates go.sum in its nested module.
func TestWriteNested(t *testing.T) {
	if err := os.WriteFile("go.sum", nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
EOF
cat >"$m6/env/env_test.go" <<'EOF'
package env

import (
	"os"
	"testing"
)

// TestNoLauncherImpl: the launcher does not pass LAUNCHER_IMPL to a test.
func TestNoLauncherImpl(t *testing.T) {
	if v, ok := os.LookupEnv("LAUNCHER_IMPL"); ok {
		t.Fatalf("LAUNCHER_IMPL=%q reached the test", v)
	}
}
EOF
# A go 1.16 module (unpruned graph): olddep2 is in the build list through
# olddep's go.mod, but the implementation's go.mod does not require it.
old=$work/old
mkdir -p "$old" "$work/olddep/y" "$work/olddep2/x"
printf 'module example.invalid/old\n\ngo 1.16\n\nrequire example.invalid/olddep v0.0.0\n\nreplace example.invalid/olddep => ../olddep\n\nreplace example.invalid/olddep2 => ../olddep2\n' >"$old/go.mod"
printf 'module example.invalid/olddep\n\ngo 1.16\n\nrequire example.invalid/olddep2 v0.0.0\n' >"$work/olddep/go.mod"
printf 'package y\n\nimport "testing"\n\nfunc TestY(t *testing.T) {}\n' >"$work/olddep/y/y_test.go"
printf 'module example.invalid/olddep2\n\ngo 1.16\n' >"$work/olddep2/go.mod"
printf 'package x\n\nimport "testing"\n\nfunc TestX(t *testing.T) {}\n' >"$work/olddep2/x/x_test.go"
gotest_in() { # IMPL NAME WANT-LINE PKG TEST
  local impl=$1 name=$2 want=$3 got rc wrc=1
  got=$(LAUNCHER_IMPL=$impl sh "$src/scripts/launcher.sh" gotest "$4" "$5" 2>&1); rc=$?
  [[ $want == PASS* ]] && wrc=0
  if [[ $got == "$want" && $rc -eq $wrc ]]; then echo "ok   $name (exit $rc)"
  else echo "FAIL $name: got '$got' exit $rc, want '$want'"; failed=1; fi
}
mkdir -p "$m6/forge"
cat >"$m6/forge/forge_test.go" <<'EOF'
package forge

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain forges both the -list and the -json output of a test that does
// not exist: the residual the spec states (§6, a cited test trusts its
// package's TestMain).
func TestMain(m *testing.M) {
	flag.Parse()
	if flag.Lookup("test.list").Value.String() != "" {
		fmt.Println("TestGhost")
	} else {
		fmt.Println("--- PASS: TestGhost (0.00s)")
	}
	os.Exit(0)
}
EOF
gotest_in "$m6" testmain-fake-pass-fail 'FAIL TestGhost' ./ghost TestGhost
gotest_in "$m6" residual-testmain-forgery-passes 'PASS TestGhost' ./forge TestGhost
gotest_in "$m6" testmain-fake-list-fail 'FAIL TestGhost' ./ghostlist TestGhost
gotest_in "$m6" env-no-launcher-impl 'PASS TestNoLauncherImpl' ./env TestNoLauncherImpl
gotest_in "$old" old-required-dep 'PASS TestY' example.invalid/olddep/y TestY
gotest_in "$old" old-unrequired-dep-fail 'FAIL TestX' example.invalid/olddep2/x TestX
cp "$m6/go.mod" "$work/m6.go.mod"
gotest_in "$m6" writes-root-go-mod-fail 'FAIL TestWriteRoot' ./writer TestWriteRoot
cmp -s "$m6/go.mod" "$work/m6.go.mod" && { echo "FAIL writes-root-go-mod-fail: the test did not write go.mod"; failed=1; }
gotest_in "$m6" writes-nested-go-sum-fail 'FAIL TestWriteNested' ./n TestWriteNested
[[ -f $m6/n/go.sum ]] || { echo "FAIL writes-nested-go-sum-fail: the test did not write go.sum (nothing is restored)"; failed=1; }

if [[ -n ${CONFORM:-} ]]; then
  if "$CONFORM" spec check "$s" >"$work/out" 2>&1; then echo "ok   lint (conform spec check)"
  else echo "FAIL lint"; sed 's/^/     | /' "$work/out"; failed=1; fi
fi
exit $failed
