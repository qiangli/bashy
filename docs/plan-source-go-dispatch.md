# `--source=go` input dispatch — Bashy CLI contract (Sprint 118, W1 Bashy half)

Status: **shipped in the default build.** `../sh` at `7bd10632` carries
`mvdan.cc/sh/v3/gosource` and `lower.NewModuleImporter`, so the temporary
`bashy_gosource` build tag, its stub and the `BASHY_GOSOURCE` make variable are
**gone**: `make build` links the front end, and no `--source=go` invocation can
be answered with "not available in this build". The refusal string is kept for
the pure `bash` drop-in, which structurally cannot link it.

What is measured, and how, is §7 and §8 — including the two places where the
chain still stops inside the sh interpreter rather than inside this repo.

Owner: Bashy worker (`internal/cli`, `internal/agentos/transpile.go` +
`internal/agentos/gosource.go`, tests, docs, `scripts/gosource-probe.sh`). The
Go front end itself is owned by the sh worker; Bashy does not reimplement Go
source parsing, and this repo contains no Go lexer, parser or type checker of
its own.

Design of record for the surrounding sprint: `sprint-118-master-execution-plan.md`
§2 "Source-front-end contract", items 6 and 7. The shared execution contract
this must satisfy is `bashpp-tests/tools/corpus/README.md`.

## 1. What this delivers

Item 7 of the sprint contract asks for Go-source selection that is **explicit
inside Bash++ mode** and leaves Classic/POSIX activation semantics untouched:

```
bashy --bashpp --source=go original.go            # interpret an unchanged Go program
bashy --bashpp --source=go --check original.go    # validate only; execute nothing
bashy transpile --bashpp --source=go original.go -o generated.go --map generated.go.map
```

Item 6 asks that **malformed Go in Go-source mode fails with Go-source
diagnostics, never shell fallback**. That is the single most important
property here and it holds structurally: once `--source=go` is accepted, the
shell parser is never constructed for that input on any path, including the
error paths.

## 2. The sh API this assumes

Recorded here **before** sh lands so the two halves can be compared rather
than guessed. Taken from the sh worker's skeleton in
`sh/gosource/source.go` (workspace `sh-7e2e7b65/workspaces/issue-28`) as of
2026-09-08:

```go
package gosource // mvdan.cc/sh/v3/gosource

const Version = "gosource-v1"

type Source     struct { Name string; Data []byte }
type SourceInfo struct { Name, SHA256 string; Base, Size uint }

type Options struct {
    RunMain  bool           // append init+main entry calls; loading never executes
    Importer types.Importer // nil uses the Go export importer
}

type Program struct {
    File          *syntax.File
    Package       string
    Sources       []SourceInfo
    InitFunctions []string
    Main          string
}

func (p *Program) SourceAt(pos syntax.Pos) (name string, offset uint, ok bool)
func Parse(r io.Reader, name string, opts Options) (*Program, error)
func Load(sources []Source, opts Options) (*Program, error)
```

**Assumptions Bashy is coded against.** If sh diverges on any of these, the
adapter in `internal/agentos/gosource_full.go` changes and nothing else does.

| # | Assumption | Why Bashy needs it |
|---|---|---|
| A1 | `Load` is pure: it parses, type-checks and converts. It never executes user code, not even with `RunMain: true`. | `--check` must execute nothing. `RunMain: false` is the check mode; the *absence* of entry-call statements is what makes it safe, so Bashy also refuses to hand the result to a runner in check mode. |
| A2 | `Load` returns a non-nil error for malformed or ill-typed Go, and that error's text is already Go-shaped (`file:line:col: message`). | Bashy prints it verbatim and exits 2. Bashy adds no prefix that would corrupt a position, and never re-tries as shell. |
| A3 | `Program.File` is a `*syntax.File` valid for both `interp.Runner.Run` and `lower.Compile`. | One front end feeds both product modes with the same semantic input (contract item 5). |
| A4 | Positions inside `Program.File` are offsets into a concatenated space described by `Sources[i].Base/.Size`, and `SourceAt` inverts that. | The transpile source map must name the **original** file per mapping (contract item 6). Bashy re-implements the same interval arithmetic over `Sources` so a map can be produced without a live `*gosource.Program`; the two must agree. |
| A5 | `Sources` carries the SHA-256 of the exact bytes read, unmodified. | Evidence binding: the map artifact records the digest so a harness can prove the tested bytes were the pinned bytes. |
| A6 | `Load` sorts by `Name`; duplicate names are an error. | Bashy passes files in the order the user gave and does not pre-sort, so sh's ordering is the only ordering. |
| A7 | `syntax.File` gains a `GoSource bool` field (the skeleton sets it). | Not required by Bashy, but Bashy must not break if it appears. Bashy reads no new `syntax` field today. |
| A8 | `Options.Importer` accepts `lower.NewModuleImporter(dir)`, the same module-aware importer lowering uses. | A program that imports a helper module must type-check identically in interpreted and compiled mode. Bashy always supplies it, keyed on the SOURCE's directory — never on the process working directory, which the harness deliberately empties. Leaving it nil resolved the standard library only, which rejected every Tour helper module at load time. |

**Divergence check against the landed sh (`7bd10632`).** Every assumption above
holds as written; the adapter compiles unchanged against the real package and
`gosource.Version` is `gosource-v1`. Two things the sh worker has announced but
has NOT yet landed, and which this repo therefore does not read:

- `syntax.File.Sources`, and `lower.Mapping.Source` / `.SourceOffset` — the
  frontend-side source metadata. Until they exist, the transpile map resolves a
  mapping through Bashy's own A4 interval arithmetic over `Program.Sources`.
  That mapping is valid today and **fails closed**: a position that resolves to
  no original file aborts the run with exit 2 rather than emitting a mapping
  with no `source_file`. When the frontend fields land, this repo switches to
  them and the fail-closed behavior stays.
- Original-file identity inside the sh runtime/lower ERROR path. A lowering
  diagnostic still speaks in the converted program's terms; the CLI's own
  diagnostics and the map do name the original file.

## 3. CLI surface

### 3.1 Selector flags

Consumed and removed from `os.Args` before Go's `flag` package sees them
(same treatment `--bashpp` already gets in `stripBashPPInvocationFlags`), so
they never become script operands or `flag` errors.

| Flag | Meaning |
|---|---|
| `--source=sh` / `--source sh` | Explicit default. Shell input. Accepted so a harness can always spell the language. |
| `--source=go` / `--source go` | The operand (or stdin, or `-c`, or `--go-file`) is Go source. |
| `--check` | Validate semantically and execute nothing. Requires `--source=go`. |
| `--go-version=go1.12` / `--go-version go1.12` | Pass the requested language version unchanged to the Go source type checker. Requires `--source=go`; accepted by both `--check` and transpile. It does not select or download an SDK. |
| `--go-file=PATH` / `--go-file PATH` | Repeatable. Names one original file of a multi-file package explicitly. |

Scanning follows the rule `commandLineBashPP` already uses: stop at `--`, at
`-c`, or at the first operand that does not start with `-`. Last one wins for
`--source`.

**All three argv scanners share one set of value-taking options**
(`invocationFlagTakesValue`: `-o -O --rcfile --init-file -bashy-plus-o
-bashy-plus-O --source --go-file`). They must, because each stops at the first
token that does not look like an option: a scanner that did not know `--source`
takes a value read `go` as the script operand and silently dropped every
selector behind it, so `bashy --source go --bashpp x.go` died with `flag
provided but not defined: -bashpp`. The Go-source scan now also runs FIRST, so
`-c` relocation and the Bash++ strip never meet a `--source go` pair at all.
`--source=go` and `--source go` are equivalent in either order, and arguments
after the operand are untouched.

### 3.2 Inputs

| Form | Meaning |
|---|---|
| `bashy --bashpp --source=go prog.go [args...]` | Single-file package. `$0` is `prog.go`; `args` are the program's arguments, exactly as the existing `--bashpp script` interface. |
| `bashy --bashpp --source=go ./dir [args...]` | Directory recipe, selected by **the Go toolchain's own rules** through `go/build` (`//go:build`, `// +build`, `_GOOS`/`_GOARCH` filename suffixes, the host build context; tests and `_`/`.`-prefixed names excluded). `go build ./dir` and this recipe select the same files on the same host — required, because a package carrying `main_darwin.go` and `main_linux.go` builds natively either way and collecting both rejected it as a duplicate `main`. A cgo package is refused by name rather than silently reduced. The directory is the module dir. |
| `bashy --bashpp --source=go --go-file a.go --go-file b.go [args...]` | Explicit multi-file package, byte-for-byte the files named, **no filtering at all** — this bypass is deliberate and is what the Tour's `OMIT` oracle uses: a caller that names files has already made the selection. `$0` is the first `--go-file`. All operands are program arguments. |
| `bashy --bashpp --source=go` with stdin, or `-s` | Go source read from stdin, named `-`. |
| `bashy --bashpp --source=go -c 'package main; …'` | Go source from the command string, named `-c`. |

`--go-file` and a file/directory operand together is an error, not a merge:
which one is `$0` and which is an argument would be a guess.

### 3.3 Refusals

All exit **2** and print one line to stderr. None of them fall back to shell
parsing.

| Condition | Diagnostic |
|---|---|
| `--source=X`, X not `sh`/`go` | `bashy: --source: unknown input language "X" (expected "sh" or "go")` |
| `--source=go` without Bash++ (or with `--no-bashpp`) | `bashy: --source=go requires --bashpp` |
| `--source=go` with POSIX selected (`--posix`, `-o posix`, `POSIXLY_CORRECT`, argv0 `sh`) | `bashy: --source=go is not available in POSIX mode` |
| `--source=go` on the `bash` drop-in | `bashy: --source=go requires the bashy front door` |
| `--check` without `--source=go` | `bashy: --check requires --source=go` |
| `--go-file` without `--source=go` | `bashy: --go-file requires --source=go` |
| `--go-file` plus a file/dir operand | `bashy: --go-file cannot be combined with a file operand` |
| `--source=go` with `--pretty-print`, `--dump-strings` or `--dump-po-strings` | `bashy: --pretty-print cannot be combined with --source=go` (and so on) |
| Front end absent from this build (the `bash` drop-in only) | `bashy: --source=go: the Go source front end (mvdan.cc/sh/v3/gosource) is not available in this build` |
| Malformed / ill-typed Go | sh's diagnostic, verbatim |

Why POSIX is refused rather than silently ignored: `--posix` is a
*conformance* selection. Sprint 114 made `bash --bashpp --posix` an inert
compatibility profile precisely so a POSIX claim can never be affected by a
Bash++ feature. Accepting Go input under it would reopen that. Classic and
POSIX activation semantics are therefore untouched by this work: a shell
invocation that does not spell `--source=go` reaches byte-identical code.

Why the shell-only modes are refused rather than ignored: `--pretty-print` and
the `--dump-strings` family are implemented by constructing the SHELL parser,
and they ran BEFORE input dispatch. `bashy --bashpp --source=go --pretty-print
prog.go` therefore reported `a command can only contain words and redirects;
encountered '('` — a shell parse error about a valid Go program, which is the
exact confusion this selector exists to end. Two independent guards now hold:
the mode is refused during resolution, and the Go dispatch in `runAll` stands
ahead of every shell-only mode and every `-c` shell preflight, so a future
preflight cannot quietly get in front of Go input either.

Why the `bash` drop-in refuses: `cmd/bash` structurally cannot link the
AgentOS surface, and the Go front end is wired through the same hook seam
(`cli.GoSourceLoad`, `cli.GoSourcePackageFiles`, `cli.GoSourceModuleDir`). The
refusal is the layer boundary, stated out loud.

**Corrected 2026-09-08 (sprint 118 final integration).** This paragraph used to
add "linking `go/types` into `bin/bash` would also grow the drop-in". That is no
longer a live argument: sh `de4ff069`'s Bash++ native bridge
(`interp/bashpp_native_bridge.go`) imports `go/types` and `go/importer` in
package `interp`, which BOTH binaries link — measured, `go list -deps
./cmd/bash` now names `go/types` and `go/build`. What the boundary still buys is
the FRONT END: `mvdan.cc/sh/v3/gosource` and `internal/agentos` are absent from
`cmd/bash`, so `--source=go` remains refused there. Pinned by
`TestGoSourceFrontEndIsNotLinkedIntoClassicBash`.

### 3.4 `--check` versus `-n`

They are different and both are kept:

- `-n` / `-o noexec` is the **shell**'s noexec. Under `--source=go` it is
  honoured as "do not execute", so a build-only corpus row can spell either.
- `--check` is **semantic** validation: the Go program is parsed, type-checked
  and converted, and then discarded. `RunMain: false`, so the loaded program
  carries no entry-call statements at all — nothing is handed to a runner.
  Exit 0 and no output on success; sh's diagnostics and exit 2 on failure.

`bashy --bashpp -n prog.go` on the *shell* path only proved the shell parser
accepted the bytes, which is what made the planning probe misleading. Under
`--source=go`, neither spelling can pass without the Go type checker agreeing.

## 4. `bashy transpile --bashpp --source=go`

```
bashy transpile --bashpp --source=go original.go -o generated.go --map generated.go.map
bashy transpile --bashpp --source=go --go-file a.go --go-file b.go -o generated.go
bashy transpile --bashpp --source=go ./dir -o generated.go
```

`--bashpp` remains required (unchanged). `--source=sh` is the existing
behavior. The output and map are still written atomically together.

**The output and the map are checked against every COLLECTED original before a
byte is written.** A directory operand expands to files the command line never
named, so checking only what was spelled let `transpile --source=go ./pkg -o
./pkg/main.go` overwrite an upstream program with its own generated output —
and unchanged-source ingestion is worth nothing if the input does not survive
it. The check uses the existing same-file/alias comparison, which resolves
symlinks and compares inode identity, so a symlink or a hard link to an
original is refused as the original. It also covers the DEFAULT map path, which
is derived from `-o` and so inherits its collision.

**A non-main package transpiles.** `go build` compiles `package p; var X int`,
so a build-phase corpus row must be able to transpile one too; asking the front
end for entry calls unconditionally rejected every such package with "Go
execution requires package main with func main()". Main-ness is a fact of the
source and Bashy does not parse Go, so the front end reports it: the load runs
once WITHOUT entry calls — which is also exactly the artifact a non-main
package needs — and is repeated with them only when that first load reports
package `main` and a `main` function. No entry call is ever synthesised here,
and *running* a non-main package is still refused, in the front end's words.

The map artifact keeps schema `bashy-transpile-map-v1` and gains fields that
are omitted entirely for shell input, so existing consumers are unaffected:

```json
{
  "schema_version": "bashy-transpile-map-v1",
  "origin": "original.go",
  "go_digest": "sha256:…",
  "source_kind": "go",
  "front_end": "gosource-v1",
  "sources": [{"name": "original.go", "sha256": "…", "base": 0, "size": 123}],
  "mappings": [
    {"go_line": 12, "go_col": 2,
     "source_line": 7, "source_col": 3, "source_offset": 88,
     "source_file": "original.go", "source_file_offset": 88,
     "node": "CallExpr"}
  ]
}
```

`source_file` / `source_file_offset` are resolved through the same interval
arithmetic sh's `Program.SourceAt` uses (assumption A4), so a mapping always
names an **original** file and an offset into that file's own bytes — which is
what a multi-file package needs and what a harness needs to point a failure at
upstream source.

This **fails closed**. For Go input, a mapping whose position resolves to no
original file aborts the run with exit 2; it is never emitted with the field
omitted. Omitting it would publish a map whose entries silently mean "some
file", and a consumer cannot tell that apart from shell input, which omits both
fields legitimately.

## 5. Implementation shape

```
internal/cli/gosource.go          selector parsing, resolution, refusals,
                                  input collection, the GoSourceLoad and
                                  GoSourcePackageFiles hook seams, origin→file
                                  position mapping, the run path. No Go parser.
internal/cli/main.go              strips the selector flags before flag.Parse
                                  (FIRST, ahead of -c relocation and the Bash++
                                  strip); runAll dispatches Go input ahead of
                                  every shell-only mode and preflight.
internal/cli/bashpp.go            commandLineBashPP shares the value-taking
                                  invocation-flag set.
internal/agentos/gosource.go      adapts mvdan.cc/sh/v3/gosource and
                                  lower.NewModuleImporter onto the hooks, and
                                  selects directory recipes through go/build.
                                  The ONLY file in this repo importing either.
internal/agentos/transpile.go     --source/--go-file parsing, Go input,
                                  collision protection, map fields.
scripts/gosource-probe.sh         the end-to-end probes of §8, driven through
                                  the DEFAULT-BUILT launcher.
```

The hook seam is the repo's existing `AgentOSDispatch` pattern, and it is what
keeps the layer boundary structural: `cmd/bash` cannot reach
`internal/agentos`, so the drop-in links no `gosource` front end no matter what
changes here. It DOES link `go/build`, `go/parser`, `go/types` and
`go/importer` — `mvdan.cc/sh/v3/interp` imports them for the Bash++ import
bridge and (since sh `de4ff069`) for its native dependency bridge. That comes
from the shared interpreter, not from this work, and the seam neither adds nor
removes it.

Both hooks are wired unconditionally in `init()`. There is no build tag and no
opt-in: a required capability that can be absent is a capability every caller
has to test for, and the review found exactly that — a valid Go hello exiting 2
from a default binary.

## 6. Boundaries, and where the chain stops today

Named so none of it is mistaken for done, and none of it is mistaken for
Bashy-side work that was skipped.

**Bashy-side, deliberate:**

- **`go:embed` and other asset-bearing directives** are not interpreted here.
- **Nested packages.** One package per invocation: the package in the given
  directory, or the given files. Its module dependencies resolve through
  `lower.NewModuleImporter`; a second package in the same recipe does not.
- **`-c` Go input** is supported but is an extension of the sprint spelling.
  Flag it at the contract gate if the harness should not have it.

**sh-side, measured, and outside this story's ownership** (see §8 for the exact
observations):

- **Interpreted mode resolves Bash++ imports in the RUNNER's working
  directory** (`interp/bashpp_import.go`, `Dir: r.Dir`), which is also the
  program's working directory. Type checking is fixed — `--check` and
  `transpile` resolve a helper module from any cwd, because the importer is
  keyed on the SOURCE's directory. The interpreted RUN cannot be given the same
  treatment from this repo: `interp` exposes one directory for both concerns,
  so pointing it at the module would move the program's cwd away from the
  harness's assets-only runtime directory — resolving the import and breaking
  every relative asset path in the same stroke, which turns a loud failure into
  a wrong answer.

  The sh worker is landing `interp.GoSourceModuleDir(dir) RunnerOption` for
  exactly this (`sh` issue-28 `.agents/gosource-api.md`; manager note
  2026-09-09 04:18). **The CLI seam is already in place and tested**:
  `cli.GoSourceModuleDir` is a nil-able hook applied to the runner with the
  source's directory, and a test pins both halves — the hook receives the
  source directory, and the working directory is NOT moved. Wiring it when the
  option lands is one line.
- **Evaluating some converted expressions** is not implemented in the
  interpreter yet: a cross-package call reports `BASHPP-EEXPR-FORM: unsupported
  scalar call`, `os.Args` reports `BASHPP-ESELECTOR-ROOT: … is not a structured
  value`, and a slice expression reports `BASHPP-EEXPR-FORM: unsupported scalar
  expression *syntax.BashPPSliceExpr`. Compiled mode has none of these
  limitations; the same programs build and run.

## 7. What is proven, and what is not

The front end is linked in the default build, so the tests load real Go source
through it. Nothing below is a hook-shaped stand-in unless it says so.

Proven by `go test ./internal/cli ./internal/agentos`:

- Every refusal in §3.3, including that a POSIX or Classic invocation is
  rejected rather than silently accepted, and that the shell-only modes are
  refused with no shell parser constructed.
- That the three argv scanners agree: `--source go --bashpp`,
  `--bashpp --source go`, `--source=go --bashpp`, a selector behind another
  option's value, and arguments after the operand left untouched.
- That the selectors never reach `flag` or the program's `$@`, and that the
  operand is `$0` while the arguments after it are `$1…` in order.
- Input collection: single file, directory, explicit `--go-file` list, stdin,
  the `--go-file` + operand conflict, and the directory refusal in a build with
  no front end.
- **Directory selection through `go/build`**: a package with a per-GOOS `main`
  selects only the host's file and then LOADS, where collecting both was a
  duplicate `main`; `//go:build ignore` and unset-tag files are excluded; a cgo
  package is refused by name.
- **That `transpile` never writes over a collected original**: output over a
  package member, map over a package member, output through a symlink alias,
  output through a hard-link alias, and the derived default map path — each
  exits 2, and the originals' bytes are compared before and after. A
  destination outside the package still transpiles.
- **A build-only non-main package** transpiles, produces a valid map, loads
  under `--check`, and is still refused for execution.
- **Module imports resolve** from an assets-only cwd with an absolute source in
  a separate module directory, with a negative control proving the importer is
  what resolves them.
- Diagnostics for malformed, ill-typed and unresolvable-import Go name the
  ORIGINAL file and carry no shell wording.
- That a `--source=go` program does not run `~/.bash_logout`, with a positive
  control proving the shell path still does.

**Not proven here, and not claimed:** that any corpus row passes. This repo's
probes are not the corpus runner — they hash nothing, build no provenance
record, and adjudicate only what they print. Two parts of the interpreted chain
stop inside sh, not here (§6), and the compiled chain is complete.

## 8. Evidence recorded for this commit

Commands run in the isolated weave workspace
`.bashy/weave/bashy-6497d06f/workspaces/issue-3`, darwin/arm64, `GOMAXPROCS=2`,
`go -p 2`, `GOTOOLCHAIN=go1.27.0`, sibling `../sh` at `7bd10632`:

| Command | Result |
|---|---|
| `go build ./...` | pass |
| `go vet ./...` | pass |
| `go test -count=1 ./...` | pass, every package |
| `go test -tags e2e -run TestE2EAllListedCommandsDispatch ./internal/agentos` | pass |
| `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/bashy` | pass |
| `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./cmd/bash` | pass |
| `make build` (DEFAULT, no tags) | pass — `bin/bash.real` 9,611,266 B, `bin/bashy.real` 97,814,818 B |
| `go list -deps ./cmd/bash \| grep '^go/types$'` | **match (corrected 2026-09-08)** — package `interp`'s native bridge imports `go/types`; the drop-in boundary is the FRONT END, not the type checker |
| `go list -deps ./cmd/bash \| grep -E 'internal/agentos\|gosource'` | no match — the layer boundary is structural |
| `go list -deps ./cmd/bashy \| grep gosource` | `mvdan.cc/sh/v3/gosource` — linked by DEFAULT |
| `scripts/gosource-probe.sh` | **72 probes passed, 0 failed** (2026-09-08: P4 foreign-cwd promoted INFO→assert, P4b added) |

### 8.1 The probes, through the default-built launcher

`scripts/gosource-probe.sh` drives `bin/bashy` (the C launcher over
`bin/bashy.real`), never `go run` and never an in-process hook. The three modes
are the corpus contract's. Highlights:

- **P1 unchanged hello** — baseline `go build -o program hello.go`, interpreted
  `bashy --bashpp --source=go hello.go`, and compiled (`transpile` → `go build`
  → **delete the generated source** → run with **`PATH=`**) all print the same
  bytes and exit 0. The original's SHA-256 is compared before and after. The
  map names the original file.
- **P2 globals and init ordering** — interpreted and compiled both match the
  native `init1 4 3 true / init2 / main 4 3`.
- **P3 build-only non-main** — `--check` on a file and on a directory exits 0
  and prints nothing (the package's `init` would have printed
  `SHOULD-NOT-RUN`); the package transpiles, builds natively, and running it
  produces no output; running it as a program is refused with exit 2.
- **P4 helper module** — `--check` and `transpile` resolve `example.com/helper`
  from a foreign cwd, and the compiled program prints the helper's string. The
  interpreted run is RECORDED, not adjudicated: from the module cwd the import
  resolves and the run then hits the interpreter's `BASHPP-EEXPR-FORM:
  unsupported scalar call`; from a foreign cwd it reports `bash++ import
  "example.com/helper": package could not be resolved` (§6).
- **P5 diagnostics** — a syntax error reports
  `…/syntaxerr.go:3:1: expected declaration, found 'if'` and a type error
  `…/typeerr.go:5:27: undefined: undefinedName`, both naming the original file,
  both exit 2, in run and in `--check`, with no shell wording. The syntax-error
  fixture's body is `if true; then echo shell-ran; fi`, which a shell would have
  happily run.
- **P6 routing** — all three flag orders run the program; `--pretty-print`,
  `--dump-strings`, `--posix`, `--no-bashpp`, an unknown language, `--check`
  without the selector, and the `bash` drop-in each exit 2 with the right
  refusal, and the `--pretty-print` refusal carries no shell parse error.
- **P7 collisions** — five destructive shapes refused, originals byte-identical
  afterwards, legitimate destination still works.
- **P8 build constraints** — a package with `main_darwin.go` and
  `main_linux.go` runs, and prints what `go build .` prints on this host.
- **P9 startup files** — a `--login` Go program runs and neither `~/.bashrc`
  nor `~/.bash_logout` executes.

The probe exports `BASHY_HINTS=off` — the documented master switch — because a
proactive agent hint on stderr is a property of the terminal driving the shell,
not of the shell under test.

### 8.2 Bash 5.3 fixture gate

`make test-bash` was run natively against a cached Bash 5.3 fixture tree
(`external/bash-5.3` → the pinned corpus under the user cache), with a clean
`PATH`:

```
Results: 82 passed, 4 failed, 0 skipped, 0 timed out
FAIL: jobs, read, test, vredir
```

Those four were then re-measured **against the pre-change build**: the working
tree was stashed, `bin/bash` rebuilt at `7de7c02`, and
`make test-bash-run TESTS="jobs read test vredir"` run again — `0 passed, 4
failed`, the same four. They are this host's native-lane environment (job
control without a controlling terminal, `read` timeouts, descriptor
behaviour), not a regression from this change, and the count is 82/86 both
before and after it.

This is the fast host-integration lane, not the release verdict. The
authoritative result is `make test-bash-container`, which needs a container
runtime this workspace does not have; that gate is still owed and is the
manager's to run.

## Checker language version

The `--go-version` value travels through `GoSourceOptions.GoVersion` into `gosource.Options.GoVersion` and `types.Config.GoVersion`. Original bytes, including upstream `// -lang=...` comments, remain unchanged. Omission preserves the existing checker default. Empty CLI values and invalid versions fail; future versions cannot silently fall back to the current language. This option specifies checker feature restrictions; it is not certification of historical runtime/compiler behavior.

Flags after the input operand or `--` remain program arguments on the execution entry point. All invocation scanners share the separated-value flag classification. Ordinary shell input cannot opt into this checker option.
