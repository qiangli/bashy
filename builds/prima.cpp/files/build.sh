#!/usr/bin/env bash
set -euo pipefail
src=${1:?source} asset=${2:?asset}
here=$(cd "$(dirname "$0")" && pwd)
metal=OFF
windows=false
case $asset in
prima-darwin-arm64) [[ $(uname -m) == arm64 ]]; metal=ON ;;
prima-darwin-amd64) [[ $(uname -m) == x86_64 ]] ;;
prima-linux-amd64) [[ $(uname -m) == x86_64 ]] ;;
prima-linux-arm64) [[ $(uname -m) == aarch64 ]] ;;
prima-windows-amd64) [[ $(uname -m) == x86_64 ]]; windows=true ;;
*) exit 1 ;;
esac
openmp=OFF
if $windows; then
  # Native MinGW-target clang pair, discovered by windows-compilers.py in the
  # setup step. No fallback compiler and no PATH-resolved build tools: the
  # recipe is MinGW Makefiles + clang + the pinned mingw32-make.exe, all
  # handed in as BASHY_WINDOWS_* paths (run 38466935835: the make package
  # installs mingw32-make.exe, never make.exe).
  [[ -n ${BASHY_WINDOWS_CC:-} && -n ${BASHY_WINDOWS_CXX:-} && -n ${BASHY_WINDOWS_MAKE:-} ]] ||
    { echo 'verified Windows clang pair missing (windows-compilers.py)' >&2; exit 1; }
  openmp=ON
  "${BASHY_WINDOWS_CMAKE:-cmake}" -S "$src" -B build -G 'MinGW Makefiles' -DCMAKE_POLICY_VERSION_MINIMUM=3.5 \
    -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
    -DCMAKE_C_COMPILER="$BASHY_WINDOWS_CC" -DCMAKE_CXX_COMPILER="$BASHY_WINDOWS_CXX" \
    -DCMAKE_MAKE_PROGRAM="$BASHY_WINDOWS_MAKE" \
    -DCMAKE_EXE_LINKER_FLAGS=-static \
    -DGGML_NATIVE=OFF -DGGML_METAL=OFF \
    -DGGML_CUDA=OFF -DGGML_HIPBLAS=OFF -DGGML_VULKAN=OFF -DGGML_SYCL=OFF -DGGML_RPC=OFF -DGGML_KOMPUTE=OFF \
    -DGGML_BLAS=OFF -DGGML_OPENMP="$openmp" -DGGML_LLAMAFILE=OFF \
    -DLLAMA_CURL=OFF -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_SERVER=ON
else
  cmake -S "$src" -B build -DCMAKE_POLICY_VERSION_MINIMUM=3.5 -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
    -DGGML_NATIVE=OFF -DGGML_METAL="$metal" -DGGML_METAL_EMBED_LIBRARY=ON \
    -DGGML_CUDA=OFF -DGGML_HIPBLAS=OFF -DGGML_VULKAN=OFF -DGGML_SYCL=OFF -DGGML_RPC=OFF -DGGML_KOMPUTE=OFF \
    -DGGML_BLAS=OFF -DGGML_OPENMP="$openmp" -DGGML_LLAMAFILE=OFF -DGGML_ACCELERATE=OFF \
    -DLLAMA_CURL=OFF -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_SERVER=ON
fi
cmake --build build --target llama-cli llama-server prima-ring-test --parallel 3
mkdir -p out
if $windows; then
  cp build/bin/llama-cli.exe "out/$asset.exe"
  server_asset=${asset/prima-/prima-server-}
  cp build/bin/llama-server.exe "out/$server_asset.exe"
else
  cp build/bin/llama-cli "out/$asset"
  server_asset=${asset/prima-/prima-server-}
  cp build/bin/llama-server "out/$server_asset"
fi
# Save actual linkage and compiler configuration as signed evidence per platform.
if $windows; then
  # The Windows recipe links the toolchain statically and ships the clang
  # OpenMP runtime beside the executables. Record every imported DLL and
  # refuse unreviewed third-party runtime dependencies; the system-DLL set
  # itself is recorded, not allowlisted, so a new Windows SDK import cannot
  # silently pass as reviewed.
  # The DLL-import evidence tool is the pinned clang64/bin/llvm-objdump.exe
  # (package mingw-w64-clang-x86_64-llvm-tools), handed in by
  # windows-compilers.py: no PATH-resolved objdump, whose name and target
  # vary across the MSYS2 shells.
  objdump=${BASHY_WINDOWS_OBJDUMP:?verified LLVM objdump missing (windows-compilers.py)}
  : >"out/$asset-dlls.txt"
  for binary in "out/$asset.exe" "out/$server_asset.exe"; do
    echo "== $binary ==" >>"out/$asset-dlls.txt"
    "$objdump" -p "$binary" | grep -i 'DLL Name:' | awk '{print $NF}' | sort -u >>"out/$asset-dlls.txt"
  done
  cat "out/$asset-dlls.txt"
  if grep -Ei 'libzmq|highs|libcurl|nng\.dll|msys-2\.0\.dll|cygwin|python[0-9]*\.dll' "out/$asset-dlls.txt"; then
    echo 'unexpected Windows dynamic dependency' >&2; exit 1
  fi
  # Ship the exact compiler OpenMP runtime the exes were built against.
  # windows-compilers.py already refused a toolchain without
  # clang64/bin/libomp.dll (pinned package
  # mingw-w64-clang-x86_64-llvm-openmp), so the first candidate always hits.
  # No C:/msys64 literal: the hosted MSYS2 root arrives as MSYS2_LOCATION
  # (run 38466935835: D:\a\_temp\msys64) and differs per run, so every
  # candidate is rooted at the verified compiler paths.
  cc_dir=$(dirname "$BASHY_WINDOWS_CC")
  omp_dll=''
  for dir in "$cc_dir" "${BASHY_MINGW_ROOT:-$cc_dir/..}/bin"; do
    if [[ -f $dir/libomp.dll ]]; then omp_dll=$dir/libomp.dll; break; fi
  done
  [[ -n $omp_dll ]] || { echo 'libomp.dll not found beside the verified clang' >&2; exit 1; }
  cp "$omp_dll" out/libomp.dll
elif [[ $(uname -s) == Darwin ]]; then
  : >"out/$asset-linkage.txt"
  # The artifact may depend only on OS-provided runtimes/frameworks.
  for binary in "out/$asset" "out/$server_asset"; do
    otool -L "$binary" >>"out/$asset-linkage.txt"
    if otool -L "$binary" | tail -n +2 | grep -Ev '^[[:space:]]+(/usr/lib/|/System/Library/Frameworks/)'; then
      echo 'non-system Darwin dependency' >&2; exit 1
    fi
  done
else
  { ldd "out/$asset"; ldd "out/$server_asset"; } >"out/$asset-linkage.txt"
  if grep -Ev '^[[:space:]]*(linux-vdso\.so|/[^ ]*/ld-linux[^ ]*|lib(c|m|pthread|dl|rt|stdc\+\+|gcc_s)\.so[^ ]* => /)' "out/$asset-linkage.txt"; then
    echo 'unreviewed Linux runtime dependency' >&2; exit 1
  fi
fi
if ! $windows; then
  if grep -Ei 'lib(zmq|highs|curl|gomp|omp|nng)\.' "out/$asset-linkage.txt"; then
    echo 'unexpected dynamic dependency' >&2; exit 1
  fi
fi
cp build/CMakeCache.txt "out/$asset-cmake.txt"

if $windows; then
  # Native Windows Python cannot parse MSYS POSIX paths: hand it Windows
  # paths (relative outputs resolve through the inherited working directory).
  python3 "$here/collect-inputs.py" "$(cygpath -m "$src")" build "audit-inputs/$asset" "out/$asset-inputs.json"
else
  python3 "$here/collect-inputs.py" "$src" build "audit-inputs/$asset" "out/$asset-inputs.json"
fi
if $windows; then
  # The shipped OpenMP runtime is a compiler input the depfiles never list;
  # record its exact bytes so merge-inputs, ScanCode and the SBOM cover it.
  mkdir -p "audit-inputs/$asset"
  cp out/libomp.dll "audit-inputs/$asset/libomp.dll"
  python3 - out/libomp.dll "out/$asset-inputs.json" <<'EOF'
import hashlib, json, sys
dll, manifest = sys.argv[1], sys.argv[2]
rows = json.loads(open(manifest).read())
rows.append({'path': 'libomp.dll',
             'sha256': hashlib.sha256(open(dll, 'rb').read()).hexdigest()})
open(manifest, 'w').write(json.dumps(rows, indent=2) + '\n')
EOF
fi
tar -czf "out/$asset-inputs.tar.gz" -C "audit-inputs/$asset" .
