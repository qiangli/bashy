---
name: cmake
description: bashy dag front door for CMake's own source — bootstrap, build, one CMakeLib test and the launched cmake, as one dependency graph
default: smoke
vars:
  JOBS ?= 8
  BOOTSTRAP_OPTS ?=
  TEST ?= CMakeLib.testArgumentParser
---

# CMake — task graph

The repo's own commands (`bootstrap`, `make`, `bin/ctest`, the built
`bin/cmake`), wired as a dependency graph so one front door replaces the
`README.rst` walkthrough: `bashy dag --list` shows the targets, `bashy dag
run` bootstraps, builds cmake, then launches it. Lives at the repo root;
needs `bashy` (github.com/qiangli/bashy), a C++17 compiler and GNU make —
and, by design, NO existing `cmake`: `bootstrap` builds a minimal cmake
from source and uses it to configure the real one. The build is
out-of-tree in `build/` (gitignored), the layout `README.rst` recommends.

## Tasks

### bootstrap
`../bootstrap` from `build/` — compiles the bootstrap cmake, then
configures the full build with it (`--parallel` drives both). Minutes;
`README.rst`'s switches (`--prefix=…`, `-- -DCMAKE_USE_OPENSSL=OFF`, …) go
in `BOOTSTRAP_OPTS`. Re-run only when the bootstrap inputs change.
Sources: bootstrap CMakeLists.txt Source/CMakeVersion.cmake
Generates: build/Makefile build/CMakeCache.txt
Effects: read, write
Timeout: 1h

```bash
mkdir -p build
cd build && ../bootstrap --parallel="$JOBS" $BOOTSTRAP_OPTS
```

### build
`make` in `build/` — cmake, ctest, cpack and the CMakeLib test driver.
Minutes cold, incremental after; `make` tracks what changed, so the
target always runs it. Spelled `env make` because bashy's in-process
`make` is a POSIX make and the generated `build/Makefile` is GNU make's:
`env` spawns the `make` on PATH, as it would outside bashy.
Requires: bootstrap
Effects: read, write
Timeout: 2h

```bash
env make -C build -j "$JOBS"
```

### test
One test through the freshly built `ctest`, the way `README.rst` and CI do
(`bin/ctest` in the build tree): `CMakeLib.testArgumentParser` by default
— a CMakeLib unit test, no generators or compilers exercised;
`TEST=CMake.List` or a regex such as `TEST='^CMakeLib\.'` on the command
line picks more. The whole suite is `bin/ctest` in `build/` — an hour or
more, not a quick gate.
Requires: build
Effects: read, write

```bash
build/bin/ctest --test-dir build -R "$TEST" --output-on-failure
```

### smoke
Call into the checkout from a Bash++ body: a `~~~cxx` fence declares
`cmake()`, which reads the checkout's own coordinates — the
`CMake_VERSION_{MAJOR,MINOR,PATCH}` values in `Source/CMakeVersion.cmake`
and the floor in `cmake_minimum_required(VERSION …)` — and hands one
string back to the shell, which cross-checks it against the same files.
No build, no wrapper script, no `c++ -o` quoting: the fence IS the
program. It is compiled once by the `c++`/`clang++` on PATH (`BASHPP_CXX`
overrides), C++20, and runs as a fresh native process in the invoking
directory, so the relative paths are the checkout's.
Effects: read

```bashpp
~~~cxx as cxx
#include <fstream>
#include <string>

static std::string cmake_value(const char* path, const std::string& name) {
	std::ifstream in(path);
	const std::string key = "set(" + name + " ";
	for (std::string line; std::getline(in, line);)
		if (line.rfind(key, 0) == 0) return line.substr(key.size(), line.find(')', key.size()) - key.size());
	return "";
}

static std::string cmake_floor() {
	std::ifstream in("CMakeLists.txt");
	const std::string key = "cmake_minimum_required(VERSION ";
	for (std::string line; std::getline(in, line);)
		if (line.rfind(key, 0) == 0) return line.substr(key.size(), line.find_first_of(" )", key.size()) - key.size());
	return "";
}

std::string cmake() {
	const char* v = "Source/CMakeVersion.cmake";
	return "cmake " + cmake_value(v, "CMake_VERSION_MAJOR") + "." + cmake_value(v, "CMake_VERSION_MINOR") + "." + cmake_value(v, "CMake_VERSION_PATCH")
		+ " cmake-min " + cmake_floor();
}
~~~
got := cxx.cmake()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
cmake_value() { # <name>: the value of `set(<name> <value>)` in Source/CMakeVersion.cmake
	name=$1
	while IFS= read -r line; do
		case "$line" in "set($name "*) set -- $line; printf '%s' "${2%)}"; return 0 ;; esac
	done <Source/CMakeVersion.cmake
	return 1
}
floor=
while IFS= read -r line; do
	case "$line" in "cmake_minimum_required(VERSION "*) set -- $line; floor=${2%)}; break ;; esac
done <CMakeLists.txt
want="cmake $(cmake_value CMake_VERSION_MAJOR).$(cmake_value CMake_VERSION_MINOR).$(cmake_value CMake_VERSION_PATCH) cmake-min $floor"
[ "$got" = "$want" ] || { echo "smoke: cxx.cmake() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built cmake from a Bash++ body: the `~~~cxx` fence's
`launch()` runs `build/bin/cmake --version` through `popen` and returns
its first line (`cmake version <version>-g<hash>` on a development
checkout); the shell checks it names the version
`Source/CMakeVersion.cmake` declares. A failed launch throws, and the
exception surfaces as the call's error. The fence is the launcher, the
`build` dependency guarantees the binary exists first.
Requires: build
Effects: read

```bashpp
~~~cxx as cxx
#include <cstdio>
#include <stdexcept>
#include <string>

std::string launch() {
	FILE* p = popen("build/bin/cmake --version", "r");
	if (!p) throw std::runtime_error("popen: build/bin/cmake");
	char line[512] = "";
	if (!std::fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) throw std::runtime_error("build/bin/cmake --version: non-zero exit");
	std::string out(line);
	while (!out.empty() && (out.back() == '\n' || out.back() == '\r')) out.pop_back();
	return out;
}
~~~
got := cxx.launch()
cmake_value() { # <name>: the value of `set(<name> <value>)` in Source/CMakeVersion.cmake
	name=$1
	while IFS= read -r line; do
		case "$line" in "set($name "*) set -- $line; printf '%s' "${2%)}"; return 0 ;; esac
	done <Source/CMakeVersion.cmake
	return 1
}
version="$(cmake_value CMake_VERSION_MAJOR).$(cmake_value CMake_VERSION_MINOR).$(cmake_value CMake_VERSION_PATCH)"
case "$got" in
	"cmake version $version"*) ;;
	*) echo "run: cxx.launch() -> '$got', want 'cmake version $version…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
