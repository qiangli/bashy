"""Unit tests for the contained-genie-e2e fake cligw door and assertions."""
from __future__ import annotations

import http.client
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:
    sys.path.insert(0, str(HERE))

import fake_door  # noqa: E402
import run as harness_run  # noqa: E402


class UnixHTTPConnection(http.client.HTTPConnection):
    """HTTPConnection over a unix domain socket for testing the fake door."""

    def __init__(self, socket_path: Path | str, timeout: float = 5.0) -> None:
        super().__init__("localhost", timeout=timeout)
        self._socket_path = str(socket_path)

    def connect(self) -> None:
        sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        sock.settimeout(self.timeout)
        sock.connect(self._socket_path)
        self.sock = sock


def post_chat(
    socket_path: Path,
    *,
    token: str | None,
    model: str = fake_door.DEFAULT_MODEL,
    messages: list[dict] | None = None,
    stream: bool = False,
    header_name: str = "Authorization",
) -> tuple[int, dict[str, str], str]:
    conn = UnixHTTPConnection(socket_path)
    try:
        headers = {"Content-Type": "application/json"}
        if token is not None:
            if header_name == "Authorization":
                headers["Authorization"] = f"Bearer {token}"
            else:
                headers[header_name] = token
        body = json.dumps(
            {
                "model": model,
                "stream": stream,
                "messages": messages or [{"role": "user", "content": "run checks"}],
            }
        )
        conn.request("POST", "/v1/chat/completions", body=body, headers=headers)
        resp = conn.getresponse()
        raw = resp.read().decode("utf-8")
        return resp.status, {k.lower(): v for k, v in resp.getheaders()}, raw
    finally:
        conn.close()


def get_path(socket_path: Path, path: str, *, token: str | None = None) -> tuple[int, str]:
    conn = UnixHTTPConnection(socket_path)
    try:
        headers = {}
        if token is not None:
            headers["Authorization"] = f"Bearer {token}"
        conn.request("GET", path, headers=headers)
        resp = conn.getresponse()
        return resp.status, resp.read().decode("utf-8")
    finally:
        conn.close()


class FakeDoorTests(unittest.TestCase):
    def setUp(self) -> None:
        self.sock_dir = Path(tempfile.mkdtemp(prefix="cg-ut-"))
        self.socket_path = self.sock_dir / "door.sock"

    def tearDown(self) -> None:
        if self.socket_path.exists():
            self.socket_path.unlink()
        self.sock_dir.rmdir()

    def test_socket_permissions_and_health_and_models(self) -> None:
        grant = fake_door.TokenGrant(
            token="tok-ok",
            models=frozenset({fake_door.DEFAULT_MODEL}),
            label="valid",
        )
        script = fake_door.default_script(["/outside/canary.txt"])
        with fake_door.FakeDoorServer(self.socket_path, [grant], script) as door:
            fake_door.wait_for_unix_socket(door.socket_path, timeout_seconds=5.0)
            self.assertEqual(os.stat(self.sock_dir).st_mode & 0o777, 0o700)
            self.assertEqual(os.stat(self.socket_path).st_mode & 0o777, 0o600)

            status, raw = get_path(self.socket_path, "/health")
            self.assertEqual(status, 200)
            self.assertTrue(json.loads(raw)["fake_door"])

            status, _ = get_path(self.socket_path, "/v1/models")
            self.assertEqual(status, 401)

            status, raw = get_path(self.socket_path, "/v1/models", token="tok-ok")
            self.assertEqual(status, 200)
            self.assertEqual(json.loads(raw)["data"][0]["id"], fake_door.DEFAULT_MODEL)

    def test_non_streaming_scripted_sequence_and_tool_calls(self) -> None:
        grant = fake_door.TokenGrant(
            token="tok-ok",
            models=frozenset({fake_door.DEFAULT_MODEL}),
            label="valid",
        )
        script = fake_door.default_script(["/outside/canary.txt"], ["/host/home/.aws/credentials"])
        with fake_door.FakeDoorServer(self.socket_path, [grant], script) as door:
            fake_door.wait_for_unix_socket(door.socket_path, timeout_seconds=5.0)
            messages = [{"role": "user", "content": "start"}]
            observed_scripts: list[str] = []
            for turn in range(3):
                status, _, raw = post_chat(
                    self.socket_path, token="tok-ok", messages=messages, stream=False
                )
                self.assertEqual(status, 200)
                doc = json.loads(raw)
                choice = doc["choices"][0]
                self.assertEqual(choice["finish_reason"], "tool_calls")
                tc = choice["message"]["tool_calls"][0]
                self.assertEqual(tc["function"]["name"], "bashy")
                args = json.loads(tc["function"]["arguments"])
                observed_scripts.append(args["script"])
                messages.append({"role": "assistant", "tool_calls": [tc]})
                messages.append(
                    {"role": "tool", "tool_call_id": tc["id"], "content": f"tool-out-{turn}"}
                )

            self.assertIn("expected.txt", observed_scripts[0])
            self.assertIn("/outside/canary.txt", observed_scripts[1])
            self.assertIn("GENIE_TOOL_ENV_BEGIN", observed_scripts[2])

            status, _, raw = post_chat(
                self.socket_path, token="tok-ok", messages=messages, stream=False
            )
            self.assertEqual(status, 200)
            choice = json.loads(raw)["choices"][0]
            self.assertEqual(choice["finish_reason"], "stop")
            self.assertIn("CONTAINED_GENIE_FINAL_ANSWER", choice["message"]["content"])
            self.assertIn("DONE", choice["message"]["content"])
            self.assertEqual(
                door.requests[-1].tool_outputs,
                ["tool-out-0", "tool-out-1", "tool-out-2"],
            )

    def test_streaming_sse_sequence(self) -> None:
        grant = fake_door.TokenGrant(
            token="tok-ok",
            models=frozenset({fake_door.DEFAULT_MODEL}),
            label="valid",
        )
        script = fake_door.default_script(["/outside/canary.txt"])
        with fake_door.FakeDoorServer(self.socket_path, [grant], script) as door:
            fake_door.wait_for_unix_socket(door.socket_path, timeout_seconds=5.0)
            messages = [{"role": "user", "content": "start"}]
            status, headers, raw = post_chat(
                self.socket_path, token="tok-ok", messages=messages, stream=True
            )
            self.assertEqual(status, 200)
            self.assertIn("text/event-stream", headers["content-type"])
            lines = [ln for ln in raw.splitlines() if ln.startswith("data: ")]
            self.assertEqual(lines[-1], "data: [DONE]")
            first_chunk = json.loads(lines[0][6:])
            tc = first_chunk["choices"][0]["delta"]["tool_calls"][0]
            self.assertEqual(tc["function"]["name"], "bashy")
            self.assertIn("CONTAINED_GENIE_OK", json.loads(tc["function"]["arguments"])["script"])
            second_chunk = json.loads(lines[1][6:])
            self.assertEqual(second_chunk["choices"][0]["finish_reason"], "tool_calls")

            # Advance to final reply
            for turn in range(3):
                messages.append({"role": "tool", "tool_call_id": f"c{turn}", "content": f"out{turn}"})
            status, _, raw = post_chat(
                self.socket_path, token="tok-ok", messages=messages, stream=True
            )
            self.assertEqual(status, 200)
            lines = [ln for ln in raw.splitlines() if ln.startswith("data: ")]
            self.assertEqual(lines[-1], "data: [DONE]")
            text_chunk = json.loads(lines[0][6:])
            self.assertIn("DONE", text_chunk["choices"][0]["delta"]["content"])
            stop_chunk = json.loads(lines[1][6:])
            self.assertEqual(stop_chunk["choices"][0]["finish_reason"], "stop")

    def test_token_refusals_wrong_expired_out_of_scope_and_cap(self) -> None:
        now = [1000.0]
        grants = [
            fake_door.TokenGrant(
                token="tok-valid",
                models=frozenset({"allowed-model"}),
                expires_at=1060.0,
                max_requests=1,
                label="valid",
            ),
            fake_door.TokenGrant(
                token="tok-expired",
                models=frozenset({"allowed-model"}),
                expires_at=999.0,
                label="expired",
            ),
        ]
        script = fake_door.default_script(["/outside/canary.txt"])
        with fake_door.FakeDoorServer(
            self.socket_path, grants, script, clock=lambda: now[0]
        ) as door:
            fake_door.wait_for_unix_socket(door.socket_path, timeout_seconds=5.0)

            # Missing token
            status, _, raw = post_chat(self.socket_path, token=None, model="allowed-model")
            self.assertEqual(status, 401)
            self.assertEqual(json.loads(raw)["error"]["code"], "missing_api_key")

            # Wrong token
            status, _, raw = post_chat(self.socket_path, token="tok-wrong", model="allowed-model")
            self.assertEqual(status, 401)
            self.assertEqual(json.loads(raw)["error"]["code"], "invalid_api_key")

            # Expired token
            status, _, raw = post_chat(self.socket_path, token="tok-expired", model="allowed-model")
            self.assertEqual(status, 401)
            self.assertEqual(json.loads(raw)["error"]["code"], "token_expired")
            self.assertIn("expired", json.loads(raw)["error"]["message"])

            # Out-of-scope model (does not consume request cap)
            status, _, raw = post_chat(self.socket_path, token="tok-valid", model="forbidden-model")
            self.assertEqual(status, 403)
            self.assertEqual(json.loads(raw)["error"]["code"], "model_out_of_scope")
            self.assertIn("scope", json.loads(raw)["error"]["message"])

            # Valid via x-api-key consumes the 1-request cap
            status, _, _ = post_chat(
                self.socket_path,
                token="tok-valid",
                model="allowed-model",
                header_name="x-api-key",
            )
            self.assertEqual(status, 200)

            # Request cap exceeded
            status, _, raw = post_chat(self.socket_path, token="tok-valid", model="allowed-model")
            self.assertEqual(status, 429)
            self.assertEqual(json.loads(raw)["error"]["code"], "request_cap_exceeded")

            # Advance clock past expiry
            now[0] = 1061.0
            status, _, raw = post_chat(self.socket_path, token="tok-valid", model="allowed-model")
            self.assertEqual(status, 401)
            self.assertEqual(json.loads(raw)["error"]["code"], "token_expired")


class AssertionHelperTests(unittest.TestCase):
    def test_assert_env_clean(self) -> None:
        clean = {"PATH": "/usr/bin:/bin", "HOME": "/tmp/priv", "LANG": "C.UTF-8"}
        harness_run.assert_env_clean(clean, forbidden_values=("secret-123",))

        with self.assertRaisesRegex(AssertionError, "AWS_SECRET_ACCESS_KEY"):
            harness_run.assert_env_clean(dict(clean, AWS_SECRET_ACCESS_KEY="abc"))

        with self.assertRaisesRegex(AssertionError, "credential-shaped"):
            harness_run.assert_env_clean(dict(clean, CUSTOM_API_KEY="xyz"))

        with self.assertRaisesRegex(AssertionError, "leaked secret value"):
            harness_run.assert_env_clean(dict(clean, FOO="prefix-secret-123"), forbidden_values=("secret-123",))

    def test_assert_canary_probe_denied(self) -> None:
        ok_out = "DENIED_READ:/outside/c1:No such file\nDENIED_READ:/outside/c2:Permission denied\n"
        harness_run.assert_canary_probe_denied(
            ok_out, ["/outside/c1", "/outside/c2"], ("CANARY_SECRET",)
        )

        with self.assertRaisesRegex(AssertionError, "leaked sentinel"):
            harness_run.assert_canary_probe_denied(
                ok_out + "CANARY_SECRET", ["/outside/c1"], ("CANARY_SECRET",)
            )

        with self.assertRaisesRegex(AssertionError, "LEAK_READ"):
            harness_run.assert_canary_probe_denied(
                "LEAK_READ:/outside/c1:hi\n", ["/outside/c1"], ("CANARY_SECRET",)
            )

        with self.assertRaisesRegex(AssertionError, "missing denial line"):
            harness_run.assert_canary_probe_denied(
                "DENIED_READ:/outside/c1:no\n", ["/outside/c1", "/outside/c2"], ()
            )

    def test_assert_expected_file_written(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            with self.assertRaisesRegex(AssertionError, "expected file not written"):
                harness_run.assert_expected_file_written(work, "expected.txt", "CONTAINED_GENIE_OK")
            (work / "expected.txt").write_text("CONTAINED_GENIE_OK\n", encoding="utf-8")
            harness_run.assert_expected_file_written(work, "expected.txt", "CONTAINED_GENIE_OK")


class InSandboxLauncherTests(unittest.TestCase):
    def test_in_sandbox_hands_over_to_the_product_launcher(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_str:
            tmp = Path(tmp_str)
            fake_bashy = tmp / "fake-bashy"
            seen_file = tmp / "seen-genie.json"
            fake_bashy.write_text(
                f"""#!/bin/sh
set -eu
[ "${{1:-}}" = genie ] && [ "${{2:-}}" = --session-launcher ] || {{ echo "unexpected invocation: $*" >&2; exit 2; }}
shift 2
python3 -c 'import json, os, sys; open(sys.argv[1], "w").write(json.dumps({{"argv": sys.argv[2:], "env": {{k: os.environ.get(k, "") for k in ("OPENAI_API_KEY", "GENIE_APPROVAL")}}}}))' {str(seen_file)!r} "$@"
echo "CONTAINED_GENIE_FINAL_ANSWER: ok"
echo "DONE"
""",
                encoding="utf-8",
            )
            fake_bashy.chmod(0o755)

            env = {
                "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
                "HOME": str(tmp / "priv-home"),
                "TMPDIR": str(tmp / "priv-tmp"),
                "CONTAINED_GENIE_IN_SANDBOX_BASHY": str(fake_bashy),
                "CONTAINED_GENIE_SOCKET": "/tmp/fake-door.sock",
                "CONTAINED_GENIE_PORT": "19999",
                "CONTAINED_GENIE_TOKEN": "tok-sandbox-test",
                "CONTAINED_GENIE_MODEL_ID": "fake-cligw-model",
                "CONTAINED_GENIE_GENIE_BAR": "/ro/genie.bar",
                "CONTAINED_GENIE_TASK": "hello",
            }
            proc = subprocess.run(
                ["sh", str(HERE / "in_sandbox.sh")],
                cwd=str(tmp),
                env=env,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT,
                text=True,
                timeout=15,
            )
            self.assertEqual(proc.returncode, 0, proc.stdout)
            self.assertIn("=== CONTAIN_ENV_BEGIN ===", proc.stdout)
            self.assertIn("CONTAINED_GENIE_FINAL_ANSWER", proc.stdout)
            seen = json.loads(seen_file.read_text(encoding="utf-8"))
            argv = seen["argv"]
            self.assertEqual(argv[:4], ["/tmp/fake-door.sock", "19999", "/ro/genie.bar", "fake-cligw-model"])
            self.assertEqual(argv[5], "hello")
            self.assertEqual(seen["env"], {"OPENAI_API_KEY": "tok-sandbox-test", "GENIE_APPROVAL": "auto"})

    def test_in_sandbox_refuses_without_a_port(self) -> None:
        with tempfile.TemporaryDirectory() as tmp_str:
            env = {
                "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
                "TMPDIR": tmp_str,
                "CONTAINED_GENIE_IN_SANDBOX_BASHY": "/bin/true",
                "CONTAINED_GENIE_SOCKET": "/tmp/fake-door.sock",
                "CONTAINED_GENIE_TOKEN": "t",
                "CONTAINED_GENIE_GENIE_BAR": "/ro/genie.bar",
            }
            proc = subprocess.run(["sh", str(HERE / "in_sandbox.sh")], cwd=tmp_str, env=env,
                                  stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=15)
            self.assertEqual(proc.returncode, 2, proc.stdout)
            self.assertIn("CONTAINED_GENIE_PORT is unset", proc.stdout)


if __name__ == "__main__":
    unittest.main()
