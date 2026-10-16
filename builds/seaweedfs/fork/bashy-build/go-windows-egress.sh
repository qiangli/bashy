#!/usr/bin/env bash
# Windows Go services use the SAME Defender harness as Ollama (from prima).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
[[ ${GITHUB_ACTIONS:-} == true && ${RUNNER_ENVIRONMENT:-} == github-hosted ]] || {
  echo 'Windows service gate requires a GitHub-hosted runner' >&2; exit 1;
}
bin=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
tool=${2:?service name required}
case $tool in
  gitea) port=13880 ;;
  zot) port=15000 ;;
  seaweedfs) port=15333 ;;
  *) echo "unsupported service: $tool" >&2; exit 1 ;;
esac
[[ -f $bin && $bin == *.exe ]] || { echo 'missing Windows executable' >&2; exit 1; }
work=$(mktemp -d)
out=$(dirname "$bin")
asset=$(basename "$bin")
# Keep native evidence beside the artifact, including on failures.
cleanup() {
  local rc=$?
  cat "$out/$asset-egress.txt" 2>/dev/null || true
  cat "$work/console.txt" 2>/dev/null || true
  rm -rf "$work"
  exit "$rc"
}
trap cleanup EXIT
winpath() { cygpath -m "$1" | tr '/' '\\'; }
pyexe=$(python3 -c 'import os,sys; print(os.path.realpath(sys.executable))')
# The harness calls its opaque exercise input ModelFile for Ollama. Here it
# carries the service name; firewall enforcement and its verdict are unchanged.
printf '%s\n' "$tool" >"$work/service.txt"
powershell.exe -NoProfile -ExecutionPolicy Bypass \
  -File "$(cygpath -m "$here/windows-defender-egress.ps1")" \
  -PythonExe "$(winpath "$pyexe")" -RunnerExe "$(winpath "$bin")" \
  -GatePy "$(cygpath -m "$here/go-service-exercise.py")" \
  -ModelFile "$(cygpath -m "$work/service.txt")" \
  -EvidenceFile "$(cygpath -m "$out/$asset-native.json")" -Port "$port" \
  -OutFile "$(cygpath -m "$out/$asset-egress.txt")" \
  -WorkDir "$(cygpath -m "$work")" >"$work/console.txt" 2>&1
for marker in 'Defender control blocked' 'pfirewall.log' \
  'protocol gate PROTOCOL_PASS' 'no non-loopback connections observed' \
  'Defender block rules removed; firewall logging restored'; do
  grep -qF "$marker" "$out/$asset-egress.txt" || {
    echo "no-egress: missing proof: $marker" >&2; exit 1;
  }
done
if grep -Ei 'CONTROL-CONNECTED|allowed non-loopback connection|FAIL:' "$out/$asset-egress.txt"; then
  echo 'no-egress: failed Defender evidence' >&2; exit 1
fi
echo "no-egress: windows: $tool exercise and Defender enforcement passed"
