# KB — relevant repo and host knowledge (may be stale; verify before relying on it)

Check these before you start — they may save you a failed approach:

- [repo/page] transpile --standalone: go mod tidy required before go build — When hello_standalone.bsh is transpiled with --standalone, the generated go.mod uses a replace directive to github.com/qiangli/sh. 'go build' fails with missing go.sum entries unless 'GOPROXY=direct GONOSUMDB="*" go mod tidy' is run first in the output dir  (kb:transpile-standalone-go-mod-tidy-required-before-go-build)

More: `bashy kb search <query>` (or `bashy kb show <slug>`).

AFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`
— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.
