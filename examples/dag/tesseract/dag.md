---
name: tesseract
description: bashy dag front door for Tesseract OCR — CMake configure, the tesseract binary and the launched CLI, as one dependency graph
default: smoke
vars:
  JOBS ?= 8
  CMAKE_OPTS ?= -DBUILD_TRAINING_TOOLS=OFF -DCMAKE_INCLUDE_DIRECTORIES_BEFORE=ON
---

# Tesseract — task graph

The repo's own commands (`cmake -B build`, `cmake --build build --target
tesseract`, the built `build/bin/tesseract`), wired as a dependency graph
so one front door replaces the `INSTALL` / `README.md` walkthrough:
`bashy dag --list` shows the targets, `bashy dag run` configures, builds
the CLI, then launches it. Lives at the repo root; needs `bashy`
(github.com/qiangli/bashy), a C++17 compiler, `cmake` on PATH (`bashy
cmake` provisions one) and Leptonica found through `pkg-config` (`lept`)
— the one hard dependency `CMakeLists.txt` names. The training tools want
more (ICU, Pango, Cairo), so `CMAKE_OPTS` turns them off by default.

## Tasks

### configure
`cmake -B build` — a configuration in `build/` (gitignored) with the
training tools off; other `-D` switches go in `CMAKE_OPTS` (for example
`-DBUILD_TRAINING_TOOLS=ON` once their libraries are installed, or
`-DOPENMP_BUILD=OFF`). `CMAKE_INCLUDE_DIRECTORIES_BEFORE=ON` is there
because `CMakeLists.txt` adds Leptonica's include directory before the
generated `build/include`: on a host with a packaged tesseract installed
beside Leptonica (Homebrew, most distros) the build otherwise compiles
that package's `tesseract/version.h` and the binary reports the installed
version, not the checkout's — the `run` target below is what caught it.
Re-run only when the CMake inputs change.
Sources: CMakeLists.txt cmake VERSION
Generates: build/CMakeCache.txt
Effects: read, write
Timeout: 30m

```bash
cmake -B build $CMAKE_OPTS
```

### build
The `tesseract` CLI and the library it links (`cmake --build build
--target tesseract`). Minutes cold, incremental after — the build tool
tracks what changed, so the target always runs it.
Requires: configure
Effects: read, write
Timeout: 1h

```bash
cmake --build build --target tesseract --parallel "$JOBS"
```

### smoke
Call into the checkout from a Bash++ body: a `~~~cxx` fence declares
`tesseract()`, which reads the checkout's own coordinates — the `VERSION`
file (`CMakeLists.txt` derives the version from it), the CMake floor in
`cmake_minimum_required(VERSION …)` and the number of `src/` component
directories — and hands one string back to the shell, which cross-checks
it against the same files. No build, no wrapper script, no `c++ -o`
quoting: the fence IS the program. It is compiled once by the
`c++`/`clang++` on PATH (`BASHPP_CXX` overrides), C++20, and runs as a
fresh native process in the invoking directory, so the relative paths are
the checkout's.
Effects: read

```bashpp
~~~cxx as cxx
#include <filesystem>
#include <fstream>
#include <string>

static std::string first_line(const char* path) {
	std::ifstream in(path);
	std::string line;
	std::getline(in, line);
	return line;
}

static std::string cmake_floor() {
	std::ifstream in("CMakeLists.txt");
	const std::string key = "cmake_minimum_required(VERSION ";
	for (std::string line; std::getline(in, line);)
		if (line.rfind(key, 0) == 0) return line.substr(key.size(), line.find_first_of(" )", key.size()) - key.size());
	return "";
}

std::string tesseract() {
	int dirs = 0;
	for (const auto& e : std::filesystem::directory_iterator("src"))
		if (e.is_directory()) ++dirs;
	return "tesseract " + first_line("VERSION") + " cmake-min " + cmake_floor() + " src-dirs " + std::to_string(dirs);
}
~~~
got := cxx.tesseract()
# The same lookups in shell builtins (no sed/grep: a body should not depend
# on the host's text tools), so the fence's answer is checked, not trusted.
IFS= read -r version <VERSION
floor=
while IFS= read -r line; do
	case "$line" in "cmake_minimum_required(VERSION "*) set -- $line; floor=${2%)} ; break ;; esac
done <CMakeLists.txt
dirs=0
for d in src/*/; do [ -d "$d" ] && dirs=$((dirs + 1)); done
want="tesseract $version cmake-min $floor src-dirs $dirs"
[ "$got" = "$want" ] || { echo "smoke: cxx.tesseract() -> '$got', want '$want'" >&2; exit 1; }
echo "smoke: $got"
```

### run
Launch the built CLI from a Bash++ body: the `~~~cxx` fence's `launch()`
runs `build/bin/tesseract --version` through `popen` and returns its first
line (the Leptonica and image-library lines follow it); the shell checks
it names the version the `VERSION` file declares. A failed launch throws,
and the exception surfaces as the call's error. The fence is the launcher,
the `build` dependency guarantees the binary exists first.
Requires: build
Effects: read

```bashpp
~~~cxx as cxx
#include <cstdio>
#include <stdexcept>
#include <string>

std::string launch() {
	FILE* p = popen("build/bin/tesseract --version 2>&1", "r");
	if (!p) throw std::runtime_error("popen: build/bin/tesseract");
	char line[512] = "";
	if (!std::fgets(line, sizeof line, p)) line[0] = 0;
	if (pclose(p) != 0) throw std::runtime_error("build/bin/tesseract --version: non-zero exit");
	std::string out(line);
	while (!out.empty() && (out.back() == '\n' || out.back() == '\r')) out.pop_back();
	return out;
}
~~~
got := cxx.launch()
IFS= read -r version <VERSION
case "$got" in
	"tesseract $version"*) ;;
	*) echo "run: cxx.launch() -> '$got', want 'tesseract $version…'" >&2; exit 1 ;;
esac
echo "run: $got"
```
