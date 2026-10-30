#!/usr/bin/env bash
# Exercise all six Bash# launchers with a Windows-style GOEXE without cross-exec.
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/tools" "$tmp/cache"
cat > "$tmp/tools/go" <<'GO'
#!/usr/bin/env bash
set -euo pipefail
if [[ ${1:-} == env && ${2:-} == GOEXE ]]; then echo .exe; exit; fi
[[ ${1:-} == build ]] || exit 1
while (( $# )); do
  if [[ $1 == -o ]]; then out=$2; break; fi
  shift
done
[[ -n ${out:-} && $out != /* && $out == *.exe ]] || exit 1
mkdir -p "$(dirname "$out")"
printf '#!/usr/bin/env bash\nexit 2\n' > "$out"
chmod +x "$out"
GO
chmod +x "$tmp/tools/go"
export PATH="$tmp/tools:$PATH" XDG_CACHE_HOME="$tmp/cache"
runner=${BASHY:-${BASHY_EXE:-bashy}}
for tool in notepad form chart viewer whiteboard brainstorm; do
  case $tool in
    notepad|form|chart|viewer) entry="$root/examples/desk/$tool/$tool.bsh" ;;
    *) entry="$root/examples/$tool/$tool.bsh" ;;
  esac
  status=0
  "$runner" --bashsharp "$entry" unsupported-desk-verb --json >"$tmp/$tool.log" 2>&1 || status=$?
  [[ $status == 2 ]] || { echo "$tool: expected exit 2, got $status" >&2; cat "$tmp/$tool.log" >&2; exit 1; }
  [[ -x "$tmp/cache/bashy/$(case $tool in form|whiteboard|brainstorm) echo "$tool" ;; *) echo "desk-$tool" ;; esac)/$tool.exe" ]] || {
    echo "$tool: missing .exe in cache" >&2; exit 1;
  }
done
