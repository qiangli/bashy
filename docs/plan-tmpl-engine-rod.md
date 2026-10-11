# Plan: tmpl engine rod (Sprint 419, Story #1901)

Spec of record: "Engine rod" in the umbrella's `docs/sprint-419-master-execution-plan.md`.
Go `text/template` stays the default engine; a compliant third-party engine plugs in
through the existing runner override. No new grammar, no `sh/` change.

- **Fence:** `~~~tmpl as T !ENGINE` is the existing runner override (`polyglot.RunnerFence`).
  ENGINE is a Bash#/shell function of the unit or a registered command (or a command bashy
  dispatches itself); PATH is never consulted. The engine's `methods` answer carries each
  method's effects, which is what `@guard` reads (`interp.ForeignEffectGate`).
- **CLI:** `bashy tmpl render --engine ENGINE FILE --data ...` (`internal/tmpl`,
  `internal/agentos/tmpl_engine.go`). Omitted or `go` is the built-in engine. A function
  engine lives in a unit, so from the command line ENGINE is a registered command or a bashy verb.
- **Contract:** `ENGINE methods FILE` must list a `render` export; `ENGINE render FILE
  DATA_JSON` prints the result, non-zero is an error. Otherwise the CLI refuses with
  "engine is not template-compliant" - never a fallback to Go.
- **Effects:** purity is guaranteed only for the built-in engine (`read`); a third-party
  engine's declared `exec`/`net` is allowed but visible and guardable.

Known engine limit (sh, out of scope): on the fence path the compliance wording is the
engine's own - a runner with no methods fails with "declared no methods", a missing
`render` fails the statement form `T.render(...)` with "package T is not imported"; the
assignment form `r := T.render(...)` of an undeclared method binds the literal expression
text instead of failing.
