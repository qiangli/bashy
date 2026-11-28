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
# Needs bashy, git and go. Everything is written under one temporary folder.
# With CONFORM=/path/to/conform it also lints the fake surface.
# Usage: bash selftest.sh    Exit 1 if any case does not behave as stated.
set -u
export BASHY_HINTS=off
src=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
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
printf 'module example.invalid/calc\n\ngo 1.21\n' >"$base/go.mod"
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
grade() { (cd "$s" && IMPL=$base bashy --bashsharp checks/candidate.bsh "$1") >"$work/out" 2>"$work/err"; }
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

if [[ -n ${CONFORM:-} ]]; then
  if "$CONFORM" spec check "$s" >"$work/out" 2>&1; then echo "ok   lint (conform spec check)"
  else echo "FAIL lint"; sed 's/^/     | /' "$work/out"; failed=1; fi
fi
exit $failed
