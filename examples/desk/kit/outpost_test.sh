#!/usr/bin/env bash
# Unit checks for outpost.bsh (publish-outpost / unpublish-outpost) and its
# wiring in every catalog app, against stubbed outpost and bashy binaries.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
examples=$(cd "$here/../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

# Stub bashy: `app show NAME [--field F]` from $tmp/apps/NAME/F, `app set`
# rewrites auth; every call is logged. Anything else runs the real bashy.
mkdir -p "$tmp/bin" "$tmp/apps"
cat > "$tmp/bin/bashy" <<EOF
#!/usr/bin/env bash
apps=$tmp/apps
if [[ \${1:-} == app ]]; then
  printf 'bashy %s\n' "\$*" >> $tmp/calls
  case \$2 in
    show) [[ -d \$apps/\$3 ]] || exit 1
      if [[ \${4:-} == --field ]]; then cat "\$apps/\$3/\$5" 2>/dev/null; fi; exit 0 ;;
    set) [[ -d \$apps/\$3 && \$4 == --auth ]] || exit 1; printf '%s\n' "\$5" > "\$apps/\$3/auth"; exit 0 ;;
  esac
  exit 1
fi
exec "$(command -v "$runner")" "\$@"
EOF
# Stub outpost: apps add/rm keep one line per app in $tmp/outpost (an upsert);
# status --json answers a paired host unless $tmp/unpaired exists.
cat > "$tmp/bin/outpost" <<EOF
#!/usr/bin/env bash
printf 'outpost %s\n' "\$*" >> $tmp/calls
reg=$tmp/outpost; touch "\$reg"
case \$1/\${2:-} in
  apps/add) grep -v "^\$3 " "\$reg" > "\$reg.new" || true; printf '%s\n' "\$*" | sed 's/^apps add //' >> "\$reg.new"; mv "\$reg.new" "\$reg" ;;
  apps/rm) grep -v "^\$3 " "\$reg" > "\$reg.new" || true; mv "\$reg.new" "\$reg" ;;
  apps/fail) exit 1 ;;
  status/--json) [[ ! -e $tmp/unpaired ]] || exit 1
    printf '{\n  "status": {\n    "agent_name": "host-a",\n    "cloudbox_url": "https://cloud.example/"\n  },\n  "config": {"cloudbox": {"portal_url": "https://other.example"}}\n}\n' ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$tmp/bin/bashy" "$tmp/bin/outpost"
export BASHY_BIN=$tmp/bin/bashy OUTPOST_BIN=$tmp/bin/outpost
register() { mkdir -p "$tmp/apps/$1"; printf '%s\n' "$2" > "$tmp/apps/$1/icon"; printf '%s\n' "${3:-system}" > "$tmp/apps/$1/auth"; }
fail() { echo "FAIL: $*" >&2; exit 1; }
helper() { (source "$here/outpost.bsh"; kit_outpost "$@"); }

register notes '📝'
register sitey '' public
register lined 'M4 5h16M4 19h16'

# Default publish: login required, console path, fleet icon as a data URL,
# the owner URL from outpost status.
: > "$tmp/calls"
out=$(helper publish-outpost notes notes 22749 false)
[[ $out == *'published notes to outpost (login required)'* ]] || fail "default publish: $out"
[[ $out == *'open https://cloud.example/matrix/h/host-a/app/notes/'* ]] || fail "owner url: $out"
line=$(cat "$tmp/outpost")
[[ $line == 'notes --url http://127.0.0.1:22749/notes/ --require-login --icon data:image/svg+xml,'* ]] || fail "add line: $line"
[[ $line == *'📝'* && $line != *'<'* && $line != *' viewBox'* ]] || fail "icon encoding: $line"
! grep -q 'app set' "$tmp/calls" || fail 'default publish changed console auth'
[[ $(cat "$tmp/apps/notes/auth") == system ]] || fail 'default publish made notes public'

# Idempotent: a re-run converges on the same single entry.
helper publish-outpost notes notes 22749 false >/dev/null
[[ $(wc -l < "$tmp/outpost") -eq 1 && $(cat "$tmp/outpost") == "$line" ]] || fail 'second publish did not converge'

# --dry-run writes nothing and shows the plan, --json is one object.
: > "$tmp/calls"; cp "$tmp/outpost" "$tmp/before"
out=$(helper publish-outpost notes notes 22750 false --dry-run --public --icon https://img.example/n.png)
[[ $out == *'would run:'*'app set notes --auth public'* ]] || fail "dry-run plan: $out"
[[ $out == *"would run: $tmp/bin/outpost apps add notes --url http://127.0.0.1:22750/notes/ --icon https://img.example/n.png"* ]] || fail "dry-run add: $out"
! grep -Eq 'app set|apps add|apps rm' "$tmp/calls" || fail 'dry-run wrote'
cmp -s "$tmp/before" "$tmp/outpost" || fail 'dry-run changed outpost'
json=$(helper publish-outpost notes notes 22749 false --json --dry-run)
[[ $json == '{"app":"notes","verb":"publish","dryRun":true,"target":"http://127.0.0.1:22749/notes/","public":false,"requireLogin":true,"icon":"data:image/svg+xml,'* ]] || fail "json head: $json"
[[ $json == *'"url":"https://cloud.example/matrix/h/host-a/app/notes/","commands":[["'"$tmp"'/bin/outpost","apps","add","notes","--url","http://127.0.0.1:22749/notes/","--require-login","--icon","data:'*'"]]}' ]] || fail "json tail: $json"

# --public: console auth public, no --require-login; a public record is not re-set.
out=$(helper publish-outpost notes notes 22749 false --public --icon https://img.example/n.png)
[[ $out == *'as a public link (no login)'* ]] || fail "public publish: $out"
[[ $(cat "$tmp/apps/notes/auth") == public ]] || fail 'public publish left auth'
[[ $(cat "$tmp/outpost") == 'notes --url http://127.0.0.1:22749/notes/ --icon https://img.example/n.png' ]] || fail "public add: $(cat "$tmp/outpost")"
: > "$tmp/calls"
helper publish-outpost sitey sitey 22749 true >/dev/null
! grep -q 'app set' "$tmp/calls" || fail 'already public record was re-set'
grep -q 'apps add sitey --url http://127.0.0.1:22749/sitey/$' "$tmp/calls" || fail "public default add: $(cat "$tmp/calls")"
helper publish-outpost sitey sitey 22749 true --private >/dev/null
grep -q '^sitey --url http://127.0.0.1:22749/sitey/ --require-login$' "$tmp/outpost" || fail '--private kept the public entry'

# SVG path icons and an optional NAME positional.
helper publish-outpost notes notes 22749 false lined >/dev/null
grep -q '^lined --url http://127.0.0.1:22749/lined/ --require-login --icon data:image/svg+xml,%3Csvg.*%3Cpath%20d=.M4%205h16M4%2019h16' "$tmp/outpost" || fail "path icon: $(cat "$tmp/outpost")"

# Unpublish: outpost rm, and a public console record returns to system.
out=$(helper unpublish-outpost notes notes 22749 false)
[[ $out == 'notes: removed notes from outpost' ]] || fail "unpublish: $out"
! grep -q '^notes ' "$tmp/outpost" || fail 'unpublish left the entry'
[[ $(cat "$tmp/apps/notes/auth") == system ]] || fail 'unpublish left notes public'
json=$(helper unpublish-outpost notes notes 22749 false --json)
[[ $json == '{"app":"notes","verb":"unpublish","dryRun":false,"target":"http://127.0.0.1:22749/notes/","commands":[["'"$tmp"'/bin/outpost","apps","rm","notes"]]}' ]] || fail "unpublish json: $json"

# Unpaired outpost: publish succeeds and says where the tile will be.
touch "$tmp/unpaired"
out=$(helper publish-outpost notes notes 22749 false)
[[ $out == *'outpost is not paired yet'* ]] || fail "unpaired: $out"
json=$(helper publish-outpost notes notes 22749 false --json --dry-run)
[[ $json == *'"url":null,'* ]] || fail "unpaired json: $json"
rm "$tmp/unpaired"

# Argument handling: usage exits 2, failures exit 1, --json errors are {code,message}.
expect() { # STATUS PATTERN ARGS...
  local want=$1 pat=$2 got=0 out; shift 2
  out=$(helper "$@" 2>&1) || got=$?
  [[ $got -eq $want ]] || fail "exit $got (want $want) for $*: $out"
  [[ $out == $pat ]] || fail "output for $*: $out"
}
expect 2 'notes: unknown option --bogus' publish-outpost notes notes 22749 false --bogus
expect 2 'notes: --icon needs a URL' publish-outpost notes notes 22749 false --icon
expect 2 'notes: --icon must be an http(s) or data: URL' publish-outpost notes notes 22749 false --icon '📝'
expect 2 'notes: --public applies to publish-outpost only' unpublish-outpost notes notes 22749 false --public
expect 2 'notes: unexpected argument b' publish-outpost notes notes 22749 false a b
expect 2 'notes: invalid app name: ../x' publish-outpost notes notes 22749 false ../x
expect 2 '{"code":"usage","message":"unknown option --bogus"}' publish-outpost notes notes 22749 false --bogus --json
expect 1 'notes: no console app named ghost; install or register it first' publish-outpost notes ghost 22749 false
expect 1 '{"code":"not_registered",*}' publish-outpost notes ghost 22749 false --json
expect 0 'notes publish-outpost \[NAME\]*login required*' publish-outpost notes notes 22749 false --help
rm "$tmp/bin/outpost"
expect 1 'notes: *outpost apps add failed: *' publish-outpost notes notes 22749 false
expect 0 '*apps rm*' unpublish-outpost notes ghost 22749 false --dry-run

# Every catalog app exposes the verbs through this one helper, with its
# name and console port; site and vlog keep the public default.
cat > "$tmp/bin/outpost" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "$tmp/bin/outpost"
for app in notes form clips site board whiteboard table vlog; do register "$app" '🧪'; done
check_app() { # SCRIPT NAME PORT PUBLIC
  local out
  out=$("$runner" --bashsharp "$1" publish-outpost --json --dry-run) || fail "$1 publish-outpost: $out"
  [[ $out == '{"app":"'"$2"'","verb":"publish","dryRun":true,"target":"http://127.0.0.1:'"$3"'/'"$2"'/","public":'"$4"',"requireLogin":'* ]] || fail "$1 publish: $out"
  out=$("$runner" --bashsharp "$1" unpublish-outpost --json --dry-run) || fail "$1 unpublish-outpost: $out"
  [[ $out == *'["'"$tmp"'/bin/outpost","apps","rm","'"$2"'"]'* ]] || fail "$1 unpublish: $out"
  grep -q 'kit_outpost ' "$1" || fail "$1 does not use the shared helper"
}
check_app "$examples/notes/notes.bsh" notes 22749 false
check_app "$examples/desk/form/form.bsh" form 22749 false
check_app "$examples/clip/clip.bsh" clips 22749 false
check_app "$examples/site/site.bsh" site 22749 true
check_app "$examples/board/board.bsh" board 22751 false
check_app "$examples/whiteboard/whiteboard.bsh" whiteboard 22752 false
check_app "$examples/table/table.bsh" table 22750 false
check_app "$examples/vlog/vlog.sh" vlog 22749 true
out=$(CONSOLE_PORT=23000 "$runner" --bashsharp "$examples/clip/clip.bsh" publish-outpost --json --dry-run)
[[ $out == *'"target":"http://127.0.0.1:23000/clips/"'* ]] || fail "CONSOLE_PORT ignored: $out"
out=$(TABLE_CONSOLE_PORT=23001 "$runner" --bashsharp "$examples/table/table.bsh" publish-outpost --json --dry-run)
[[ $out == *'"target":"http://127.0.0.1:23001/table/"'* ]] || fail "TABLE_CONSOLE_PORT ignored: $out"
out=$("$runner" --bashsharp "$examples/site/site.bsh" publish-outpost mysite --private --json --dry-run 2>&1) || true
[[ $out == '{"code":"not_registered",'* ]] || fail "site NAME positional: $out"
if grep -n 'outpost apps add' "$examples/site/site.bsh" "$examples/vlog/vlog.sh"; then
  fail 'site/vlog still carry their own outpost apps add'
fi
echo 'outpost helper unit checks passed'
