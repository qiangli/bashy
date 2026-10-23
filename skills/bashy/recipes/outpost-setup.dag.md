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
  - ALWAYS_ON ?= 1
  - AUTHORIZE_KEY ?=
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

Get the file on a machine that has only the release archive: download it with
`bashy fetch` from the link in bashy's setup guide (`docs/setup.md`).

Variables:

- `SATELLITE` - satellite base URL; empty pairs with the Tessaro portal.
- `NAME` - host name to register; empty uses this machine's host name.
- `INSTALL_DIR` - where bashy and outpost go; empty picks the standard per-user
  directory (`%LOCALAPPDATA%\outpost` on Windows, `~/.local/bin` elsewhere).
- `CODE_FILE` - a file holding the one-time pairing code; it is read once and
  deleted. Empty asks the human with `bashy ask` (never type it into a chat).
- `ALWAYS_ON` - `1` (default) prepares the machine to stay reachable: no sleep
  on AC power, a Private network profile on Windows. `0` skips it (laptops).
- `AUTHORIZE_KEY` - a public key (OpenSSH format, a file or the key text) the
  `sshd` step authorizes for the supervised `outpost.sshd` unit (port 2223), so
  agents connect without a password.
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
the `pair` step. Safe to rerun: a complete install of the same release is left
as is.
Requires: detect
Effects: exec, net, persist, read, write

```bash
os=$(uname -s)
case "$os" in
  Windows*|MINGW*|MSYS*) dir="${INSTALL_DIR:-$LOCALAPPDATA/outpost}"; win=1 ;;
  *) dir="${INSTALL_DIR:-$HOME/.local/bin}"; win=0 ;;
esac
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: $BASHY self install --dir $dir"; exit 0; fi
case "$os" in Windows*|MINGW*|MSYS*) ext=.exe ;; *) ext= ;; esac
self_dir=$(cd "$(dirname "$BASHY")" && pwd -P)
want_dir=$(mkdir -p "$dir" && cd "$dir" && pwd -P)
complete=yes; for m in bashy bash sh outpost; do [ -f "$want_dir/$m$ext" ] || complete=no; done
if [ "$self_dir" = "$want_dir" ]; then
  # Running the installed bashy: never install onto itself.
  missing=; for m in bashy bash sh outpost; do [ -f "$want_dir/$m$ext" ] || missing="$missing $m"; done
  echo "install: already installed in $dir (running from it)${missing:+; missing:$missing}; to upgrade, run this from a newer extracted release"
  exit 0
fi
if [ "$complete" = yes ] && [ "$("$want_dir/bashy$ext" --version 2>/dev/null | head -1)" = "$("$BASHY" --version 2>/dev/null | head -1)" ]; then
  echo "install: $dir already has this release"
else
  "$BASHY" self install --dir "$dir" || exit 1
fi
if [ "$win" = 1 ]; then
  powershell -NoProfile -NonInteractive -Command "\$d='$dir'.Replace('/','\\'); \$p=[Environment]::GetEnvironmentVariable('Path','User'); if (-not (\$p -split ';' | Where-Object { \$_ -eq \$d })) { [Environment]::SetEnvironmentVariable('Path', \$p + ';' + \$d, 'User'); 'added to user Path: ' + \$d } else { 'already on user Path: ' + \$d }" </dev/null
else
  case ":$PATH:" in *":$dir:"*) echo "on PATH: $dir" ;; *) echo "add to your shell profile: export PATH=\"$dir:\$PATH\"" ;; esac
fi
```

### host-prep
Make the machine fit to be an always-on outpost (skipped with `ALWAYS_ON=0`):
it must not sleep, and on Windows its networks must not be **Public** (the
firewall drops every inbound connection on a Public network, so the machine
silently disappears). Windows: no standby or hibernate on AC power, connected
networks set to Private. macOS: no sleep on power, wake on network. Linux: the
sleep, suspend and hibernate targets are masked. These need administrator
rights; without them the step stops and prints exactly what to run elevated,
rather than half-doing it. It changes machine settings, so it runs only as part
of `setup` or when named explicitly, never as another step's dependency.
Requires: detect
Effects: exec, read, write

```bash
[ "${ALWAYS_ON:-1}" = 0 ] && { echo "host-prep: skipped (ALWAYS_ON=0)"; exit 0; }
os=$(uname -s)
case "$os" in
  Windows*|MINGW*|MSYS*)
    ps='powercfg /change standby-timeout-ac 0; powercfg /change hibernate-timeout-ac 0; Get-NetConnectionProfile | Where-Object { $_.NetworkCategory -eq "Public" } | ForEach-Object { Set-NetConnectionProfile -InterfaceIndex $_.InterfaceIndex -NetworkCategory Private }; Get-NetConnectionProfile | ForEach-Object { "network: " + $_.InterfaceAlias + " " + $_.NetworkCategory }'
    if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run (elevated PowerShell): $ps"; exit 0; fi
    admin=$(powershell -NoProfile -NonInteractive -Command '([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole("Administrators")' </dev/null | tr -d '\r')
    if [ "$admin" != True ]; then
      echo "host-prep: needs Administrator. Run this in an elevated PowerShell, then rerun setup:" >&2
      echo "  $ps" >&2
      exit 1
    fi
    powershell -NoProfile -NonInteractive -Command "$ps" </dev/null ;;
  Darwin)
    if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: sudo pmset -c sleep 0 disksleep 0 womp 1"; exit 0; fi
    sudo -n pmset -c sleep 0 disksleep 0 womp 1 2>/dev/null || { echo "host-prep: needs sudo. Run: sudo pmset -c sleep 0 disksleep 0 womp 1" >&2; exit 1; }
    pmset -g | grep -E '^ *(sleep|womp) ' ;;
  *)
    if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target"; exit 0; fi
    if command -v systemctl >/dev/null 2>&1; then
      sudo -n systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target >/dev/null 2>&1 || { echo "host-prep: needs sudo. Run: sudo systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target" >&2; exit 1; }
      echo "host-prep: sleep targets masked"
    else
      echo "host-prep: no systemd; leaving sleep settings unchanged"
    fi ;;
esac
```

### pair
Exchange the one-time pairing code for this machine's identity and start
outpost automatically at logon (Task Scheduler on Windows, launchd on macOS,
a systemd user unit on Linux). The code comes from `CODE_FILE` (read, then
deleted) or from the human through `bashy ask`; it is never printed. On a
headless Linux machine outpost also needs linger to run without a login.
Safe to rerun: a machine already paired with `SATELLITE` is left as is.
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
st=$("$op" status 2>/dev/null)
if printf '%s\n' "$st" | grep -qE '^ +state +active' && printf '%s\n' "$st" | grep -qE "^ +cloudbox +${server%/}\$"; then
  echo "pair: already paired with $server; nothing to do"
  exit 0
fi
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

### sshd
Let agents reach this machine without a password: authorize `AUTHORIZE_KEY`
for the supervised `outpost.sshd` unit (port 2223, started and kept up by the
outpost supervisor, with its own firewall rule and mDNS name). The temporary
`outpost sshd` bootstrap (port 2222) is only for the very first contact and can be
closed once this step has run. Needs an outpost that has the `outpost.sshd` unit;
an older outpost is reported, not treated as a failure.
Requires: pair
Effects: exec, read, write

```bash
os=$(uname -s)
case "$os" in
  Windows*|MINGW*|MSYS*) op="${INSTALL_DIR:-$LOCALAPPDATA/outpost}/outpost.exe" ;;
  *) op="${INSTALL_DIR:-$HOME/.local/bin}/outpost" ;;
esac
if ! "$op" services 2>/dev/null | grep -qE '^ *sshd '; then
  echo "sshd: this outpost has no outpost.sshd unit yet; upgrade outpost, then rerun: bashy dag -f outpost-setup.dag.md sshd AUTHORIZE_KEY=…"
  exit 0
fi
if [ -n "${AUTHORIZE_KEY:-}" ]; then
  if [ "${DRY_RUN:-0}" = 1 ]; then echo "would run: $op ssh authorize add <key>"; exit 0; fi
  if [ -f "$AUTHORIZE_KEY" ]; then "$op" ssh authorize add - < "$AUTHORIZE_KEY"; else printf '%s\n' "$AUTHORIZE_KEY" | "$op" ssh authorize add -; fi || exit 1
fi
"$op" ssh authorize ls 2>/dev/null | head -5
"$op" services 2>/dev/null | grep -E '^ *sshd ' || echo "sshd: unit status not reported (outpost services)"
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
if [ "${DRY_RUN:-0}" = 1 ]; then echo "would check: $op status reports active and paired with $want"; exit 0; fi
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
The whole job on the new machine: detect, install, host-prep, pair, sshd, verify.
Requires: detect install host-prep pair sshd verify
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
url=$("$BASHY" self fetch --print-url --version "$tag") || { echo "cloud-init: this bashy cannot report its release URL; upgrade bashy" >&2; exit 1; }
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
url=$url
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

**Validation.** After `setup` (or after adding a machine), an agent runs `check` first; it is
mandatory. If it passes, the agent **offers** the two optional stages to the
user and runs them only if the user says yes:

- `spotcheck` - quick, laddered probes of the three layers and six tiers plus
  local Ollama. Each rung prints `PASS`, `SKIP` (not set up here - not an error)
  or `FAIL`; the ladder stops at the first `FAIL`. Nothing is installed, started
  or downloaded.
- `tutorial` - a short guided tour: one runnable example per layer.

### check
Mandatory: prove the installation itself works. bashy, bash and outpost run;
outpost is active and paired (with `SATELLITE` when given); the `outpost.sshd`
unit is up when this outpost has it. Exits non-zero on any failure, with the
reason.
Effects: exec, read

```bash
os=$(uname -s)
case "$os" in Windows*|MINGW*|MSYS*) d="${INSTALL_DIR:-$LOCALAPPDATA/outpost}"; x=.exe ;; *) d="${INSTALL_DIR:-$HOME/.local/bin}"; x= ;; esac
fail=0
ok()  { echo "PASS  $1"; }
bad() { echo "FAIL  $1"; fail=1; }
v=$("$d/bashy$x" --version 2>/dev/null | head -1) && ok "bashy runs: $v" || bad "bashy does not run from $d"
[ "$("$d/bash$x" -c 'echo $((6*7))' 2>/dev/null)" = 42 ] && ok "bash runs" || bad "bash does not run from $d"
st=$("$d/outpost$x" status 2>/dev/null) || st=
printf '%s\n' "$st" | grep -qE '^ +state +active' && ok "outpost active ($(printf '%s\n' "$st" | sed -n 's/^ *agent_name *//p'))" || bad "outpost not active (outpost status)"
if [ -n "${SATELLITE:-}" ]; then
  printf '%s\n' "$st" | grep -qE "^ +cloudbox +${SATELLITE%/}\$" && ok "paired with $SATELLITE" || bad "not paired with $SATELLITE"
fi
svc=$("$d/outpost$x" services 2>/dev/null)
if printf '%s\n' "$svc" | grep -qE '^ *sshd '; then
  printf '%s\n' "$svc" | grep -E '^ *sshd ' | grep -qiE 'active|degraded' && ok "outpost.sshd unit up" || bad "outpost.sshd unit not up (outpost services)"
else
  echo "SKIP  outpost.sshd (this outpost predates the unit)"
fi
[ "$fail" = 0 ] && echo "check: all mandatory checks passed" || { echo "check: FAILED" >&2; exit 1; }
```

### spotcheck
Optional, offered to the user after `check`: one quick probe per layer and per
tier, plus local Ollama. Layers: Classic (bash + coreutils), Bash#, Yoke (the
agent framework). Tiers: userland, workplace, sandbox, sphere, cluster, cloud.
Tiers 3-6 report from outpost's own state and configured providers; nothing is
started or pulled. `OLLAMA_MODEL=<name>` adds a one-line generation with a model
that is already present.
Effects: exec, net, read

```bash
pass() { echo "PASS  $1"; }
skip() { echo "SKIP  $1"; }
stop() { echo "FAIL  $1"; echo "spotcheck: stopped at the first failure" >&2; exit 1; }
B="$BASHY"
[ "$(printf 'b\na\n' | sort | head -1)" = a ] && [ "$(echo $((2**10)))" = 1024 ] && pass "layer 1 Classic: bash + coreutils" || stop "layer 1 Classic"
[ "$("$B" --bashsharp -c 'x := 2 + 3; println(x)' 2>/dev/null)" = 5 ] && pass "layer 2 Bash#" || stop "layer 2 Bash#"
[ -n "$("$B" commands --agentic 2>/dev/null | head -1)" ] && pass "layer 3 Yoke: agent command catalog" || stop "layer 3 Yoke"
"$B" ls / >/dev/null 2>&1 && pass "tier 1 userland" || stop "tier 1 userland"
"$B" kb --help >/dev/null 2>&1 && "$B" sprint --help >/dev/null 2>&1 && pass "tier 2 workplace (kb, sprint)" || stop "tier 2 workplace"
svc=$(outpost services 2>/dev/null); st=$(outpost status 2>/dev/null)
printf '%s\n' "$svc" | grep -E '^ *sandbox ' | grep -qi active && pass "tier 3 sandbox (outpost.sandbox active)" || skip "tier 3 sandbox (not enabled on this outpost)"
printf '%s\n' "$st" | grep -qE '^ +mesh +true' && pass "tier 4 sphere (mesh on)" || skip "tier 4 sphere (mesh off)"
printf '%s\n' "$st" | grep -qE '^ +cluster +true' && pass "tier 5 cluster (DKS member)" || skip "tier 5 cluster (not a DKS member)"
"$B" infra list 2>/dev/null | grep -q . && pass "tier 6 cloud (provider configured)" || skip "tier 6 cloud (no provider configured)"
models=$("$B" ollama list 2>/dev/null | tail -n +2)
if [ -z "$models" ]; then skip "ollama (no local models; pull one with bashy ollama pull <model>)"
else
  pass "ollama ($(printf '%s\n' "$models" | wc -l | tr -d ' ') local model(s))"
  if [ -n "${OLLAMA_MODEL:-}" ]; then
    out=$("$B" ollama run "$OLLAMA_MODEL" "Reply with the single word: ready" 2>/dev/null | head -1)
    [ -n "$out" ] && pass "ollama generation ($OLLAMA_MODEL): $out" || stop "ollama generation with $OLLAMA_MODEL"
  fi
fi
echo "spotcheck: done"
```

### tutorial
Optional, offered after `check`: a guided tour with one runnable example per
layer, each printed before it runs.
Effects: exec, read

```bash
B="$BASHY"
demo() { echo; echo "\$ $1"; eval "$1"; }
echo "Layer 1 - Classic: GNU Bash 5.3 + POSIX coreutils, in pure Go"
demo 'printf "%s\n" banana apple cherry | sort | head -2'
echo; echo "Layer 2 - Bash#: Go-shaped scripting inside bash"
echo; echo '$ bashy --bashsharp -c '"'"'x := 42; func twice(n int) int { return n * 2 }; y := twice(x); echo "x=$x twice=$y"'"'"
"$B" --bashsharp -c 'x := 42
func twice(n int) int { return n * 2 }
y := twice(x)
echo "x=$x twice=$y"'
echo "(more in the Bash# tour: https://github.com/bashsharp/tour)"
echo; echo "Layer 3 - Yoke: the agent framework (tools, knowledge, coordination)"
demo '"$B" commands --agentic | head -5'
demo '"$B" kb --help | head -3'
echo; echo "Every bashy command is also a skill: try \"$B <command> --help\" or \"$B skill list\"."
```
