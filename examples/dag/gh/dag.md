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
Effects: read, write, net

```bash
go test -mod=readonly ./internal/build
```

### build
Build the real `gh` command at the repository's gitignored output path.
Effects: read, write, net
Generates: bin/gh
Timeout: 30m

```bash
go build -mod=readonly -o bin/gh ./cmd/gh
```

### smoke
Import the checkout's own `internal/build` package and return its version.
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
got := go.Gh()
[ -n "$got" ] || { echo "smoke: go.Gh() returned empty" >&2; exit 1; }
banner := helper.heading("gh $got")
case "$banner" in *"== gh $got"*) ;; *) echo "smoke: checkout helper returned '$banner'" >&2; exit 1 ;; esac
echo "smoke: gh $got"
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
