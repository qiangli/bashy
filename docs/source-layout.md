# bashy source layout

The annotated file-by-file map of the repo, moved verbatim from `CLAUDE.md` §Source layout on 2026-09-20. `CLAUDE.md` keeps the top-level orientation.

- `cmd/bash/main.go` — pure drop-in entry point: `cli.Main()`, no AgentOS imports.
- `cmd/bashy/main.go` — AgentOS entry point: wires `internal/agentos` hooks into
  `internal/cli`, then `cli.Main()`.
- `internal/cli/` — the shared shell core (`package cli`):
  - `main.go` — `Main()`: flag parsing, runner setup, script/command/stdin
    dispatch, startup-file loading, bash-format parse-error remapping, static
    alias expansion; defines the `AgentOSDispatch`/`AgentOSWireExec` hook vars.
  - `interactive.go` — readline-backed interactive loop (delegates to
    `mvdan.cc/sh/v3/interactive`).
  - `forced_interactive.go` — minimal readline emulation for `bash -i` with a
    non-TTY stdin (history, C-r/C-p, multi-line accumulation).
  - `prompt.go` — Bash prompt escape expansion (`\u`, `\h`, `\w`, `\D{}`, …)
    plus posix parameter/`!!` prompt expansion (uses `Runner.LiveVar`).
  - `version.go` — `bashVersion` (a `var`, stampable via
    `-ldflags "-X github.com/qiangli/bashy/internal/cli.bashVersion=..."`).
  - `main_test.go` — CLI-level tests.
- `internal/agentos/agentos.go` — the AgentOS wiring (imports coreutils):
  `WireExec()` (coreutils ExecHandler) and `Dispatch()` (front-door subcommands
  `bashy weave …` via `coreutils/pkg/weave`; `bashy podman …` via
  `coreutils/external/podman/engine` — the **embedded, isolated** in-process
  podman engine, `CONTAINER_HOST` pinned to a private `bashy` machine, never a
  shared host one; `bashy ollama …` via `coreutils/external/ollama`'s
  `NewManagedOllamaCmd` — isolated daemon, own port/models; plus `bashy run`
  (result envelope), `commands` (command-surface lister), `doctor` (environment
  self-diagnostic), `act-runner`, `loom`, `zot`, `seaweedfs`, `kopia`). Imported
  only by `cmd/bashy`, so the lean `bash` binary never links any of it. The
  coreutils userland also carries the agentic tools `fetch` (REST/URL client),
  `tokens` (LLM token counter), and `clip` (system clipboard) — see
  `docs/slash-command-priorart-survey.md`.
  - **The agent-facing envelope verbs** live beside it as one file each, and are
    the intended entry points for an agentic tool driving bashy (see
    `skills/bashy/`): `context.go` (`bashy inspect context --json` — machine-readable
    host/session/capability snapshot, the *first* call an agent makes), `run.go`
    (result envelope), `dryrun.go` + `check.go` (`--dry-run` / script check
    before execution), `verify.go`, `commands.go` + `atlas.go` (the Command
    Atlas lister), `doctor.go` (environment self-diagnostic), `nudge.go`,
    `installagent.go` (`bashy install-agent` — point an agent CLI's shell at
    bashy), `git.go`/`git_verbs.go`, `self.go`, `awd.go` (`bashy awd DIR --
    CMD` — the front-door form of the `awd` builtin, the ONE "run it over
    there" mechanism, so no verb grows a `-C`/`-D` flag). Adding a verb means
    touching its file **and** its atlas entry — the coverage tests and the CI
    e2e dispatch gate both fail otherwise.

    The dispatcher now carries ~90 verbs, so **do not read this list as the
    surface** — `agentos.go`'s `switch` is the dispatch truth and
    `bashy commands --atlas` is the catalog. Orchestration/agent-fleet verbs
    landed as their own files the same way: `coord.go` (`bashy claim` — a WRITE
    is refused while another agent holds the project; enforced in the SHELL
    because no document is mandatory across agent CLIs), `messageboard.go`
    (`bashy mb` over `coreutils/pkg/bus`; `wireMessageBoard` has a test
    asserting every hook is non-nil, because an unwired seam looks finished),
    `steward.go`/`steward_meet.go`/`steward_mediator.go` (`bashy steward
    start|stop` — putting an agent ON the seat, not just describing it),
    `kbrecall.go` (`bashy kb recall`, mounted here rather than in `pkg/kb` to
    keep that package an import leaf), `exechist.go`, `release.go`,
    `agents.go`, `shellsession.go` + `session/` (the live-session socket, with
    peer-credential checks per OS).
  - `internal/agentos/advisor*.go` — the **space-time advisor**: a non-intrusive
    post-exec `ExecHandler` middleware that, only when a command fails, appends one
    advisory hint explaining a space-determined failure (wrong cwd, host gone
    remote, OOM, full/read-only disk) so an agent stops the doomed retry loop. Has
    its own memory (per-session doomed-loop counter + a persisted host-success
    ledger keyed by a network fingerprint). Agent-mode/`BASHY_ADVISOR` gated, off
    in `--posix`, never linked into `cmd/bash`. Self-contained — depends on no
    other feature. See `docs/space-time-advisor.md`.
  - **Bare-name verb shims** (`Preamble()`): front-door verbs are exposed without
    the `bashy ` prefix via overridable shell functions (`weave(){ command bashy
    weave "$@"; }`, …). Shadowing policy: native verbs (weave/sprint/dag/run/
    commands/doctor/schedule/secrets/ask/skills/kb) + identical drop-in passthroughs
    (gh/act/rclone/podman/ollama/loom/zot/seaweedfs/kopia/mirror)
    always shimmed; version-sensitive provisioners (go/cmake/clang) only in agent
    mode; `time` (keyword) and jobs/fg/bg/kill (builtins) never. Override with
    `unset -f <name>`; reach a specific binary by absolute path.
  - **Embed tags:** the `Makefile` adds `-tags embed_podman/embed_vfkit/
    embed_gvproxy` to the `cmd/bashy` build for whichever
    `../coreutils/external/podman/engine/*_embed/*.gz` blobs exist (built by
    `coreutils/scripts/embed-*.sh`). With the blobs, `bashy podman` is fully
    self-contained (no host podman); without them it falls back to a PATH podman.
    `cmd/bash` never gets these tags. Embedding the engine makes `bin/bashy` large
    (~259 MB with blobs); `bin/bash` stays ~6 MB.

    **Binary size — measured 2026-08-05, darwin/arm64, `make install` (meetspa
    embed only, no engine blobs): `bashy` 81 MB, `bash` 6.2 MB.** The older
    "~121 MB unix / ~47 MB Windows" figure below has not been re-measured since;
    treat it as stale until `make dist` confirms it. Where the 75 MB goes, by
    segment: `__rodata` 28.9 MB (vs 0.1 MB in `bash`), `__text` 20.7 MB,
    `__gopclntab` 16.9 MB, `__DATA_CONST` 12.1 MB.

    **The single largest item is one dependency: ~21.9 MB of tree-sitter grammar
    tables.** A probe importing `coreutils/pkg/treesitter` is 24.3 MB against a
    2.4 MB hello-world baseline. **Dead-code elimination does not help**, and the
    obvious fix is a no-op: a probe referencing only `grammars.GoLanguage` builds
    to the same 24.3 MB as one referencing all nine, because `//go:embed
    grammar_blobs/*.bin` pulls the whole directory into one `embed.FS` regardless
    of which loaders are called. Trimming `pkg/treesitter/languages.go` would save
    zero bytes. Upstream already ships both tiers as build tags —
    `-tags grammar_set_core` (100 langs) is 17.9 MB, `-tags grammar_blobs_external`
    (read from disk) is 4.0 MB. Adopting them is a tracked item in
    `docs/TODO.md` §Tree-sitter grammar tiering. **Verify any size claim by
    measuring the binary, not by counting references.**
  - **Core vs ext / build profiles:** the default `cmd/bashy` is the **lean
    worker** — shell + coreutils userland + git + dag + `bashy go`
    (self-provisioning Go toolchain via `coreutils/external/gotoolchain` on
    binmgr's tree-mode `Ensure`) + weave/secrets/jobs/mirror + the binmgr-managed
    externals (loom/zot/seaweedfs/kopia/rclone — download-on-demand, not compiled
    in). It is pure-Go and **cross-compiles to every platform with
    `CGO_ENABLED=0`** (~121 MB unix, ~47 MB Windows) — this is what GoReleaser
    ships. Two opt-in, unix-only, heavier **host** layers, both default-EXCLUDED
    so the worker stays lean and portable:
    - `-tags bashy_engines` (`engines_{full,stub}.go`) — the *in-process linked*
      container/LLM engines `bashy podman`/`ollama` (cgo + btrfs/MLX). Always
      excluded on Windows. In the default lean build the stub does NOT error —
      per the settled dispatch ladder (Tier 0 shell → 1 pure-Go userland →
      2 managed engine, **exec'd, never linked** → 3 PATH fallback → 4 mesh
      delegate) it falls through to **Tier 3**: resolve a host/binmgr-cached
      `podman`/`ollama` and exec it transparently (no rebuild), or, if none is
      found, point to install/a paired host node — so a `bashy commands` verb
      always runs without a rebuild step.
    - `-tags bashy_obs` (`obs_{full,stub}.go`) — the observability stack
      `bashy otel` (OpenTelemetry Collector + VictoriaMetrics/Logs + Jaeger +
      Perses + k8s/aws, **193 MB**).

    `make build` = lean; `make build-host` (= `BASHY_ENGINES=1 BASHY_OBS=1`,
    pulling in the embed blobs too) = full unix host. Rule of thumb: a worker
    essential that's pure-Go + cross-platform is **core** (compiled in); a heavy
    or cgo host service is **ext** (build-tag, or binmgr download-on-demand).

