#!/usr/bin/env bash
set -euo pipefail
src=${1:?source} asset=${2:?asset}
here=$(cd "$(dirname "$0")" && pwd)
metal=OFF
case $asset in
prima-darwin-arm64) [[ $(uname -m) == arm64 ]]; metal=ON ;;
prima-darwin-amd64) [[ $(uname -m) == x86_64 ]] ;;
prima-linux-amd64) [[ $(uname -m) == x86_64 ]] ;;
prima-linux-arm64) [[ $(uname -m) == aarch64 ]] ;;
*) exit 1 ;;
esac
cmake -S "$src" -B build -DCMAKE_POLICY_VERSION_MINIMUM=3.5 -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF \
  -DGGML_NATIVE=OFF -DGGML_METAL="$metal" -DGGML_METAL_EMBED_LIBRARY=ON \
  -DGGML_CUDA=OFF -DGGML_HIPBLAS=OFF -DGGML_VULKAN=OFF -DGGML_SYCL=OFF -DGGML_RPC=OFF -DGGML_KOMPUTE=OFF \
  -DGGML_BLAS=OFF -DGGML_OPENMP=OFF -DGGML_LLAMAFILE=OFF -DGGML_ACCELERATE=OFF \
  -DLLAMA_CURL=OFF -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_SERVER=OFF
cmake --build build --target llama-cli prima-ring-test --parallel 3
mkdir -p out
cp build/bin/llama-cli "out/$asset"
# Save actual linkage and compiler configuration as signed evidence per platform.
if [[ $(uname -s) == Darwin ]]; then
  otool -L "out/$asset" >"out/$asset-linkage.txt"
  # The artifact may depend only on OS-provided runtimes/frameworks.
  if tail -n +2 "out/$asset-linkage.txt" | grep -Ev '^[[:space:]]+(/usr/lib/|/System/Library/Frameworks/)'; then
    echo 'non-system Darwin dependency' >&2; exit 1
  fi
else
  ldd "out/$asset" >"out/$asset-linkage.txt"
  if grep -Ev '^[[:space:]]*(linux-vdso\.so|/[^ ]*/ld-linux[^ ]*|lib(c|m|pthread|dl|rt|stdc\+\+|gcc_s)\.so[^ ]* => /)' "out/$asset-linkage.txt"; then
    echo 'unreviewed Linux runtime dependency' >&2; exit 1
  fi
fi
if grep -Ei 'lib(zmq|highs|curl|gomp|omp|nng)\.' "out/$asset-linkage.txt"; then
  echo 'unexpected dynamic dependency' >&2; exit 1
fi
cp build/CMakeCache.txt "out/$asset-cmake.txt"

python3 "$here/collect-inputs.py" "$src" build "audit-inputs/$asset" "out/$asset-inputs.json"
tar -czf "out/$asset-inputs.tar.gz" -C "audit-inputs/$asset" .
