#!/bin/sh
# Installed-product smoke for the rod (Sprint 238, S238.4): a language or
# toolchain bashy has never heard of, added with a runner and nothing else —
# an interpreter (awk, runner REGISTERED in a private ring), a compiler (zig,
# inline Bash# func builder over the toolchain bashy provisions) and a config
# (env, inline shell function) — plus the refusal of an unregistered program.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-runners.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
initial_java_home="${JAVA_HOME:-}"

fail() {
	echo "runner-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
[ -n "$bashy" ] || bashy=$(command -v bashy 2>/dev/null || true)
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in "$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;; esac

export BASHY_HINTS=off
export BASHY_COMMANDS_DIR="$tmp/ring"
mkdir -p "$BASHY_COMMANDS_DIR"

expect() { # <label> <output-file> <substring>
	grep -q -- "$3" "$2" || { cat "$2" >&2; fail "$1: expected $3"; }
}

# The registered path: register once, name it from a script that declares nothing.
"$bashy" commands register awk-runner --set exec.0="$root/examples/runners/awk-runner.sh" --set effects.0=exec >/dev/null 2>&1 || fail "commands register awk-runner"
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/runners/awk.bsh" >"$tmp/awk.out" 2>&1) || { cat "$tmp/awk.out" >&2; fail "awk.bsh"; }
expect awk "$tmp/awk.out" "awk: ok"
expect awk "$tmp/awk.out" "this script has [0-9]* words"
echo "awk: ok (interpreter, registered runner)"

# The inline compiler: build into the fence's directory, run, guard-capped.
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/runners/zig.bsh" >"$tmp/zig.out" 2>&1) || { cat "$tmp/zig.out" >&2; fail "zig.bsh"; }
expect zig "$tmp/zig.out" "built app"
expect zig "$tmp/zig.out" "hello from zig, built by a fenced builder"
expect zig "$tmp/zig.out" "guarded run status=126"
echo "zig: ok (compiler, inline func builder)"

# The inline config: show, apply denied under a read cap, then applied.
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/runners/env.bsh" >"$tmp/env.out" 2>&1) || { cat "$tmp/env.out" >&2; fail "env.bsh"; }
expect env "$tmp/env.out" "REGION=eu-west"
expect env "$tmp/env.out" "rehearsal: status=126"
expect env "$tmp/env.out" "applied -> rendered.env"
echo "env: ok (config, inline shell runner)"

# A program that is not registered is never a runner.
printf '~~~cfg as c !not-a-registered-tool\nx\n~~~\nc.run()\n' >"$tmp/refuse.bsh"
if "$bashy" --bashsharp "$tmp/refuse.bsh" >"$tmp/refuse.out" 2>&1; then fail "unregistered runner accepted"; fi
expect refuse "$tmp/refuse.out" "PATH is never consulted"
echo "refusal: ok (unregistered program)"

# The registered Java runner (Sprint 384, #2388): private ring registration.
"$bashy" commands register java-runner --set exec.0="$root/examples/runners/java-runner.sh" --set effects.0=exec >/dev/null 2>&1 || fail "commands register java-runner"

# 1) Java stub legs (always run, no real JDK)
fake_jh="$tmp/fake-jdk"
mkdir -p "$fake_jh/bin"
cat >"$fake_jh/bin/javac" <<'EOF'
#!/bin/sh
out="$2"
mkdir -p "$out"
touch "$out/Hello.class"
EOF
chmod +x "$fake_jh/bin/javac"

cat >"$fake_jh/bin/java" <<'EOF'
#!/bin/sh
shift 2
echo "stub java $*"
EOF
chmod +x "$fake_jh/bin/java"

cat >"$tmp/java-stub.bsh" <<'EOF'
~~~java as j !java-runner
public class Hello {
    public static void main(String[] args) {}
}
~~~
res := j.main("a", "b")
echo "$res"

@guard("read")
func peek() {
  x := j.main("nobody")
  echo "guarded run status=$? x=[$x]"
}
peek()
EOF

(cd "$tmp" && JAVA_HOME="$fake_jh" "$bashy" --bashsharp "$tmp/java-stub.bsh" >"$tmp/java-stub.out" 2>&1) || { cat "$tmp/java-stub.out" >&2; fail "java-stub.bsh"; }
expect java-stub "$tmp/java-stub.out" "stub java Hello a b"
expect java-stub "$tmp/java-stub.out" "guarded run status=126"

cat >"$tmp/java-nope.bsh" <<'EOF'
~~~java as j !java-runner
public class Hello {}
~~~
j.nope()
EOF
if (cd "$tmp" && JAVA_HOME="$fake_jh" "$bashy" --bashsharp "$tmp/java-nope.bsh" >"$tmp/java-nope.out" 2>&1); then
	fail "undeclared java method accepted"
fi

cat >"$tmp/java-call.bsh" <<'EOF'
~~~java as j !java-runner
public class Hello {
    public static void main(String[] args) {}
}
~~~
j.main()
EOF

empty_jh="$tmp/empty-jdk"
mkdir -p "$empty_jh"
if (cd "$tmp" && JAVA_HOME="$empty_jh" "$bashy" --bashsharp "$tmp/java-call.bsh" >"$tmp/java-empty-jh.out" 2>&1); then
	fail "empty JAVA_HOME accepted"
fi
expect java-empty-jh "$tmp/java-empty-jh.out" "no JDK: set JAVA_HOME or JAVA_RUNNER_FETCH=1"

host_os=$(uname -s | tr '[:upper:]' '[:lower:]')
host_arch=$(uname -m)
case "$host_arch" in aarch64) host_arch=arm64 ;; esac

fake_archive="$tmp/fake.tar.gz"
fake_src="$tmp/fake-src"
mkdir -p "$fake_src"
touch "$fake_src/unwanted.txt"
tar -czf "$fake_archive" -C "$fake_src" .

fake_pin="$tmp/fake-pin.tsv"
printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
	"$host_os" "$host_arch" "fake-1.0" "file://$fake_archive" \
	"0000000000000000000000000000000000000000000000000000000000000000" \
	"GPL-2.0-only WITH Classpath-exception-2.0" > "$fake_pin"

fake_home="$tmp/home"
mkdir -p "$fake_home"

if (cd "$tmp" && env -u JAVA_HOME BASHY_HOME="$fake_home" JAVA_RUNNER_FETCH=1 JAVA_RUNNER_PIN="$fake_pin" "$bashy" --bashsharp "$tmp/java-call.bsh" >"$tmp/java-mismatch.out" 2>&1); then
	fail "checksum mismatch accepted"
fi
expect java-mismatch "$tmp/java-mismatch.out" "checksum mismatch"
[ ! -d "$fake_home/custom/java/fake-1.0" ] || [ -z "$(ls -A "$fake_home/custom/java/fake-1.0" 2>/dev/null)" ] || fail "checksum mismatch extracted files"

fake_home2="$tmp/home2"
fake_pin2="$tmp/fake-pin2.tsv"
printf '%s\t%s\t%s\t%s\t%s\t%s\n' \
	"$host_os" "$host_arch" "unmarked-1.0" "file://$fake_archive" \
	"0000000000000000000000000000000000000000000000000000000000000000" \
	"GPL-2.0-only WITH Classpath-exception-2.0" > "$fake_pin2"

fake_cached_jdk="$fake_home2/custom/java/unmarked-1.0"
mkdir -p "$fake_cached_jdk/bin"
cp "$fake_jh/bin/javac" "$fake_cached_jdk/bin/javac"
cp "$fake_jh/bin/java" "$fake_cached_jdk/bin/java"
chmod +x "$fake_cached_jdk/bin/javac" "$fake_cached_jdk/bin/java"

if (cd "$tmp" && env -u JAVA_HOME -u JAVA_RUNNER_FETCH BASHY_HOME="$fake_home2" JAVA_RUNNER_PIN="$fake_pin2" "$bashy" --bashsharp "$tmp/java-call.bsh" >"$tmp/java-no-marker.out" 2>&1); then
	fail "cached JDK without .marker accepted"
fi
expect java-no-marker "$tmp/java-no-marker.out" "no JDK: set JAVA_HOME or JAVA_RUNNER_FETCH=1"
echo "java-stub: ok (compiler, registered runner)"

# 2) Java real leg
java_real_ran=0
if [ -n "$initial_java_home" ] || [ "${RUNNER_SMOKE_JAVA_FETCH:-0}" = "1" ]; then
	if [ -n "$initial_java_home" ]; then
		(cd "$tmp" && JAVA_HOME="$initial_java_home" "$bashy" --bashsharp "$root/examples/runners/java.bsh" >"$tmp/java-real.out" 2>&1) || { cat "$tmp/java-real.out" >&2; fail "java.bsh"; }
	else
		(cd "$tmp" && env -u JAVA_HOME JAVA_RUNNER_FETCH=1 "$bashy" --bashsharp "$root/examples/runners/java.bsh" >"$tmp/java-real.out" 2>&1) || { cat "$tmp/java-real.out" >&2; fail "java.bsh"; }
	fi
	expect java-real "$tmp/java-real.out" "hello from java, run by a custom fence"
	echo "java: ok (compiler, registered runner)"
	java_real_ran=1
else
	echo "java-real: NOT RUN (no JDK)"
fi

[ ! -e "$tmp/rendered.env" ] && [ ! -e "$tmp/app" ] && [ ! -e "$tmp/classes" ] && [ ! -e "$tmp/src" ] || fail "an example wrote into the caller's directory"
if [ "$java_real_ran" -eq 0 ]; then
	echo "runner-examples-smoke: java-real NOT RUN"
fi
echo "runner-examples-smoke: OK"
