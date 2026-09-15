---
name: curl
description: bashy dag front door for curl — CMake configure, the curl binary, the test suite and the launched CLI, as one dependency graph
default: smoke
vars:
  JOBS ?= 8
  CMAKE_OPTS ?=
  TESTS ?= 1 2 3
---

# curl — task graph

The repo's own commands (`cmake -B build`, `cmake --build build --target
curl`, `runtests.pl` through the `test-quiet` target, the built
`build/src/curl`), wired as a dependency graph so one front door replaces
the `docs/INSTALL-CMAKE.md` walkthrough: `bashy dag --list` shows the
targets, `bashy dag run` configures, builds the CLI, then launches it. Lives
at the repo root; needs `bashy` (github.com/qiangli/bashy), a C compiler
and `cmake` on PATH (`bashy cmake` provisions one), plus the libraries
curl's CMake looks for on this host — libpsl is required by default;
`CMAKE_OPTS=-DCURL_USE_LIBPSL=OFF` is curl's own documented switch when it
is not installed.

## Tasks

### configure
`cmake -B build` — a Release configuration in `build/` (gitignored). Which
TLS backend and which optional libraries were found is printed at the end;
extra `-D` switches go in `CMAKE_OPTS`. Re-run only when the CMake inputs
change.
Sources: CMakeLists.txt CMake lib/CMakeLists.txt src/CMakeLists.txt
Generates: build/CMakeCache.txt
Effects: read, write
Timeout: 30m

```bash
cmake -B build -DCMAKE_BUILD_TYPE=Release $CMAKE_OPTS
```

### build
The `curl` binary (`cmake --build build --target curl`); under a minute
cold, incremental after — `build/` is gitignored. The build tool tracks
what changed, so the target always runs it.
Requires: configure
Effects: read, write
Timeout: 1h

```bash
cmake --build build --target curl --parallel "$JOBS"
```

### test
A handful of `tests/data/test<N>` cases through curl's own runner: the
`test-quiet` CMake target builds the test servers and helpers (`testdeps`),
then runs `tests/runtests.pl -a -s` with `TFLAGS` appended — `1 2 3` by
default (the first HTTP GET cases); `TESTS='1 to 20'` or a keyword such as
`TESTS=HTTPS` on the command line picks more, and an empty `TESTS` is the
whole suite (many minutes, not a quick gate). Needs `perl`.
Requires: build
Effects: read, write, net
Timeout: 1h

```bash
TFLAGS="$TESTS" cmake --build build --target test-quiet --parallel "$JOBS"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~c` fence declares
`curl()`, which `#include`s the checkout's own `include/curl/curlver.h` —
the checkout root is an include root for the fence — and reports the
version macros it defines: `LIBCURL_VERSION`, `LIBCURL_VERSION_NUM` and the
major/minor/patch components. The shell cross-checks the answer against the
same header, read as text. No build, no wrapper script, no `cc -o`
quoting: the fence IS the program. It is compiled once by the `cc`/`clang`
on PATH (`BASHPP_CC` overrides), C17, and runs as a fresh native process in
the invoking directory, so relative paths are the checkout's.
Effects: read

```bashpp
~~~c as c
#include <stdio.h>
#include "include/curl/curlver.h"

const char* curl(void) {
	static char out[128];
	snprintf(out, sizeof out, "curl %s num 0x%06x major %d minor %d patch %d",
		LIBCURL_VERSION, LIBCURL_VERSION_NUM,
		LIBCURL_VERSION_MAJOR, LIBCURL_VERSION_MINOR, LIBCURL_VERSION_PATCH);
	return out;
}
~~~
got := c.curl()
# The same macros read as text with shell builtins (no sed/grep: a body
# should not depend on the host's text tools), so the fence's answer is
# checked, not trusted.
macro() { # <name> <file>: the token after `#define <name>`
	name=$1
	while IFS= read -r line; do
		case "$line" in "#define $name "*) set -- $line; printf '%s' "$3"; return 0 ;; esac
	done <"$2"
	return 1
}
header=include/curl/curlver.h
version=$(macro LIBCURL_VERSION "$header"); version=${version#\"}; version=${version%\"}
num=$(macro LIBCURL_VERSION_NUM "$header")
want=$(printf 'curl %s num 0x%06x major %d minor %d patch %d' "$version" "$((num))" \
	"$(macro LIBCURL_VERSION_MAJOR "$header")" "$(macro LIBCURL_VERSION_MINOR "$header")" "$(macro LIBCURL_VERSION_PATCH "$header")")
[ "$got" = "$want" ] || { echo "smoke: c.curl() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built CLI from a Bash++ body: the `~~~c` fence's `launch()`
runs `build/src/curl --version` through `popen` and returns its first
line; the shell checks it names the version `include/curl/curlver.h`
declares. The fence is the launcher, the `build` dependency guarantees the
binary exists first.
Requires: build
Effects: read

```bashpp
~~~c as c
#include <stdio.h>
#include <string.h>

const char* launch(void) {
	static char line[512];
	FILE* p = popen("build/src/curl --version", "r");
	if (!p) return "popen: build/src/curl";
	if (!fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) return "build/src/curl --version: non-zero exit";
	line[strcspn(line, "\n")] = 0;
	return line;
}
~~~
got := c.launch()
version=
while IFS= read -r line; do
	case "$line" in '#define LIBCURL_VERSION "'*) version=${line#*\"}; version=${version%\"*}; break ;; esac
done <include/curl/curlver.h
case "$got" in
	"curl $version "*) ;;
	*) echo "run: c.launch() -> '$got', want 'curl $version …'" >&2; exit 1 ;;
esac
echo "run: $got"
```
