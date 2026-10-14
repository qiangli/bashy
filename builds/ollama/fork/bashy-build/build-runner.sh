#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$here/PIN"
[[ $# == 3 ]] || { echo 'usage: build-runner.sh SRC OUT runner-OS-ARCH' >&2; exit 2; }
src=$(cd "$1" && pwd)
mkdir -p "$2"
out=$(cd "$2" && pwd)
asset=$3
case $asset in
  runner-linux-amd64|runner-linux-arm64|runner-darwin-amd64|runner-darwin-arm64|runner-windows-amd64) ;;
  *) echo "unsupported runner target: $asset" >&2; exit 1;;
esac
os=${asset#runner-}; arch=${os##*-}; os=${os%-*}
export GOWORK=off GOENV=off GOTOOLCHAIN=local CGO_ENABLED=1 GOFLAGS='' GOOS=$os GOARCH=$arch
export GOAMD64=v1 GOARM64=v8.0
[[ $(go env GOHOSTOS)/$(go env GOHOSTARCH) == "$os/$arch" ]] || {
  echo "native builder required for $os/$arch" >&2; exit 1;
}
[[ $(go env GOVERSION) == "go$GO_TOOLCHAIN" ]] || { echo 'wrong Go toolchain' >&2; exit 1; }
bash "$here/source-guard.sh" "$src"
# Reviewed module bumps (PIN GO_SECURITY_OVERRIDES): after the pin gates, before
# go list/go build, so the inventory, deps, SBOM and binary all reflect the
# overridden tree. S436/#2028: nine x/* modules at their MVS-selected
# versions plus github.com/buger/jsonparser@v1.1.2 (GO-2026-4514). The one
# OSV waiver (GO-2026-5932) lives in vuln-ignore for the audit job's OSV gate.
bash "$here/go-security-overrides.sh" "$src"
# No caller-selected optimization flags, overlays, alternate compilers or tags.
unset CGO_CFLAGS CGO_CPPFLAGS CGO_CXXFLAGS CGO_LDFLAGS GOEXPERIMENT
# The build compiles exactly ./cmd/runner — never ./... or sibling mains —
# so only the runner's required package closure compiles.
if [[ $os == windows ]]; then
  # REQUIRED, not caller-selected: llama/llama.cpp/src/llama-graph.h calls
  # getenv/atoi without including <cstdlib>. Glibc/libc++ transitively
  # provide them; MinGW clang in strict C++17 does not (CI run 38503381935:
  # models package, xverse.cpp "use of undeclared identifier 'atoi'").
  # Force-include the header through the toolchain flags: editing src/ would
  # trip the pristine-tree pin gate, and the models package is a needed
  # runner input (llamarunner), so no thinner package set avoids it.
  export CGO_CXXFLAGS="-include cstdlib"
fi
if [[ $os == windows ]]; then
  # The verified MinGW clang pair is the ONLY compiler on Windows: resolving
  # a stock gcc/g++ first fails silently (set -e, no diagnostic) on hosts
  # whose PATH has no GCC, even though the result is discarded below.
  real_cc=${BASHY_WINDOWS_CC:?verified Windows MinGW clang missing (windows-compilers.py)}
  real_cxx=${BASHY_WINDOWS_CXX:?verified Windows MinGW clang++ missing (windows-compilers.py)}
else
  if [[ $os == darwin ]]; then
    real_cc=clang real_cxx=clang++
    export MACOSX_DEPLOYMENT_TARGET=14.0
  else
    real_cc=gcc real_cxx=g++
  fi
  real_cc=$(command -v "$real_cc") || { echo "build-runner: missing C compiler $real_cc" >&2; exit 1; }
  real_cxx=$(command -v "$real_cxx") || { echo "build-runner: missing C++ compiler $real_cxx" >&2; exit 1; }
fi
python3 "$here/compiler-identity.py" "$out/$asset.compiler-identity.json" "$real_cc" "$real_cxx"
capture="$out/$asset.compiler-inputs"
mkdir -p "$capture"
export BASHY_COMPILER_CAPTURE_DIR="$capture"
BASHY_COMPILER_GOROOT=$(go env GOROOT)
BASHY_COMPILER_TMP=$(mktemp -d)
export BASHY_COMPILER_SOURCE="$src" BASHY_COMPILER_GOROOT BASHY_COMPILER_TMP
# Native Windows Go uses TMP/TEMP; normalize shell paths for native Python/Go.
if [[ $os == windows ]]; then
  BASHY_COMPILER_TMP=$(cygpath -m "$BASHY_COMPILER_TMP")
  BASHY_COMPILER_SOURCE=$(cygpath -m "$src")
  BASHY_COMPILER_CAPTURE_DIR=$(cygpath -m "$capture")
fi
export TMPDIR="$BASHY_COMPILER_TMP" GOTMPDIR="$BASHY_COMPILER_TMP"
export TMP="$BASHY_COMPILER_TMP" TEMP="$BASHY_COMPILER_TMP"
# No cached native objects may bypass this run's compiler evidence.
export GOCACHE="$BASHY_COMPILER_TMP/cache"
trap 'rm -rf "$BASHY_COMPILER_TMP"' EXIT
export BASHY_REAL_CC="$real_cc" BASHY_REAL_CXX="$real_cxx"
if [[ $os == windows ]]; then
  # Go CreateProcess cannot execute a Python shebang script on Windows.
  native_here=$(cygpath -m "$here")
  export CC="python3 \"$native_here/compiler-capture-cc\"" CXX="python3 \"$native_here/compiler-capture-cxx\""
else
  export CC="$here/compiler-capture-cc" CXX="$here/compiler-capture-cxx"
fi
cd "$src"
if [[ $os/$arch == darwin/arm64 ]]; then
  go generate ./ml/backend/ggml/ggml/src/ggml-metal
fi
# Inventory includes tracked nested vendor headers and generated shader bytes.
# This is a source inventory, not a compiled-input license verdict or SBOM.
python3 "$here/package-runner.py" inventory "$src" "$out/$asset.inputs.json"
go list -mod=readonly -deps -json ./cmd/runner > "$out/$asset.deps.json"
exe=$asset
[[ $os != windows ]] || exe=$asset.exe
go build -mod=readonly -trimpath -o "$out/$exe" ./cmd/runner
# Link-aware inventory (prima.cpp-proven pattern): record the ACTUAL linkage
# with the platform's native tool at build time, on the native runner. The
# audit job reviews these per-target records with per-format parsers; no
# single cross-format reader (llvm-readobj --needed-libs has no
# NeededLibraries table for Mach-O/PE, which failed CI runs 38448516095,
# 38450982320, 38452584047). Build records, audit adjudicates: the grep
# below fails fast on obvious third-party runtimes, and the audit allowlist
# admits only reviewed platform libraries.
case $os in
linux)
  ldd "$out/$exe" >"$out/$asset-linkage.txt"
  cat "$out/$asset-linkage.txt"
  if grep -Ev '^[[:space:]]*(linux-vdso\.so|/[^ ]*/ld-linux[^ ]*|lib(c|m|pthread|dl|rt|stdc\+\+|gcc_s)\.so[^ ]* => /)' "$out/$asset-linkage.txt"; then
    echo 'unreviewed Linux runtime dependency' >&2; exit 1
  fi
  ;;
darwin)
  otool -L "$out/$exe" >"$out/$asset-linkage.txt"
  cat "$out/$asset-linkage.txt"
  if otool -L "$out/$exe" | tail -n +2 | grep -Ev '^[[:space:]]+(/usr/lib/|/System/Library/Frameworks/)'; then
    echo 'non-system Darwin dependency' >&2; exit 1
  fi
  ;;
windows)
  # The DLL-import evidence tool is the pinned clang64/bin/llvm-objdump.exe
  # (package mingw-w64-clang-x86_64-llvm-tools), handed in by
  # windows-compilers.py: no PATH-resolved objdump, whose name and target
  # vary across the MSYS2 shells.
  objdump=${BASHY_WINDOWS_OBJDUMP:?verified LLVM objdump missing (windows-compilers.py)}
  : >"$out/$asset-dlls.txt"
  echo "== $out/$exe ==" >>"$out/$asset-dlls.txt"
  # Case-sensitive 'DLL Name:': llvm-objdump prints imports as 'DLL Name:' but
  # the runner's own cgo //export table as 'DLL name: a.out.exe' — grep -i
  # would record the export table as an import (CI run 38537053528). The
  # audit's windows_dlls parser stays strict (bare 'name.dll' lines only).
  "$objdump" -p "$out/$exe" | grep 'DLL Name:' | awk '{print $NF}' | sort -u >>"$out/$asset-dlls.txt"
  cat "$out/$asset-dlls.txt"
  if grep -Ei 'libzmq|highs|libcurl|nng\.dll|msys-2\.0\.dll|cygwin|python[0-9]*\.dll' "$out/$asset-dlls.txt"; then
    echo 'unexpected Windows dynamic dependency' >&2; exit 1
  fi
  ;;
esac
python3 "$here/audit-runner.py" compiler-inputs --capture "$capture" --source "$src" \
  --generated "$src/ml/backend/ggml/ggml/src/ggml-metal" --output "$out/$asset.compiler-inputs.json"
python3 "$here/package-runner.py" package "$out/$exe" "$out/$asset.gz" "$out/$asset.json" "$os/$arch" "$UPSTREAM_COMMIT" "$GO_TOOLCHAIN"
