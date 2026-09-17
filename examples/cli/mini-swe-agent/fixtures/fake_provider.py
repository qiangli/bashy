"""A loopback, deterministic OpenAI-compatible chat/completions provider.

For fixtures only. Binds to 127.0.0.1 on an ephemeral port and replays a scripted
sequence of assistant messages (each a bash code block) so the LiveModel's real
urllib transport is exercised with NO network egress and NO paid model call.

Usage:
    from fake_provider import run_server
    with run_server(["```bash\\necho hi\\n```", ...]) as base_url:
        ...  # point OPENAI_BASE_URL / --base-url at base_url
"""

from __future__ import annotations

import contextlib
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def _make_handler(replies: list[str], state: dict):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):  # silence
            pass

        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            _ = self.rfile.read(length)  # consume request body
            if not self.path.rstrip("/").endswith("/chat/completions"):
                self.send_error(404, "not found")
                return
            idx = state["i"]
            state["i"] += 1
            if idx >= len(replies):
                # Deterministic end: always submit if the script runs out.
                content = "```bash\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n```"
            else:
                content = replies[idx]
            payload = {
                "id": f"fake-{idx}",
                "object": "chat.completion",
                "model": "fake-model",
                "choices": [{"index": 0, "message": {"role": "assistant", "content": content},
                             "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
            }
            body = json.dumps(payload).encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    return Handler


@contextlib.contextmanager
def run_server(replies: list[str]):
    state = {"i": 0}
    server = ThreadingHTTPServer(("127.0.0.1", 0), _make_handler(replies, state))
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    host, port = server.server_address
    try:
        yield f"http://{host}:{port}/v1"
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


if __name__ == "__main__":
    # Manual smoke: print a base_url and serve until Ctrl-C.
    import time

    with run_server(["```bash\necho hello\n```",
                     "```bash\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\necho done\n```"]) as url:
        print(url, flush=True)
        with contextlib.suppress(KeyboardInterrupt):
            while True:
                time.sleep(1)
