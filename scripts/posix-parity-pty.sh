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
PROMPT = "@@@PROMPT@@@ "


class Probe:
    def __init__(self, num, name, lines=None, env=None, initial_prompt=False, info=""):
        self.num = num
        self.name = name
        self.lines = lines or []
        self.env = env or {}
        self.initial_prompt = initial_prompt
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
        "POSIX PS1 history/bang/parameter expansion",
        env={
            "PVAR": "VALUE",
            "PS1": "P! !! ${PVAR}> ",
        },
        initial_prompt=True,
    ),
    Probe(
        46,
        "interactive comments remain enabled",
        [
            "echo before # after",
        ],
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
    return env


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
        docker_env.extend(["-e", f"{key}={value}"])
    return [
        "docker",
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


def read_available(master, sel, proc, deadline, stop=None):
    out = bytearray()
    while time.time() < deadline:
        timeout = max(0.0, min(0.05, deadline - time.time()))
        events = sel.select(timeout)
        if not events:
            if proc.poll() is not None:
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

    deadline = time.time() + 4.0
    first = read_available(master, sel, proc, deadline, PROMPT.encode())
    first_text = strip_terminal_noise(first)
    if proc.poll() is not None and PROMPT not in first_text and not probe.initial_prompt:
        return normalize_output(first_text, []), "err", "shell exited before prompt"

    if probe.initial_prompt:
        if proc.poll() is not None:
            return normalize_prompt(first), "err", "shell exited before prompt"
        prompt_out = normalize_prompt(first)
        if not prompt_out:
            return prompt_out, "err", "missing prompt"
        try:
            os.write(master, b"exit\n")
        except OSError:
            pass
        read_available(master, sel, proc, time.time() + 1.0)
        terminate(proc)
        return prompt_out, "ok", ""

    begin = f"printf '@@@P:{probe.num}@@@\\n'"
    end = f"printf '@@@X:{probe.num}:%s@@@\\n' \"$?\""
    sent_lines = [begin, *probe.lines, end, "exit"]
    script = "\n".join(sent_lines) + "\n"
    try:
        os.write(master, script.encode())
    except OSError as exc:
        terminate(proc)
        return "", "err", f"write failed: {exc}"

    end_marker = f"@@@X:{probe.num}:".encode()
    raw = first + read_available(master, sel, proc, time.time() + 4.0, end_marker)
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


def compare():
    results = []
    for probe in PROBES:
        by_out, by_ok, by_note = run_probe(bashy_cmd(probe), probe)
        bh_out, bh_ok, bh_note = run_probe(ref_cmd(probe), probe)
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
