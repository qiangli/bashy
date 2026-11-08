import json
import os
import shutil
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
BASHY = os.environ.get("BASHY_BIN") or shutil.which("bashy") or "bashy"


def run(args, env=None, stdin=""):
    e = dict(os.environ, **(env or {}))
    return subprocess.run([BASHY, "--bashsharp", str(HERE / "klatch.bsh")] + args,
                          input=stdin, text=True, capture_output=True, env=e)


class Wrapper(unittest.TestCase):
    def fake(self, d):
        p = Path(d) / "klatch-helper"
        p.write_text('#!/bin/sh\nin=$(cat)\nprintf \'{"ok":true,"result":{"id":"T1","argv":"%s","in":%s}}\\n\' "$*" "${in:-null}"\n')
        p.chmod(p.stat().st_mode | stat.S_IXUSR)
        return p

    def test_forwards_verbs_and_stdin(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["messages-send", "--json"], {"KLATCH_HELPER": str(self.fake(d))}, '{"text":"x"}')
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(json.loads(r.stdout)["result"]["argv"], "messages-send --json")

    def test_shapes_fields_into_json(self):
        with tempfile.TemporaryDirectory() as d:
            env = {"KLATCH_HELPER": str(self.fake(d)), "KLATCH_ASSISTANT": "Bot"}
            r = run(["messages-send", "channelId=c1", "text=say \"hi\" \\ ok", "count:=3"], env)
            self.assertEqual(r.returncode, 0, r.stderr)
            res = json.loads(r.stdout)["result"]
            self.assertEqual(res["in"], {"channelId": "c1", "text": "say \"hi\" \\ ok", "count": 3, "assistant": "Bot"})
            self.assertIn("messages-send --json", res["argv"])

    def test_secret_refused_in_argv(self):
        r = run(["auth-login", "password=x"], {"KLATCH_HELPER": "/bin/true"})
        self.assertEqual(r.returncode, 2)
        self.assertEqual(json.loads(r.stderr)["error"]["code"], "SECRET_VIA_ARGV")

    def test_credential_like_keys_refused_case_insensitively(self):
        for k in ("token", "Token", "accessToken", "clientSecret", "PRIVATEKEY", "private_key",
                  "Authorization", "credentials", "sealPassword", "MFA", "apiKey"):
            with self.subTest(key=k):
                r = run(["messages-send", k + "=x"], {"KLATCH_HELPER": "/bin/true"})
                self.assertEqual(r.returncode, 2)
                self.assertEqual(json.loads(r.stderr)["error"]["code"], "SECRET_VIA_ARGV")

    def test_more_credential_keys_and_nested_json_refused(self):
        for a in ("mfaCode=1", "otpCode=1", "totp=1", "verificationCode=1", "sealCode=1",
                  "recoveryKey=x", 'opts:={"auth":{"accessToken":"x"}}', 'l:=[{"clientSecret":"y"}]'):
            with self.subTest(arg=a):
                r = run(["messages-send", a], {"KLATCH_HELPER": "/bin/true"})
                self.assertEqual(r.returncode, 2)
                self.assertEqual(json.loads(r.stderr)["error"]["code"], "SECRET_VIA_ARGV")

    def test_assistant_default_in_stdin(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["messages-send", "--json"], {"KLATCH_HELPER": str(self.fake(d)), "KLATCH_ASSISTANT": "Bot"}, '{"text":"x"}')
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(json.loads(r.stdout)["result"]["in"], {"text": "x", "assistant": "Bot"})

    def test_usage_is_envelope_and_help_without_helper(self):
        r = run(["messages-send", "notkv"], {"KLATCH_HELPER": "/bin/true"})
        self.assertEqual(r.returncode, 2)
        self.assertEqual(json.loads(r.stderr)["error"]["code"], "USAGE")
        r = run(["--help"], {"KLATCH_HELPER": "", "PATH": "/usr/bin:/bin"})
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertIn("Usage: klatch", r.stdout)

    def test_post_file_upload_failure_keeps_topic(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "klatch-helper"
            p.write_text('#!/bin/sh\ncat >/dev/null\ncase "$1" in messages-send) echo \'{"ok":true,"result":{"id":"T\\"1"}}\' ;; *) echo \'{"ok":false,"error":{"code":"DISK"}}\' >&2; exit 1 ;; esac\n')
            p.chmod(p.stat().st_mode | stat.S_IXUSR)
            r = run(["post-file", "channelId=c1", "text=t", "assistant=Bot", "path=/x"], {"KLATCH_HELPER": str(p)})
            self.assertEqual(r.returncode, 1)
            e = json.loads(r.stderr)["error"]
            self.assertEqual((e["code"], e["topicId"], e["cause"]["code"]), ("UPLOAD_FAILED", 'T"1', "DISK"))

    def test_non_secret_fields_still_shape(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["messages-send", "channelId=c1", "text=hello token world", "codeBlock:=true"],
                    {"KLATCH_HELPER": str(self.fake(d))})
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(json.loads(r.stdout)["result"]["in"]["codeBlock"], True)

    def test_sealed_flag_is_not_secret(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["conversations-create", "name=x", "sealed:=false"], {"KLATCH_HELPER": str(self.fake(d))})
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIs(json.loads(r.stdout)["result"]["in"]["sealed"], False)
        r = run(["conversations-create", 'o:={"sealed":false,"sealPassword":"x"}'], {"KLATCH_HELPER": "/bin/true"})
        self.assertEqual(r.returncode, 2)
        self.assertEqual(json.loads(r.stderr)["error"]["code"], "SECRET_VIA_ARGV")

    def test_post_file_workflow(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["post-file", "channelId=c1", "text=t", "assistant=Bot", "path=/x"], {"KLATCH_HELPER": str(self.fake(d))})
            self.assertEqual(r.returncode, 0, r.stderr)
            res = json.loads(r.stdout)["result"]
            self.assertEqual(res["topicId"], "T1")
            self.assertEqual(res["upload"]["in"]["topicId"], "T1")
            self.assertIn("attachments-upload", res["upload"]["argv"])

    def test_missing_cli(self):
        r = run(["watch"], {"KLATCH_HELPER": "", "PATH": "/usr/bin:/bin"})
        self.assertEqual(r.returncode, 1)
        self.assertEqual(json.loads(r.stderr)["error"]["code"], "CLI_NOT_FOUND")

    def test_unknown_verb_usage(self):
        self.assertEqual(run(["bogus"]).returncode, 2)

    def test_setup_arity_preserved(self):
        self.assertEqual(run(["status"]).returncode, 2)


if __name__ == "__main__":
    unittest.main()
