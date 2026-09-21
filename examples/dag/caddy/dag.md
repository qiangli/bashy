---
name: caddy
description: bashy dag front door for Caddy — focused test, build, fenced package smoke, and launched CLI
default: smoke
---

# Caddy — task graph

Run from an unchanged Caddy checkout. The fenced function imports the root
Caddy package from the checkout's module; its worker is built with an overlay,
not with a generated file in the checkout.

## Tasks

### test
The repository's focused root-package tests.
Effects: read, write, net, exec

```bash
go test -mod=readonly .
```

### build
Build the real command at Caddy's gitignored development output path.
Effects: read, write, net, exec
Generates: cmd/caddy/caddy
Timeout: 30m

```bash
go build -mod=readonly -o cmd/caddy/caddy ./cmd/caddy
```

### smoke
Call Caddy's own `Version` function and return its simple form.
Effects: read, write, net

```bashpp
~~~go as go
import caddypkg "github.com/caddyserver/caddy/v2"

func Caddy() string {
	simple, _ := caddypkg.Version()
	return simple
}
~~~
got := go.Caddy()
[ -n "$got" ] || { echo "smoke: go.Caddy() returned empty" >&2; exit 1; }
echo "smoke: caddy $got"
```

### run
Launch the built Caddy command from the Go fence.
Requires: build
Effects: read, write, net

```bashpp
~~~go as go
import "os/exec"
import "strings"

func Launch() (string, error) {
	out, err := exec.Command("cmd/caddy/caddy", "version").Output()
	return strings.TrimSpace(string(out)), err
}
~~~
got := go.Launch()
[ -n "$got" ] || { echo "run: caddy version returned empty" >&2; exit 1; }
echo "run: $got"
```

### image
Package the checkout's Linux build into an image declared by a fenced
Dockerfile, then run `caddy version` from that image. The binary is built
outside the checkout (the tree stays byte-identical) and reaches the build as
a named build context; the fence body is the whole image definition.
Requires: test
Effects: read, write, net, exec

```bashpp
~~~dockerfile as img
FROM docker.io/library/alpine:3.20
COPY --from=src caddy /usr/bin/caddy
ENTRYPOINT ["/usr/bin/caddy"]
~~~
out="${DAG_CACHE_DIR:-${TMPDIR:-/tmp}}/caddy-image"
mkdir -p "$out"
GOOS=linux GOARCH="$(go env GOARCH)" CGO_ENABLED=0 go build -o "$out/caddy" ./cmd/caddy || exit 1
id := img.build("--build-context", "src=$out")
[ -n "$id" ] || { echo "image: img.build() returned no image id" >&2; exit 1; }
got := img.run("$id", "version")
[ -n "$got" ] || { echo "image: caddy version from the image returned empty" >&2; exit 1; }
echo "image: caddy $got"
```
