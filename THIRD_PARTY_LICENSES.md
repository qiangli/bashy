# Third-party licenses (bashy)

Scope: what the `bashy` and `bash` binaries compile in, embed, link or vendor
**directly**. Policy of record: `docs/licensing-supply-chain-policy.md` §1
(BSD / MIT / Apache-2.0 only for anything compiled in). The sibling modules
carry their own inventories and are not repeated here:
`../sh/THIRD_PARTY_LICENSES.md`, `../yoke/THIRD_PARTY_LICENSES.md`,
`../coreutils/THIRD_PARTY_LICENSES.md`.

Programs bashy **downloads and runs** (toolchains, engines, providers) are a
different category and are NOT in this file: see
`docs/fence-toolchain-licenses.md` for the Bash# fence toolchains and the
Sprint 350 runtime-acquisition inventory for everything else.

Licenses below were read from the module's own `LICENSE` file in the Go module
cache or the sibling checkout, not inferred from a repository badge.

## First-party siblings (flat `replace` directives in `go.mod`)

| Module | Dir | License |
|---|---|---|
| `mvdan.cc/sh/v3` (qiangli fork of mvdan/sh; the shell engine, Bash# evaluator, `polyglot`) | `../sh` | BSD-3-Clause (Daniel Martí; clean-room, no GNU Bash code) |
| `github.com/bashsharp/bashsharp` | `../bashsharp` | BSD-3-Clause |
| `github.com/qiangli/coreutils` | `../coreutils` | MIT |
| `github.com/qiangli/yoke` (+ `external/otel`, `pkg/oci`, `pkg/llmgw`) | `../yoke` | MIT |
| `github.com/qiangli/outpost` | `../outpost` | MIT |
| `github.com/ergochat/readline` (qiangli fork) | `../readline` | MIT |
| `github.com/filebrowser/filebrowser/v2` (qiangli fork) | `../filebrowser` | Apache-2.0 |
| `github.com/dhnt/dhnt` | module | Apache-2.0 |

## Direct third-party modules

| Module | Purpose | License |
|---|---|---|
| `github.com/spf13/cobra` | CLI tree | Apache-2.0 |
| `github.com/creack/pty/v2` | pty for agent sessions | MIT |
| `github.com/aymanbagabas/go-pty` | cross-platform pty | MIT |
| `github.com/rjeczalik/notify` | recursive fs watch (`bashy mirror`) | MIT |
| `github.com/pkg/sftp` | sftp client | BSD-2-Clause |
| `gopkg.in/yaml.v3` | YAML | MIT + Apache-2.0 (dual, per its LICENSE) |
| `github.com/fatih/color` | terminal color | MIT |
| `go.opentelemetry.io/otel`, `/sdk`, `/trace` | telemetry | Apache-2.0 |
| `golang.org/x/{sys,term,text,crypto}` incl. `x509roots/fallback` | stdlib extensions | BSD-3-Clause |

## Embedded in the binary (our own code)

- `skills/` — `//go:embed` of bashy's shipped skills (first-party).
- `native/siglaunch.c.in` — the unix C launcher (first-party; not cgo).

## Open findings against §1 (owner: Sprint 350, story 385)

Recorded here so the "permissive-only shipped binary" claim is not made
until they are resolved. Measured with `CGO_ENABLED=0 go list -deps ./cmd/bashy`
on 2026-10-02.

1. **`github.com/hashicorp/yamux` — MPL-2.0 — IS in the lean `cmd/bashy`
   dependency closure** (transitively via `github.com/fatedier/frp`, the
   matrix-tunnel client). MPL-2.0 is file-level weak copyleft; §1 says no
   MPL compiled in. **Operator ruling 2026-10-02: bashy stays permissive-only;
   §1 is not amended.** The chain is `internal/agentos` → `outpost/pkg/sshserver`
   → `outpost/internal/agent` → `frp/client` → yamux; bashy never opens a
   tunnel. Because the module is linked UNMODIFIED, it is tolerated for now;
   its removal is filed as **Sprint 368** (S368.1: cut the ssh-server → agent
   import so frp and yamux leave bashy's closure; S368.2: clean-room
   permissive yamux client for outpost's own tunnel). Not scheduled yet.
2. **`github.com/odvcencio/gotreesitter` grammar blobs** — the runtime is MIT,
   but without build tags it embeds all 206 generated grammars (~21 MB) whose
   upstream licenses are not recorded anywhere (`grammars/languages.lock`
   lists the source repos). bashy uses 9 languages. Fix path is
   `grammar_set_core` / `grammar_blobs_external` plus a per-grammar license
   list (`docs/TODO.md` already tracks the size half).
3. MPL-2.0 modules present in `go.mod` through the podman and filebrowser
   graphs but **not** in the lean closure: `cyphar.com/go-pathrs`,
   `hashicorp/errwrap`, `hashicorp/go-multierror`, `hashicorp/golang-lru/v2`.
   Re-check on every build-tag variant the SBOM covers.
4. cgo wrappers over LGPL C libraries (`proglottis/gpgme`,
   `seccomp/libseccomp-golang`) and `mattn/go-sqlite3` drop out of the
   `CGO_ENABLED=0` release build; the host (`make build-host`) variant must be
   inventoried separately.
5. `../yoke/THIRD_PARTY_LICENSES.md` omits several linked modules
   (gotreesitter, `modernc.org/sqlite` + `libc`, `dlclark/regexp2`,
   `bytedance/sonic`, `twitchyliquid64/golang-asm`, `nlpodyssey/gopickle`,
   `tiktoken-go/tokenizer`, `xuri/efp`; purego version drift) — all
   permissive on inspection, but unrecorded.
