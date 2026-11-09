#!/usr/bin/env python3
"""Hermetic offline unit tests for examples/google/gmail_threads.py.

No live Google credentials are required: every test points GWS_BIN at a fake
`gws` executable and GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE at a temporary
credentials file.
"""

import base64
import importlib.util
import json
import os
import stat
import subprocess
import sys
import tempfile
import textwrap
import unittest


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
MODULE_PATH = os.path.join(SCRIPT_DIR, "gmail_threads.py")

_spec = importlib.util.spec_from_file_location("gmail_threads", MODULE_PATH)
gmail_threads = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(gmail_threads)

ATTACHMENT_BYTES = b"Hello attachment bytes for hermetic test."
ATTACHMENT_B64 = base64.urlsafe_b64encode(ATTACHMENT_BYTES).decode("ascii").rstrip("=")

FAKE_GWS_SCRIPT = textwrap.dedent(
    """\
    #!/usr/bin/env python3
    import base64
    import json
    import os
    import sys

    ATTACHMENT_BYTES = %r

    log_path = os.environ.get("FAKE_GWS_LOG")
    if log_path:
        entry = {
            "argv": sys.argv[1:],
            "env": {
                "GWS_ACCOUNT": os.environ.get("GWS_ACCOUNT"),
                "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE": os.environ.get(
                    "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"
                ),
                "GOOGLE_APPLICATION_CREDENTIALS": os.environ.get(
                    "GOOGLE_APPLICATION_CREDENTIALS"
                ),
                "GOOGLE_WORKSPACE_CLI_TOKEN": os.environ.get(
                    "GOOGLE_WORKSPACE_CLI_TOKEN"
                ),
            },
        }
        with open(log_path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry) + "\\n")

    def params_of(args):
        if "--params" in args:
            return json.loads(args[args.index("--params") + 1])
        return {}

    args = sys.argv[1:]

    if args[:4] == ["gmail", "users", "threads", "get"]:
        params = params_of(args)
        print(json.dumps({
            "id": params.get("id", "thread_1"),
            "messages": [
                {
                    "id": "msg_aaa",
                    "threadId": params.get("id", "thread_1"),
                    "snippet": "First message snippet.",
                    "payload": {"headers": [
                        {"name": "From", "value": "alice@example.com"},
                        {"name": "Subject", "value": "Sprint Planning"},
                        {"name": "Date", "value": "Mon, 09 Nov 2026 10:00:00 +0000"},
                    ]},
                },
                {
                    "id": "msg_bbb",
                    "threadId": params.get("id", "thread_1"),
                    "snippet": "Second message snippet.",
                    "payload": {"headers": [
                        {"name": "From", "value": "Bob <bob@example.com>"},
                        {"name": "Subject", "value": "Re: Sprint Planning"},
                        {"name": "Date", "value": "Mon, 09 Nov 2026 11:00:00 +0000"},
                    ]},
                },
            ],
        }))
        sys.exit(0)

    if args[:4] == ["gmail", "users", "messages", "get"]:
        print(json.dumps({
            "id": "msg_aaa",
            "threadId": "thread_1",
            "snippet": "Please review the plan.",
            "payload": {"headers": [
                {"name": "Subject", "value": "Sprint Planning"},
                {"name": "From", "value": "Bob <bob@example.com>"},
                {"name": "Reply-To", "value": "carol@example.com"},
                {"name": "Message-ID", "value": "<orig-123@example.com>"},
                {"name": "References", "value": "<root-1@example.com>"},
                {"name": "Date", "value": "Mon, 09 Nov 2026 10:00:00 +0000"},
            ]},
        }))
        sys.exit(0)

    if args[:4] == ["gmail", "users", "messages", "send"]:
        body = json.loads(args[args.index("--json") + 1])
        print(json.dumps({
            "id": "sent_1",
            "threadId": body.get("threadId", "thread_1"),
            "labelIds": ["SENT"],
        }))
        sys.exit(0)

    if args[:5] == ["gmail", "users", "messages", "attachments", "get"]:
        params = params_of(args)
        if params.get("id") == "big_att":
            print(json.dumps({
                "data": base64.urlsafe_b64encode(b"tiny").decode("ascii"),
                "size": 26214401,
            }))
            sys.exit(0)
        print(json.dumps({
            "data": base64.urlsafe_b64encode(ATTACHMENT_BYTES).decode("ascii").rstrip("="),
            "size": len(ATTACHMENT_BYTES),
        }))
        sys.exit(0)

    print(json.dumps({"ok": True, "echo_argv": args}))
    """
) % (ATTACHMENT_BYTES,)


class GmailThreadsTest(unittest.TestCase):
    def setUp(self):
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = self.tempdir.name
        self.log_file = os.path.join(self.root, "gws_calls.jsonl")
        self.fake_gws = os.path.join(self.root, "fake-gws")
        with open(self.fake_gws, "w", encoding="utf-8") as fh:
            fh.write(FAKE_GWS_SCRIPT)
        os.chmod(self.fake_gws, 0o755)

        self.cred_file = os.path.join(self.root, "gws_creds.json")
        with open(self.cred_file, "w", encoding="utf-8") as fh:
            json.dump({"type": "authorized_user", "refresh_token": "fake"}, fh)
        os.chmod(self.cred_file, 0o600)

        self._saved_env = dict(os.environ)

    def tearDown(self):
        os.environ.clear()
        os.environ.update(self._saved_env)
        self.tempdir.cleanup()

    # --- helpers ---
    def base_env(self, account="alice@example.com"):
        for key in (
            "GWS_ACCOUNT",
            "GWS_BIN",
            "GWS_CREDENTIALS_FILE",
            "GWS_SERVICE_ACCOUNT_KEY_FILE",
            "GWS_CONFIG_ROOT",
            "GOOGLE_APPLICATION_CREDENTIALS",
            "GOOGLE_WORKSPACE_CLI_TOKEN",
            "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE",
        ):
            os.environ.pop(key, None)
        os.environ["GWS_ACCOUNT"] = account
        os.environ["GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"] = self.cred_file
        os.environ["GWS_BIN"] = self.fake_gws
        os.environ["FAKE_GWS_LOG"] = self.log_file

    def run_cli(self, args, extra_env=None):
        env = dict(os.environ)
        for key in (
            "GWS_ACCOUNT",
            "GWS_BIN",
            "GWS_CREDENTIALS_FILE",
            "GWS_SERVICE_ACCOUNT_KEY_FILE",
            "GWS_CONFIG_ROOT",
            "GOOGLE_APPLICATION_CREDENTIALS",
            "GOOGLE_WORKSPACE_CLI_TOKEN",
            "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE",
            "FAKE_GWS_LOG",
        ):
            env.pop(key, None)
        env["GWS_ACCOUNT"] = "alice@example.com"
        env["GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"] = self.cred_file
        env["GWS_BIN"] = self.fake_gws
        env["FAKE_GWS_LOG"] = self.log_file
        if extra_env:
            for key, value in extra_env.items():
                if value is None:
                    env.pop(key, None)
                else:
                    env[key] = value
        return subprocess.run(
            [sys.executable, MODULE_PATH] + list(args),
            env=env, capture_output=True, text=True,
        )

    def read_calls(self):
        if not os.path.exists(self.log_file):
            return []
        with open(self.log_file, "r", encoding="utf-8") as fh:
            return [json.loads(line) for line in fh if line.strip()]

    def decode_raw(self, raw_b64):
        padded = raw_b64 + "=" * (-len(raw_b64) % 4)
        return base64.urlsafe_b64decode(padded).decode("utf-8")

    # --- 1. gmail-thread-get ---
    def test_thread_get_fetches_and_summarizes_messages(self):
        res = self.run_cli(["gmail-thread-get", "thread_1"])
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertEqual(out["id"], "thread_1")
        self.assertEqual(out["messageCount"], 2)
        first, second = out["messages"]
        self.assertEqual(first["id"], "msg_aaa")
        self.assertEqual(first["from"], "alice@example.com")
        self.assertEqual(first["subject"], "Sprint Planning")
        self.assertEqual(first["date"], "Mon, 09 Nov 2026 10:00:00 +0000")
        self.assertEqual(first["snippet"], "First message snippet.")
        self.assertEqual(second["from"], "Bob <bob@example.com>")

        calls = self.read_calls()
        self.assertEqual(len(calls), 1)
        argv = calls[0]["argv"]
        self.assertEqual(argv[:4], ["gmail", "users", "threads", "get"])
        self.assertIn("--format=json", argv)
        params = json.loads(argv[argv.index("--params") + 1])
        self.assertEqual(
            params, {"userId": "me", "id": "thread_1", "format": "full"}
        )

    def test_thread_get_custom_format_passes_through(self):
        res = self.run_cli(["gmail-thread-get", "thread_1", "format=minimal"])
        self.assertEqual(res.returncode, 0, res.stderr)
        params = json.loads(
            self.read_calls()[0]["argv"][
                self.read_calls()[0]["argv"].index("--params") + 1
            ]
        )
        self.assertEqual(params["format"], "minimal")

    def test_thread_get_requires_thread_id(self):
        res = self.run_cli(["gmail-thread-get"])
        self.assertEqual(res.returncode, 2)
        self.assertEqual(json.loads(res.stderr)["error"]["code"], "USAGE")
        self.assertEqual(self.read_calls(), [])

    def test_thread_get_callable_and_alias(self):
        self.base_env()
        summary = gmail_threads.gmail_thread_get("thread_1")
        self.assertEqual(summary["id"], "thread_1")
        self.assertEqual(summary["messageCount"], 2)
        self.assertEqual(summary["messages"][0]["subject"], "Sprint Planning")
        aliased = gmail_threads.get_thread("thread_1")
        self.assertEqual(aliased["id"], "thread_1")

    # --- 2. gmail-reply ---
    def test_reply_sends_rfc2822_with_threading_headers(self):
        res = self.run_cli(["gmail-reply", "msg_aaa", "body=On it, reviewing now."])
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])

        calls = self.read_calls()
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[0]["argv"][:4], ["gmail", "users", "messages", "get"])
        send_argv = calls[1]["argv"]
        self.assertEqual(send_argv[:4], ["gmail", "users", "messages", "send"])
        self.assertIn("--format=json", send_argv)
        send_params = json.loads(send_argv[send_argv.index("--params") + 1])
        self.assertEqual(send_params, {"userId": "me"})
        send_body = json.loads(send_argv[send_argv.index("--json") + 1])
        self.assertEqual(send_body["threadId"], "thread_1")

        decoded = self.decode_raw(send_body["raw"])
        self.assertIn("To: carol@example.com", decoded)
        self.assertIn("Subject: Re: Sprint Planning", decoded)
        self.assertIn("In-Reply-To: <orig-123@example.com>", decoded)
        self.assertIn(
            "References: <root-1@example.com> <orig-123@example.com>", decoded
        )
        self.assertIn("On it, reviewing now.", decoded)

    def test_reply_with_cc_header(self):
        res = self.run_cli(
            ["gmail-reply", "msg_aaa", "body=Looping you in.", "cc=dave@example.com"]
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        send_body = json.loads(
            self.read_calls()[1]["argv"][self.read_calls()[1]["argv"].index("--json") + 1]
        )
        self.assertIn("Cc: dave@example.com", self.decode_raw(send_body["raw"]))

    def test_reply_dry_run_does_not_send(self):
        res = self.run_cli(
            ["gmail-reply", "msg_aaa", "body=Draft only.", "--dry-run"]
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["dry_run"])
        self.assertEqual(out["to"], "carol@example.com")
        self.assertEqual(out["subject"], "Re: Sprint Planning")
        decoded = self.decode_raw(out["raw"])
        self.assertIn("In-Reply-To: <orig-123@example.com>", decoded)
        calls = self.read_calls()
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0]["argv"][:4], ["gmail", "users", "messages", "get"])

    def test_reply_callable_and_alias(self):
        self.base_env()
        result = gmail_threads.gmail_reply("msg_aaa", "Callable body.")
        self.assertTrue(result["ok"])
        self.assertEqual(result["subject"], "Re: Sprint Planning")
        self.assertEqual(result["id"], "sent_1")
        preview = gmail_threads.reply_to_message("msg_aaa", "Preview.", dry_run=True)
        self.assertTrue(preview["dry_run"])

    def test_reply_requires_body(self):
        res = self.run_cli(["gmail-reply", "msg_aaa"])
        self.assertEqual(res.returncode, 2)
        self.assertEqual(json.loads(res.stderr)["error"]["code"], "USAGE")

    # --- 3. gmail-attachment-get ---
    def test_attachment_get_writes_file(self):
        dest = os.path.join(self.root, "download.bin")
        res = self.run_cli(["gmail-attachment-get", "msg_aaa", "att_1", "output=" + dest])
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertEqual(out["attachmentId"], "att_1")
        self.assertEqual(out["messageId"], "msg_aaa")
        self.assertEqual(out["bytesWritten"], len(ATTACHMENT_BYTES))
        with open(dest, "rb") as fh:
            self.assertEqual(fh.read(), ATTACHMENT_BYTES)

        calls = self.read_calls()
        self.assertEqual(len(calls), 1)
        argv = calls[0]["argv"]
        self.assertEqual(
            argv[:5], ["gmail", "users", "messages", "attachments", "get"]
        )
        self.assertIn("--format=json", argv)
        params = json.loads(argv[argv.index("--params") + 1])
        self.assertEqual(
            params, {"userId": "me", "messageId": "msg_aaa", "id": "att_1"}
        )

    def test_attachment_get_callable_returns_decoded_bytes(self):
        self.base_env()
        result = gmail_threads.gmail_attachment_get("msg_aaa", "att_1")
        self.assertEqual(result["data"], ATTACHMENT_BYTES)
        self.assertEqual(result["size"], len(ATTACHMENT_BYTES))
        aliased = gmail_threads.get_attachment("msg_aaa", "att_1")
        self.assertEqual(aliased["data"], ATTACHMENT_BYTES)

    def test_attachment_size_cap_refused(self):
        self.assertEqual(gmail_threads.MAX_ATTACHMENT_BYTES, 26214400)
        dest = os.path.join(self.root, "too_big.bin")
        res = self.run_cli(
            ["gmail-attachment-get", "msg_aaa", "big_att", "output=" + dest]
        )
        self.assertEqual(res.returncode, 2)
        self.assertEqual(
            json.loads(res.stderr)["error"]["code"], "ATTACHMENT_TOO_LARGE"
        )
        self.assertFalse(os.path.exists(dest))

    def test_attachment_requires_ids(self):
        res = self.run_cli(["gmail-attachment-get", "msg_aaa"])
        self.assertEqual(res.returncode, 2)
        self.assertEqual(json.loads(res.stderr)["error"]["code"], "USAGE")

    # --- 4. preflight & hygiene ---
    def test_missing_account_refused(self):
        res = self.run_cli(["gmail-thread-get", "thread_1"], extra_env={"GWS_ACCOUNT": None})
        self.assertEqual(res.returncode, 2)
        self.assertEqual(json.loads(res.stderr)["error"]["code"], "MISSING_ACCOUNT")
        self.assertEqual(self.read_calls(), [])

    def test_missing_credentials_file_refused(self):
        res = self.run_cli(
            ["gmail-thread-get", "thread_1"],
            extra_env={"GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE": "/nonexistent/creds.json"},
        )
        self.assertEqual(res.returncode, 2)
        self.assertEqual(
            json.loads(res.stderr)["error"]["code"], "MISSING_CREDENTIALS"
        )
        self.assertEqual(self.read_calls(), [])

    def test_missing_gws_binary_refused(self):
        res = self.run_cli(
            ["gmail-thread-get", "thread_1"],
            extra_env={"GWS_BIN": "/nonexistent/gws", "PATH": "/usr/bin:/bin"},
        )
        self.assertNotEqual(res.returncode, 0)
        self.assertEqual(json.loads(res.stderr)["error"]["code"], "GWS_NOT_FOUND")

    def test_secret_in_argv_refused(self):
        for bad_arg in (
            "password=hunter2",
            "access_token=ya29.secret",
            "client_secret=mysecret",
            "--api-key=secret",
        ):
            res = self.run_cli(["gmail-reply", "msg_aaa", "body=Hi", bad_arg])
            self.assertEqual(res.returncode, 2, "Expected refusal for %s" % bad_arg)
            self.assertEqual(
                json.loads(res.stderr)["error"]["code"], "SECRET_VIA_ARGV"
            )
        self.assertEqual(self.read_calls(), [])

    def test_body_value_containing_secret_text_is_allowed(self):
        res = self.run_cli(
            ["gmail-reply", "msg_aaa", "body=the password=hunter2 note", "--dry-run"]
        )
        self.assertEqual(res.returncode, 0, res.stderr)

    def test_ambient_credentials_scrubbed(self):
        res = self.run_cli(
            ["gmail-thread-get", "thread_1"],
            extra_env={
                "GOOGLE_APPLICATION_CREDENTIALS": "/tmp/ambient-adc.json",
                "GOOGLE_WORKSPACE_CLI_TOKEN": "ambient-token",
            },
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        call = self.read_calls()[0]
        self.assertIsNone(call["env"]["GOOGLE_APPLICATION_CREDENTIALS"])
        self.assertIsNone(call["env"]["GOOGLE_WORKSPACE_CLI_TOKEN"])
        self.assertEqual(
            call["env"]["GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"], self.cred_file
        )

    def test_unknown_verb_rejected(self):
        res = self.run_cli(["gmail-nope", "x"])
        self.assertNotEqual(res.returncode, 0)
        self.assertEqual(json.loads(res.stderr)["error"]["code"], "USAGE")

    def test_preflight_callable_raises(self):
        self.base_env()
        os.environ.pop("GWS_ACCOUNT", None)
        with self.assertRaises(gmail_threads.GmailThreadsError) as ctx:
            gmail_threads.gmail_thread_get("thread_1")
        self.assertEqual(ctx.exception.code, "MISSING_ACCOUNT")


if __name__ == "__main__":
    unittest.main()
