---
name: hugo
description: bashy dag front door for Hugo — focused test, build, fenced package smoke, and launched CLI
default: smoke
---

# Hugo — task graph

Run from an unchanged Hugo checkout. The fenced function imports Hugo's own
`common/hugo` package; generated worker source is visible only through Go's
overlay and the normal module/toolchain rules remain authoritative.

## Tasks

### test
The repository's focused version-package tests.
Effects: read, write, net, exec

```bash
go test -mod=readonly ./common/hugo
```

### build
Build Hugo at the repository's gitignored `dist/hugo` path.
Effects: read, write, net, exec
Generates: dist/hugo
Timeout: 30m

```bash
go build -mod=readonly -o dist/hugo .
```

### smoke
Read `CurrentVersion` through Hugo's own Go package.
Effects: read, write, net

```bsh
~~~go as go
import hugopkg "github.com/gohugoio/hugo/common/hugo"

func Hugo() string { return hugopkg.CurrentVersion.String() }
~~~
got := go.Hugo()
[ -n "$got" ] || { echo "smoke: go.Hugo() returned empty" >&2; exit 1; }
echo "smoke: hugo $got"
```

### run
Launch the built Hugo command from the Go fence.
Requires: build
Effects: read, write, net

```bsh
~~~go as go
import "os/exec"
import "strings"

func Launch() (string, error) {
	out, err := exec.Command("dist/hugo", "version").Output()
	return strings.TrimSpace(string(out)), err
}
~~~
got := go.Launch()
case "$got" in 'hugo v'*) ;; *) echo "run: unexpected hugo output: $got" >&2; exit 1 ;; esac
echo "run: $got"
```
