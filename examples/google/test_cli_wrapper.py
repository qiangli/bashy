#!/usr/bin/env python3
"""Hermetic offline unit tests for bashy/examples/google/google.bsh."""

import base64
import json
import os
import stat
import subprocess
import tempfile
import textwrap
import unittest


SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
GOOGLE_BSH = os.path.join(SCRIPT_DIR, "google.bsh")
REGISTER_SH = os.path.join(SCRIPT_DIR, "register.sh")
REPO_ROOT = os.path.abspath(os.path.join(SCRIPT_DIR, "..", ".."))
BASHY_BIN = os.path.join(REPO_ROOT, "bin", "bashy")


FAKE_GWS_SCRIPT = textwrap.dedent(
    """\
    #!/usr/bin/env python3
    import json
    import os
    import sys

    log_path = os.environ.get("FAKE_GWS_LOG")
    if log_path:
        entry = {
            "argv": sys.argv[1:],
            "env": {
                "GWS_ACCOUNT": os.environ.get("GWS_ACCOUNT"),
                "GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE": os.environ.get("GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"),
                "GOOGLE_WORKSPACE_CLI_CONFIG_DIR": os.environ.get("GOOGLE_WORKSPACE_CLI_CONFIG_DIR"),
                "GOOGLE_APPLICATION_CREDENTIALS": os.environ.get("GOOGLE_APPLICATION_CREDENTIALS"),
                "GOOGLE_WORKSPACE_CLI_TOKEN": os.environ.get("GOOGLE_WORKSPACE_CLI_TOKEN"),
            },
        }
        with open(log_path, "a", encoding="utf-8") as fh:
            fh.write(json.dumps(entry) + "\\n")

    args = sys.argv[1:]

    if args[:2] == ["auth", "login"]:
        if os.environ.get("FAKE_GWS_FAIL_LOGIN") == "1":
            sys.stderr.write("login failed\\n")
            sys.exit(1)
        print(json.dumps({"status": "logged_in"}))
        sys.exit(0)

    if args[:2] == ["auth", "setup"]:
        if os.environ.get("FAKE_GWS_FAIL_SETUP") == "1":
            sys.stderr.write("setup failed\\n")
            sys.exit(1)
        print(json.dumps({"status": "setup_complete"}))
        sys.exit(0)

    if args[:2] == ["auth", "export"]:
        print(json.dumps({
            "type": "authorized_user",
            "client_id": "fake-client-id.apps.googleusercontent.com",
            "client_secret": "fake-exported-secret",
            "refresh_token": "fake-exported-refresh-token",
        }))
        sys.exit(0)

    if args[:4] == ["gmail", "users", "messages", "get"]:
        print(json.dumps({
            "id": "msg_123",
            "threadId": "thr_456",
            "snippet": "Please review the Q4 architecture doc by Friday.",
            "payload": {
                "headers": [
                    {"name": "Subject", "value": "Q4 Architecture Review"},
                    {"name": "From", "value": "bob@example.com"},
                ]
            },
        }))
        sys.exit(0)

    if args[:4] == ["gmail", "users", "messages", "list"]:
        print(json.dumps({
            "messages": [{"id": "msg_123", "threadId": "thr_456"}],
            "resultSizeEstimate": 1,
        }))
        sys.exit(0)

    if args[:4] == ["gmail", "users", "messages", "send"]:
        if os.environ.get("FAKE_GWS_FAIL_GMAIL_SEND") == "1":
            sys.stderr.write("gmail send quota exceeded\\n")
            sys.exit(1)
        print(json.dumps({"id": "sent_999", "threadId": "thr_999", "labelIds": ["SENT"]}))
        sys.exit(0)

    if args[:4] == ["gmail", "users", "drafts", "create"]:
        print(json.dumps({"id": "draft_111", "message": {"id": "msg_draft_111"}}))
        sys.exit(0)

    if args[:3] == ["calendar", "events", "list"]:
        print(json.dumps({
            "items": [{"id": "evt_1", "summary": "Team Standup"}],
        }))
        sys.exit(0)

    if args[:3] == ["calendar", "events", "insert"]:
        print(json.dumps({"id": "evt_created", "status": "confirmed"}))
        sys.exit(0)

    if args[:3] == ["calendar", "events", "patch"]:
        print(json.dumps({"id": "evt_patched", "status": "confirmed"}))
        sys.exit(0)

    if args[:3] == ["calendar", "events", "delete"]:
        print(json.dumps({"deleted": True}))
        sys.exit(0)

    if args[:3] == ["calendar", "freebusy", "query"]:
        print(json.dumps({"calendars": {"primary": {"busy": []}}}))
        sys.exit(0)

    if args[:3] == ["tasks", "tasks", "list"]:
        print(json.dumps({
            "items": [{"id": "tsk_1", "title": "Ship Google wrapper", "status": "needsAction"}],
        }))
        sys.exit(0)

    if args[:3] == ["tasks", "tasks", "insert"]:
        print(json.dumps({"id": "tsk_created", "status": "needsAction"}))
        sys.exit(0)

    if args[:3] == ["tasks", "tasks", "patch"]:
        print(json.dumps({"id": "tsk_patched", "status": "completed"}))
        sys.exit(0)

    if args[:3] == ["drive", "files", "list"]:
        print(json.dumps({
            "files": [{"id": "drv_1", "name": "notes.md", "mimeType": "text/markdown"}],
        }))
        sys.exit(0)

    if args[:3] == ["drive", "files", "create"]:
        print(json.dumps({
            "id": "drv_uploaded_777",
            "name": "uploaded.pdf",
            "webViewLink": "https://drive.google.com/file/d/drv_uploaded_777/view",
        }))
        sys.exit(0)

    print(json.dumps({"ok": True, "echo_argv": args}))
    """
)


class GoogleWrapperTest(unittest.TestCase):
    def setUp(self):
        os.chmod(GOOGLE_BSH, 0o755)
        os.chmod(REGISTER_SH, 0o755)
        self.tempdir = tempfile.TemporaryDirectory()
        self.root = self.tempdir.name
        self.config_root = os.path.join(self.root, "config")
        self.log_file = os.path.join(self.root, "gws_calls.jsonl")
        self.fake_gws = os.path.join(self.root, "fake-gws")
        with open(self.fake_gws, "w", encoding="utf-8") as fh:
            fh.write(FAKE_GWS_SCRIPT)
        os.chmod(self.fake_gws, 0o755)

        self.cred_file = os.path.join(self.root, "authorized_user.json")
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

    def tearDown(self):
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
        env["FAKE_GWS_LOG"] = self.log_file
        if os.path.exists(BASHY_BIN):
            env["PATH"] = os.path.dirname(BASHY_BIN) + os.pathsep + env.get("PATH", "")
        if env_overrides:
            for k, v in env_overrides.items():
                if v is None:
                    env.pop(k, None)
                else:
                    env[k] = v
        cmd = [BASHY_BIN, "--bashsharp", GOOGLE_BSH] + list(args)
        return subprocess.run(cmd, env=env, input=stdin_text, capture_output=True, text=True)

    def read_calls(self):
        if not os.path.exists(self.log_file):
            return []
        with open(self.log_file, "r", encoding="utf-8") as fh:
            return [json.loads(line) for line in fh if line.strip()]

    def auth_env(self, account="alice@example.com"):
        return {
            "GWS_ACCOUNT": account,
            "GWS_CREDENTIALS_FILE": self.cred_file,
        }

    # --- 1. Discovery & Contract ---
    def test_help_and_skill_work_without_credentials(self):
        for flag in ("--help", "--skill", "help"):
            res = self.run_wrapper([flag], env_overrides={"GWS_BIN": "/nonexistent/gws"})
            self.assertEqual(res.returncode, 0, res.stderr)
            self.assertIn("name: google", res.stdout)
            self.assertIn("Bring-Your-Own-Project (BYOP)", res.stdout)
        self.assertEqual(len(self.read_calls()), 0)

    def test_contract_emits_structured_metadata(self):
        res = self.run_wrapper(["--contract"], env_overrides={"GWS_BIN": "/nonexistent/gws"})
        self.assertEqual(res.returncode, 0, res.stderr)
        data = json.loads(res.stdout)
        self.assertEqual(data["schema"], "bashy-command-contract-v1")
        self.assertEqual(data["name"], "google")
        self.assertEqual(data["auth"]["ambient_fallback"], False)
        self.assertIn("mail-to-task", data["verbs"]["workflows"])
        self.assertIn("auth-guide", data["verbs"]["onboarding"])

    def test_register_script_installs_custom_command_and_skill(self):
        cmds_dir = os.path.join(self.root, "commands")
        skills_dir = os.path.join(self.root, "skills")
        os.makedirs(cmds_dir, exist_ok=True)
        os.makedirs(skills_dir, exist_ok=True)
        env = dict(os.environ)
        env["BASHY_COMMANDS_DIR"] = cmds_dir
        env["BASHY_SKILLS_DIR"] = skills_dir
        env["BASHY_BIN"] = BASHY_BIN
        env["PATH"] = os.path.dirname(BASHY_BIN) + os.pathsep + env.get("PATH", "")

        reg = subprocess.run([REGISTER_SH], env=env, capture_output=True, text=True)
        self.assertEqual(reg.returncode, 0, reg.stderr)

        # Invoke registered custom command through `bashy google auth-guide`
        inv = subprocess.run(
            [BASHY_BIN, "google", "auth-guide", "account=alice@example.com"],
            env=env,
            capture_output=True,
            text=True,
        )
        self.assertEqual(inv.returncode, 0, inv.stderr)
        guide = json.loads(inv.stdout)
        self.assertEqual(guide["mode"], "byop-onboarding-guide")

    # --- 2. Self-Service BYOP Onboarding ---
    def test_auth_guide_emits_console_links_and_byop_rationale(self):
        res = self.run_wrapper(["auth-guide", "account=alice@example.com", "project=alice-gws-proj"])
        self.assertEqual(res.returncode, 0, res.stderr)
        guide = json.loads(res.stdout)
        self.assertEqual(guide["mode"], "byop-onboarding-guide")
        self.assertEqual(guide["account"], "alice@example.com")
        self.assertEqual(guide["project"], "alice-gws-proj")
        self.assertIn("gmail.googleapis.com", guide["required_apis"])
        self.assertIn("alice-gws-proj", guide["console_links"]["2_enable_apis"])
        self.assertEqual(len(self.read_calls()), 0)

    def test_auth_import_client_secret_and_per_account_resolution(self):
        client_secret_path = os.path.join(self.root, "client_secret.json")
        with open(client_secret_path, "w", encoding="utf-8") as fh:
            json.dump(
                {
                    "installed": {
                        "client_id": "123.apps.googleusercontent.com",
                        "client_secret": "top-secret-client",
                    }
                },
                fh,
            )

        res = self.run_wrapper(["auth-import", client_secret_path, "account=alice@example.com"])
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])
        self.assertEqual(out["account"], "alice@example.com")
        self.assertNotIn("top-secret-client", res.stdout)

        creds_path = out["credentials_file"]
        self.assertTrue(os.path.isfile(creds_path))
        mode_bits = stat.S_IMODE(os.stat(creds_path).st_mode)
        self.assertEqual(mode_bits, 0o600)

        # Verify auth-status now reports ready using per-account store (without GWS_CREDENTIALS_FILE set)
        status_res = self.run_wrapper(["auth-status", "account=alice@example.com"])
        self.assertEqual(status_res.returncode, 0, status_res.stderr)
        status_data = json.loads(status_res.stdout)
        self.assertTrue(status_data["ready"])
        self.assertEqual(status_data["source"], "per_account_store")
        self.assertTrue(status_data["mode_ok"])

        # Verify a service call succeeds using the per-account store with only GWS_ACCOUNT set
        list_res = self.run_wrapper(["gmail-list"], env_overrides={"GWS_ACCOUNT": "alice@example.com"})
        self.assertEqual(list_res.returncode, 0, list_res.stderr)

    def test_auth_import_rejects_invalid_json_structure(self):
        bad_path = os.path.join(self.root, "bad.json")
        with open(bad_path, "w", encoding="utf-8") as fh:
            json.dump({"unexpected": "object"}, fh)

        res = self.run_wrapper(["auth-import", bad_path, "account=alice@example.com"])
        self.assertEqual(res.returncode, 2)
        err = json.loads(res.stderr)
        self.assertEqual(err["error"]["code"], "INVALID_CREDENTIAL_JSON")

    # --- 3. Credential & Secret Hygiene Guards ---
    def test_missing_account_refused(self):
        res = self.run_wrapper(["gmail-list"], env_overrides={"GWS_CREDENTIALS_FILE": self.cred_file})
        self.assertEqual(res.returncode, 2)
        err = json.loads(res.stderr)
        self.assertEqual(err["error"]["code"], "MISSING_ACCOUNT")
        self.assertEqual(len(self.read_calls()), 0)

    def test_missing_credentials_refused_with_no_ambient_fallback(self):
        res = self.run_wrapper(
            ["gmail-list"],
            env_overrides={
                "GWS_ACCOUNT": "unconfigured@example.com",
                "GOOGLE_APPLICATION_CREDENTIALS": self.cred_file,
            },
        )
        self.assertEqual(res.returncode, 2)
        err = json.loads(res.stderr)
        self.assertEqual(err["error"]["code"], "MISSING_CREDENTIALS")
        self.assertEqual(len(self.read_calls()), 0)

    def test_ambiguous_credentials_refused(self):
        res = self.run_wrapper(
            ["gmail-list"],
            env_overrides={
                "GWS_ACCOUNT": "alice@example.com",
                "GWS_CREDENTIALS_FILE": self.cred_file,
                "GWS_SERVICE_ACCOUNT_KEY_FILE": self.cred_file,
            },
        )
        self.assertEqual(res.returncode, 2)
        err = json.loads(res.stderr)
        self.assertEqual(err["error"]["code"], "AMBIGUOUS_CREDENTIALS")
        self.assertEqual(len(self.read_calls()), 0)

    def test_secret_in_argv_refused_while_pagetoken_allowed(self):
        for bad_arg in (
            "access_token=ya29.secret",
            "client_secret=mysecret",
            "password=hunter2",
            'meta:={"refresh_token":"1//secret"}',
            "--api-key=secret",
        ):
            res = self.run_wrapper(["gmail-list", bad_arg], env_overrides=self.auth_env())
            self.assertEqual(res.returncode, 2, f"Expected refusal for {bad_arg}")
            err = json.loads(res.stderr)
            self.assertEqual(err["error"]["code"], "SECRET_VIA_ARGV")
        self.assertEqual(len(self.read_calls()), 0)

        # Standard pagination token key pageToken must NOT be refused as a secret
        ok_res = self.run_wrapper(["gmail-list", "pageToken=next_page_123"], env_overrides=self.auth_env())
        self.assertEqual(ok_res.returncode, 0, ok_res.stderr)
        calls = self.read_calls()
        self.assertEqual(len(calls), 1)
        params = json.loads(calls[0]["argv"][calls[0]["argv"].index("--params") + 1])
        self.assertEqual(params["pageToken"], "next_page_123")

    def test_ambient_credentials_scrubbed_on_gws_invocation(self):
        env = self.auth_env()
        env["GOOGLE_APPLICATION_CREDENTIALS"] = "/tmp/ambient-adc.json"
        env["GOOGLE_WORKSPACE_CLI_TOKEN"] = "ambient-token"
        res = self.run_wrapper(["gmail-list"], env_overrides=env)
        self.assertEqual(res.returncode, 0, res.stderr)
        call = self.read_calls()[0]
        self.assertIsNone(call["env"]["GOOGLE_APPLICATION_CREDENTIALS"])
        self.assertIsNone(call["env"]["GOOGLE_WORKSPACE_CLI_TOKEN"])
        self.assertEqual(call["env"]["GOOGLE_WORKSPACE_CLI_CREDENTIALS_FILE"], self.cred_file)

    # --- 4. Single-Service Verbs (Gmail, Calendar, Tasks, Drive) ---
    def test_gmail_send_encodes_rfc2822_message(self):
        res = self.run_wrapper(
            ["gmail-send", "to=bob@example.com", "subject=Hello Bob", "body=Test body text"],
            env_overrides=self.auth_env(),
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        call = self.read_calls()[0]
        self.assertEqual(call["argv"][:4], ["gmail", "users", "messages", "send"])
        body_json = json.loads(call["argv"][call["argv"].index("--json") + 1])
        padded = body_json["raw"] + "=" * (-len(body_json["raw"]) % 4)
        decoded = base64.urlsafe_b64decode(padded).decode("utf-8")
        self.assertIn("To: bob@example.com", decoded)
        self.assertIn("Subject: Hello Bob", decoded)
        self.assertIn("Test body text", decoded)

    def test_calendar_create_shapes_times_and_attendees(self):
        res = self.run_wrapper(
            [
                "calendar-create",
                "summary=Architecture Sync",
                "start=2026-11-10T10:00:00-08:00",
                "end=2026-11-10T10:30:00-08:00",
                'attendees:=["bob@example.com","carol@example.com"]',
            ],
            env_overrides=self.auth_env(),
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        call = self.read_calls()[0]
        self.assertEqual(call["argv"][:3], ["calendar", "events", "insert"])
        body_json = json.loads(call["argv"][call["argv"].index("--json") + 1])
        self.assertEqual(body_json["summary"], "Architecture Sync")
        self.assertEqual(body_json["start"], {"dateTime": "2026-11-10T10:00:00-08:00"})
        self.assertEqual(
            body_json["attendees"],
            [{"email": "bob@example.com"}, {"email": "carol@example.com"}],
        )

    def test_tasks_create_and_complete(self):
        res1 = self.run_wrapper(
            ["tasks-create", "title=Write unit tests", "due=2026-11-12T00:00:00Z"],
            env_overrides=self.auth_env(),
        )
        self.assertEqual(res1.returncode, 0, res1.stderr)
        res2 = self.run_wrapper(["tasks-complete", "tsk_123"], env_overrides=self.auth_env())
        self.assertEqual(res2.returncode, 0, res2.stderr)

        calls = self.read_calls()
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[0]["argv"][:3], ["tasks", "tasks", "insert"])
        self.assertEqual(calls[1]["argv"][:3], ["tasks", "tasks", "patch"])
        patch_body = json.loads(calls[1]["argv"][calls[1]["argv"].index("--json") + 1])
        self.assertEqual(patch_body, {"status": "completed"})

    def test_drive_upload_validates_bounds_and_media_type(self):
        # 1. Unsupported extension/media type refused before spawning gws
        exe_path = os.path.join(self.root, "payload.bin")
        with open(exe_path, "wb") as fh:
            fh.write(b"binary data")
        res_bad_mime = self.run_wrapper(["drive-upload", exe_path], env_overrides=self.auth_env())
        self.assertEqual(res_bad_mime.returncode, 2)
        self.assertEqual(json.loads(res_bad_mime.stderr)["error"]["code"], "UNSUPPORTED_MEDIA_TYPE")

        # 2. Empty file refused
        empty_pdf = os.path.join(self.root, "empty.pdf")
        open(empty_pdf, "wb").close()
        res_empty = self.run_wrapper(["drive-upload", empty_pdf], env_overrides=self.auth_env())
        self.assertEqual(res_empty.returncode, 2)
        self.assertEqual(json.loads(res_empty.stderr)["error"]["code"], "UPLOAD_EMPTY_FILE")
        self.assertEqual(len(self.read_calls()), 0)

        # 3. File over 25 MiB refused with UPLOAD_TOO_LARGE
        oversized_pdf = os.path.join(self.root, "huge.pdf")
        with open(oversized_pdf, "wb") as fh:
            fh.seek(25 * 1024 * 1024)
            fh.write(b"x")
        res_huge = self.run_wrapper(["drive-upload", oversized_pdf], env_overrides=self.auth_env())
        self.assertEqual(res_huge.returncode, 2)
        self.assertEqual(json.loads(res_huge.stderr)["error"]["code"], "UPLOAD_TOO_LARGE")
        self.assertEqual(len(self.read_calls()), 0)

        # 4. Valid PDF upload succeeds
        valid_pdf = os.path.join(self.root, "report.pdf")
        with open(valid_pdf, "wb") as fh:
            fh.write(b"%PDF-1.4 sample content")
        res_ok = self.run_wrapper(
            ["drive-upload", valid_pdf, "name=Q4-Report.pdf", "parent=folder_abc"],
            env_overrides=self.auth_env(),
        )
        self.assertEqual(res_ok.returncode, 0, res_ok.stderr)
        call = self.read_calls()[0]
        self.assertEqual(call["argv"][:4], ["drive", "files", "create", "--upload"])
        meta = json.loads(call["argv"][call["argv"].index("--json") + 1])
        self.assertEqual(meta["name"], "Q4-Report.pdf")
        self.assertEqual(meta["parents"], ["folder_abc"])
        self.assertEqual(meta["mimeType"], "application/pdf")

    def test_dry_run_previews_without_calling_gws(self):
        for args in (
            ["auth-setup", "account=alice@example.com", "project=my-proj", "--dry-run"],
            ["gmail-send", "to=bob@example.com", "subject=Hi", "body=Test", "--dry-run"],
            [
                "calendar-create",
                "summary=Sync",
                "start=2026-11-10T10:00:00Z",
                "end=2026-11-10T10:30:00Z",
                "--dry-run",
            ],
            ["tasks-create", "title=Todo item", "--dry-run"],
            ["drive-mkdir", "Reports", "parent=folder_1", "--dry-run"],
        ):
            res = self.run_wrapper(args, env_overrides=self.auth_env())
            self.assertEqual(res.returncode, 0, res.stderr)
            out = json.loads(res.stdout)
            self.assertTrue(out["dry_run"])
        self.assertEqual(len(self.read_calls()), 0)

    # --- 5. Composed Multi-Service Workflows ---
    def test_mail_to_task_links_gmail_message_into_task(self):
        res = self.run_wrapper(
            ["mail-to-task", "msg_123", "due=2026-11-15T00:00:00Z", "notes=High priority"],
            env_overrides=self.auth_env(),
        )
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])
        self.assertEqual(out["workflow"], "mail-to-task")
        self.assertEqual(out["source_message"]["subject"], "Q4 Architecture Review")
        self.assertEqual(out["task"]["id"], "tsk_created")

        calls = self.read_calls()
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[0]["argv"][:4], ["gmail", "users", "messages", "get"])
        self.assertEqual(calls[1]["argv"][:3], ["tasks", "tasks", "insert"])
        task_body = json.loads(calls[1]["argv"][calls[1]["argv"].index("--json") + 1])
        self.assertEqual(task_body["title"], "[Email] Q4 Architecture Review")
        self.assertIn("gmail_message_id: msg_123", task_body["notes"])

    def test_drive_share_mail_reports_partial_mutation_without_retry(self):
        doc_path = os.path.join(self.root, "brief.md")
        with open(doc_path, "w", encoding="utf-8") as fh:
            fh.write("# Sprint Brief\n")

        env = self.auth_env()
        env["FAKE_GWS_FAIL_GMAIL_SEND"] = "1"
        res = self.run_wrapper(
            ["drive-share-mail", doc_path, "to=bob@example.com", "subject=Brief"],
            env_overrides=env,
        )
        self.assertNotEqual(res.returncode, 0)
        err = json.loads(res.stderr)["error"]
        self.assertEqual(err["code"], "PARTIAL_WORKFLOW_FAILURE")
        self.assertEqual(err["completed_step"], "drive-upload")
        self.assertEqual(err["failed_step"], "gmail-send")
        self.assertFalse(err["retry_safe"])
        self.assertEqual(err["partial_result"]["drive_file"]["id"], "drv_uploaded_777")

        # Ensure upload was only called ONCE (never retried)
        calls = self.read_calls()
        upload_calls = [c for c in calls if c["argv"][:3] == ["drive", "files", "create"]]
        self.assertEqual(len(upload_calls), 1)

    def test_agenda_combines_calendar_tasks_and_gmail(self):
        res = self.run_wrapper(["agenda", "maxResults:=5"], env_overrides=self.auth_env())
        self.assertEqual(res.returncode, 0, res.stderr)
        out = json.loads(res.stdout)
        self.assertTrue(out["ok"])
        self.assertEqual(out["workflow"], "agenda")
        self.assertEqual(len(out["calendar_events"]), 1)
        self.assertEqual(len(out["tasks"]), 1)
        self.assertEqual(len(out["messages"]), 1)


if __name__ == "__main__":
    unittest.main()
