#!/bin/sh
# Installed-product acceptance smoke for the examples/dag C and C++ front
# doors (Sprint 190): FFmpeg (configure + GNU make, in-tree), curl (CMake),
# git (GNU make, in-tree); tesseract (CMake + Leptonica), llama.cpp (CMake),
# CMake itself (bootstrap + GNU make, out-of-tree). Not part of build or
# test: it needs six local third-party checkouts plus a C/C++ compiler
# (`cc`/`c++` — clang or gcc), GNU `make`, `git`, `perl` (curl's test runner),
# `pkg-config` with Leptonica (tesseract) and a `cmake`. It drives each
# example graph against its checkout with `bashy awd DIR -- bashy dag -f FILE
# …` (bodies run in the invoking cwd, so no file is copied into the
# checkout), asserts the JSON envelope and the fenced results, and verifies
# every checkout has exactly the same git status before and after.
#
#   BASHY_BIN=~/.local/bin/bashy scripts/dag-c-examples-smoke.sh
#
# The six checkouts are dependencies the gate provisions itself: each repo is
# pinned (URL + commit, the coordinates this gate was measured against) and,
# when its *_ROOT variable is not set, cloned shallow at that commit into
# bashy's cache — <user cache dir>/bashy/examples/<name>, i.e.
# ~/Library/Caches/bashy/examples on macOS, $XDG_CACHE_HOME/bashy/examples
# (~/.cache/bashy/examples) on Linux — on first use and reused after (the
# builds in it stay warm; a moved pin re-fetches). FFMPEG_ROOT, CURL_ROOT,
# GIT_ROOT, TESSERACT_ROOT, LLAMACPP_ROOT and CMAKE_ROOT each name an existing
# checkout instead, at whatever commit it is — never touched by the gate.
# BASHY_EXAMPLES_CACHE overrides the cache directory.
#
# Every graph BUILDS its binary (minutes cold, seconds warm; every product is
# gitignored by its repo) because the `run` targets launch the built binary
# from a `~~~c` / `~~~cxx` fence — a launcher that is never launched is a
# vacuous gate. A dag body sees PATH only (bashy's front-door shims are not
# applied inside it), so `cmake`/`ctest` must be on PATH: CMAKE_BIN names a
# directory to front PATH with; without it, and with no `cmake` on PATH, the
# gate asks `bashy cmake` (bashy's self-provisioned CMake) where it lives and
# fronts PATH with that bin/. JOBS (default 8) is the parallelism handed to every
# graph. curl needs libpsl unless CURL_CMAKE_OPTS says
# `-DCURL_USE_LIBPSL=OFF` (curl's own documented switch); the gate sets that
# when `pkg-config` cannot find libpsl.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-dag-c.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "dag-c-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
if [ -z "$bashy" ]; then
	bashy=$(command -v bashy 2>/dev/null || true)
fi
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in
	"$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;;
esac

# cmake: a directory named by the caller goes first; otherwise the PATH one;
# otherwise bashy's own provisioned CMake through a wrapper on PATH.
if [ -n "${CMAKE_BIN:-}" ]; then
	[ -x "$CMAKE_BIN/cmake" ] || fail "CMAKE_BIN=$CMAKE_BIN has no cmake"
	PATH=$CMAKE_BIN:$PATH
	export PATH
elif ! command -v cmake >/dev/null 2>&1; then
	# `bashy cmake` provisions a CMake tree; its bin/ carries ctest too, so
	# ask that cmake where it lives (script mode, CMAKE_COMMAND) and front
	# PATH with the directory rather than wrapping one name.
	printf 'execute_process(COMMAND "${CMAKE_COMMAND}" -E echo "${CMAKE_COMMAND}")\n' >"$tmp/which-cmake.cmake"
	provisioned=$("$bashy" cmake -P "$tmp/which-cmake.cmake" 2>/dev/null | tail -n 1)
	[ -n "$provisioned" ] && [ -x "$provisioned" ] || fail "no cmake on PATH and \`bashy cmake\` provisioned none"
	PATH=$(CDPATH= cd -- "$(dirname "$provisioned")" && pwd -P):$PATH
	export PATH
fi
for tool in cc c++ make git perl pkg-config python3 cmake ctest; do
	command -v "$tool" >/dev/null 2>&1 || fail "$tool unavailable"
done
make --version 2>/dev/null | grep -q 'GNU Make' || fail "the PATH make is not GNU make (git, FFmpeg and CMake's Makefiles need it)"
pkg-config --exists lept || fail "Leptonica (pkg-config lept) unavailable — tesseract cannot configure"
jobs=${JOBS:-8}
curl_opts=${CURL_CMAKE_OPTS-}
if [ -z "$curl_opts" ] && ! pkg-config --exists libpsl; then
	curl_opts=-DCURL_USE_LIBPSL=OFF
fi

# The pinned checkouts (name, URL, commit) — the gate's dependencies.
cache=${BASHY_EXAMPLES_CACHE:-}
if [ -z "$cache" ]; then
	case $(uname -s) in
		Darwin) cache=$HOME/Library/Caches/bashy/examples ;;
		*) cache=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/examples ;;
	esac
fi
checkout() { # <root-or-empty> <name> <url> <commit>: prints the checkout to use
	if [ -n "$1" ]; then
		git -C "$1" rev-parse --is-inside-work-tree >/dev/null 2>&1 || fail "missing checkout: $1"
		printf '%s\n' "$1"
		return 0
	fi
	dir=$cache/$2
	if ! git -C "$dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		echo "dag-c-examples-smoke: cloning $3 @ ${4%"${4#???????????}"} into $dir" >&2
		rm -rf "$dir"
		mkdir -p "$dir"
		git -C "$dir" init -q
		git -C "$dir" remote add origin "$3"
	fi
	if [ "$(git -C "$dir" rev-parse HEAD 2>/dev/null)" != "$4" ]; then
		git -C "$dir" fetch -q --depth 1 origin "$4" || fail "$2: cannot fetch $4 from $3"
		git -C "$dir" checkout -q --detach FETCH_HEAD || fail "$2: cannot check out $4"
	fi
	printf '%s\n' "$dir"
}
ffmpeg=$(checkout "${FFMPEG_ROOT:-}" ffmpeg https://github.com/FFmpeg/FFmpeg.git 30afbfcf6fe590b0ff58e0194223fec6353fffc5)
curl=$(checkout "${CURL_ROOT:-}" curl https://github.com/curl/curl.git 91e0ed51a57d65c33a09d912eb4dec23e7efa667)
gitroot=$(checkout "${GIT_ROOT:-}" git https://github.com/git/git.git 339ab2a8f14c0c304ae2f28df1a859f3d2cf610c)
tesseract=$(checkout "${TESSERACT_ROOT:-}" tesseract https://github.com/tesseract-ocr/tesseract.git 8ae68101439b3f7df123499a784e8896c805179d)
llama=$(checkout "${LLAMACPP_ROOT:-}" llama.cpp https://github.com/ggml-org/llama.cpp.git fc82583e65ad753710fbd69a9244d9a35dca667a)
cmakeroot=$(checkout "${CMAKE_ROOT:-}" cmake https://github.com/Kitware/CMake.git d099b15c627b1eeb43b2122e4f42317c155b5e78)
[ -f "$ffmpeg/configure" ] && [ -f "$ffmpeg/libavutil/version.h" ] || fail "$ffmpeg is not the FFmpeg source root"
[ -f "$curl/include/curl/curlver.h" ] || fail "$curl is not the curl source root"
[ -f "$gitroot/GIT-VERSION-GEN" ] || fail "$gitroot is not the git source root"
[ -f "$tesseract/VERSION" ] && [ -f "$tesseract/CMakeLists.txt" ] || fail "$tesseract is not the tesseract source root"
[ -f "$llama/ggml/include/ggml.h" ] || fail "$llama is not the llama.cpp source root"
[ -f "$cmakeroot/bootstrap" ] && [ -f "$cmakeroot/Source/CMakeVersion.cmake" ] || fail "$cmakeroot is not the CMake source root"
status() { git -C "$1" status --porcelain=v1 --untracked-files=all; }
status "$ffmpeg" >"$tmp/ffmpeg.before"
status "$curl" >"$tmp/curl.before"
status "$gitroot" >"$tmp/git.before"
status "$tesseract" >"$tmp/tesseract.before"
status "$llama" >"$tmp/llama.before"
status "$cmakeroot" >"$tmp/cmake.before"

export BASHY_HINTS=off
export DAG_CACHE_DIR="$tmp/dag-cache" # never leave a run journal in the checkouts

# Every task in the envelope must be done or up-to-date; a target that
# "failed" or "skipped" is a red gate even if the process exits 0. An error
# envelope (`status: error`, a failed target) is written to stderr, so the
# graph runs are captured `>x.json 2>x.err` and the envelope is whichever
# stream carries it — the failed target's own stderr is then reported.
check_envelope() { # <label> <json-file> <expected-target>...
	label=$1
	json=$2
	shift 2
	[ -s "$json" ] || [ ! -s "$json.err" ] || json=$json.err
	python3 - "$label" "$json" "$@" <<'PY' || exit 1
import json, sys
label, path, expected = sys.argv[1], sys.argv[2], sys.argv[3:]
try:
    with open(path) as f:
        env = json.load(f)
except ValueError as e:
    sys.exit(f"dag-c-examples-smoke: FAIL: {label}: no JSON envelope ({e})")
tasks = {t["name"]: t for t in env.get("result", {}).get("tasks", [])}
for t in tasks.values():
    if t["status"] not in ("done", "up-to-date", "skipped"):
        sys.exit(f"dag-c-examples-smoke: FAIL: {label}: target {t['name']} is {t['status']} (exit {t.get('exit_code')})\n{(t.get('stderr') or '')[-2000:]}")
if env.get("status") != "ok":
    sys.exit(f"dag-c-examples-smoke: FAIL: {label}: envelope status {env.get('status')!r}")
for name in expected:
    t = tasks.get(name)
    if t is None:
        sys.exit(f"dag-c-examples-smoke: FAIL: {label}: target {name} missing from envelope")
    if t["status"] not in ("done", "up-to-date"):
        sys.exit(f"dag-c-examples-smoke: FAIL: {label}: target {name} is {t['status']} (exit {t.get('exit_code')})\n{t.get('stderr','')[-2000:]}")
print(f"{label}: " + " ".join(f"{n}={tasks[n]['status']}" for n in expected))
PY
}
task_stdout() { # <json-file> <target>: the target's captured stdout, decoded
	python3 -c 'import json,sys; t={x["name"]:x for x in json.load(open(sys.argv[1]))["result"]["tasks"]}[sys.argv[2]]; print(t.get("stdout",""))' "$1" "$2"
}
# What the fences report and what this gate expects them to report, read
# independently here with the same builtin-only lookups the examples use.
macro() { # <name> <file>: the token after `#define <name>`
	name=$1
	while IFS= read -r line; do
		case "$line" in "#define $name "*) set -- $line; printf '%s' "$3"; return 0 ;; esac
	done <"$2"
	return 1
}
cmake_value() { # <file> <name>: the value of `set(<name> <value>)`
	name=$2
	while IFS= read -r line; do
		case "$line" in "set($name "*) set -- $line; printf '%s' "${2%)}"; return 0 ;; esac
	done <"$1"
	return 1
}
first_line() { IFS= read -r line <"$1"; printf '%s' "$line"; }

ex_ffmpeg="$root/examples/dag/ffmpeg/dag.md"
ex_curl="$root/examples/dag/curl/dag.md"
ex_git="$root/examples/dag/git/dag.md"
ex_tesseract="$root/examples/dag/tesseract/dag.md"
ex_llama="$root/examples/dag/llama.cpp/dag.md"
ex_cmake="$root/examples/dag/cmake/dag.md"
for f in "$ex_ffmpeg" "$ex_curl" "$ex_git" "$ex_tesseract" "$ex_llama" "$ex_cmake"; do
	[ -f "$f" ] || fail "examples/dag file missing: $f"
done

# Discovery from another directory: the graph lists, and --explain reports the
# checkout (not the example's directory) as the effective working directory.
"$bashy" awd "$curl" -- "$bashy" dag -f "$ex_curl" --list >"$tmp/curl.list" || fail "curl --list"
grep -q '^smoke' "$tmp/curl.list" && grep -q '^run' "$tmp/curl.list" || fail "curl --list lacks smoke/run"
"$bashy" awd "$llama" -- "$bashy" dag -f "$ex_llama" --explain --json smoke >"$tmp/llama.explain" || fail "llama.cpp --explain"
# Output reduction canonicalizes the home directory to the literal `$HOME`
# token (lossless: expand it back before comparing).
explained=$(python3 -c 'import json,os,sys; d=json.load(open(sys.argv[1]))["result"]["dir"]; print(d.replace("$HOME", os.environ["HOME"], 1) if d.startswith("$HOME") else d)' "$tmp/llama.explain")
[ "$(cd "$explained" && pwd -P)" = "$(cd "$llama" && pwd -P)" ] || fail "llama.cpp --explain dir = $explained, want the checkout"

run_graph() { # <label> <checkout> <example> <targets…>: one dag run, envelope captured
	label=$1 checkout=$2 example=$3
	shift 3
	"$bashy" awd "$checkout" -- "$bashy" dag -f "$example" --json "$@" >"$tmp/$label.json" 2>"$tmp/$label.json.err" || true # an error envelope lands on stderr
}

# ── C ──────────────────────────────────────────────────────────────────

# curl (CMake, out-of-tree): configure → build, three runtests.pl cases,
# the fenced-C smoke (compile-time include of include/curl/curlver.h) and
# the fenced-C launcher over build/src/curl.
run_graph curl "$curl" "$ex_curl" test smoke run "JOBS=$jobs" "CMAKE_OPTS=$curl_opts"
check_envelope curl "$tmp/curl.json" configure build test smoke run
curl_version=$(macro LIBCURL_VERSION "$curl/include/curl/curlver.h")
curl_version=${curl_version#\"}
curl_version=${curl_version%\"}
task_stdout "$tmp/curl.json" smoke | grep -q "^smoke: curl $curl_version num 0x[0-9a-f]* major [0-9]* minor [0-9]* patch [0-9]*\$" || fail "curl smoke did not report the curlver.h macros"
task_stdout "$tmp/curl.json" run | grep -q "^run: curl $curl_version " || fail "curl run did not launch build/src/curl at $curl_version"
task_stdout "$tmp/curl.json" test | grep -q 'TESTDONE: [1-9][0-9]* tests out of [1-9][0-9]* reported OK: 100%' || fail "curl test summary is not clean"

# git (GNU make, in-tree): build, t0000-basic.sh, the fenced-C smoke and
# the fenced-C launcher over ./git.
run_graph git "$gitroot" "$ex_git" test smoke run "JOBS=$jobs"
check_envelope git "$tmp/git.json" build test smoke run
git_defver=
while IFS= read -r line; do case "$line" in DEF_VER=*) git_defver=${line#DEF_VER=} ;; esac; done <"$gitroot/GIT-VERSION-GEN"
git_release=${git_defver#v}
git_release=${git_release%%-*}
task_stdout "$tmp/git.json" smoke | grep -q "^smoke: git $git_defver builtins [1-9][0-9]*\$" || fail "git smoke did not report GIT-VERSION-GEN + the builtin count"
task_stdout "$tmp/git.json" run | grep -q "^run: git version $git_release" || fail "git run did not launch ./git at $git_release"
task_stdout "$tmp/git.json" test | grep -q '^# passed all [1-9][0-9]* test(s)' || fail "git test script did not pass cleanly"

# FFmpeg (configure + GNU make, in-tree): configure → build, fate-source,
# the fenced-C smoke (RELEASE + libavutil/version.h as text) and the
# fenced-C launcher that includes the generated libavutil/ffversion.h and
# runs ./ffmpeg.
run_graph ffmpeg "$ffmpeg" "$ex_ffmpeg" test smoke run "JOBS=$jobs"
check_envelope ffmpeg "$tmp/ffmpeg.json" configure build test smoke run
ffmpeg_release=$(first_line "$ffmpeg/RELEASE")
ffmpeg_avutil="$(macro LIBAVUTIL_VERSION_MAJOR "$ffmpeg/libavutil/version.h").$(macro LIBAVUTIL_VERSION_MINOR "$ffmpeg/libavutil/version.h").$(macro LIBAVUTIL_VERSION_MICRO "$ffmpeg/libavutil/version.h")"
task_stdout "$tmp/ffmpeg.json" smoke | grep -q "^smoke: ffmpeg $ffmpeg_release avutil $ffmpeg_avutil libraries [1-9][0-9]*\$" || fail "ffmpeg smoke did not report RELEASE + the avutil version"
task_stdout "$tmp/ffmpeg.json" run | grep -q '^run: ffmpeg version [^ ]* Copyright' || fail "ffmpeg run did not launch ./ffmpeg"
task_stdout "$tmp/ffmpeg.json" test | grep -q '^TEST *source' || fail "ffmpeg fate-source did not run"

# ── C++ ────────────────────────────────────────────────────────────────

# tesseract (CMake + Leptonica): configure → build, the fenced-C++ smoke
# (VERSION, the CMake floor, src/ components) and the fenced-C++ launcher
# over build/bin/tesseract.
run_graph tesseract "$tesseract" "$ex_tesseract" smoke run "JOBS=$jobs"
check_envelope tesseract "$tmp/tesseract.json" configure build smoke run
tesseract_version=$(first_line "$tesseract/VERSION")
task_stdout "$tmp/tesseract.json" smoke | grep -q "^smoke: tesseract $tesseract_version cmake-min [0-9][0-9.]* src-dirs [1-9][0-9]*\$" || fail "tesseract smoke did not report VERSION + the CMake floor"
task_stdout "$tmp/tesseract.json" run | grep -q "^run: tesseract $tesseract_version" || fail "tesseract run did not launch build/bin/tesseract at $tesseract_version"

# llama.cpp (CMake): configure → build, test-arg-parser through ctest, the
# fenced-C++ smoke (compile-time include of ggml/include/ggml.h +
# CMakeLists.txt as text) and the fenced-C++ launcher over
# build/bin/llama-cli.
run_graph llama "$llama" "$ex_llama" test smoke run "JOBS=$jobs"
check_envelope llama.cpp "$tmp/llama.json" configure build test smoke run
llama_version="$(cmake_value "$llama/CMakeLists.txt" LLAMA_VERSION_MAJOR).$(cmake_value "$llama/CMakeLists.txt" LLAMA_VERSION_MINOR).$(cmake_value "$llama/CMakeLists.txt" LLAMA_VERSION_PATCH)"
llama_ggml=$(macro GGML_FILE_VERSION "$llama/ggml/include/ggml.h")
task_stdout "$tmp/llama.json" smoke | grep -q "^smoke: llama $llama_version ggml-file-version $llama_ggml max-dims [1-9][0-9]*\$" || fail "llama.cpp smoke did not report LLAMA_VERSION + the ggml.h macros"
task_stdout "$tmp/llama.json" run | grep -q "^run: version: $llama_version.*commit [0-9a-f]" || fail "llama.cpp run did not launch build/bin/llama-cli at $llama_version"
task_stdout "$tmp/llama.json" test | grep -q '100% tests passed' || fail "llama.cpp ctest did not pass cleanly"

# CMake (bootstrap + GNU make, out-of-tree; no cmake needed): bootstrap →
# build, one CMakeLib test through the built ctest, the fenced-C++ smoke
# (Source/CMakeVersion.cmake) and the fenced-C++ launcher over
# build/bin/cmake.
run_graph cmake "$cmakeroot" "$ex_cmake" test smoke run "JOBS=$jobs"
check_envelope cmake "$tmp/cmake.json" bootstrap build test smoke run
cmake_version="$(cmake_value "$cmakeroot/Source/CMakeVersion.cmake" CMake_VERSION_MAJOR).$(cmake_value "$cmakeroot/Source/CMakeVersion.cmake" CMake_VERSION_MINOR).$(cmake_value "$cmakeroot/Source/CMakeVersion.cmake" CMake_VERSION_PATCH)"
task_stdout "$tmp/cmake.json" smoke | grep -q "^smoke: cmake $cmake_version cmake-min [0-9][0-9.]*" || fail "cmake smoke did not report CMakeVersion.cmake"
task_stdout "$tmp/cmake.json" run | grep -q "^run: cmake version $cmake_version" || fail "cmake run did not launch build/bin/cmake at $cmake_version"
task_stdout "$tmp/cmake.json" test | grep -q '100% tests passed' || fail "cmake ctest did not pass cleanly"

status "$ffmpeg" >"$tmp/ffmpeg.after"
status "$curl" >"$tmp/curl.after"
status "$gitroot" >"$tmp/git.after"
status "$tesseract" >"$tmp/tesseract.after"
status "$llama" >"$tmp/llama.after"
status "$cmakeroot" >"$tmp/cmake.after"
cmp -s "$tmp/ffmpeg.before" "$tmp/ffmpeg.after" || fail "FFmpeg checkout changed during the run"
cmp -s "$tmp/curl.before" "$tmp/curl.after" || fail "curl checkout changed during the run"
cmp -s "$tmp/git.before" "$tmp/git.after" || fail "git checkout changed during the run"
cmp -s "$tmp/tesseract.before" "$tmp/tesseract.after" || fail "tesseract checkout changed during the run"
cmp -s "$tmp/llama.before" "$tmp/llama.after" || fail "llama.cpp checkout changed during the run"
cmp -s "$tmp/cmake.before" "$tmp/cmake.after" || fail "CMake checkout changed during the run"

echo "bashy=$bashy"
echo "cc=$(cc --version 2>&1 | head -n 1) cmake=$(cmake --version 2>&1 | head -n 1)"
echo "ffmpeg_commit=$(git -C "$ffmpeg" rev-parse HEAD) release=$ffmpeg_release"
echo "curl_commit=$(git -C "$curl" rev-parse HEAD) version=$curl_version"
echo "git_commit=$(git -C "$gitroot" rev-parse HEAD) version=$git_defver"
echo "tesseract_commit=$(git -C "$tesseract" rev-parse HEAD) version=$tesseract_version"
echo "llama_commit=$(git -C "$llama" rev-parse HEAD) version=$llama_version"
echo "cmake_commit=$(git -C "$cmakeroot" rev-parse HEAD) version=$cmake_version"
echo "dag-c-examples-smoke: PASS"
