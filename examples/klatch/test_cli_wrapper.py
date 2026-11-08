import json
import os
import stat
import subprocess
import tempfile
import unittest
from pathlib import Path

HERE = Path(__file__).resolve().parent
BASHY = os.environ.get("BASHY_BIN", "bashy")


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

    def test_non_secret_fields_still_shape(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["messages-send", "channelId=c1", "text=hello token world", "codeBlock:=true"],
                    {"KLATCH_HELPER": str(self.fake(d))})
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(json.loads(r.stdout)["result"]["in"]["codeBlock"], True)

    def test_post_file_workflow(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["post-file", "channelId=c1", "text=t", "assistant=Bot", "path=/x"], {"KLATCH_HELPER": str(self.fake(d))})
            self.assertEqual(r.returncode, 0, r.stderr)
            res = json.loads(r.stdout)["result"]
            self.assertEqual(res["topicId"], "T1")
            self.assertEqual(res["upload"]["in"]["topicId"], "T1")
            self.assertIn("attachments-upload", res["upload"]["argv"])

    def test_missing_cli(self):
        r = run(["watch"], {"KLATCH_HELPER": "", "KLATCH_CLI": "", "PATH": "/usr/bin:/bin"})
        self.assertEqual(r.returncode, 1)
        self.assertEqual(json.loads(r.stderr)["error"]["code"], "CLI_NOT_FOUND")

    def test_unknown_verb_usage(self):
        self.assertEqual(run(["bogus"]).returncode, 2)

    def test_setup_arity_preserved(self):
        self.assertEqual(run(["status"]).returncode, 2)


if __name__ == "__main__":
    unittest.main()
