---
name: outpost-setup
description: Set up bashy + outpost on a new Windows, macOS or Linux machine and pair it with the Tessaro portal or a satellite. Read it as the setup skill; run it as the setup script (bashy dag). Covers docs/setup.md paths 1-3.
type: dag
default: setup
vars:
  - SATELLITE ?=
  - NAME ?=
  - INSTALL_DIR ?=
  - CODE_FILE ?=
  - DRY_RUN ?= 0
  - RELEASE_TAG ?= latest
  - INVITE_TTL ?= 30m
  - USER_DATA ?=
  - VERIFY_TIMEOUT ?= 900
---

# outpost-setup - install and pair a new machine

bashy practices what it preaches: this file is both the setup **skill** (read it)
and the setup **script** (run it). Every step below is plain prose followed by
the Bash# body that does it. The bodies run inside bashy, so the same file works
on Windows, macOS and Linux; nothing else is needed on the machine.

Run it on the new machine, from the folder you extracted the release into:

```sh
bashy dag -f outpost-setup.dag.md setup SATELLITE=https://<your-satellite> NAME=<host>
```

Get the file on a machine that has only the release archive:

```sh
bashy fetch https://raw.githubusercontent.com/qiangli/bashy/main/skills/bashy/recipes/outpost-setup.dag.md -o outpost-setup.dag.md
```

Variables:

- `SATELLITE` - satellite base URL; empty pairs with the Tessaro portal.
- `NAME` - host name to register; empty uses this machine's host name.
- `INSTALL_DIR` - where bashy and outpost go; empty picks the standard per-user
  directory (`%LOCALAPPDATA%\outpost` on Windows, `~/.local/bin` elsewhere).
- `CODE_FILE` - a file holding the one-time pairing code; it is read once and
  deleted. Empty asks the human with `bashy ask` (never type it into a chat).
- `DRY_RUN=1` - print what each step would do, change nothing.

The three ways to use it (docs/setup.md):

1. **Manual**: run `setup` yourself on the new machine.
2. **AI agent on the machine**: the agent runs `setup`; you answer its
   `bashy ask` prompt with the pairing code.
3. **Bootstrap, then an agent remotely**: you run `bootstrap` on the new
   machine; the agent on another machine runs `invite` there, copies the code
   file over, and runs `setup CODE_FILE=…` on the new machine through the
   bootstrap session.

## Tasks

### detect
Report what this machine is and where things will go: OS, architecture, the
login name to use for remote sessions, the install directory, and whether
outpost is already paired. On Windows also the network profile, because a
**Public** profile makes the firewall drop inbound connections.
Effects: exec, read

```bash
os=$(uname -s); arch=$(uname -m)
case "$os" in
  Windows*|MINGW*|MSYS*) dir="${INSTALL_DIR:-$LOCALAPPDATA/outpost}"; plat=windows ;;
  Darwin) dir="${INSTALL_DIR:-$HOME/.local/bin}"; plat=darwin ;;
  *) dir="${INSTALL_DIR:-$HOME/.local/bin}"; plat=linux ;;
esac
echo "os:          $plat ($os $arch)"
echo "login name:  $(whoami)"
echo "host name:   ${NAME:-$(hostname)}"
echo "install dir: $dir"
echo "pair with:   ${SATELLITE:-Tessaro portal (https://ai.dhnt.io)}"
if command -v outpost >/dev/null 2>&1; then
  outpost status 2>/dev/null | grep -E '^ +(state|cloudbox) ' || echo "outpost:     installed, not running"
else
  echo "outpost:     not installed yet"
fi
if [ "$plat" = windows ]; then
  powershell -NoProfile -NonInteractive -Command "Get-NetConnectionProfile | ForEach-Object { 'network:     ' + \$_.InterfaceAlias + ' ' + \$_.NetworkCategory }" </dev/null
fi
```

### install
Copy bashy, outpost, bash and sh from the extracted release into the install
directory and make sure that directory is on PATH. outpost stays dormant until
the `pair` step.
Requires: detect
Effects: exec, read, write

```bash
os=$(uname -s)
case "$os" in
  Windows*|MINGW*|MSYS*) dir="${INSTALL_DIR:-$LOCALAPPDATA/outpost}"; win=1 ;;
  *) dir="${INSTALL_DIR:-$HOME/.local/bin}"; win=0 ;;
esac
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: $BASHY self install --dir $dir"; exit 0; fi
mkdir -p "$dir"
"$BASHY" self install --dir "$dir" || exit 1
if [ "$win" = 1 ]; then
  powershell -NoProfile -NonInteractive -Command "\$d='$dir'.Replace('/','\\'); \$p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not (\$p -split ';' | Where-Object { \$_ -eq \$d })) { [Environment]::SetEnvironmentVariable('Path', \$p + ';' + \$d, 'User'); 'added to user Path: ' + \$d } else { 'already on user Path: ' + \$d }" </dev/null
else
  case ":$PATH:" in *":$dir:"*) echo "on PATH: $dir" ;; *) echo "add to your shell profile: export PATH=\"$dir:\$PATH\"" ;; esac
fi
```

### pair
Exchange the one-time pairing code for this machine's identity and start
outpost automatically at logon (Task Scheduler on Windows, launchd on macOS,
a systemd user unit on Linux). The code comes from `CODE_FILE` (read, then
deleted) or from the human through `bashy ask`; it is never printed. On a
headless Linux machine outpost also needs linger to run without a login.
Requires: install
Effects: exec, net, read, write

```bash
os=$(uname -s)
case "$os" in
  Windows*|MINGW*|MSYS*) dir="${INSTALL_DIR:-$LOCALAPPDATA/outpost}"; op="$dir/outpost.exe" ;;
  *) dir="${INSTALL_DIR:-$HOME/.local/bin}"; op="$dir/outpost" ;;
esac
server="${SATELLITE:-https://ai.dhnt.io}"
name="${NAME:-$(hostname)}"
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: $op register --server $server --code <code> --name $name"; exit 0; fi
if [ -n "${CODE_FILE:-}" ]; then
  [ -f "$CODE_FILE" ] || { echo "CODE_FILE $CODE_FILE not found" >&2; exit 1; }
  code=$(tr -d ' \r\n' < "$CODE_FILE"); rm -f "$CODE_FILE"
else
  vf=$("$BASHY" ask --prompt "One-time pairing code for $name (from the portal or bashy satellite invite)" --name OUTPOST_PAIR_CODE | tail -1)
  code=$(tr -d ' \r\n' < "$vf"); rm -f "$vf"
fi
[ -n "$code" ] || { echo "no pairing code" >&2; exit 1; }
"$op" register --server "$server" --code "$code" --name "$name" 2>&1 | grep -viE 'code|token'
rc=${PIPESTATUS[0]}; unset code
[ "$rc" = 0 ] || exit "$rc"
case "$os" in Linux) command -v loginctl >/dev/null 2>&1 && loginctl enable-linger "$(whoami)" 2>/dev/null || true ;; esac
```

### verify
Check that outpost runs, is paired with the intended server, and reports the
expected name. Fails (non-zero) otherwise, so an agent can tell success from a
half-finished setup.
Effects: exec, read

```bash
os=$(uname -s)
case "$os" in
  Windows*|MINGW*|MSYS*) op="${INSTALL_DIR:-$LOCALAPPDATA/outpost}/outpost.exe" ;;
  *) op="${INSTALL_DIR:-$HOME/.local/bin}/outpost" ;;
esac
want="${SATELLITE:-https://ai.dhnt.io}"
for i in 1 2 3 4 5 6; do
  st=$("$op" status 2>/dev/null) && break
  sleep 5
done
echo "$st" | grep -E '^ +(state|agent_name|cloudbox|version) '
echo "$st" | grep -q 'state *active' || { echo "verify: outpost is not active" >&2; exit 1; }
echo "$st" | grep -q "cloudbox *${want%/}" || { echo "verify: paired with another server (want $want)" >&2; exit 1; }
echo "verify: ok"
```

### setup
The whole job on the new machine: detect, install, pair, verify.
Requires: detect install pair verify
Effects: exec, net, read, write

```bash
echo "setup complete: ${NAME:-$(hostname)} paired with ${SATELLITE:-the Tessaro portal}"
```

### bootstrap
For path 3: start the temporary SSH server so an agent on another machine can
finish the setup. It listens on port 2222, takes this machine's login name and
OS password, and announces `<host>.local` over mDNS. Leave it running until the
agent is done, then stop it with Ctrl-C. On Windows, set a **Public** network to
Private first or the firewall will block it.
Effects: exec, net

```bash
os=$(uname -s)
case "$os" in Windows*|MINGW*|MSYS*) op=./outpost.exe ;; *) op=./outpost ;; esac
[ -x "$op" ] || op=outpost
echo "bootstrap sshd on :2222 as $(whoami); give the agent: $(hostname).local, login $(whoami)"
exec "$op" sshd
```

### invite
For path 3, run on the agent's machine (already paired with the owner's
account): mint a one-time pairing code for the new host into `CODE_FILE`
(default `<NAME>.invite`, mode 0600). Copy that file to the new machine and run
`setup CODE_FILE=<file>` there; `pair` deletes it after reading.
Effects: exec, net, write

```bash
[ -n "${NAME:-}" ] || { echo "invite: set NAME=<new host>" >&2; exit 1; }
[ -n "${SATELLITE:-}" ] || { echo "invite: set SATELLITE=<satellite URL> (portal invites come from the portal)" >&2; exit 1; }
out="${CODE_FILE:-$NAME.invite}"
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: $BASHY satellite invite --satellite $SATELLITE --name $NAME --to-file $out"; exit 0; fi
"$BASHY" satellite invite --satellite "$SATELLITE" --name "$NAME" --to-file "$out" >/dev/null && echo "invite written to $out (0600, single use, short-lived)"
```

### cloud-init
For cloud servers (DigitalOcean, AWS, Azure, Google Cloud, or any provider with
first-boot user-data): run on the agent's machine to write a user-data script
into `USER_DATA` (default `<NAME>.user-data.sh`, mode 0600). Pass it as the VM's
user-data / custom-data / startup-script. On first boot it downloads bashy for
the VM's architecture, checks the release checksum, installs bashy and outpost,
pairs with `SATELLITE` using a single-use invite minted now, and keeps outpost
running as a boot-time service. Mint it shortly before creating the VM: the invite
expires (`INVITE_TTL`, default 30m). The script holds a one-time secret; delete
the file once the VM is created. Linux VMs; Windows VMs follow path 3.
outpost runs as a dedicated `outpost` account (it refuses to run as root).
Effects: exec, net, read, write, destroy

```bash
[ -n "${NAME:-}" ] || { echo "cloud-init: set NAME=<new host>" >&2; exit 1; }
[ -n "${SATELLITE:-}" ] || { echo "cloud-init: set SATELLITE=<satellite URL>" >&2; exit 1; }
out="${USER_DATA:-$NAME.user-data.sh}"
tag="${RELEASE_TAG:-latest}"
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would write $out: install bashy $tag, pair $NAME with $SATELLITE (invite ttl ${INVITE_TTL:-30m})"; exit 0; fi
inv=$(mktemp); trap 'rm -f "$inv"' EXIT
"$BASHY" satellite invite --satellite "$SATELLITE" --name "$NAME" --ttl "${INVITE_TTL:-30m}" --to-file "$inv" >/dev/null || exit 1
code=$(tr -d ' \r\n' < "$inv")
umask 077
cat > "$out" <<USERDATA
#!/bin/sh
# bashy onboard user-data: install bashy + outpost, pair $NAME with $SATELLITE
set -eu
case "\$(uname -m)" in x86_64|amd64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo "unsupported arch" >&2; exit 1 ;; esac
base=https://github.com/qiangli/bashy/releases
if [ "$tag" = latest ]; then url=\$base/latest/download; else url=\$base/download/$tag; fi
work=\$(mktemp -d); cd "\$work"
curl -fsSLO "\$url/bashy-linux-\$arch.tar.gz"
curl -fsSL "\$url/checksums.txt" -o checksums.txt
grep " bashy-linux-\$arch.tar.gz\\\$" checksums.txt | sha256sum -c -
tar -xzf "bashy-linux-\$arch.tar.gz"
./bashy self install --dir /usr/local/bin
# outpost refuses to run as root: give it its own account, pair as that
# account (config lands in its home), then run the boot-time service as it.
id outpost >/dev/null 2>&1 || useradd --create-home --shell /bin/sh outpost
su -s /bin/sh outpost -c '/usr/local/bin/outpost register --server "$SATELLITE" --code "$code" --name "$NAME" --no-service'
/usr/local/bin/outpost service install --system --run-as outpost
cd /; rm -rf "\$work"
USERDATA
unset code
echo "user-data written to $out (0600; holds a single-use invite valid ${INVITE_TTL:-30m}; delete it after the VM is created)"
```

### verify-remote
Run on the agent's machine after creating a cloud VM (or after path 3):
confirm on the satellite that `NAME` came online, polling up to
`VERIFY_TIMEOUT` seconds (default 900). Needs the owner Bearer token in
`BASHY_FLEET_TOKEN`, the same one `invite` and `cloud-init` use. If the host
registers but stays offline, first boot failed after pairing: read the VM's boot
log from the provider console or its serial output (`/var/log/cloud-init-output.log`
on most Linux images) — every command the user-data ran is there.
Effects: exec, net, read, write

```bash
[ -n "${NAME:-}" ] && [ -n "${SATELLITE:-}" ] || { echo "verify-remote: set NAME and SATELLITE" >&2; exit 1; }
[ -n "${BASHY_FLEET_TOKEN:-}" ] || { echo "verify-remote: set BASHY_FLEET_TOKEN (owner Bearer)" >&2; exit 1; }
tok=$(curl -s -m 20 -X POST -H "Authorization: Bearer $BASHY_FLEET_TOKEN" -H 'Content-Type: application/json' \
  -d '{"scopes":["ssh:read"],"ttl":"1h"}' "$SATELLITE/api/v1/tokens/exchange" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
[ -n "$tok" ] || { echo "verify-remote: token exchange on $SATELLITE failed" >&2; exit 1; }
limit=${VERIFY_TIMEOUT:-900}; start=$(date +%s); seen=no
while :; do
  body=$(curl -s -m 20 -H "Authorization: Bearer $tok" "$SATELLITE/api/v1/ssh/hosts")
  entry=$(printf '%s' "$body" | tr '{' '\n' | grep "\"hostname\":\"$NAME\"")
  [ -n "$entry" ] && seen=yes
  if printf '%s' "$entry" | grep -q '"online":true'; then echo "verify-remote: $NAME online after $(( $(date +%s) - start ))s"; exit 0; fi
  [ $(( $(date +%s) - start )) -ge "$limit" ] && break
  sleep 15
done
if [ "$seen" = yes ]; then echo "verify-remote: $NAME registered but offline after ${limit}s: first boot failed after pairing; read /var/log/cloud-init-output.log on the VM" >&2
else echo "verify-remote: $NAME never registered: the user-data did not run or could not pair (expired invite? no network?)" >&2; fi
exit 1
```
