# Java in Bash#: a custom fence, not a built-in row

**Status:** decided 2026-10-18 (operator). This closes Sprint 370 ("Java fence:
built-in `~~~java` with a provisioned JDK") as **won't do**. Its two stories,
S370.1 (Temurin JDK through binmgr, with `bashy java`/`javac`/`jar` front doors)
and S370.2 (an engine-level `~~~java` polyglot row with lowering parity), are
closed `wontfix` and point here.

## Decision

Java is **not part of the bashy official release**. Bash# supports Java the same
way it supports any language without a built-in row: through the runner fence
(`~~~<type> as <alias> !<runner>`, see `examples/runners/README.md`). The worked
example is `examples/runners/java-runner.sh` with `examples/runners/java.bsh`.
That example is the showcase for adding a custom fenced block without touching
bashy core.

- No `bashy java`, `javac` or `jar` verb, no binmgr or external provisioner
  entry, no `sh/polyglot` language row, no `check --prepare` row.
- No release SBOM or runtime-acquisition inventory entry. `make dist` does not
  package `examples/`.
- If you need Java, register the runner once
  (`bashy commands register java-runner --set exec.0=$PWD/examples/runners/java-runner.sh --set effects.0=exec`)
  and name it from any script (`~~~java as j !java-runner`).

## Why

1. **Licensing.** Every OpenJDK build, Eclipse Temurin included, is
   `GPL-2.0-only WITH Classpath-exception-2.0`. HotSpot additionally carries the
   OpenJDK Assembly Exception. This was read from a real Temurin 21 archive's
   `legal/java.base/{LICENSE,ASSEMBLY_EXCEPTION}`. bashy's release stays
   permissive-only, so a GPL toolchain is never provisioned, advertised or
   shipped by it. The example runner only downloads and executes a JDK the user
   chooses. Nothing is linked or bundled.
2. **The rod already exists.** The runner seam gives a Java fence its methods,
   effects, `@guard` capping, scratch directory and error surface without any
   engine change. The example was proven on an unmodified, older installed bashy.
3. **KISS.** A built-in row would add a persistent JVM worker, typed values,
   lowering parity and per-OS provisioning for a language with no current
   consumer inside bashy.

## What the example does

- `methods` declares `main` (effects `read`, `write`, `exec`).
- `main` compiles the fence body with `javac` into the fence's own scratch
  directory and runs it with `java`. It adds `JAVA_RUNNER_CLASSPATH` (for
  example a tool jar) to the classpath and forwards arguments and status.
- JDK resolution: an explicit `JAVA_HOME` wins. Otherwise a pinned Temurin LTS
  (`java-pin.tsv`: URL, SHA-256 and license per platform) is fetched only when
  `JAVA_RUNNER_FETCH=1`, verified before extraction and cached under
  `${BASHY_HOME:-~/.bashy}/custom/java/`. Only pinned platforms run; others are
  refused.
- `make smoke-runners` runs stub legs everywhere: forwarding, `@guard` 126,
  undeclared method, empty `JAVA_HOME` and checksum mismatch. The real-JDK leg
  runs only when a JDK is supplied, and otherwise reports `NOT RUN`.

## Improving over time, on demand

Custom support grows only when a real consumer asks for it. Each item becomes a
todo when needed, never speculatively:

- More platforms in `java-pin.tsv` (Windows, Linux arm64).
- Seam features from the runner gap table in
  `bashsharp/docs/fenced-text-blocks-plan.md` ("Rod vs fish"): a `shadow` or
  overlay field in the `methods` answer, `ModuleFor`, and lowering for
  registered-command runners. These help every custom fence, not just Java.
- Richer verbs (for example `jar`, or typed results) inside the example runner.

Reopening a built-in Java row would need a new operator decision that revisits
reason 1.
