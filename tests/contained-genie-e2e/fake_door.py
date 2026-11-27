#!/usr/bin/env python3
"""Tiny fake cligw HTTP server on a unix socket for contained-genie e2e.

Speaks OpenAI-compatible `/v1/chat/completions` (streaming SSE and non-streaming
JSON), `/v1/models`, and `/health` over an owner-only unix domain socket.
Validates bearer tokens (valid, wrong, expired, out-of-scope model, and
request caps) and returns deterministic scripted completions with `bashy` tool
calls followed by a final answer ending with DONE.
"""
from __future__ import annotations

import argparse
from dataclasses import dataclass, field
from http.server import BaseHTTPRequestHandler
import json
import os
from pathlib import Path
import shlex
import socket
import socketserver
import threading
import time
from typing import Callable


DEFAULT_MODEL = "fake-cligw-model"
EXPECTED_FILENAME = "expected.txt"
EXPECTED_CONTENT = "CONTAINED_GENIE_OK"


@dataclass(frozen=True)
class TokenGrant:
    """Authorization policy for one bearer token."""

    token: str
    models: frozenset[str] = frozenset()
    expires_at: float | None = None
    max_requests: int | None = None
    label: str = "run"


@dataclass(frozen=True)
class ScriptStep:
    """One scripted assistant response for `/v1/chat/completions`."""

    content: str = ""
    tool_calls: tuple[tuple[str, dict], ...] = ()


@dataclass
class RecordedRequest:
    """Captured HTTP request observed by the fake door."""

    method: str
    path: str
    status: int
    model: str = ""
    stream: bool = False
    token_label: str = ""
    error_code: str = ""
    error_message: str = ""
    messages: list[dict] = field(default_factory=list)
    tool_outputs: list[str] = field(default_factory=list)


def default_script(
    canary_paths: list[str] | tuple[str, ...],
    cred_paths: list[str] | tuple[str, ...] = (),
    expected_file: str = EXPECTED_FILENAME,
    expected_marker: str = EXPECTED_CONTENT,
) -> list[ScriptStep]:
    """Build the Seam D scripted tool-call sequence and final answer."""
    probe_targets = [str(p) for p in (*canary_paths, *cred_paths)]
    if not probe_targets:
        raise ValueError("default_script requires at least one canary path")
    quoted_targets = " ".join(shlex.quote(p) for p in probe_targets)
    write_cmd = (
        f"printf '%s\\n' {shlex.quote(expected_marker)} > {shlex.quote(expected_file)} "
        f"&& cat {shlex.quote(expected_file)}"
    )
    canary_cmd = "\n".join([
        '@effects("read,write,exec")',
        '@guard("read,write,exec")',
        "function probe_canary() {",
        f"  for p in {quoted_targets}; do",
        '    out=$(cat "$p" 2>&1) && echo "LEAK_READ:$p:$out" || echo "DENIED_READ:$p:$out"',
        "  done",
        "}",
        "probe_canary",
    ])
    env_cmd = "\n".join([
        '@effects("read,write,exec")',
        '@guard("read,write,exec")',
        "function probe_env() {",
        '  echo "=== GENIE_TOOL_ENV_BEGIN ==="',
        "  env",
        '  echo "=== GENIE_TOOL_ENV_END ==="',
        '  echo "=== GENIE_HOME_LS_BEGIN ==="',
        '  ls -la "$HOME" 2>&1 || true',
        '  echo "=== GENIE_HOME_LS_END ==="',
        "}",
        "probe_env",
    ])
    return [
        ScriptStep(tool_calls=(("bashy", {"script": write_cmd}),)),
        ScriptStep(tool_calls=(("bashy", {"script": canary_cmd}),)),
        ScriptStep(tool_calls=(("bashy", {"script": env_cmd}),)),
        ScriptStep(
            content=(
                f"CONTAINED_GENIE_FINAL_ANSWER: Wrote {expected_file}, verified outside canaries "
                "are unreadable, and inspected the sandbox environment.\nDONE"
            )
        ),
    ]


class _UnixHTTPServer(socketserver.ThreadingUnixStreamServer):
    allow_reuse_address = True
    daemon_threads = True


class FakeDoorServer:
    """Thread-safe fake cligw HTTP server bound to an owner-only unix socket."""

    def __init__(
        self,
        socket_path: Path | str,
        grants: list[TokenGrant],
        script: list[ScriptStep],
        *,
        clock: Callable[[], float] = time.time,
        socket_mode: int = 0o600,
    ) -> None:
        self.socket_path = Path(socket_path)
        if len(str(self.socket_path).encode("utf-8")) >= 100:
            raise ValueError(
                f"unix socket path too long ({len(str(self.socket_path))} bytes): {self.socket_path}"
            )
        self.grants = {g.token: g for g in grants}
        self.script = list(script)
        if not self.script:
            raise ValueError("script must not be empty")
        self.clock = clock
        self.socket_mode = socket_mode
        self._lock = threading.Lock()
        self._token_counts: dict[str, int] = {}
        self._requests: list[RecordedRequest] = []
        self._httpd: _UnixHTTPServer | None = None
        self._thread: threading.Thread | None = None

    @property
    def requests(self) -> list[RecordedRequest]:
        with self._lock:
            return list(self._requests)

    def start(self) -> "FakeDoorServer":
        if self._httpd is not None:
            return self
        self.socket_path.parent.mkdir(parents=True, exist_ok=True)
        os.chmod(self.socket_path.parent, 0o700)
        if self.socket_path.exists():
            self.socket_path.unlink()

        door = self

        class Handler(BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"

            def address_string(self) -> str:
                return "unix"

            def log_message(self, format: str, *args: object) -> None:  # noqa: A002
                return

            def do_GET(self) -> None:
                path = self.path.split("?", 1)[0]
                if path == "/health":
                    door._record(RecordedRequest(method="GET", path=path, status=200))
                    self._write_json(200, {"status": "ok", "fake_door": True})
                    return
                if path in ("/v1/models", "/models"):
                    grant, err = door._authorize(self.headers, model=None, count_request=False)
                    if err is not None:
                        status, code, msg, err_type = err
                        door._record(
                            RecordedRequest(
                                method="GET",
                                path=path,
                                status=status,
                                error_code=code,
                                error_message=msg,
                            )
                        )
                        self._write_error(status, code, msg, err_type)
                        return
                    models = sorted(grant.models) if grant.models else [DEFAULT_MODEL]
                    door._record(
                        RecordedRequest(
                            method="GET",
                            path=path,
                            status=200,
                            token_label=grant.label,
                        )
                    )
                    self._write_json(
                        200,
                        {
                            "object": "list",
                            "data": [{"id": m, "object": "model"} for m in models],
                        },
                    )
                    return
                door._record(
                    RecordedRequest(
                        method="GET",
                        path=path,
                        status=404,
                        error_code="not_found",
                        error_message="unknown path",
                    )
                )
                self._write_error(404, "not_found", f"unknown path: {path}", "invalid_request_error")

            def do_POST(self) -> None:
                path = self.path.split("?", 1)[0]
                if path not in ("/v1/chat/completions", "/chat/completions"):
                    door._record(
                        RecordedRequest(
                            method="POST",
                            path=path,
                            status=404,
                            error_code="not_found",
                            error_message=f"unsupported path: {path}",
                        )
                    )
                    self._write_error(
                        404, "not_found", f"unsupported path: {path}", "invalid_request_error"
                    )
                    return

                length = int(self.headers.get("Content-Length", "0") or "0")
                raw_body = self.rfile.read(length) if length > 0 else b"{}"
                try:
                    payload = json.loads(raw_body.decode("utf-8"))
                except (UnicodeDecodeError, json.JSONDecodeError) as exc:
                    door._record(
                        RecordedRequest(
                            method="POST",
                            path=path,
                            status=400,
                            error_code="invalid_json",
                            error_message=str(exc),
                        )
                    )
                    self._write_error(400, "invalid_json", f"invalid json: {exc}", "invalid_request_error")
                    return

                model = str(payload.get("model") or "")
                stream = bool(payload.get("stream", False))
                messages = payload.get("messages") or []
                if not isinstance(messages, list):
                    messages = []
                tool_outputs = [
                    str(m.get("content", ""))
                    for m in messages
                    if isinstance(m, dict) and m.get("role") == "tool"
                ]

                grant, err = door._authorize(self.headers, model=model, count_request=True)
                if err is not None:
                    status, code, msg, err_type = err
                    door._record(
                        RecordedRequest(
                            method="POST",
                            path=path,
                            status=status,
                            model=model,
                            stream=stream,
                            error_code=code,
                            error_message=msg,
                            messages=messages,
                            tool_outputs=tool_outputs,
                        )
                    )
                    self._write_error(status, code, msg, err_type)
                    return

                step_idx = min(len(tool_outputs), len(door.script) - 1)
                step = door.script[step_idx]
                call_seq = door._next_seq()
                door._record(
                    RecordedRequest(
                        method="POST",
                        path=path,
                        status=200,
                        model=model,
                        stream=stream,
                        token_label=grant.label,
                        messages=messages,
                        tool_outputs=tool_outputs,
                    )
                )
                if stream:
                    self._write_sse(model or DEFAULT_MODEL, step, call_seq)
                else:
                    self._write_json(200, _completion_json(model or DEFAULT_MODEL, step, call_seq))

            def _write_json(self, status: int, obj: dict) -> None:
                data = (json.dumps(obj) + "\n").encode("utf-8")
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(data)
                self.wfile.flush()

            def _write_error(self, status: int, code: str, message: str, err_type: str) -> None:
                self._write_json(
                    status,
                    {"error": {"message": message, "type": err_type, "code": code}},
                )

            def _write_sse(self, model: str, step: ScriptStep, seq: int) -> None:
                chunks = _completion_sse_chunks(model, step, seq)
                payload = "".join(f"data: {c}\n\n" for c in chunks) + "data: [DONE]\n\n"
                raw = payload.encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Cache-Control", "no-cache")
                self.send_header("Content-Length", str(len(raw)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(raw)
                self.wfile.flush()

        self._httpd = _UnixHTTPServer(str(self.socket_path), Handler)
        os.chmod(self.socket_path, self.socket_mode)
        self._thread = threading.Thread(target=self._httpd.serve_forever, daemon=True)
        self._thread.start()
        return self

    def close(self) -> None:
        if self._httpd is not None:
            self._httpd.shutdown()
            self._httpd.server_close()
            self._httpd = None
        if self._thread is not None:
            self._thread.join(timeout=5)
            self._thread = None
        if self.socket_path.exists():
            try:
                self.socket_path.unlink()
            except OSError:
                pass

    def __enter__(self) -> "FakeDoorServer":
        return self.start()

    def __exit__(self, *_exc: object) -> None:
        self.close()

    def _record(self, rec: RecordedRequest) -> None:
        with self._lock:
            self._requests.append(rec)

    def _next_seq(self) -> int:
        with self._lock:
            return len(self._requests) + 1

    def _authorize(
        self, headers: object, *, model: str | None, count_request: bool
    ) -> tuple[TokenGrant | None, tuple[int, str, str, str] | None]:
        presented = ""
        auth_header = headers.get("Authorization", "") if hasattr(headers, "get") else ""
        if isinstance(auth_header, str) and auth_header.lower().startswith("bearer "):
            presented = auth_header[7:].strip()
        if not presented and hasattr(headers, "get"):
            api_key = headers.get("x-api-key", "")
            if isinstance(api_key, str):
                presented = api_key.strip()
        if not presented:
            return None, (401, "missing_api_key", "missing bearer token", "authentication_error")

        grant = self.grants.get(presented)
        if grant is None:
            return None, (401, "invalid_api_key", "invalid bearer token", "authentication_error")

        now = self.clock()
        if grant.expires_at is not None and now >= grant.expires_at:
            return None, (
                401,
                "token_expired",
                "session token expired (ttl exceeded)",
                "authentication_error",
            )

        if model is not None and grant.models and model not in grant.models:
            allowed = ", ".join(sorted(grant.models))
            return None, (
                403,
                "model_out_of_scope",
                f"model {model!r} is outside session token scope ({allowed})",
                "permission_error",
            )

        if count_request and grant.max_requests is not None:
            with self._lock:
                used = self._token_counts.get(presented, 0)
                if used >= grant.max_requests:
                    return None, (
                        429,
                        "request_cap_exceeded",
                        f"session request cap ({grant.max_requests}) exceeded",
                        "rate_limit_error",
                    )
                self._token_counts[presented] = used + 1

        return grant, None


def _completion_json(model: str, step: ScriptStep, seq: int) -> dict:
    message: dict = {"role": "assistant", "content": step.content}
    finish_reason = "stop"
    if step.tool_calls:
        finish_reason = "tool_calls"
        message["tool_calls"] = [
            {
                "id": f"call_fake_{seq}_{idx}",
                "type": "function",
                "function": {"name": name, "arguments": json.dumps(args)},
            }
            for idx, (name, args) in enumerate(step.tool_calls)
        ]
    return {
        "id": f"chatcmpl-fake-{seq}",
        "object": "chat.completion",
        "created": 1700000000 + seq,
        "model": model,
        "choices": [
            {
                "index": 0,
                "message": message,
                "finish_reason": finish_reason,
            }
        ],
        "usage": {
            "prompt_tokens": 16,
            "completion_tokens": 8,
            "total_tokens": 24,
        },
    }


def _completion_sse_chunks(model: str, step: ScriptStep, seq: int) -> list[str]:
    cid = f"chatcmpl-fake-{seq}"
    created = 1700000000 + seq
    chunks: list[dict] = []
    if step.tool_calls:
        delta_calls = [
            {
                "index": idx,
                "id": f"call_fake_{seq}_{idx}",
                "type": "function",
                "function": {"name": name, "arguments": json.dumps(args)},
            }
            for idx, (name, args) in enumerate(step.tool_calls)
        ]
        chunks.append(
            {
                "id": cid,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model,
                "choices": [
                    {
                        "index": 0,
                        "delta": {
                            "role": "assistant",
                            "content": step.content,
                            "tool_calls": delta_calls,
                        },
                        "finish_reason": None,
                    }
                ],
            }
        )
        chunks.append(
            {
                "id": cid,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model,
                "choices": [{"index": 0, "delta": {}, "finish_reason": "tool_calls"}],
                "usage": {
                    "prompt_tokens": 16,
                    "completion_tokens": 8,
                    "total_tokens": 24,
                },
            }
        )
    else:
        chunks.append(
            {
                "id": cid,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model,
                "choices": [
                    {
                        "index": 0,
                        "delta": {"role": "assistant", "content": step.content},
                        "finish_reason": None,
                    }
                ],
            }
        )
        chunks.append(
            {
                "id": cid,
                "object": "chat.completion.chunk",
                "created": created,
                "model": model,
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                "usage": {
                    "prompt_tokens": 16,
                    "completion_tokens": 8,
                    "total_tokens": 24,
                },
            }
        )
    return [json.dumps(c) for c in chunks]


def wait_for_unix_socket(socket_path: Path | str, timeout_seconds: float = 15.0) -> None:
    """Wait until a unix domain socket accepts a connection."""
    deadline = time.monotonic() + timeout_seconds
    last_err: OSError | None = None
    while time.monotonic() < deadline:
        try:
            with socket.socket(socket.AF_UNIX, socket.SOCK_STREAM) as sock:
                sock.settimeout(1.0)
                sock.connect(str(socket_path))
                return
        except OSError as exc:
            last_err = exc
            time.sleep(0.05)
    raise TimeoutError(f"unix socket {socket_path} did not accept connections: {last_err}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--socket", type=Path, required=True, help="unix socket path to bind")
    parser.add_argument("--token", required=True, help="valid bearer token")
    parser.add_argument("--model", default=DEFAULT_MODEL, help="allowed model id")
    parser.add_argument("--canary", action="append", default=["/etc/hosts"], help="canary path to probe")
    args = parser.parse_args()

    grant = TokenGrant(token=args.token, models=frozenset({args.model}), label="cli")
    script = default_script(args.canary)
    with FakeDoorServer(args.socket, [grant], script) as server:
        wait_for_unix_socket(server.socket_path, timeout_seconds=5.0)
        print(f"fake door listening on {server.socket_path}", flush=True)
        try:
            while True:
                time.sleep(1.0)
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
