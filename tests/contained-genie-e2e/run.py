#!/usr/bin/env python3
"""Run the contained-genie e2e gate (Seam D) on a claimed Linux gate lane."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import subprocess
import sys
import tempfile
import time

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))

import fake_door  # noqa: E402

PASS = "PASS contained genie e2e"
CANARY_SENTINEL = "CANARY_OUTSIDE_DIR_SECRET_738"
CRED_FILE_SENTINEL = "SYNTHETIC_CREDENTIAL_FILE_SECRET_738"
HOST_ENV_SENTINEL = "SYNTHETIC_HOST_CRED_ENV_738"

HOST_CRED_ENV_VARS = (
    "AWS_SECRET_ACCESS_KEY",
    "AWS_ACCESS_KEY_ID",
    "ANTHROPIC_API_KEY",
    "GITHUB_TOKEN",
    "GH_TOKEN",
    "CLAUDE_CODE_OAUTH_TOKEN",
    "BASHY_VAULT_KEY",
    "CLOUD_TOKEN",
)

CREDENTIAL_ENV_NAME_RE = re.compile(
    r"(?:API_KEY|SECRET|TOKEN|PASSWORD|PASSWD|CREDENTIAL|AUTH|PRIVATE_KEY|COOKIE)",
    re.IGNORECASE,
)


def env_float(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    val = float(raw)
    if val <= 0:
        raise ValueError(f"{name} must be positive, got {raw!r}")
    return val


def env_int(name: str, default: int) -> int:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    val = int(raw)
    if val <= 0:
        raise ValueError(f"{name} must be positive, got {raw!r}")
    return val


def load_image_lock(lock_path: Path) -> dict[str, str]:
    pairs: dict[str, str] = {}
    for line in lock_path.read_text(encoding="utf-8").splitlines():
        if line and not line.startswith("#") and "=" in line:
            k, v = line.split("=", 1)
            pairs[k.strip()] = v.strip()
    return pairs


def extract_delimited_block(text: str, begin_marker: str, end_marker: str) -> str:
    if begin_marker not in text or end_marker not in text:
        raise AssertionError(f"missing {begin_marker}..{end_marker} block in output")
    after = text.split(begin_marker, 1)[1]
    return after.split(end_marker, 1)[0]


def parse_env_lines(block: str) -> dict[str, str]:
    env_map: dict[str, str] = {}
    for line in block.splitlines():
        line = line.strip()
        if not line or "=" not in line:
            continue
        k, v = line.split("=", 1)
        env_map[k] = v
    return env_map


def assert_env_clean(
    env_map: dict[str, str],
    *,
    forbidden_vars: tuple[str, ...] = HOST_CRED_ENV_VARS,
    forbidden_values: tuple[str, ...] = (),
    allow_vars: frozenset[str] = frozenset(),
    context_label: str = "env",
) -> None:
    for var in forbidden_vars:
        if var in env_map and var not in allow_vars:
            raise AssertionError(f"{context_label} leaked forbidden host variable {var}")
    for key, val in env_map.items():
        if key in allow_vars:
            continue
        if CREDENTIAL_ENV_NAME_RE.search(key):
            raise AssertionError(f"{context_label} exposed credential-shaped variable {key!r}")
        for secret_val in forbidden_values:
            if secret_val and secret_val in val:
                raise AssertionError(f"{context_label} leaked secret value under {key!r}")


def assert_canary_probe_denied(
    probe_output: str,
    expected_paths: list[str] | tuple[str, ...],
    forbidden_sentinels: tuple[str, ...],
) -> None:
    for sentinel in forbidden_sentinels:
        if sentinel and sentinel in probe_output:
            raise AssertionError(f"canary probe leaked sentinel {sentinel!r}")
    if "LEAK_READ:" in probe_output:
        raise AssertionError(f"canary probe reported LEAK_READ:\n{probe_output}")
    for path in expected_paths:
        marker = f"DENIED_READ:{path}:"
        if marker not in probe_output:
            raise AssertionError(f"canary probe missing denial line for {path}:\n{probe_output}")


def assert_expected_file_written(work_dir: Path, filename: str, expected_text: str) -> None:
    target = work_dir / filename
    if not target.is_file():
        raise AssertionError(f"expected file not written in DIR: {target}")
    actual = target.read_text(encoding="utf-8")
    if expected_text not in actual:
        raise AssertionError(
            f"expected file {target} has {actual!r}, want substring {expected_text!r}"
        )


def stage_host_home_and_canaries(base_dir: Path) -> tuple[Path, list[str], list[str], tuple[str, ...]]:
    """Create a synthetic host HOME with credential files and outside canaries."""
    host_home = base_dir / "host-home"
    host_home.mkdir(parents=True, exist_ok=True)
    outside_dir = base_dir / "outside"
    outside_dir.mkdir(parents=True, exist_ok=True)

    outside_canary = outside_dir / "canary.txt"
    outside_canary.write_text(f"{CANARY_SENTINEL}:outside\n", encoding="utf-8")
    os.chmod(outside_canary, 0o644)

    home_canary = host_home / ".canary-home.txt"
    home_canary.write_text(f"{CANARY_SENTINEL}:home\n", encoding="utf-8")
    os.chmod(home_canary, 0o644)

    cred_relpaths = (
        ".aws/credentials",
        ".ssh/id_rsa",
        ".config/bashy/secrets.json",
        ".claude/.credentials.json",
        ".codex/auth.json",
    )
    cred_paths: list[str] = []
    for idx, rel in enumerate(cred_relpaths):
        path = host_home / rel
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"{CRED_FILE_SENTINEL}:{idx}:{rel}\n", encoding="utf-8")
        os.chmod(path, 0o644)
        cred_paths.append(str(path))

    sentinels = (CANARY_SENTINEL, CRED_FILE_SENTINEL, HOST_ENV_SENTINEL)
    return host_home, [str(outside_canary), str(home_canary)], cred_paths, sentinels


def build_genie_bundle(bashy_bin: Path, staging_bashy_home: Path, wait_seconds: float) -> Path:
    """Build genie.bar into staging_bashy_home using the candidate bashy."""
    env = dict(os.environ, BASHY_HOME=str(staging_bashy_home), BASHY_HINTS="off")
    proc = subprocess.run(
        [str(bashy_bin), "genie", "build"],
        cwd=str(ROOT),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        timeout=wait_seconds,
    )
    if proc.returncode != 0:
        raise RuntimeError(f"bashy genie build failed ({proc.returncode}):\n{proc.stdout}")
    bar_path = staging_bashy_home / "genie" / "genie.bar"
    if not bar_path.is_file():
        raise RuntimeError(f"bashy genie build did not produce {bar_path}")
    return bar_path


def run_contained_leg(
    *,
    bashy_bin: Path,
    work_dir: Path,
    socket_path: Path,
    bar_path: Path,
    host_home: Path,
    contain_image: str,
    token: str,
    model_id: str,
    forward_wait_seconds: float,
    request_timeout_ms: int,
    step_wait_seconds: float,
    host_sentinels_env: dict[str, str],
) -> subprocess.CompletedProcess[str]:
    """Invoke `bashy contain --net deny --dir ... --socket ...` running in_sandbox.sh."""
    env = dict(os.environ)
    env.update(host_sentinels_env)
    env.update({
        "HOME": str(host_home),
        "BASHY_HINTS": "off",
        "BASHY_ADVISOR": "off",
        "BASHY_CONTAIN_IMAGE": contain_image,
        # The bashy that runs INSIDE the image: a static linux build (the
        # scratch profile the released image carries), not the host binary.
        "BASHY_CONTAIN_BASHY": os.environ.get("CONTAINED_GENIE_SANDBOX_BASHY", str(bashy_bin)),
        "CONTAINED_GENIE_SOCKET": str(socket_path),
        "CONTAINED_GENIE_TOKEN": token,
        "CONTAINED_GENIE_MODEL_ID": model_id,
        "CONTAINED_GENIE_GENIE_BAR": str(bar_path),
        "CONTAINED_GENIE_TASK": (
            "Write expected.txt in the workspace, try to read the canary outside DIR, "
            "and print the environment."
        ),
        "CONTAINED_GENIE_FORWARD_READY_SECONDS": str(max(1, int(forward_wait_seconds))),
        "CONTAINED_GENIE_REQUEST_TIMEOUT_MS": str(request_timeout_ms),
    })

    with socket.socket() as probe:
        probe.bind(("127.0.0.1", 0))
        port = str(probe.getsockname()[1])
    env["CONTAINED_GENIE_PORT"] = port

    argv = [
        str(bashy_bin),
        "contain",
        "--net",
        "deny",
        "--dir",
        str(work_dir),
        "--socket",
        str(socket_path),
        "--loopback",
        port,
        "--env",
        "CONTAINED_GENIE_PORT",
        "--ro",
        str(HERE),
        "--ro",
        str(bar_path.parent),
        "--env",
        "CONTAINED_GENIE_SOCKET",
        "--env",
        "CONTAINED_GENIE_TOKEN",
        "--env",
        "CONTAINED_GENIE_MODEL_ID",
        "--env",
        "CONTAINED_GENIE_GENIE_BAR",
        "--env",
        "CONTAINED_GENIE_TASK",
        "--env",
        "CONTAINED_GENIE_FORWARD_READY_SECONDS",
        "--env",
        "CONTAINED_GENIE_REQUEST_TIMEOUT_MS",
        "--",
        "bashy",
        str(HERE / "in_sandbox.sh"),
    ]
    return subprocess.run(
        argv,
        cwd=str(work_dir),
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
        timeout=step_wait_seconds,
    )


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, required=True, help="new raw log file")
    parser.add_argument(
        "--wait-seconds",
        type=float,
        default=env_float("CONTAINED_GENIE_WAIT_SECONDS", 120.0),
        help="per-leg timeout in seconds (env: CONTAINED_GENIE_WAIT_SECONDS)",
    )
    parser.add_argument(
        "--forward-wait-seconds",
        type=float,
        default=env_float("CONTAINED_GENIE_FORWARD_READY_SECONDS", 20.0),
        help="timeout waiting for llm session forward (env: CONTAINED_GENIE_FORWARD_READY_SECONDS)",
    )
    parser.add_argument(
        "--socket-wait-seconds",
        type=float,
        default=env_float("CONTAINED_GENIE_SOCKET_READY_SECONDS", 15.0),
        help="timeout waiting for fake door unix socket (env: CONTAINED_GENIE_SOCKET_READY_SECONDS)",
    )
    parser.add_argument(
        "--request-timeout-ms",
        type=int,
        default=env_int("CONTAINED_GENIE_REQUEST_TIMEOUT_MS", 30000),
        help="per-request genie timeout in ms (env: CONTAINED_GENIE_REQUEST_TIMEOUT_MS)",
    )
    args = parser.parse_args()

    if sys.platform != "linux" or os.environ.get("CONTAINED_GENIE_GATE") != "1":
        parser.error("run on a claimed Linux gate lane with CONTAINED_GENIE_GATE=1")

    raw_bin = os.environ.get("CONTAINED_GENIE_BASHY_BIN", str(ROOT / "bin" / "bashy"))
    bashy_bin = Path(raw_bin).resolve()
    if not bashy_bin.is_file() or not os.access(bashy_bin, os.X_OK):
        parser.error(f"executable bashy binary not found: {bashy_bin} (set CONTAINED_GENIE_BASHY_BIN)")

    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    with args.evidence.open("x", encoding="utf-8") as log:
        def record(msg: str) -> None:
            print(msg, file=log, flush=True)
            print(msg, flush=True)

        def check(cond: bool, msg: str) -> None:
            if not cond:
                raise AssertionError(msg)
            record(f"ok - {msg}")

        for path in (Path(__file__), HERE / "fake_door.py", HERE / "in_sandbox.sh", ROOT / "dag.md"):
            record(f"{path.name} sha256={hashlib.sha256(path.read_bytes()).hexdigest()}")
        record(f"bashy sha256={hashlib.sha256(bashy_bin.read_bytes()).hexdigest()}")

        lock = load_image_lock(ROOT / "tools/bashy-build/image.lock")
        contain_image = os.environ.get("CONTAINED_GENIE_IMAGE", "").strip() or lock["base_image"]
        info = subprocess.check_output(
            [str(bashy_bin), "oci", "image", "inspect", "--format", "{{.Digest}} {{.Id}}", contain_image],
            env=dict(os.environ, BASHY_HINTS="off"),
            text=True,
            timeout=args.socket_wait_seconds,
        ).split()
        record(f"image {contain_image} {' '.join(info)}")
        if not os.environ.get("CONTAINED_GENIE_IMAGE", "").strip():
            if len(info) != 2 or info[0] != lock["base_digest"]:
                raise RuntimeError("cached base image differs from image.lock")

        with tempfile.TemporaryDirectory(prefix="cg-e2e-") as tmp_str:
            tmp_dir = Path(tmp_str).resolve()
            # Keep unix socket path well under sun_path limit.
            sock_dir = Path(tempfile.mkdtemp(prefix="cg-sock-"))
            socket_path = sock_dir / "door.sock"
            try:
                host_home, canary_paths, cred_paths, sentinels = stage_host_home_and_canaries(tmp_dir)
                staging_home = tmp_dir / "staging-bashy-home"
                if env_bar := os.environ.get("CONTAINED_GENIE_GENIE_BAR", "").strip():
                    bar_path = Path(env_bar).resolve()
                    check(bar_path.is_file(), f"using prebuilt genie.bar {bar_path}")
                else:
                    bar_path = build_genie_bundle(bashy_bin, staging_home, args.wait_seconds)
                    record(f"built genie.bar sha256={hashlib.sha256(bar_path.read_bytes()).hexdigest()}")

                valid_token = "s738-valid-" + secrets.token_hex(12)
                expired_token = "s738-expired-" + secrets.token_hex(12)
                wrong_token = "s738-wrong-" + secrets.token_hex(12)
                allowed_model = fake_door.DEFAULT_MODEL
                out_of_scope_model = "out-of-scope-model"

                grants = [
                    fake_door.TokenGrant(
                        token=valid_token,
                        models=frozenset({allowed_model}),
                        expires_at=time.time() + 3600.0,
                        max_requests=20,
                        label="valid",
                    ),
                    fake_door.TokenGrant(
                        token=expired_token,
                        models=frozenset({allowed_model}),
                        expires_at=time.time() - 60.0,
                        max_requests=20,
                        label="expired",
                    ),
                ]
                script = fake_door.default_script(canary_paths, cred_paths)
                host_sentinels_env = {
                    name: f"{HOST_ENV_SENTINEL}_{name}" for name in HOST_CRED_ENV_VARS
                }
                host_sentinels_env["OPENAI_API_KEY"] = f"{HOST_ENV_SENTINEL}_HOST_OPENAI_KEY"

                with fake_door.FakeDoorServer(socket_path, grants, script) as door:
                    fake_door.wait_for_unix_socket(socket_path, timeout_seconds=args.socket_wait_seconds)
                    check(
                        (os.stat(socket_path).st_mode & 0o777) == 0o600
                        and (os.stat(sock_dir).st_mode & 0o777) == 0o700,
                        "fake door unix socket is 0600 inside a 0700 directory",
                    )

                    # --- Leg 1: valid contained run ---
                    record("== Leg 1: contained genie with valid token ==")
                    work_dir = tmp_dir / "work-valid"
                    work_dir.mkdir()
                    res1 = run_contained_leg(
                        bashy_bin=bashy_bin,
                        work_dir=work_dir,
                        socket_path=socket_path,
                        bar_path=bar_path,
                        host_home=host_home,
                        contain_image=contain_image,
                        token=valid_token,
                        model_id=allowed_model,
                        forward_wait_seconds=args.forward_wait_seconds,
                        request_timeout_ms=args.request_timeout_ms,
                        step_wait_seconds=args.wait_seconds,
                        host_sentinels_env=host_sentinels_env,
                    )
                    record(res1.stdout.rstrip())
                    check(res1.returncode == 0, f"contained genie valid run exited 0 (got {res1.returncode})")
                    assert_expected_file_written(work_dir, fake_door.EXPECTED_FILENAME, fake_door.EXPECTED_CONTENT)
                    check(True, f"{fake_door.EXPECTED_FILENAME} written inside DIR with expected content")

                    contain_env = parse_env_lines(
                        extract_delimited_block(res1.stdout, "=== CONTAIN_ENV_BEGIN ===", "=== CONTAIN_ENV_END ===")
                    )
                    assert_env_clean(
                        contain_env,
                        forbidden_values=tuple(host_sentinels_env.values()) + sentinels,
                        allow_vars=frozenset({"CONTAINED_GENIE_TOKEN"}),
                        context_label="contain environment",
                    )
                    check(True, "bashy contain scrubbed host credential variables")

                    valid_calls = [
                        r for r in door.requests
                        if r.method == "POST" and r.path.endswith("/chat/completions") and r.token_label == "valid" and r.status == 200
                    ]
                    check(len(valid_calls) >= 4, f"fake door served full 4-step scripted sequence ({len(valid_calls)} calls)")
                    final_outputs = valid_calls[-1].tool_outputs
                    check(len(final_outputs) >= 3, f"genie returned {len(final_outputs)} tool outputs to fake door")

                    canary_output = final_outputs[1]
                    assert_canary_probe_denied(canary_output, [*canary_paths, *cred_paths], sentinels)
                    check(True, "outside canary and host credential files are unreadable inside sandbox")

                    genie_env_output = final_outputs[2]
                    genie_env = parse_env_lines(
                        extract_delimited_block(
                            genie_env_output, "=== GENIE_TOOL_ENV_BEGIN ===", "=== GENIE_TOOL_ENV_END ==="
                        )
                    )
                    assert_env_clean(
                        genie_env,
                        forbidden_values=tuple(host_sentinels_env.values()) + sentinels + (valid_token,),
                        context_label="genie tool environment",
                    )
                    check(True, "genie tool environment exposes no credential-shaped variable or run token")

                    home_ls = extract_delimited_block(
                        genie_env_output, "=== GENIE_HOME_LS_BEGIN ===", "=== GENIE_HOME_LS_END ==="
                    )
                    for cred_name in (".aws", ".ssh", ".claude", ".codex", ".canary-home.txt"):
                        check(cred_name not in home_ls, f"sandbox HOME does not expose {cred_name}")
                    check(
                        "CONTAINED_GENIE_FINAL_ANSWER" in res1.stdout and "DONE" in res1.stdout,
                        "genie final answer printed CONTAINED_GENIE_FINAL_ANSWER and DONE",
                    )

                    # --- Leg 2: wrong token refused ---
                    record("== Leg 2: wrong token refused ==")
                    before_count = len(door.requests)
                    work_wrong = tmp_dir / "work-wrong"
                    work_wrong.mkdir()
                    res2 = run_contained_leg(
                        bashy_bin=bashy_bin,
                        work_dir=work_wrong,
                        socket_path=socket_path,
                        bar_path=bar_path,
                        host_home=host_home,
                        contain_image=contain_image,
                        token=wrong_token,
                        model_id=allowed_model,
                        forward_wait_seconds=args.forward_wait_seconds,
                        request_timeout_ms=args.request_timeout_ms,
                        step_wait_seconds=args.wait_seconds,
                        host_sentinels_env=host_sentinels_env,
                    )
                    record(res2.stdout.rstrip())
                    check(res2.returncode != 0, f"wrong token exited non-zero ({res2.returncode})")
                    wrong_reqs = [
                        r for r in door.requests[before_count:]
                        if r.status == 401 and r.error_code == "invalid_api_key"
                    ]
                    check(bool(wrong_reqs), "fake door refused wrong token with HTTP 401 invalid_api_key")

                    # --- Leg 3: expired token refused ---
                    record("== Leg 3: expired token refused ==")
                    before_count = len(door.requests)
                    work_exp = tmp_dir / "work-expired"
                    work_exp.mkdir()
                    res3 = run_contained_leg(
                        bashy_bin=bashy_bin,
                        work_dir=work_exp,
                        socket_path=socket_path,
                        bar_path=bar_path,
                        host_home=host_home,
                        contain_image=contain_image,
                        token=expired_token,
                        model_id=allowed_model,
                        forward_wait_seconds=args.forward_wait_seconds,
                        request_timeout_ms=args.request_timeout_ms,
                        step_wait_seconds=args.wait_seconds,
                        host_sentinels_env=host_sentinels_env,
                    )
                    record(res3.stdout.rstrip())
                    check(res3.returncode != 0, f"expired token exited non-zero ({res3.returncode})")
                    exp_reqs = [
                        r for r in door.requests[before_count:]
                        if r.status == 401 and r.error_code == "token_expired"
                    ]
                    check(bool(exp_reqs), "fake door refused expired token with HTTP 401 token_expired")

                    # --- Leg 4: out-of-scope model refused ---
                    record("== Leg 4: out-of-scope model refused ==")
                    before_count = len(door.requests)
                    work_scope = tmp_dir / "work-scope"
                    work_scope.mkdir()
                    res4 = run_contained_leg(
                        bashy_bin=bashy_bin,
                        work_dir=work_scope,
                        socket_path=socket_path,
                        bar_path=bar_path,
                        host_home=host_home,
                        contain_image=contain_image,
                        token=valid_token,
                        model_id=out_of_scope_model,
                        forward_wait_seconds=args.forward_wait_seconds,
                        request_timeout_ms=args.request_timeout_ms,
                        step_wait_seconds=args.wait_seconds,
                        host_sentinels_env=host_sentinels_env,
                    )
                    record(res4.stdout.rstrip())
                    check(res4.returncode != 0, f"out-of-scope model exited non-zero ({res4.returncode})")
                    scope_reqs = [
                        r for r in door.requests[before_count:]
                        if r.status == 403 and r.error_code == "model_out_of_scope"
                    ]
                    check(bool(scope_reqs), "fake door refused out-of-scope model with HTTP 403 model_out_of_scope")

                    record("== Recorded fake door requests ==")
                    record(
                        json.dumps(
                            [
                                {
                                    "method": r.method,
                                    "path": r.path,
                                    "status": r.status,
                                    "model": r.model,
                                    "stream": r.stream,
                                    "token_label": r.token_label,
                                    "error_code": r.error_code,
                                }
                                for r in door.requests
                            ],
                            indent=2,
                        )
                    )
            finally:
                shutil.rmtree(sock_dir, ignore_errors=True)

        record(f"{PASS}; evidence {args.evidence}")


if __name__ == "__main__":
    main()
