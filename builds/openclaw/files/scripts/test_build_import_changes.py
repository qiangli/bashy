#!/usr/bin/env python3
"""Tests for build_import_changes.py: every --change record must survive
docker's Dockerfile-instruction parse of `docker import` back to the exact
source config value.

The CI failure (run 38469059131): the LABEL
  org.opencontainers.image.description=OpenClaw gateway and CLI runtime container image
reached `docker import` unquoted, split into bare words, and died with
  Syntax error - can't find = in "gateway". Must be of the form: name=value
(the message comes from the daemon's name=value word parser; the quoting
below was validated against that parser and its shell expansion).
"""

import json
import os
import subprocess
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import build_import_changes as bic  # noqa: E402


# The source-config shape `docker inspect --format={{json .Config}}`
# reports for the pinned upstream runtime image.
SOURCE_LIKE = {
    "Env": [
        "PATH=/home/node/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
        "NODE_ENV=production",
        "EMPTY_OK=",
    ],
    "User": "node",
    "WorkingDir": "/app",
    "Entrypoint": ["tini", "-s", "--", "node", "/app/docker-entrypoint.mjs"],
    "Cmd": ["node", "openclaw.mjs", "gateway"],
    "Labels": {
        "org.opencontainers.image.title": "OpenClaw",
        "org.opencontainers.image.description": "OpenClaw gateway and CLI runtime container image",
    },
}


class BuildChanges(unittest.TestCase):
    def test_label_with_spaces_is_quoted(self):
        # The exact CI failure: this record must stay ONE name=value word.
        self.assertIn(
            'LABEL org.opencontainers.image.description='
            '"OpenClaw gateway and CLI runtime container image"',
            bic.build_changes(SOURCE_LIKE),
        )

    def test_cmd_with_spaces_and_json_stays_one_json_record(self):
        cfg = {"Cmd": ["node", "run with spaces.mjs", "--opt", '{"a": [1, 2]}']}
        (rec,) = [c for c in bic.build_changes(cfg) if c.startswith("CMD ")]
        self.assertEqual(
            rec, 'CMD ["node", "run with spaces.mjs", "--opt", "{\\"a\\": [1, 2]}"]'
        )
        self.assertEqual(json.loads(rec[len("CMD "):]), cfg["Cmd"])

    def test_entrypoint_json_round_trips(self):
        (rec,) = [c for c in bic.build_changes(SOURCE_LIKE) if c.startswith("ENTRYPOINT ")]
        self.assertEqual(json.loads(rec[len("ENTRYPOINT "):]), SOURCE_LIKE["Entrypoint"])

    def test_env_passthrough_and_empty_value(self):
        changes = bic.build_changes(SOURCE_LIKE)
        self.assertIn(
            "ENV PATH=/home/node/.local/bin:/usr/local/sbin:/usr/local/bin:"
            "/usr/sbin:/usr/bin:/sbin:/bin",
            changes,
        )
        self.assertIn('ENV EMPTY_OK=""', changes)

    def test_env_value_with_equals_and_dollar(self):
        cfg = {"Env": ["A=b=c", "B=price is $5"]}
        changes = bic.build_changes(cfg)
        self.assertIn("ENV A=b=c", changes)
        # An unescaped $ would expand against the (empty) import-time env.
        self.assertIn('ENV B="price is \\$5"', changes)

    def test_quote_backslash_singlequote_hash_quoting(self):
        cfg = {"Labels": {
            "q": 'say "hi"',
            "b": "a\\b",
            "s": "it's ok",
            "h": "#tag me",
        }}
        changes = bic.build_changes(cfg)
        self.assertIn('LABEL q="say \\"hi\\""', changes)
        self.assertIn('LABEL b="a\\\\b"', changes)
        self.assertIn('LABEL s="it\'s ok"', changes)
        self.assertIn('LABEL h="#tag me"', changes)

    def test_labels_sorted_workdir_user(self):
        changes = bic.build_changes(SOURCE_LIKE)
        labels = [c for c in changes if c.startswith("LABEL ")]
        self.assertEqual(sorted(labels), labels)
        self.assertIn("WORKDIR /app", changes)
        self.assertIn("USER node", changes)

    def test_workdir_with_space_is_quoted(self):
        self.assertIn('WORKDIR "/my dir"', bic.build_changes({"WorkingDir": "/my dir"}))

    def test_missing_sections_emit_nothing(self):
        self.assertEqual(bic.build_changes({}), [])

    def test_newline_fails_closed(self):
        with self.assertRaises(ValueError):
            bic.build_changes({"Labels": {"evil": "a\nb"}})

    def test_cli_writes_nul_separated_records(self):
        with tempfile.TemporaryDirectory() as d:
            src = os.path.join(d, "source-config.json")
            out = os.path.join(d, "changes.nul")
            with open(src, "w") as f:
                json.dump(SOURCE_LIKE, f)
            r = subprocess.run(
                [sys.executable, os.path.join(HERE, "build_import_changes.py"), src, out],
                capture_output=True, text=True,
            )
            self.assertEqual(r.returncode, 0, r.stderr)
            with open(out, "rb") as f:
                records = f.read().split(b"\0")
            self.assertEqual(records[-1], b"")
            self.assertEqual(
                [c.decode() for c in records[:-1]], bic.build_changes(SOURCE_LIKE)
            )

    def test_cli_rejects_newline_with_nonzero_exit(self):
        with tempfile.TemporaryDirectory() as d:
            src = os.path.join(d, "source-config.json")
            out = os.path.join(d, "changes.nul")
            with open(src, "w") as f:
                json.dump({"Labels": {"evil": "a\nb"}}, f)
            r = subprocess.run(
                [sys.executable, os.path.join(HERE, "build_import_changes.py"), src, out],
                capture_output=True, text=True,
            )
            self.assertNotEqual(r.returncode, 0)


if __name__ == "__main__":
    unittest.main()
