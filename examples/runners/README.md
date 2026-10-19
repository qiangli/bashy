# Write your own fence — the rod

Every built-in fence row is a *fish*. This directory is the *rod*: adding a
language or a toolchain bashy has never heard of takes a **runner** and
nothing else. Two ways to provide one:

```sh
# (a) register it once, name it from any script
bashy commands register awk-runner --set exec.0=$PWD/examples/runners/awk-runner.sh --set effects.0=exec
~~~awk as aw !awk-runner
{ words += NF }
~~~
n := aw.run("file.txt")
```

```sh
# (b) inline, in the script itself — a Bash# func or a shell function
func builder(verb string, file string, args ...string) string { ... }
~~~zig as z !builder
~~~
bin := z.build()
```

A runner is called as `runner <verb> <file> [args…]` in the caller's
directory with the caller's environment. Its one reserved verb, `methods`,
answers the verb table — one JSON object per line: `name`, optional
`signature`, optional `effects` — and that answer *is* the alias: nothing is
guessed, an empty answer is a refusal. Every other verb serves the fence: the
materialized body is `$2`, stdout is the value, a non-zero status is the call
error. A verb's declared `effects` are what `@guard` checks, exactly as for a
built-in row. The runner resolves to a function in the script, a builtin, or a
command bashy dispatches itself (`bashy commands register`) — never to a
program on PATH, so a fence means the same thing on every host.

| example | shape | runner | provisioning |
|---|---|---|---|
| `awk.bsh` | interpreter | `awk-runner.sh`, registered | bashy's own awk |
| `zig.bsh` | compiler | inline `func builder` | `"$BASH" zig` — the toolchain bashy already provisions for its C fences |
| `env.bsh` | config | inline shell function `env-runner` | none |
| `java.bsh` | compiler/runtime | `java-runner.sh`, registered | caller JDK or pinned Temurin 21 cache |

`make smoke-runners` registers `awk-runner` and `java-runner` in a private ring
and runs the examples on the installed binary. What a built-in row can do that a
runner cannot today — shadow entries, the go overlay, being another fence's module,
lowering — is listed in `bashsharp/docs/fenced-text-blocks-plan.md` §Rod vs fish.

## Java: a complete custom fence

Java is not part of the bashy release. The showcase in `java-runner.sh` and
`java.bsh` demonstrates that adding a compiler/runtime with zero built-in rows
requires zero bashy core changes:

1. **Runner**: `examples/runners/java-runner.sh` implements `runner <verb> <file> [args…]`.
   When invoked with `main`, it locates the first `public class NAME` in the fence
   file, copies the source into `<dir>/src/NAME.java`, compiles with `javac -d <dir>/classes`
   in the fence's scratch tree (never the caller's directory), and executes
   `java -cp <classes>[:$JAVA_RUNNER_CLASSPATH] NAME ARGS...`.
2. **Methods**: On the reserved `methods` verb, the runner prints:
   `{"name":"main","effects":["read","write","exec"]}`. Declaring `write` and
   `exec` ensures that calling `j.main` under `@guard("read")` is refused with
   status 126.
3. **Register**:
   ```sh
   bashy commands register java-runner --set exec.0=$PWD/examples/runners/java-runner.sh --set effects.0=exec
   ```
4. **Fence and call**:
   ```bsh
   ~~~java as j !java-runner
   public class Hello {
       public static void main(String[] args) {
           System.out.println("hello from java, run by a custom fence");
       }
   }
   ~~~
   res := j.main("a", "b")
   echo "$res"
   ```

### JDK Resolution and Licensing

The runner resolves a JDK through:
1. `$JAVA_HOME/bin/javac` and `java` if `JAVA_HOME` is set.
2. The local cache at `${BASHY_HOME:-$HOME/.bashy}/custom/java/<pin-id>/`.
3. On-demand download if the cache is empty and `JAVA_RUNNER_FETCH=1`, fetching
   the pinned archive from `java-pin.tsv` (overridden by `JAVA_RUNNER_PIN` for tests)
   and verifying its SHA-256 before extraction.

The pinned Eclipse Temurin 21 LTS JDK is licensed under `GPL-2.0-only WITH Classpath-exception-2.0`.
The JDK is downloaded and executed only, never shipped or linked, and is not part
of the bashy release.
