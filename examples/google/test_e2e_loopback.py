#!/usr/bin/env python3
"""Hermetic loopback E2E test suite for Google Workspace custom command.

Spins up an in-process ThreadingHTTPServer on 127.0.0.1:0 to simulate
Google Workspace API endpoints (Gmail, Calendar, Tasks, Drive, Auth).
A fake gws executable shim translates CLI invocations into loopback HTTP
requests over TCP, verifying:
- End-to-end HTTP payload delivery (headers, query parameters, JSON bodies).
- HTTP status code propagation and error handling (400, 403, 404, 500, 503).
- Partial-mutation handling in drive-share-mail (upload succeeds, send fails, no retry).
- Mail-to-task linkage (fetching email via GET, inserting linked task via POST).
- Agenda aggregation across Calendar, Tasks, Gmail (and Drive search combinations).
"""

from __future__ import annotations

import base64
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import textwrap
import threading
import unittest
import urllib.error
import urllib.parse
import urllib.request


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
GOOGLE_BSH = os.path.join(SCRIPT_DIR, "google.bsh")
REPO_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))

# Locate bashy binary
CANDIDATE_BASHY = os.path.join(REPO_ROOT, "bin", "bashy")
if os.path.isfile(CANDIDATE_BASHY) and os.access(CANDIDATE_BASHY, os.X_OK):
    BASHY_BIN = CANDIDATE_BASHY
else:
    BASHY_BIN = os.environ.get("BASHY_BIN") or shutil.which("bashy") or "bashy"


def make_fake_gws_shim(loopback_url: str) -> str:
    return textwrap.dedent(
        f"""\
        #!/usr/bin/env python3
        import json
        import os
        import sys
        import urllib.error
        import urllib.parse
        import urllib.request

        loopback_url = os.environ.get("GWS_LOOPBACK_URL", "{loopback_url}").rstrip("/")
        if not loopback_url:
            sys.stderr.write("FAKE_GWS_SHIM_ERROR: Missing GWS_LOOPBACK_URL\\n")
            sys.exit(1)

        log_path = os.environ.get("FAKE_GWS_LOG")
        if log_path:
            entry = {{
                "argv": sys.argv[1:],
                "env": {{
                    "GWS_ACCOUNT": os.environ.get("GWS_ACCOUNT"),
                    "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE": os.environ.get(
                        "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"
                    ),
                }},
            }}
            with open(log_path, "a", encoding="utf-8") as fh:
                fh.write(json.dumps(entry) + "\\n")

        raw_args = sys.argv[1:]
        params = {{}}
        body_obj = None
        upload_file = None
        words = []

        i = 0
        while i < len(raw_args):
            arg = raw_args[i]
            if arg == "--format=json" or arg.startswith("--format="):
                i += 1
                continue
            if arg == "--format":
                i += 2
                continue
            if arg == "--params":
                if i + 1 < len(raw_args):
                    try:
                        params = json.loads(raw_args[i + 1])
                    except Exception:
                        params = {{}}
                    i += 2
                    continue
            if arg.startswith("--params="):
                try:
                    params = json.loads(arg.split("=", 1)[1])
                except Exception:
                    params = {{}}
                i += 1
                continue
            if arg == "--json":
                if i + 1 < len(raw_args):
                    try:
                        body_obj = json.loads(raw_args[i + 1])
                    except Exception:
                        body_obj = raw_args[i + 1]
                    i += 2
                    continue
            if arg.startswith("--json="):
                try:
                    body_obj = json.loads(arg.split("=", 1)[1])
                except Exception:
                    body_obj = arg.split("=", 1)[1]
                i += 1
                continue
            if arg == "--upload":
                if i + 1 < len(raw_args):
                    upload_file = raw_args[i + 1]
                    i += 2
                    continue
            if arg.startswith("--upload="):
                upload_file = arg.split("=", 1)[1]
                i += 1
                continue
            if arg.startswith("-"):
                i += 1
                continue
            words.append(arg)
            i += 1

        method = "GET"
        path = ""
        body_data = None

        if words[:2] == ["auth", "login"]:
            method = "POST"
            path = "/auth/login"
            body_data = {{"status": "login_requested"}}
        elif words[:2] == ["auth", "setup"]:
            method = "POST"
            path = "/auth/setup"
            body_data = {{"status": "setup_requested"}}
        elif words[:2] == ["auth", "export"]:
            method = "GET"
            path = "/auth/export"
        elif words == ["gmail", "users", "messages", "list"]:
            method = "GET"
            uid = params.pop("userId", "me")
            path = f"/gmail/v1/users/{{urllib.parse.quote(str(uid))}}/messages"
        elif words == ["gmail", "users", "messages", "get"]:
            method = "GET"
            uid = params.pop("userId", "me")
            msg_id = params.pop("id", "")
            path = f"/gmail/v1/users/{{urllib.parse.quote(str(uid))}}/messages/{{urllib.parse.quote(str(msg_id))}}"
        elif words == ["gmail", "users", "messages", "send"]:
            method = "POST"
            uid = params.pop("userId", "me")
            path = f"/gmail/v1/users/{{urllib.parse.quote(str(uid))}}/messages/send"
            body_data = body_obj or {{}}
        elif words == ["gmail", "users", "drafts", "create"]:
            method = "POST"
            uid = params.pop("userId", "me")
            path = f"/gmail/v1/users/{{urllib.parse.quote(str(uid))}}/drafts"
            body_data = body_obj or {{}}
        elif words == ["calendar", "events", "list"]:
            method = "GET"
            cid = params.pop("calendarId", "primary")
            path = f"/calendar/v3/calendars/{{urllib.parse.quote(str(cid))}}/events"
        elif words == ["calendar", "events", "get"]:
            method = "GET"
            cid = params.pop("calendarId", "primary")
            eid = params.pop("eventId", "")
            path = f"/calendar/v3/calendars/{{urllib.parse.quote(str(cid))}}/events/{{urllib.parse.quote(str(eid))}}"
        elif words == ["calendar", "events", "insert"]:
            method = "POST"
            cid = params.pop("calendarId", "primary")
            path = f"/calendar/v3/calendars/{{urllib.parse.quote(str(cid))}}/events"
            body_data = body_obj or {{}}
        elif words == ["calendar", "events", "patch"]:
            method = "PATCH"
            cid = params.pop("calendarId", "primary")
            eid = params.pop("eventId", "")
            path = f"/calendar/v3/calendars/{{urllib.parse.quote(str(cid))}}/events/{{urllib.parse.quote(str(eid))}}"
            body_data = body_obj or {{}}
        elif words == ["calendar", "events", "delete"]:
            method = "DELETE"
            cid = params.pop("calendarId", "primary")
            eid = params.pop("eventId", "")
            path = f"/calendar/v3/calendars/{{urllib.parse.quote(str(cid))}}/events/{{urllib.parse.quote(str(eid))}}"
        elif words == ["calendar", "freebusy", "query"]:
            method = "POST"
            path = "/calendar/v3/freeBusy"
            body_data = body_obj or {{}}
        elif words == ["tasks", "tasklists", "list"]:
            method = "GET"
            path = "/tasks/v1/users/@me/lists"
        elif words == ["tasks", "tasks", "list"]:
            method = "GET"
            tlist = params.pop("tasklist", "@default")
            path = f"/tasks/v1/lists/{{urllib.parse.quote(str(tlist), safe='@')}}/tasks"
        elif words == ["tasks", "tasks", "get"]:
            method = "GET"
            tlist = params.pop("tasklist", "@default")
            tid = params.pop("task", "")
            path = f"/tasks/v1/lists/{{urllib.parse.quote(str(tlist), safe='@')}}/tasks/{{urllib.parse.quote(str(tid))}}"
        elif words == ["tasks", "tasks", "insert"]:
            method = "POST"
            tlist = params.pop("tasklist", "@default")
            path = f"/tasks/v1/lists/{{urllib.parse.quote(str(tlist), safe='@')}}/tasks"
            body_data = body_obj or {{}}
        elif words == ["tasks", "tasks", "patch"]:
            method = "PATCH"
            tlist = params.pop("tasklist", "@default")
            tid = params.pop("task", "")
            path = f"/tasks/v1/lists/{{urllib.parse.quote(str(tlist), safe='@')}}/tasks/{{urllib.parse.quote(str(tid))}}"
            body_data = body_obj or {{}}
        elif words == ["tasks", "tasks", "delete"]:
            method = "DELETE"
            tlist = params.pop("tasklist", "@default")
            tid = params.pop("task", "")
            path = f"/tasks/v1/lists/{{urllib.parse.quote(str(tlist), safe='@')}}/tasks/{{urllib.parse.quote(str(tid))}}"
        elif words == ["drive", "files", "list"]:
            method = "GET"
            path = "/drive/v3/files"
        elif words == ["drive", "files", "get"]:
            method = "GET"
            fid = params.pop("fileId", "")
            path = f"/drive/v3/files/{{urllib.parse.quote(str(fid))}}"
        elif words == ["drive", "files", "create"]:
            method = "POST"
            body_data = dict(body_obj or {{}})
            if upload_file:
                path = "/upload/drive/v3/files"
                params["uploadType"] = "multipart"
                if os.path.exists(upload_file):
                    body_data["_uploaded_file"] = {{
                        "path": upload_file,
                        "size": os.path.getsize(upload_file),
                    }}
            else:
                path = "/drive/v3/files"
        else:
            method = "POST" if (body_obj is not None or upload_file) else "GET"
            path = "/" + "/".join(urllib.parse.quote(str(w)) for w in words)
            body_data = body_obj

        if params:
            query_str = urllib.parse.urlencode({{k: "true" if v is True else ("false" if v is False else str(v)) for k, v in params.items()}})
            target_url = f"{{loopback_url}}{{path}}?{{query_str}}"
        else:
            target_url = f"{{loopback_url}}{{path}}"

        account = os.environ.get("GWS_ACCOUNT", "anonymous@example.com")
        headers = {{
            "User-Agent": "googleworkspace-cli-shim/1.0",
            "Accept": "application/json",
            "X-GWS-Account": account,
            "Authorization": f"Bearer mock-token-{{account}}",
        }}

        req_body_bytes = None
        if body_data is not None:
            headers["Content-Type"] = "application/json; charset=utf-8"
            req_body_bytes = json.dumps(body_data).encode("utf-8")

        req = urllib.request.Request(target_url, data=req_body_bytes, headers=headers, method=method)

        try:
            with urllib.request.urlopen(req) as resp:
                resp_bytes = resp.read()
                sys.stdout.buffer.write(resp_bytes)
                if not resp_bytes.endswith(b"\\n"):
                    sys.stdout.buffer.write(b"\\n")
                sys.exit(0)
        except urllib.error.HTTPError as exc:
            err_bytes = exc.read()
            sys.stderr.buffer.write(err_bytes)
            if not err_bytes.endswith(b"\\n"):
                sys.stderr.buffer.write(b"\\n")
            sys.exit(1)
        except Exception as exc:
            sys.stderr.write(f"HTTP_CONNECT_ERROR: {{exc}}\\n")
            sys.exit(1)
        """
    )


class MockWorkspaceHandler(BaseHTTPRequestHandler):
    """In-process mock server handling simulated Google Workspace HTTP requests."""

    protocol_version = "HTTP/1.1"

    def log_message(self, format, *args):
        # Silence console log noise during unittest runs
        pass

    def _read_body(self):
        length = int(self.headers.get("Content-Length", 0))
        if length > 0:
            raw = self.rfile.read(length)
            try:
                parsed_json = json.loads(raw.decode("utf-8"))
            except Exception:
                parsed_json = None
            return raw, parsed_json
        return b"", None

    def _record_request(self, body_bytes, body_json):
        parsed = urllib.parse.urlparse(self.path)
        query = urllib.parse.parse_qs(parsed.query)
        query_flat = {k: v[0] if len(v) == 1 else v for k, v in query.items()}
        record = {
            "method": self.command,
            "raw_path": self.path,
            "path": parsed.path,
            "query": query_flat,
            "headers": dict(self.headers),
            "body_bytes": body_bytes,
            "body_json": body_json,
        }
        with self.server.lock:
            self.server.recorded_requests.append(record)
        return record

    def _send_json_response(self, status_code, data):
        body = json.dumps(data).encode("utf-8")
        self.send_response(status_code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Connection", "close")
        self.end_headers()
        self.wfile.write(body)

    def _dispatch(self):
        body_bytes, body_json = self._read_body()
        req = self._record_request(body_bytes, body_json)
        path = req["path"]

        # 1. Fault injection evaluation
        with self.server.lock:
            matched_fault = None
            for rule in self.server.fault_rules:
                if rule.get("method") and rule["method"] != self.command:
                    continue
                prefix = rule.get("path_prefix")
                exact = rule.get("path")
                if (prefix and path.startswith(prefix)) or (exact and path == exact):
                    rem = rule.get("remaining")
                    if rem is not None:
                        if rem <= 0:
                            continue
                        rule["remaining"] -= 1
                    matched_fault = rule
                    break

        if matched_fault:
            status = matched_fault.get("status", 500)
            err_data = matched_fault.get("response")
            if err_data is None:
                err_data = {
                    "error": {
                        "code": status,
                        "message": matched_fault.get("message", "Fault injected"),
                    }
                }
            self._send_json_response(status, err_data)
            return

        # 2. Custom route overrides
        with self.server.lock:
            custom_handler = self.server.custom_routes.get((self.command, path))
        if custom_handler:
            status, resp_data = custom_handler(req)
            self._send_json_response(status, resp_data)
            return

        # 3. Standard Google Workspace REST Endpoints
        # Auth endpoints
        if path == "/auth/login":
            self._send_json_response(200, {"status": "logged_in"})
            return
        if path == "/auth/setup":
            self._send_json_response(200, {"status": "setup_complete"})
            return
        if path == "/auth/export":
            self._send_json_response(
                200,
                {
                    "type": "authorized_user",
                    "client_id": "loopback-client.apps.googleusercontent.com",
                    "client_secret": "loopback-secret",
                    "refresh_token": "loopback-refresh",
                },
            )
            return

        # Gmail endpoints
        if path.startswith("/gmail/v1/users/") and path.endswith("/messages/send"):
            self._send_json_response(200, {"id": "sent_999", "threadId": "thr_999", "labelIds": ["SENT"]})
            return
        if path.startswith("/gmail/v1/users/") and path.endswith("/drafts"):
            self._send_json_response(200, {"id": "draft_111", "message": {"id": "msg_draft_111"}})
            return
        if path.startswith("/gmail/v1/users/") and "/messages/" in path:
            msg_id = path.split("/messages/")[-1]
            self._send_json_response(
                200,
                {
                    "id": msg_id,
                    "threadId": "thr_456",
                    "snippet": "Please review the Q4 architecture doc by Friday.",
                    "payload": {
                        "headers": [
                            {"name": "Subject", "value": "Q4 Architecture Review"},
                            {"name": "From", "value": "bob@example.com"},
                            {"name": "To", "value": "alice@example.com"},
                        ]
                    },
                },
            )
            return
        if path.startswith("/gmail/v1/users/") and path.endswith("/messages"):
            self._send_json_response(
                200,
                {
                    "messages": [{"id": "msg_123", "threadId": "thr_456"}],
                    "resultSizeEstimate": 1,
                },
            )
            return

        # Calendar endpoints
        if path == "/calendar/v3/freeBusy":
            self._send_json_response(200, {"calendars": {"primary": {"busy": []}}})
            return
        if path.startswith("/calendar/v3/calendars/") and "/events/" in path:
            event_id = path.split("/events/")[-1]
            if self.command == "GET":
                self._send_json_response(200, {"id": event_id, "summary": "Team Standup"})
                return
            if self.command == "PATCH":
                self._send_json_response(
                    200,
                    {
                        "id": event_id,
                        "status": "confirmed",
                        "summary": (body_json or {}).get("summary", "Updated Event"),
                    },
                )
                return
            if self.command == "DELETE":
                self._send_json_response(200, {"deleted": True, "id": event_id})
                return
        if path.startswith("/calendar/v3/calendars/") and path.endswith("/events"):
            if self.command == "GET":
                self._send_json_response(
                    200,
                    {
                        "items": [
                            {
                                "id": "evt_1",
                                "summary": "Team Standup",
                                "start": {"dateTime": "2026-11-10T10:00:00Z"},
                                "end": {"dateTime": "2026-11-10T10:30:00Z"},
                            }
                        ]
                    },
                )
                return
            if self.command == "POST":
                self._send_json_response(
                    201,
                    {
                        "id": "evt_created",
                        "status": "confirmed",
                        "summary": (body_json or {}).get("summary", "New Event"),
                    },
                )
                return

        # Tasks endpoints
        if path == "/tasks/v1/users/@me/lists":
            self._send_json_response(200, {"items": [{"id": "@default", "title": "My Tasks"}]})
            return
        if path.startswith("/tasks/v1/lists/"):
            parts = path.split("/")
            if len(parts) >= 7 and parts[5] == "tasks":
                task_id = urllib.parse.unquote(parts[6])
                if self.command == "GET":
                    self._send_json_response(
                        200, {"id": task_id, "title": "Ship Google wrapper", "status": "needsAction"}
                    )
                    return
                if self.command == "PATCH":
                    status = (body_json or {}).get("status", "completed")
                    self._send_json_response(200, {"id": task_id, "status": status})
                    return
                if self.command == "DELETE":
                    self._send_json_response(200, {"deleted": True, "id": task_id})
                    return
            elif len(parts) == 6 and parts[5] == "tasks":
                if self.command == "GET":
                    self._send_json_response(
                        200,
                        {
                            "items": [
                                {
                                    "id": "tsk_1",
                                    "title": "Ship Google wrapper",
                                    "status": "needsAction",
                                }
                            ]
                        },
                    )
                    return
                if self.command == "POST":
                    title = (body_json or {}).get("title", "Untitled Task")
                    notes = (body_json or {}).get("notes", "")
                    due = (body_json or {}).get("due", None)
                    resp_item = {"id": "tsk_created", "status": "needsAction", "title": title, "notes": notes}
                    if due:
                        resp_item["due"] = due
                    self._send_json_response(201, resp_item)
                    return

        # Drive endpoints
        if path == "/upload/drive/v3/files":
            name = (body_json or {}).get("name", "uploaded.pdf")
            mime = (body_json or {}).get("mimeType", "application/pdf")
            self._send_json_response(
                201,
                {
                    "id": "drv_uploaded_777",
                    "name": name,
                    "mimeType": mime,
                    "webViewLink": "https://drive.google.com/file/d/drv_uploaded_777/view",
                },
            )
            return
        if path.startswith("/drive/v3/files/") and len(path.split("/drive/v3/files/")) == 2:
            file_id = path.split("/drive/v3/files/")[-1]
            self._send_json_response(
                200,
                {
                    "id": file_id,
                    "name": "notes.md",
                    "mimeType": "text/markdown",
                    "webViewLink": f"https://drive.google.com/file/d/{file_id}/view",
                },
            )
            return
        if path == "/drive/v3/files":
            if self.command == "GET":
                self._send_json_response(
                    200,
                    {
                        "files": [
                            {
                                "id": "drv_1",
                                "name": "notes.md",
                                "mimeType": "text/markdown",
                                "webViewLink": "https://drive.google.com/file/d/drv_1/view",
                            }
                        ]
                    },
                )
                return
            if self.command == "POST":
                name = (body_json or {}).get("name", "New Folder")
                mime = (body_json or {}).get("mimeType", "application/vnd.google-apps.folder")
                self._send_json_response(
                    201,
                    {
                        "id": "drv_folder_888",
                        "name": name,
                        "mimeType": mime,
                    },
                )
                return

        # Fallback route
        self._send_json_response(200, {"ok": True, "path": path, "method": self.command, "echo": body_json})

    do_GET = _dispatch
    do_POST = _dispatch
    do_PATCH = _dispatch
    do_DELETE = _dispatch
    do_PUT = _dispatch


class MockWorkspaceServer(ThreadingHTTPServer):
    def __init__(self, server_address, request_handler_class):
        super().__init__(server_address, request_handler_class)
        self.lock = threading.Lock()
        self.recorded_requests = []
        self.fault_rules = []
        self.custom_routes = {}

    def get_requests(self):
        with self.lock:
            return list(self.recorded_requests)

    def clear_requests(self):
        with self.lock:
            self.recorded_requests.clear()

    def inject_fault(self, *, path_prefix=None, path=None, method=None, status=500, message="Simulated Error", remaining=None, response=None):
        with self.lock:
            rule = {
                "path_prefix": path_prefix,
                "path": path,
                "method": method,
                "status": status,
                "message": message,
                "remaining": remaining,
                "response": response,
            }
            self.fault_rules.append(rule)

    def clear_faults(self):
        with self.lock:
            self.fault_rules.clear()


class GoogleLoopbackE2ETest(unittest.TestCase):
    """End-to-End Loopback Test Suite verifying Google Workspace operations over HTTP."""

    def setUp(self):
        # 1. Spin up loopback mock HTTP server on 127.0.0.1:0 (ephemeral port)
        self.server = MockWorkspaceServer(("127.0.0.1", 0), MockWorkspaceHandler)
        self.server_port = self.server.server_address[1]
        self.loopback_url = f"http://127.0.0.1:{self.server_port}"
        self.server_thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.server_thread.start()

        # 2. Temporary filesystem sandbox
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = self.tempdir.name
        self.config_root = os.path.join(self.root, "config")
        self.log_file = os.path.join(self.root, "fake_gws.log")

        # 3. Create fake gws executable shim that forwards to loopback server
        self.fake_gws = os.path.join(self.root, "fake-gws")
        with open(self.fake_gws, "w", encoding="utf-8") as fh:
            fh.write(make_fake_gws_shim(self.loopback_url))
        os.chmod(self.fake_gws, 0o755)

        # 4. Create authorized user credentials file
        self.cred_file = os.path.join(self.root, "credentials.json")
        with open(self.cred_file, "w", encoding="utf-8") as fh:
            json.dump(
                {
                    "type": "authorized_user",
                    "client_id": "test-client.apps.googleusercontent.com",
                    "client_secret": "test-secret",
                    "refresh_token": "test-refresh",
                },
                fh,
            )
        os.chmod(self.cred_file, 0o600)
        os.chmod(GOOGLE_BSH, 0o755)

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.server_thread.join(timeout=2)
        self.tempdir.cleanup()

    def run_wrapper(self, args, env_overrides=None, stdin_text=None):
        env = dict(os.environ)
        for k in (
            "GWS_ACCOUNT",
            "GWS_CREDENTIALS_FILE",
            "GWS_SERVICE_ACCOUNT_KEY_FILE",
            "GOOGLE_APPLICATION_CREDENTIALS",
            "GOOGLE_WORKSPACE_CLI_TOKEN",
        ):
            env.pop(k, None)

        env["GWS_CONFIG_ROOT"] = self.config_root
        env["GWS_BIN"] = self.fake_gws
        env["GWS_LOOPBACK_URL"] = self.loopback_url
        env["FAKE_GWS_LOG"] = self.log_file
        env["GWS_ACCOUNT"] = "alice@example.com"
        env["GWS_CREDENTIALS_FILE"] = self.cred_file

        if env_overrides:
            for k, v in env_overrides.items():
                if v is None:
                    env.pop(k, None)
                else:
                    env[k] = v

        cmd = [BASHY_BIN, "--bashsharp", GOOGLE_BSH] + list(args)
        return subprocess.run(cmd, env=env, input=stdin_text, capture_output=True, text=True)

    # =========================================================================
    # 1. End-to-End HTTP Payload Delivery & Header Verification
    # =========================================================================
    def test_http_payload_delivery_and_headers(self):
        """Verify request method, path, headers, and body delivery over loopback HTTP."""
        res = self.run_wrapper(
            ["gmail-send", "to=bob@example.com", "subject=Loopback Check", "body=Verified body delivery"]
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertEqual(out["id"], "sent_999")

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 1)
        req = requests[0]

        # Verify HTTP wire transport
        self.assertEqual(req["method"], "POST")
        self.assertEqual(req["path"], "/gmail/v1/users/me/messages/send")
        self.assertIn("Authorization", req["headers"])
        self.assertEqual(req["headers"]["Authorization"], "Bearer mock-token-alice@example.com")
        self.assertEqual(req["headers"]["X-Gws-Account"], "alice@example.com")
        self.assertIn("application/json", req["headers"].get("Content-Type", ""))

        # Verify decoded payload
        body_json = req["body_json"]
        self.assertIn("raw", body_json)
        padded = body_json["raw"] + "=" * (-len(body_json["raw"]) % 4)
        raw_msg = base64.urlsafe_b64decode(padded).decode("utf-8")
        self.assertIn("To: bob@example.com", raw_msg)
        self.assertIn("Subject: Loopback Check", raw_msg)
        self.assertIn("Verified body delivery", raw_msg)

    # =========================================================================
    # 2. HTTP Status Code Handling
    # =========================================================================
    def test_status_code_400_bad_request_propagation(self):
        """Verify HTTP 400 Bad Request is properly caught and surfaced."""
        self.server.inject_fault(
            path_prefix="/gmail/v1/users/me/messages/send",
            status=400,
            message="Invalid recipient address",
        )
        res = self.run_wrapper(["gmail-send", "to=bad_addr", "subject=Hi", "body=Test"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Invalid recipient address", res.stderr)

    def test_status_code_403_forbidden_propagation(self):
        """Verify HTTP 403 Forbidden is surfaced with permission error."""
        self.server.inject_fault(
            path_prefix="/calendar/v3/calendars/primary/events",
            method="POST",
            status=403,
            message="Caller does not have permission to insert events",
        )
        res = self.run_wrapper(
            ["calendar-create", "summary=Blocked", "start=2026-11-10T10:00:00Z", "end=2026-11-10T10:30:00Z"]
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Caller does not have permission", res.stderr)

    def test_status_code_404_not_found_propagation(self):
        """Verify HTTP 404 Not Found is handled when fetching a missing item."""
        self.server.inject_fault(
            path="/tasks/v1/lists/@default/tasks/nonexistent_id",
            status=404,
            message="Task not found",
        )
        res = self.run_wrapper(["tasks-get", "nonexistent_id"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Task not found", res.stderr)

    def test_status_code_500_server_error_propagation(self):
        """Verify HTTP 500 Internal Server Error propagates failure."""
        self.server.inject_fault(
            path="/drive/v3/files",
            method="GET",
            status=500,
            message="Backend internal database error",
        )
        res = self.run_wrapper(["drive-list"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Backend internal database error", res.stderr)

    def test_status_code_503_service_unavailable_propagation(self):
        """Verify HTTP 503 Service Unavailable triggers failure."""
        self.server.inject_fault(
            path_prefix="/gmail/v1/users/me/messages",
            method="GET",
            status=503,
            message="Gmail service temporarily overloaded",
        )
        res = self.run_wrapper(["gmail-list"])
        self.assertNotEqual(res.returncode, 0)
        self.assertIn("Gmail service temporarily overloaded", res.stderr)

    # =========================================================================
    # 3. Partial-Mutation Handling in drive-share-mail
    # =========================================================================
    def test_drive_share_mail_partial_mutation_handling(self):
        """Verify partial-mutation handling when upload succeeds but mail fails."""
        # 1. Create valid local file to upload
        doc_path = os.path.join(self.root, "sprint_report.pdf")
        with open(doc_path, "wb") as fh:
            fh.write(b"%PDF-1.4 sample sprint report content")

        # 2. Inject fault specifically on Gmail send (HTTP 500)
        self.server.inject_fault(
            path_prefix="/gmail/v1/users/me/messages/send",
            status=500,
            message="Gmail daily sending quota exceeded",
        )

        res = self.run_wrapper(
            ["drive-share-mail", doc_path, "to=bob@example.com", "subject=Sprint Report"]
        )

        # 3. Must fail with structured PARTIAL_WORKFLOW_FAILURE
        self.assertNotEqual(res.returncode, 0)
        err = json.loads(res.stderr)["error"]
        self.assertEqual(err["code"], "PARTIAL_WORKFLOW_FAILURE")
        self.assertEqual(err["completed_step"], "drive-upload")
        self.assertEqual(err["failed_step"], "gmail-send")
        self.assertFalse(err["retry_safe"])
        self.assertEqual(err["partial_result"]["drive_file"]["id"], "drv_uploaded_777")

        # 4. Verify HTTP interactions: Drive upload was called EXACTLY ONCE; NO RETRY occurred
        requests = self.server.get_requests()
        upload_reqs = [r for r in requests if r["path"] == "/upload/drive/v3/files"]
        send_reqs = [r for r in requests if r["path"] == "/gmail/v1/users/me/messages/send"]
        self.assertEqual(len(upload_reqs), 1, "Drive upload must only be called once")
        self.assertEqual(len(send_reqs), 1, "Gmail send must be attempted once")

    def test_drive_share_mail_happy_path_delivers_file_link(self):
        """Verify successful drive-share-mail workflow delivers file link via email."""
        doc_path = os.path.join(self.root, "summary.md")
        with open(doc_path, "w", encoding="utf-8") as fh:
            fh.write("# Summary\nAll deliverables green.\n")

        res = self.run_wrapper(
            ["drive-share-mail", doc_path, "to=carol@example.com", "subject=Sprint Summary"]
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])
        self.assertEqual(out["workflow"], "drive-share-mail")
        self.assertEqual(out["drive_file"]["id"], "drv_uploaded_777")
        self.assertEqual(out["message"]["id"], "sent_999")

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 2)
        self.assertEqual(requests[0]["path"], "/upload/drive/v3/files")
        self.assertEqual(requests[1]["path"], "/gmail/v1/users/me/messages/send")

        # Verify email body payload contains the uploaded drive link
        mail_body_raw = requests[1]["body_json"]["raw"]
        padded = mail_body_raw + "=" * (-len(mail_body_raw) % 4)
        decoded_mail = base64.urlsafe_b64decode(padded).decode("utf-8")
        self.assertIn("https://drive.google.com/file/d/drv_uploaded_777/view", decoded_mail)

    # =========================================================================
    # 4. Mail-to-Task Linkage
    # =========================================================================
    def test_mail_to_task_linkage(self):
        """Verify mail-to-task workflow: Gmail fetch + Task insert with metadata link."""
        res = self.run_wrapper(
            ["mail-to-task", "msg_123", "due=2026-11-15T00:00:00Z", "notes=Prioritized follow-up"]
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])
        self.assertEqual(out["workflow"], "mail-to-task")
        self.assertEqual(out["source_message"]["subject"], "Q4 Architecture Review")
        self.assertEqual(out["source_message"]["from"], "bob@example.com")
        self.assertEqual(out["task"]["id"], "tsk_created")

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 2)

        # 1. Fetch message via GET
        get_req = requests[0]
        self.assertEqual(get_req["method"], "GET")
        self.assertEqual(get_req["path"], "/gmail/v1/users/me/messages/msg_123")
        self.assertEqual(get_req["query"].get("format"), "full")

        # 2. Insert linked task via POST
        task_req = requests[1]
        self.assertEqual(task_req["method"], "POST")
        self.assertEqual(task_req["path"], "/tasks/v1/lists/@default/tasks")
        task_payload = task_req["body_json"]

        # Verify task title and metadata linkage in notes
        self.assertEqual(task_payload["title"], "[Email] Q4 Architecture Review")
        self.assertIn("Prioritized follow-up", task_payload["notes"])
        self.assertIn("From: bob@example.com", task_payload["notes"])
        self.assertIn("Snippet: Please review the Q4 architecture doc by Friday.", task_payload["notes"])
        self.assertIn("gmail_message_id: msg_123", task_payload["notes"])
        self.assertIn("gmail_thread_id: thr_456", task_payload["notes"])
        self.assertEqual(task_payload["due"], "2026-11-15T00:00:00Z")

    # =========================================================================
    # 5. Agenda Aggregation Across Gmail, Calendar, Tasks, and Drive
    # =========================================================================
    def test_agenda_aggregation_across_gmail_calendar_tasks(self):
        """Verify agenda combines events, tasks, and unread messages via loopback HTTP."""
        res = self.run_wrapper(["agenda", "maxResults:=5", "q=is:unread"])
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])
        self.assertEqual(out["workflow"], "agenda")
        self.assertEqual(len(out["calendar_events"]), 1)
        self.assertEqual(out["calendar_events"][0]["summary"], "Team Standup")
        self.assertEqual(len(out["tasks"]), 1)
        self.assertEqual(out["tasks"][0]["title"], "Ship Google wrapper")
        self.assertEqual(len(out["messages"]), 1)
        self.assertEqual(out["messages"][0]["id"], "msg_123")

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 3)

        # 1. Calendar list
        cal_req = requests[0]
        self.assertEqual(cal_req["path"], "/calendar/v3/calendars/primary/events")
        self.assertEqual(cal_req["query"]["maxResults"], "5")
        self.assertEqual(cal_req["query"]["singleEvents"].lower(), "true")
        self.assertEqual(cal_req["query"]["orderBy"], "startTime")

        # 2. Tasks list
        tsk_req = requests[1]
        self.assertEqual(tsk_req["path"], "/tasks/v1/lists/@default/tasks")
        self.assertEqual(tsk_req["query"]["maxResults"], "5")
        self.assertEqual(tsk_req["query"]["showCompleted"].lower(), "false")

        # 3. Gmail list
        msg_req = requests[2]
        self.assertEqual(msg_req["path"], "/gmail/v1/users/me/messages")
        self.assertEqual(msg_req["query"]["maxResults"], "5")
        self.assertEqual(msg_req["query"]["q"], "is:unread")

    def test_agenda_aggregation_and_drive_search(self):
        """Verify full aggregation across Gmail, Calendar, Tasks, and Drive."""
        res_agenda = self.run_wrapper(["agenda", "maxResults:=3"])
        self.assertEqual(res_agenda.returncode, 0, res_agenda.stderr)
        agenda_data = json.loads(res_agenda.stdout)

        res_drive = self.run_wrapper(["drive-search", "notes"])
        self.assertEqual(res_drive.returncode, 0, res_drive.stderr)
        drive_data = json.loads(res_drive.stdout)

        # Verify all 4 domains returned structured data via HTTP loopback
        self.assertTrue(len(agenda_data["calendar_events"]) >= 1)
        self.assertTrue(len(agenda_data["tasks"]) >= 1)
        self.assertTrue(len(agenda_data["messages"]) >= 1)
        self.assertTrue(len(drive_data["files"]) >= 1)
        self.assertEqual(drive_data["files"][0]["name"], "notes.md")

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 4)
        paths = [r["path"] for r in requests]
        self.assertEqual(
            paths,
            [
                "/calendar/v3/calendars/primary/events",
                "/tasks/v1/lists/@default/tasks",
                "/gmail/v1/users/me/messages",
                "/drive/v3/files",
            ],
        )

    # =========================================================================
    # 6. Single-Service Verbs over HTTP Loopback
    # =========================================================================
    def test_calendar_crud_over_http(self):
        """Verify Calendar insert, patch, delete, and freebusy delivery."""
        # Insert
        res_ins = self.run_wrapper(
            [
                "calendar-create",
                "summary=Design Sync",
                "start=2026-11-10T10:00:00Z",
                "end=2026-11-10T11:00:00Z",
                'attendees:=["bob@example.com"]',
            ]
        )
        self.assertEqual(res_ins.returncode, 0, res_ins.stderr)

        # Patch
        res_patch = self.run_wrapper(["calendar-update", "evt_1", "summary=Renamed Sync"])
        self.assertEqual(res_patch.returncode, 0, res_patch.stderr)

        # Delete
        res_del = self.run_wrapper(["calendar-delete", "evt_1"])
        self.assertEqual(res_del.returncode, 0, res_del.stderr)

        # FreeBusy
        res_fb = self.run_wrapper(
            ["calendar-freebusy", "timeMin=2026-11-10T00:00:00Z", "timeMax=2026-11-11T00:00:00Z"]
        )
        self.assertEqual(res_fb.returncode, 0, res_fb.stderr)

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 4)
        self.assertEqual(requests[0]["method"], "POST")
        self.assertEqual(requests[0]["path"], "/calendar/v3/calendars/primary/events")
        self.assertEqual(requests[1]["method"], "PATCH")
        self.assertEqual(requests[1]["path"], "/calendar/v3/calendars/primary/events/evt_1")
        self.assertEqual(requests[2]["method"], "DELETE")
        self.assertEqual(requests[2]["path"], "/calendar/v3/calendars/primary/events/evt_1")
        self.assertEqual(requests[3]["method"], "POST")
        self.assertEqual(requests[3]["path"], "/calendar/v3/freeBusy")

    def test_tasks_crud_over_http(self):
        """Verify Tasks insert, complete, and delete delivery."""
        res_create = self.run_wrapper(["tasks-create", "title=Deliverables check", "due=2026-11-12T00:00:00Z"])
        self.assertEqual(res_create.returncode, 0, res_create.stderr)

        res_complete = self.run_wrapper(["tasks-complete", "tsk_1"])
        self.assertEqual(res_complete.returncode, 0, res_complete.stderr)

        res_del = self.run_wrapper(["tasks-delete", "tsk_1"])
        self.assertEqual(res_del.returncode, 0, res_del.stderr)

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 3)
        self.assertEqual(requests[0]["method"], "POST")
        self.assertEqual(requests[0]["path"], "/tasks/v1/lists/@default/tasks")
        self.assertEqual(requests[1]["method"], "PATCH")
        self.assertEqual(requests[1]["path"], "/tasks/v1/lists/@default/tasks/tsk_1")
        self.assertEqual(requests[1]["body_json"], {"status": "completed"})
        self.assertEqual(requests[2]["method"], "DELETE")
        self.assertEqual(requests[2]["path"], "/tasks/v1/lists/@default/tasks/tsk_1")

    def test_drive_upload_and_mkdir_over_http(self):
        """Verify Drive multipart upload and mkdir over loopback HTTP."""
        # 1. Drive mkdir
        res_mkdir = self.run_wrapper(["drive-mkdir", "Project Alpha"])
        self.assertEqual(res_mkdir.returncode, 0, res_mkdir.stderr)

        # 2. Drive upload
        test_file = os.path.join(self.root, "notes.txt")
        with open(test_file, "w", encoding="utf-8") as fh:
            fh.write("Sprint notes content.")

        res_upload = self.run_wrapper(["drive-upload", test_file, "name=Notes-Final.txt"])
        self.assertEqual(res_upload.returncode, 0, res_upload.stderr)

        requests = self.server.get_requests()
        self.assertEqual(len(requests), 2)
        self.assertEqual(requests[0]["method"], "POST")
        self.assertEqual(requests[0]["path"], "/drive/v3/files")
        self.assertEqual(requests[0]["body_json"]["mimeType"], "application/vnd.google-apps.folder")

        self.assertEqual(requests[1]["method"], "POST")
        self.assertEqual(requests[1]["path"], "/upload/drive/v3/files")
        self.assertEqual(requests[1]["query"].get("uploadType"), "multipart")
        self.assertEqual(requests[1]["body_json"]["name"], "Notes-Final.txt")

    # =========================================================================
    # 7. Fake GWS Shim Direct Invocation Tests
    # =========================================================================
    def test_fake_gws_shim_direct_invocation(self):
        """Verify the fake gws executable shim directly executes HTTP requests."""
        env = {
            "GWS_LOOPBACK_URL": self.loopback_url,
            "GWS_ACCOUNT": "direct@example.com",
            "PATH": os.environ.get("PATH", ""),
        }

        # Direct GET
        proc_get = subprocess.run(
            [self.fake_gws, "gmail", "users", "messages", "get", "--params", '{"userId":"me","id":"msg_99"}'],
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(proc_get.returncode, 0, proc_get.stderr)
        data = json.loads(proc_get.stdout)
        self.assertEqual(data["id"], "msg_99")

        # Direct POST
        proc_post = subprocess.run(
            [self.fake_gws, "tasks", "tasks", "insert", "--json", '{"title":"Direct Task"}'],
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(proc_post.returncode, 0, proc_post.stderr)
        task_data = json.loads(proc_post.stdout)
        self.assertEqual(task_data["title"], "Direct Task")


if __name__ == "__main__":
    unittest.main()
