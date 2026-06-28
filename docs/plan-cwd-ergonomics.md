# Design note — cwd ergonomics: `--chdir`, `--chroot`, `env -C`, and an `--agentic` `cd` hint

Status: **proposal**, not implemented. Captures the design discussion (and the
rejected alternatives) so they don't get re-litigated, plus the phased build plan
for per-command cwd (`env -C`) and the agentic hint. Covers four threads that all
orbit "where do paths resolve": the `--chdir`/`--chroot` invocation flags, the
dropped `--basedir`, the per-command-cwd resolution (`env -C`, not `@DIR`), and
the `--agentic` `cd DIR; cmd` → `env -C` nudge.

## Motivation

Scripts and one-liners constantly preface commands with `cd <dir>` boilerplate,
and there's recurring interest in *confining* a script's filesystem access to a
subtree. Two distinct needs, often conflated. The right framing is **two
independent axes**:

| Axis | Question it answers | Flag |
|---|---|---|
| **cwd** | where do *relative* paths start? | `--chdir DIR` |
| **root** | where does `/` point, and can you escape it? | `--chroot DIR` |

Keep these orthogonal. A third proposal (`--basedir`) tried to live between
them and is **dropped** — see the last section.

Both flags are bashy **extensions**: additive, and inert under `--posix`
(an unknown long option is rejected exactly as bash rejects it, so fidelity is
preserved).

---

## `--chdir DIR` (runtime cwd)

`bashy --chdir /tmp/x script.sh` ≡ `pushd /tmp/x; bashy script.sh; popd`.

- **Bash-faithful.** Relative paths resolve against `DIR`; **absolute paths
  (`/etc/x`) still hit the real `/etc`.** This is the only behavior that doesn't
  surprise — it's just "start somewhere else".
- **Well-precedented:** `git -C`, `make -C`, `tar -C`, `env --chdir`,
  systemd `WorkingDirectory`.
- **Scope:** strict pushd/popd around the whole invocation, so it cannot leak
  into `&` background jobs or alter the rest of an interactive session.

### Naming — use the long form, **not** `-C`

`-C` is **already taken by bash**: it is the `noclobber` shell option, accepted
at invocation (the usage line lists `-abefhkmnptuvxBCEHPT`, where the `C` is
noclobber). Verified: `bashy -c 'echo $-'` → `hBc` vs `bashy -C -c 'echo $-'` →
`hB`**`C`**`c`. Repurposing `-C` for chdir would silently change
`bashy -C script.sh` from "enable noclobber" to "change directory" — a
drop-in-fidelity break. So:

- Ship **`--chdir`** only; leave `-C` bound to noclobber.
- `--chdir` is safe as a long option: bash's GNU long-option set is small and
  fixed (`--norc --noprofile --posix --rcfile --login --restricted …`) and does
  **not** include `--chdir`.

> Aside found during this review: via the PATH `bash` shim,
> `bash -C -c '…'` returned a Go-style `flag provided but not defined: -C`
> while the direct `bin/bash` accepted it. A wrapper/arg-forwarding path may not
> pass set-style single-letter options cleanly — worth verifying separately.

### Per-command cwd — implement `env -C`, don't invent new syntax

The frequent papercut is **per-command** cwd, not the invocation: agent
harnesses **reset the shell's cwd between commands**, so a persistent `cd`
doesn't stick and every command re-establishes its directory — today
`cd DIR && …` on *every* call.

**The fix already exists as a standard: `env -C DIR cmd`** (GNU coreutils
≥ 8.30, 2018). It runs one command with cwd = `DIR`; the tool never sees the
flag; it works in real bash; zero new syntax; perfectly portable. There is no
need to invent a `@DIR` shell prefix — that would be a *non-portable
reinvention* of something POSIX-land already standardized.

The real gap is **awareness + coverage**, not a missing feature:
- `env -C` is genuinely underknown (it did not surface in a 4-tool fleet poll
  until pointed out — and that poll was itself biased by a brief that *led with*
  `@DIR`, a good reminder that a poll measures the framing as much as the
  question).
- **bashy's bundled pure-Go coreutils `env` does not implement `-C`/`--chdir`
  yet** — `env -C /tmp pwd` → `env: unknown shorthand flag: 'C'`. So in bashy's
  self-contained mode the standard answer currently fails.

So the work item is **not** "add a flag" — it is **the deferred env-COMMAND-exec
feature**, which `-C` then rides on. See "Phased plan" below; the short version:
`env CMD` must reproduce **all of a child process's inheritance** (fds, signal
dispositions, process-group / job-control, environment, cwd, umask, exit-status
propagation) **in-process**, because coreutils may not `os/exec`. That needs a
new public **exec seam in the `sh` interp fork** (re-dispatch argv through the
handler chain with an overridden `Dir`/`Env`) — `interp.HandlerCtx` is read-only
today and `handlerCtxKey` is unexported, so a coreutils tool cannot do it. It is
a **3-repo** change (sh → coreutils → bashy), not a one-liner.

**Rejected — a `@DIR` shell prefix** (earlier proposal in this note): redundant
with `env -C`, non-portable (a command using it breaks in real bash), and would
need collision rules (`curl @file`, `@reboot`) it shouldn't have to carry.
Dropped in favor of implementing the standard.

**Rejected — a magic flag stripped from every command's argv:** breaks bash
fidelity (means something different under bashy; breaks when copied to real
bash), no name is collision-proof in argv, "which arg is mine" is ambiguous.

---

## `--chroot DIR` (runtime enforced re-root)

`bashy --chroot DIR script.sh` runs the script as if `/` were `DIR`. Pairs
naturally with `--chdir` (like `chroot(8)` does chroot-then-cd).

**Naming — strongly consider NOT calling it `--chroot`.** The fleet poll was
unanimous that the name `chroot` will be read as a security boundary no matter
how loud the docs disclaim it ("users will assume safety"; "ignored
disclaimer"). A name that signals *path remap, not jail* — e.g. `--path-root`
or `--vfs-root` — removes the footgun at the source. Decide the name before
shipping; the `chroot` spelling is the single biggest misuse risk in this whole
proposal.

### The hard truth: a userland VFS confines only *in-process* file access

bashy has two command classes:
- **In-process** — builtins, the coreutils userland, and the shell's own path
  machinery (redirections, globs, `cd`, `~`, here-doc temp files, `PATH`
  lookup). bashy controls every `open()` these make → a VFS can confine them.
- **External binaries** — `exec`'d real executables. They `open()` the **real**
  kernel FS directly. **A userland VFS cannot contain them.**

So `--chroot` is **path-confinement for bashy's own userland, not a security
boundary.** It must never be described as a sandbox (real `chroot(2)` isn't a
security boundary either — it's escapable; a userland one is weaker still). It
is *coherent and enforceable* precisely in bashy's self-contained mode: confine
in-process FS ops to `DIR` and apply an explicit **external-exec policy**
(`deny`, or `allow-and-warn-it-escapes`). With `deny`, it becomes a clean,
cross-platform, hermetic "run using only builtins + coreutils, all FS under
`DIR`" mode — which fits the self-contained north star.

### Implementation ladder (pick by threat model)

1. **Blind prefix** (`DIR + path`) — **do not ship.** Escapes instantly via
   `..`: `cat /../../etc/passwd` → `DIR/../../etc/passwd`.
2. **Prefix + normalize + clamp** — `Clean` each path and fold `..` at the jail
   root (never climb above `DIR`). Cheap string math, no VFS object. Contains
   accidents and cooperative scripts; **leaks via symlinks**. *Good enough for
   the convenience/hermetic use case if labeled "not a security boundary."*
   Start here.
3. **+ self-resolved symlinks** — resolve symlinks yourself, per component,
   clamped to `DIR` (Linux `openat2(RESOLVE_IN_ROOT)` does this in-kernel but is
   Linux-only and fd-relative). This is the real in-process VFS; still cannot
   contain external binaries.
4. **Adversarial isolation** → **not this flag.** Use the container sandbox
   (`bashy podman`, the existing layer-3 of the 4-layer execution taxonomy).

Also note: `pwd -P`/`realpath`/`$PWD` will report `DIR/foo` unless you add
**two-way translation** (strip `DIR` on output) for a faithful `/`-illusion; and
special paths (`/dev/null`, `/tmp`, `/proc`) need a passthrough allowlist or a
populated `DIR` (real chroot has the same chore via bind-mounts).

### Two points in its favor

- **More portable than real chroot.** `chroot(2)` needs root and is Unix-only
  (no Windows; unprivileged unavailable on macOS). A path-rewriting VFS works
  identically on Linux/macOS/Windows with no privileges.
- **Reuses an abstraction we already want.** The Windows cross-platform work
  wants a "passthrough VFS (`C:\` as root `/`)". Build **one** injectable
  filesystem interface (Go `io/fs` + a writable extension, or an `afero`-style
  layer — `afero` is Apache-2.0 + pure-Go, clears the license/no-cgo rules)
  threaded through the `sh` interp path-resolution points **and** coreutils, and
  it serves both Windows path-normalization and `--chroot` confinement.

---

## Dropped: `--basedir` (the confusing middle)

A third proposal: `--basedir DIR` that resolves command-line paths via
`filepath.Join(DIR, filepath.Rel(DIR, x))` — i.e. re-root **absolute** paths
under `DIR` **without** enforcement. **Rejected.** Reasons:

1. **Wrong axis.** It is a *build/script-time path remap* (DESTDIR-style
   staging), whereas `--chdir`/`--chroot` are *runtime* shell semantics. Putting
   a remap on the shell-invocation axis is a category error.
2. **The confusing-and-unsafe quadrant.** It silently rewrites absolute paths
   (so `/etc/x` → `DIR/etc/x`, breaking bash fidelity — the thing bashy exists
   to preserve) **but** doesn't enforce, so `cat /../../etc` still escapes.
   Surprising *and* not a boundary. Sitting next to `--chdir` (which leaves
   absolute paths real), users can't predict behavior — the name `--basedir`
   reads like a sibling of `--chdir`/`--base` but behaves differently.
3. **Not a distinct mechanism.** "Re-root absolutes under `DIR`" is exactly
   `--chroot`'s path core minus the clamp/symlink enforcement — a *mode* of the
   root axis, not a third feature.
4. **The formula doesn't do what it looks like.**
   `Join(DIR, Rel(DIR, x))` is ~identity for absolutes already under `DIR`,
   re-derives the original for paths outside it (the `../..` round-trips back
   out), and `Rel` *errors* when `DIR` is absolute and `x` is relative.

If a staging use case ever materializes, express it as a clearly-named
*non-enforcing mode of the root axis* (`--root`/`--rootmap`/`--destdir`,
documented "absolute paths are rewritten; **not** confinement") — never as a
`--basedir` look-alike of `--chdir`, and never on the runtime shell axis.

---

## Phased plan — `env -C`, then the `--agentic` `cd DIR; cmd` hint

The per-command-cwd goal (`env -C DIR cmd`) and the proposed agentic nudge are a
**3-phase, 3-repo** effort, not a quick fix. Sequenced because nothing downstream
is honest until Phase 1 lands.

**Phase 1 — `env COMMAND` execution + `-C` (foundation).** The hard, deferred
piece. `env` running a COMMAND must behave like a real `execve` child *minus the
new process*: same fds (incl. redirections), signal dispositions, process group /
job-control placement, environment (with `-i`/`-u`/`NAME=VALUE` applied), cwd
(with `-C`/`--chdir` applied), umask, and exit-status propagation — all
in-process, since coreutils may not `os/exec`. Steps:
1. **`sh`**: add a public exec seam — run an argv through the interpreter's
   handler chain with an overridden `Dir`/`Env`, inheriting everything else.
   (Today `interp.HandlerCtx` is read-only and the handler-context key is
   unexported; a coreutils tool can't re-dispatch.)
2. **`coreutils`**: implement env's COMMAND path on that seam, then add
   `-C`/`--chdir`. Honors the "no `os/exec`" rule (re-enters the shell handler,
   never spawns). Bonus: `env VAR=val cmd` works generally.
3. Cannot be faked at the coreutils adapter level — a dir-less `env -C` would put
   **PATH** commands (`go`, `make`, `git`) in the wrong directory, which is
   exactly the agentic case.

**Phase 2 — docs.** Flip the advice from `cd DIR && cmd` (the honest
works-today recommendation) to **`env -C DIR cmd`**. One line, true only after
Phase 1.

**Phase 3 — `bashy --agentic` mode + a `cd DIR; cmd` hint (new subsystem).**
bashy has **no `--agentic` flag and no hint engine today** — designed in
`docs/agentic-extensions.md` / `docs/bash-agentic-ext.md`, unbuilt. This phase
adds:
- an `--agentic` mode (opt-in; an extension layer, inert in plain/`--posix` use);
- a pattern detector for `cd DIR; cmd` / `cd DIR && cmd` (a `cd` to a literal dir
  immediately followed by one command, scoped to a single list);
- a non-fatal **stderr hint** ("note: `env -C DIR cmd` runs one command in DIR
  without changing the shell's cwd") — advisory only, never rewrites the user's
  command, modeled on ycode's agent-mode hint engine (stderr suggestions).

**Recommended execution:** dogfood it through the weave/conductor machinery (the
conformance campaign proved it) — natural stories are *sh-exec-seam*,
*coreutils-env-COMMAND+`-C`*, *bashy-`--agentic`-hint*. Until Phase 1 lands, the
docs advise `cd DIR && cmd`.

## Decision summary

- **Per-command cwd** — **highest value; no new syntax**, but **not cheap.** The
  standard `env -C DIR cmd` is the answer (portable, real-bash-identical), but
  bashy's coreutils `env` neither implements `-C` *nor* runs a COMMAND at all —
  so this is the deferred env-COMMAND-exec feature (full child-inheritance via a
  new `sh` interp exec-seam), a **3-repo phased build** (see Phased plan). The
  `@DIR` shell-prefix idea is **dropped** as a non-portable reinvention. Until it
  lands, advise `cd DIR && cmd`.
- **`--agentic` `cd DIR; cmd` hint** — a new opt-in mode + stderr hint engine
  (Phase 3) that nudges toward `env -C DIR cmd`. Depends on Phase 1; bashy has no
  agentic mode/hint engine yet.
- **`--chdir DIR`** — ship. Runtime cwd for the *invocation* (script-runner),
  bash-faithful, long form only (`-C` stays noclobber).
- **`--chroot DIR`** — ship as the enforced VFS re-root (start at ladder rung 2;
  in-process only; explicit external-exec policy; reuse the Windows VFS; real
  isolation stays with the container sandbox). Spec it as *hermetic
  path-confinement*, never a security sandbox.
- **`--basedir`** — **dropped** (build-time remap on the wrong axis; confusing
  and unsafe; redundant with `--chroot`'s core).
