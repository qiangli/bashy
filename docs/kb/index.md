# kb index

1 page(s). Search: `bashy kb search <query>` — check before starting a task; `bashy kb retro` after. Pages live under pages/.

- #1 [transpile-standalone-go-mod-tidy-required-before-go-build](pages/transpile-standalone-go-mod-tidy-required-before-go-build.md) `candidate/lesson` transpile --standalone: go mod tidy required before go build — When hello_standalone.bsh is transpiled with --standalone, the generated go.mod uses a replace directive to github.com/qiangli/sh. 'go build' fails with missing go.sum entries unless 'GOPROXY=direct GONOSUMDB="*" go mod tidy' is run first in the output directory. This applies any time you transpile a Bash# script to a standalone Go binary from a fresh directory.
