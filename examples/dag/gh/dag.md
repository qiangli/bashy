---
name: gh
description: bashy dag front door for GitHub CLI — focused test, build, fenced package smoke, and launched CLI
default: smoke
---

# GitHub CLI — task graph

Run this graph from an unchanged `cli/cli` checkout with
`bashy awd DIR -- bashy dag -f THIS_FILE TARGET`. The Go fence is compiled in
the checkout's module through an overlay, so it can import `internal/build`
without placing generated source in the repository.

## Tasks

### test
The repository's focused version-package tests.
Effects: read, write, net, exec

```bash
go test -mod=readonly ./internal/build
```

### build
Build the real `gh` command at the repository's gitignored output path.
Effects: read, write, net, exec
Generates: bin/gh
Timeout: 30m

```bash
go build -mod=readonly -o bin/gh ./cmd/gh
```

### smoke
Import the checkout's own `internal/build` package and return its version —
under a contract. The island call is wrapped in ONE agentic function carrying
`@require`/`@ensure`/`@guard`, and the body is the harness: it drives the
function through every exit status the contract can produce and asserts each
one, so the target's own exit is the verdict (Sprint 216, Story 541).

- **3** — `@require` refuses an empty argument; the body never runs.
- **126** — `@guard(effects: "read")` denies the write the `leak` probe
  attempts (dag's cap handler, the same `advice.Cap` seam as `Effects:`).
- **6** — *input required*: with no `GH_SMOKE_LABEL` the function yields;
  `@ensure` does not run, so a harness never sees a postcondition mask it.
- **0** — the resume: the harness supplies the answer explicitly in the
  environment and the same call completes; `@ensure` sees the result.

Every completion appends one receipt to the skills/craft ledger
(`docs/function-attestation.md`); `bashy craft history gh_version --all`
reads it back — 3 and 126 as `FAIL`, 6 as `yield`, 0 as `pass`.
Offline and deterministic: no model, no network, no file left behind.
Effects: read, write, net

```bashpp
~~~go as go
import buildpkg "github.com/cli/cli/v2/internal/build"

func Gh() string {
	if buildpkg.Version == "" { return "DEV" }
	return buildpkg.Version
}
~~~
~~~sh as helper
# Verbatim from script/api-host-gateway/test.sh.
heading() {
	printf '\n== %s\n' "$1"
}
~~~
# The guard probe's target: under the cap it is never created (asserted below).
leak=${TMPDIR:-/tmp}/gh-smoke-leak.$$

@guard(effects: "read")
@require('test -n "$1"')
@ensure('test -n "$gh_version_result"')
agentic function gh_version() {
	# "leak": try to write under a read-only cap — denied before it runs.
	[ "$1" != leak ] || { touch "$leak"; return $?; }
	# Input required: the label is the answer the harness must supply.
	[ -n "${GH_SMOKE_LABEL-}" ] || return 6
	version := go.Gh()
	gh_version_result="$version ($GH_SMOKE_LABEL)"
	echo "smoke: gh $version"
}

# expect NAME WANT GOT — one assertion per exit status, no transcript diff.
expect() {
	[ "$3" = "$2" ] && { echo "smoke: gh_version $1 -> $3"; return 0; }
	echo "smoke: gh_version $1 -> $3, want $2" >&2
	exit 1
}

agentic {
	gh_version "";      expect require 3 $?
	gh_version leak;    expect guard 126 $?
	gh_version ask;     expect yield 6 $?
	GH_SMOKE_LABEL=cli/cli gh_version ask
	expect resume 0 $?
}
[ ! -e "$leak" ] || { echo "smoke: guard did not deny the write: $leak" >&2; exit 1; }
case "$gh_version_result" in ?*" (cli/cli)") ;; *) echo "smoke: unexpected result '$gh_version_result'" >&2; exit 1 ;; esac
banner := helper.heading("gh ${gh_version_result% (cli/cli)}")
case "$banner" in *"== gh "*) ;; *) echo "smoke: checkout helper returned '$banner'" >&2; exit 1 ;; esac
```

### run
Launch the binary produced by `build` from the Go fence.
Requires: build
Effects: read, write, net

```bashpp
~~~go as go
import "os/exec"
import "strings"

func Launch() (string, error) {
	out, err := exec.Command("bin/gh", "--version").Output()
	return strings.TrimSpace(string(out)), err
}
~~~
got := go.Launch()
case "$got" in 'gh version '*) ;; *) echo "run: unexpected gh output: $got" >&2; exit 1 ;; esac
echo "run: ${got%%$'\n'*}"
```
