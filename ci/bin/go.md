---
name: go
provides: go
run: go
---

# go — repair-profile wrapper unit

`go` for the repair profiles (S549.R4b): every suite runs inside the bashy
shell, and there `go` resolves to bashy's own pinned Go toolchain provider
(`bashy go`, digest-verified, cached under the user cache) — never a host
binary. The profile runner materializes this unit onto the suite's private
PATH, so bare `go` inside a suite (including from a Makefile recipe or
`go list` expansion) is bashy's `go` on a host that has none.

`{{bashy}}` is substituted with the pinned bashy executable at
materialization time; the runner executes the fenced body through that same
bashy (`#!/…/bashy --bashsharp`), so nothing is installed on the host.

```bashpp
"{{bashy}}" go "$@"
```
