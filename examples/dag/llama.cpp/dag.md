---
name: llama.cpp
description: bashy dag front door for llama.cpp — CMake configure, the llama-cli binary, one ctest and the launched CLI, as one dependency graph
default: smoke
vars:
  JOBS ?= 8
  CMAKE_OPTS ?=
  TEST ?= test-arg-parser
---

# llama.cpp — task graph

The repo's own commands (`cmake -B build`, `cmake --build build --target
llama-cli`, `ctest --test-dir build`, the built `build/bin/llama-cli`),
wired as a dependency graph so one front door replaces the
`docs/build.md` walkthrough: `bashy dag --list` shows the targets, `bashy
dag run` configures, builds the CLI, then launches it. Lives at the repo
root; needs `bashy` (github.com/qiangli/bashy), a C/C++17 compiler and
`cmake` on PATH (`bashy cmake` provisions one). No model is needed for any
target here: the launcher asks the binary for its version, which is the
checkout's own `LLAMA_VERSION_*` plus the commit it was built from.

## Tasks

### configure
`cmake -B build` — the default configuration in `build/` (gitignored):
Metal on macOS, the CPU backend everywhere; `docs/build.md`'s `-D` switches
(`-DGGML_CUDA=ON`, `-DGGML_METAL=OFF`, `-DCMAKE_BUILD_TYPE=Release`, …) go
in `CMAKE_OPTS`. Re-run only when the CMake inputs change.
Sources: CMakeLists.txt cmake ggml/CMakeLists.txt src/CMakeLists.txt tools/CMakeLists.txt
Generates: build/CMakeCache.txt
Effects: read, write
Timeout: 30m

```bash
cmake -B build $CMAKE_OPTS
```

### build
The `llama-cli` binary and the `llama`/`ggml` libraries it links
(`cmake --build build --target llama-cli`). Minutes cold, incremental
after — the build tool tracks what changed, so the target always runs it.
Requires: configure
Effects: read, write
Timeout: 1h

```bash
cmake --build build --target llama-cli --parallel "$JOBS"
```

### test
One test from `tests/`, built then run through `ctest` the way
`docs/build.md` and CI do: `test-arg-parser` by default (the CLI argument
parser, no model needed); `TEST=test-log` or `TEST=test-json-schema` on
the command line picks another. The whole suite is `ctest --test-dir
build` after `cmake --build build` — minutes, and the model tests need
files, so not a quick gate.
Requires: configure
Effects: read, write

```bash
cmake --build build --target "$TEST" --parallel "$JOBS"
ctest --test-dir build -R "^$TEST\$" --output-on-failure
```

### smoke
Call into the checkout from a Bash++ body: a `~~~cxx` fence declares
`llama()`, which reports the checkout's own coordinates two ways — the
`LLAMA_VERSION_{MAJOR,MINOR,PATCH}` values read from `CMakeLists.txt` as
text, and `GGML_FILE_VERSION` / `GGML_MAX_DIMS` taken at compile time by
`#include`-ing the checkout's own `ggml/include/ggml.h` (the checkout root
is an include root for the fence; the header is self-contained) — and
hands one string back to the shell, which cross-checks it against the
same files. No build, no wrapper script, no `c++ -o` quoting: the fence
IS the program. It is compiled once by the `c++`/`clang++` on PATH
(`BASHPP_CXX` overrides), C++20, and runs as a fresh native process in the
invoking directory, so the relative paths are the checkout's.
Effects: read

```bashpp
~~~cxx as cxx
#include <fstream>
#include <string>
#include "ggml/include/ggml.h"

static std::string cmake_value(const std::string& name) {
	std::ifstream in("CMakeLists.txt");
	const std::string key = "set(" + name + " ";
	for (std::string line; std::getline(in, line);)
		if (line.rfind(key, 0) == 0) return line.substr(key.size(), line.find(')', key.size()) - key.size());
	return "";
}

std::string llama() {
	return "llama " + cmake_value("LLAMA_VERSION_MAJOR") + "." + cmake_value("LLAMA_VERSION_MINOR") + "." + cmake_value("LLAMA_VERSION_PATCH")
		+ " ggml-file-version " + std::to_string(GGML_FILE_VERSION) + " max-dims " + std::to_string(GGML_MAX_DIMS);
}
~~~
got := cxx.llama()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
cmake_value() { # <name>: the value of `set(<name> <value>)` in CMakeLists.txt
	name=$1
	while IFS= read -r line; do
		case "$line" in "set($name "*) set -- $line; printf '%s' "${2%)}"; return 0 ;; esac
	done <CMakeLists.txt
	return 1
}
macro() { # <name> <file>: the token after `#define <name>`
	name=$1
	while IFS= read -r line; do
		case "$line" in "#define $name "*) set -- $line; printf '%s' "$3"; return 0 ;; esac
	done <"$2"
	return 1
}
want="llama $(cmake_value LLAMA_VERSION_MAJOR).$(cmake_value LLAMA_VERSION_MINOR).$(cmake_value LLAMA_VERSION_PATCH) ggml-file-version $(macro GGML_FILE_VERSION ggml/include/ggml.h) max-dims $(macro GGML_MAX_DIMS ggml/include/ggml.h)"
[ "$got" = "$want" ] || { echo "smoke: cxx.llama() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built CLI from a Bash++ body: the `~~~cxx` fence's `launch()`
runs `build/bin/llama-cli --version` through `popen` and returns its first
line (`version: <LLAMA_VERSION>-dev (build <n>, commit <hash>)`); the
shell checks it names the version `CMakeLists.txt` declares and the commit
the checkout is at. A failed launch throws, and the exception surfaces as
the call's error. The fence is the launcher, the `build` dependency
guarantees the binary exists first.
Requires: build
Effects: read

```bashpp
~~~cxx as cxx
#include <cstdio>
#include <stdexcept>
#include <string>

std::string launch() {
	FILE* p = popen("build/bin/llama-cli --version 2>&1", "r");
	if (!p) throw std::runtime_error("popen: build/bin/llama-cli");
	char line[512] = "";
	if (!std::fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) throw std::runtime_error("build/bin/llama-cli --version: non-zero exit");
	std::string out(line);
	while (!out.empty() && (out.back() == '\n' || out.back() == '\r')) out.pop_back();
	return out;
}
~~~
got := cxx.launch()
cmake_value() { # <name>: the value of `set(<name> <value>)` in CMakeLists.txt
	name=$1
	while IFS= read -r line; do
		case "$line" in "set($name "*) set -- $line; printf '%s' "${2%)}"; return 0 ;; esac
	done <CMakeLists.txt
	return 1
}
version="$(cmake_value LLAMA_VERSION_MAJOR).$(cmake_value LLAMA_VERSION_MINOR).$(cmake_value LLAMA_VERSION_PATCH)"
commit=$(git rev-parse HEAD)
commit=${commit%"${commit#???????}"}
case "$got" in
	"version: $version"*"commit $commit"*) ;;
	*) echo "run: cxx.launch() -> '$got', want 'version: $version… commit $commit…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
