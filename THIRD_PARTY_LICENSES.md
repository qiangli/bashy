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
   tunnel. Because the module is linked UNMODIFIED, it is tolerated for now.
   **Re-measured 2026-10-06** (full sweep of the 206 modules in the lean
   closure: 82 MIT, 69 Apache-2.0, 51 BSD, 1 ISC, 1 MPL-2.0): yamux is the
   ONLY non-permissive module. **No permissive Go drop-in exists** — every
   Go implementation of the yamux wire protocol is MPL-2.0 (hashicorp/yamux,
   libp2p/go-yamux, the 0magnet and bingoohuang copies); the permissive
   implementations (libp2p/rust-yamux, tetcoin/remux: Apache-2.0 OR MIT;
   yamux-js: MIT) are Rust or TypeScript; xtaci/smux is MIT but a different
   wire format (cross-repo frp fork + cloudbox deploy). frp has no muxer seam
   (`client/connector.go` calls yamux directly). Sprint 368 was closed on
   2026-10-06 without the removal (operator decision); the work is now the
   **post-v1.0 umbrella story #1584**: a clean-room MIT module exposing the
   hashicorp/yamux API behind a `replace` directive, interop-tested against
   stock yamux and an unmodified cloudbox, plus the ssh-server → agent import
   cut. Until it lands this finding stays open and the unmodified module stays
   tolerated; §1 is not widened.
2. **`github.com/odvcencio/gotreesitter` grammar blobs** — the runtime is MIT
   and embeds all 206 grammar parse tables. Operator decision 2026-10-02: keep
   them (the `ast` verbs serve code agents, not only fences) and attribute.
   Every grammar's license is now read and recorded in
   `../yoke/THIRD_PARTY_GRAMMARS.md` (201 permissive). **Five are
   non-permissive and are still embedded today: `caddy`, `disassembly`,
   `jq`, `ebnf` (GPL-3.0) and `nim` (MPL-2.0).** gotreesitter offers no
   per-grammar exclusion that removes the bytes (`grammar_set_core` still
   carries three of them), so dropping them needs a pinned fork with those
   blobs and registrations deleted. Tracked as a Sprint 350 story.
3. MPL-2.0 modules present in `go.mod` through the podman and filebrowser
   graphs but **not** in the lean closure: `cyphar.com/go-pathrs`,
   `hashicorp/errwrap`, `hashicorp/go-multierror`, `hashicorp/golang-lru/v2`.
   Re-check on every build-tag variant the SBOM covers.
4. cgo wrappers over LGPL C libraries (`proglottis/gpgme`,
   `seccomp/libseccomp-golang`) and `mattn/go-sqlite3` drop out of the
   `CGO_ENABLED=0` release build; the host (`make build-host`) variant must be
   inventoried separately.
5. `../yoke/THIRD_PARTY_LICENSES.md` is by its own header a list of COPIED or
   ADAPTED code, not of linked modules; the linked-module list is what the
   Sprint 350 SBOM generates. gotreesitter now has a row there (it embeds
   third-party data) and the purego version drift is fixed. Modules only
   linked (`modernc.org/sqlite` + `libc`, `dlclark/regexp2`, `bytedance/sonic`,
   `twitchyliquid64/golang-asm`, `nlpodyssey/gopickle`, `tiktoken-go/tokenizer`,
   `xuri/efp`) were verified permissive in the module cache and belong to the
   generated SBOM, not to a hand-kept list.
6. **Java.** Operator decision 2026-10-02 (final): Java is a BUILT-IN fence
   language like Python, Rust and C/C++, with a bashy-provisioned Temurin JDK.
   Every OpenJDK build is GPL-2.0 WITH Classpath-exception-2.0 and no
   permissive JDK exists, so it is admissible exactly as GNU make (GPL-3.0)
   and OpenTofu (MPL-2.0) are: download + exec under §2, recorded with its
   real license, never embedded or redistributed. The stale `java`/`javac`/
   `mvn` synopsis entries stay removed until the provisioner exists; Sprint
   370 (deferred) delivers the JDK provisioner and the `~~~java` fence.
