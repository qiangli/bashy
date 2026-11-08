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
        p = Path(d) / "klatch"
        p.write_text('#!/bin/sh\ncat >/dev/null\nprintf \'{"ok":true,"result":{"argv":"%s"}}\\n\' "$*"\n')
        p.chmod(p.stat().st_mode | stat.S_IXUSR)
        return p

    def test_forwards_verbs_and_stdin(self):
        with tempfile.TemporaryDirectory() as d:
            r = run(["messages-send", "--json"], {"KLATCH_CLI": str(self.fake(d))}, '{"text":"x"}')
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual(json.loads(r.stdout)["result"]["argv"], "messages-send --json")

    def test_missing_cli(self):
        r = run(["watch"], {"KLATCH_CLI": "", "PATH": "/usr/bin:/bin"})
        self.assertEqual(r.returncode, 1)
        self.assertEqual(json.loads(r.stderr)["error"]["code"], "CLI_NOT_FOUND")

    def test_unknown_verb_usage(self):
        self.assertEqual(run(["bogus"]).returncode, 2)

    def test_setup_arity_preserved(self):
        self.assertEqual(run(["status"]).returncode, 2)


if __name__ == "__main__":
    unittest.main()
