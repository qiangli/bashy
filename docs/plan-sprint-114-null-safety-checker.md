# Sprint 114 null-safety checker plan

Status: implementation plan for Sprint 114 story #173.

## Contract

`bashy check --bashpp FILE` statically rejects a possibly nil pointer
dereference, slice index, or function call. A guard may narrow a value for the
guarded path; a later assignment invalidates that narrowing. Successful checks
are silent. Rejections exit 2 and emit only the stable `BASHPP-ENULL-*`
diagnostic required by the Bash# acceptance corpus.

Without `--bashpp`, the existing Classic/POSIX checker remains unchanged and
must never emit a `BASHPP-ENULL-*` diagnostic. This work adds no runtime syntax,
optional chaining, nullish operator, transpiler, or lowering surface.

## Implementation

1. Add `--bashpp` to the existing `check` option parser and help/atlas prose.
2. Run a checker-local Go AST analysis for Go-shaped Bash++ function source.
   Track nil-capable parameters, branch refinements, terminating guards, and
   reassignment invalidation.
3. Preserve the existing shell parser and command-closure analysis as the
   fallback for non-Go-shaped input; select `syntax.LangBashPP` only for that
   static parse when requested.
4. Add byte-exact tests for all six null-safety oracle cases, selector-off
   isolation, help, and JSON/report integration.
5. Run focused tests, the sibling Sprint 114 executable oracle, the repository
   gate, and cross-build checks before committing.
