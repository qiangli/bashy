#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
src=${1:?source checkout required}
# This checks HEAD, the peeled official tag and a pristine Git checkout.
# A module zip or the Yoke fork cannot pass.
bash "$here/verify-pin.sh" "$src"
for f in cmd/runner/main.go llama/llama.cpp/vendor/nlohmann/json.hpp \
  llama/llama.cpp/vendor/nlohmann/json_fwd.hpp \
  llama/llama.cpp/vendor/miniaudio/miniaudio.h \
  llama/llama.cpp/vendor/stb/stb_image.h \
  ml/backend/ggml/ggml/src/ggml-metal/metal.go; do
  [[ -s $src/$f ]] || { echo "source-guard: missing full Git source input $f" >&2; exit 1; }
done
[[ -z $(git -C "$src" ls-files --stage | awk '$1 == "160000" {print}') ]] || {
  echo 'source-guard: unreviewed submodules' >&2; exit 1;
}
echo 'source-guard: full pinned source present (not native/security admission)'
