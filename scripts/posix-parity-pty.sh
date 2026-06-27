#!/usr/bin/env bash
# posix-parity-pty.sh — Phase 2 POSIX-mode interactive conformance probe.
#
# Drives `bashy --posix -i` through a real pseudo-terminal and compares it with
# GNU bash 5.3 `bash --posix -i` running in docker. This covers POSIX-mode
# behavior that only appears for interactive shells: prompt rendering, readline
# input, alias expansion across entered lines, and visible interactive option
# state.
#
# Like scripts/posix-parity.sh, this is a semantic parity harness. It compares
# normalized observable output plus success/fail and deliberately ignores
# non-mandated diagnostic wording. Keep probes small and deterministic; mark
# inherently host/terminal-specific checks INFO when adding them.
#
# Usage: scripts/posix-parity-pty.sh
#        BASHY=./bin/bashy scripts/posix-parity-pty.sh
#        BASH_REF=/path/to/bash scripts/posix-parity-pty.sh   # smoke fallback
#
# Exit: 0 iff every non-INFO probe matches.
set -u

BASHY=${BASHY:-./bin/bashy}
export BASHY
export BASH_REF=${BASH_REF:-}

# Container runtime for the bash 5.3 oracle (same convention as
# scripts/posix-parity.sh): default docker, fall back to `bashy podman`.
# Ignored when BASH_REF points at a local bash binary.
OCI=${OCI:-}
if [ -z "$OCI" ] && [ -z "$BASH_REF" ]; then
  if command -v docker >/dev/null 2>&1; then OCI=docker
  elif command -v bashy  >/dev/null 2>&1; then OCI="bashy podman"
  fi
fi
export OCI

exec python3 - "$@" <<'PY'
import os
import pty
import re
import selectors
import subprocess
import sys
import termios
import time

BASHY = os.environ.get("BASHY", "./bin/bashy")
BASH_REF = os.environ.get("BASH_REF", "")
# Container runtime command (e.g. "docker" or "bashy podman"), space-split.
OCI = os.environ.get("OCI", "docker").split()
PROMPT = "@@@PROMPT@@@ "


class Probe:
    def __init__(self, num, name, lines=None, env=None, prompt_ps1=None, info=""):
        self.num = num
        self.name = name
        self.lines = lines or []
        self.env = env or {}
        # When set, this probe assigns PS1 to this value IN-SESSION and captures
        # the resulting rendered prompt (real bash does not import PS1 from the
        # environment for interactive shells, so it must be set interactively).
        self.prompt_ps1 = prompt_ps1
        self.info = info


PROBES = [
    Probe(
        3,
        "interactive alias expansion",
        [
            "alias hi='printf \"alias-ok\\n\"'",
            "hi",
        ],
    ),
    Probe(
        29,
        "POSIX PS1 parameter + !! expansion",
        # POSIX behavior #29: in posix mode bash performs parameter expansion on
        # PS1 and expands !! -> ! (and ! -> history number). We test the
        # deterministic parts: ${PVAR} -> VALUE and !! -> ! . The bare !
        # (history number) is omitted because it depends on per-shell history
        # counting, which is not a POSIX-mandated value.
        env={"PVAR": "VALUE"},
        prompt_ps1="q:${PVAR}:!!>",
    ),
    Probe(
        46,
        "interactive comments remain enabled",
        [
            "echo before # after",
        ],
    ),
    Probe(
        30,
        "POSIX default HISTFILE is ~/.sh_history",
        # #30: in posix mode the default $HISTFILE is ~/.sh_history. Unset the
        # harness HISTFILE override so we observe the shell's own default.
        ['printf "hf=%s\\n" "${HISTFILE-UNSET}"'],
        env={"HISTFILE": None},
    ),
    Probe(
        31,
        "POSIX ! no history expansion in double quotes",
        # #31: in posix mode, ! does not introduce history expansion inside a
        # double-quoted string even with histexpand on, so this prints x!!y.
        ['echo "x!!y"'],
    ),
    Probe(
        0,
        "interactive shell reports posix option state",
        [
            "case $- in *i*) echo interactive;; *) echo not-interactive;; esac",
            "set -o | grep '^posix'",
        ],
    ),
]


def base_env(extra):
    env = {
        "BASH_SILENCE_DEPRECATION_WARNING": "1",
        "HISTFILE": "/dev/null",
        "HOME": "/tmp",
        "LC_ALL": "C",
        "PATH": "/usr/bin:/bin",
        "TERM": "xterm",
        "PS1": PROMPT,
    }
    env.update(extra)
    # A probe may set a base key to None to UNSET it (e.g. to observe the
    # shell's own default for HISTFILE rather than the harness override).
    return {k: v for k, v in env.items() if v is not None}


def env_cmd(env):
    args = ["env", "-i"]
    for key, value in env.items():
        args.append(f"{key}={value}")
    return args


def bashy_cmd(probe):
    return env_cmd(base_env(probe.env)) + [BASHY, "--noprofile", "--norc", "--posix", "-i"]


def ref_cmd(probe):
    env = base_env(probe.env)
    if BASH_REF:
        return env_cmd(env) + [BASH_REF, "--noprofile", "--norc", "--posix", "-i"]
    docker_env = []
    for key, value in env.items():
        if key == "PATH":
            # The bash:5.3 image keeps bash and its docker-entrypoint.sh in
            # /usr/local/bin; restricting PATH to the bashy side's /usr/bin:/bin
            # would hide them and the container fails to start. Use the image's
            # default PATH for the oracle (it still contains /bin/grep etc., so
            # the probes resolve the same external commands).
            value = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
        docker_env.extend(["-e", f"{key}={value}"])
    return [
        *OCI,
        "run",
        "--rm",
        "-i",
        "-t",
        *docker_env,
        "bash:5.3",
        "bash",
        "--noprofile",
        "--norc",
        "--posix",
        "-i",
    ]


ANSI_RE = re.compile(
    r"""
    \x1b
    (?:
        \[[0-?]*[ -/]*[@-~] |
        \][^\x07]*(?:\x07|\x1b\\) |
        [@-Z\\-_]
    )
    """,
    re.VERBOSE,
)


def strip_terminal_noise(data):
    text = data.decode("utf-8", "replace").replace("\r", "")
    text = ANSI_RE.sub("", text)
    text = text.replace(" \b", "")
    text = text.replace("\b", "")
    return text


def normalize_output(text, sent_lines):
    lines = []
    sent = set(sent_lines)
    for raw in text.split("\n"):
        line = raw.strip()
        if not line:
            continue
        if line.startswith(PROMPT):
            line = line[len(PROMPT):].strip()
        if PROMPT in line:
            line = line.replace(PROMPT, "").strip()
        if line in sent:
            continue
        if line.startswith("@@@P:") or line.startswith("@@@X:"):
            continue
        line = re.sub(r"[^ ]*(bashy|bash):", "SH:", line)
        line = re.sub(r"line [0-9]+", "line N", line)
        lines.append(line)
    return "~".join(lines)


def normalize_prompt(text):
    text = strip_terminal_noise(text)
    text = text.strip()
    text = re.sub(r"\s+", " ", text)
    return text


def read_available(master, sel, proc, deadline, stop=None, idle=None):
    # Read until `deadline`, or until `stop` bytes are seen, or — when `idle`
    # is set — until the stream has produced nothing for `idle` seconds after
    # having produced something (useful to settle on a prompt we can't predict).
    out = bytearray()
    last_data = None
    while time.time() < deadline:
        timeout = max(0.0, min(0.05, deadline - time.time()))
        events = sel.select(timeout)
        if not events:
            if proc.poll() is not None:
                break
            if idle is not None and last_data is not None and (time.time() - last_data) >= idle:
                break
            continue
        for _key, _mask in events:
            try:
                chunk = os.read(master, 4096)
            except OSError:
                return bytes(out)
            if not chunk:
                return bytes(out)
            out.extend(chunk)
            last_data = time.time()
            # bashy's readline asks the terminal for cursor position. Answer
            # with a stable top-left coordinate so prompt redraw can continue.
            if b"\x1b[6n" in chunk:
                os.write(master, b"\x1b[1;1R")
            if stop and stop in out:
                return bytes(out)
    return bytes(out)


def terminate(proc):
    if proc.poll() is not None:
        return
    try:
        proc.terminate()
        proc.wait(timeout=0.3)
    except Exception:
        try:
            proc.kill()
        except Exception:
            pass


def spawn(cmd):
    master, slave = pty.openpty()
    attrs = termios.tcgetattr(slave)
    attrs[3] &= ~termios.ECHO
    termios.tcsetattr(slave, termios.TCSANOW, attrs)
    proc = subprocess.Popen(
        cmd,
        stdin=slave,
        stdout=slave,
        stderr=slave,
        close_fds=True,
        start_new_session=True,
    )
    os.close(slave)
    os.set_blocking(master, False)
    sel = selectors.DefaultSelector()
    sel.register(master, selectors.EVENT_READ)
    return proc, master, sel


def run_probe(cmd, probe):
    try:
        proc, master, sel = spawn(cmd)
    except FileNotFoundError as exc:
        return "", "err", f"missing command: {exc.filename}"

    # Let the shell start and print its first (default) prompt. The oracle's
    # prompt is not our sentinel, so don't key off PROMPT here — just settle
    # until the startup output goes idle.
    first = read_available(master, sel, proc, time.time() + 6.0, stop=None, idle=0.4)
    first_text = strip_terminal_noise(first)
    if proc.poll() is not None:
        return normalize_output(first_text, []), "err", "shell exited before prompt"

    if probe.prompt_ps1:
        # Set PS1 in-session, then echo a marker. The rendered prompt is the
        # text printed immediately before the marker command's echo.
        marker = "@@@PE@@@"
        script = f"PS1='{probe.prompt_ps1}'\necho {marker}\nexit\n"
        try:
            os.write(master, script.encode())
        except OSError as exc:
            terminate(proc)
            return "", "err", f"write failed: {exc}"
        raw = read_available(master, sel, proc, time.time() + 6.0, stop=marker.encode())
        terminate(proc)
        text = strip_terminal_noise(raw)
        # The stream after `PS1=...` is: <renderedPrompt>echo @@@PE@@@\n@@@PE@@@.
        # Grab the rendered prompt = text on the line that contains the echoed
        # `echo @@@PE@@@` command, before that command.
        prompt_out = ""
        for line in text.split("\n"):
            idx = line.find(f"echo {marker}")
            if idx > 0:
                prompt_out = line[:idx].strip()
                break
        if not prompt_out:
            return "", "err", "missing rendered prompt"
        return prompt_out, "ok", ""

    begin = f"printf '@@@P:{probe.num}@@@\\n'"
    end = f"printf '@@@X:{probe.num}:%s@@@\\n' \"$?\""
    # Neutralize the prompt in BOTH shells by assigning a fixed, strippable
    # sentinel in-session (real bash does not import PS1 from the environment
    # for interactive shells, so it must be set this way). normalize_output()
    # strips the PROMPT sentinel and drops readline's echo of each typed line.
    # NB: an *empty* PS1 is not usable here — bash renders no prompt, but bashy
    # treats empty as unset and falls back to its default \u@\h:\w\$ prompt.
    prompt_reset = [f"PS1='{PROMPT}'", "PS2='> '"]
    sent_lines = [*prompt_reset, begin, *probe.lines, end, "exit"]
    script = "\n".join(sent_lines) + "\n"
    try:
        os.write(master, script.encode())
    except OSError as exc:
        terminate(proc)
        return "", "err", f"write failed: {exc}"

    end_marker = f"@@@X:{probe.num}:".encode()
    raw = first + read_available(master, sel, proc, time.time() + 10.0, end_marker)
    terminate(proc)
    text = strip_terminal_noise(raw)

    start = f"@@@P:{probe.num}@@@"
    end_re = re.compile(rf"^@@@X:{re.escape(str(probe.num))}:(\d+)@@@$")
    in_body = False
    body_lines = []
    rc = None
    for raw_line in text.split("\n"):
        line = raw_line.strip()
        if line.startswith(PROMPT):
            line = line[len(PROMPT):].strip()
        if line == start:
            in_body = True
            body_lines = []
            continue
        match = end_re.match(line)
        if in_body and match:
            rc = int(match.group(1))
            break
        if in_body:
            body_lines.append(raw_line)
    if rc is None:
        return normalize_output(text, sent_lines), "err", "missing sentinel"

    body = "\n".join(body_lines)
    ok = "ok" if rc == 0 else "err"
    return normalize_output(body, sent_lines), ok, ""


def run_probe_retry(cmd, probe, attempts=3):
    # Spawning a fresh container per probe occasionally cold-starts slowly and
    # the sentinels miss the read window ("missing sentinel"/"before prompt").
    # Those are transient infra hiccups, not behavior differences — retry a few
    # times before trusting an error verdict.
    out, ok, note = run_probe(cmd, probe)
    transient = ("sentinel", "before prompt", "missing prompt", "missing rendered prompt")
    for _ in range(attempts - 1):
        if not note or not any(t in note for t in transient):
            break
        out, ok, note = run_probe(cmd, probe)
    return out, ok, note


def compare():
    results = []
    for probe in PROBES:
        by_out, by_ok, by_note = run_probe_retry(bashy_cmd(probe), probe)
        bh_out, bh_ok, bh_note = run_probe_retry(ref_cmd(probe), probe)
        results.append((probe, by_out, by_ok, by_note, bh_out, bh_ok, bh_note))
    return results


def main():
    if not os.path.exists(BASHY):
        print(f"error: {BASHY} does not exist; run `go build -o bin/bashy .` first", file=sys.stderr)
        return 2

    match = diff = infon = 0
    for probe, by_out, by_ok, by_note, bh_out, bh_ok, bh_note in compare():
        same = by_out == bh_out and by_ok == bh_ok and not by_note and not bh_note
        label = f"#{probe.num}" if probe.num else "probe"
        if probe.info:
            status = "INFO=" if same else "INFO~"
            print(f"{status}  {label} {probe.name} — {probe.info}")
            infon += 1
            continue
        if by_note or bh_note:
            note = by_note or bh_note
            print(f"ERROR  {label} {probe.name} — {note}")
            if by_out or bh_out:
                print(f"   bashy: [{by_out}] ({by_ok})")
                print(f"   bash:  [{bh_out}] ({bh_ok})")
            diff += 1
            continue
        if same:
            print(f"MATCH  {label} {probe.name}")
            match += 1
        else:
            print(f"DIFF   {label} {probe.name}")
            print(f"   bashy: [{by_out}] ({by_ok})")
            print(f"   bash:  [{bh_out}] ({bh_ok})")
            diff += 1
    print(f"=== {match} match / {diff} diff / {infon} info / {match + diff + infon} probed ===")
    return 0 if diff == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
PY
