#!/usr/bin/env python3
import importlib.util
import json
import tempfile
from unittest import mock
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("pins", Path(__file__).with_name("update-src-pins.py"))
pins = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pins)


class SourcePinsTest(unittest.TestCase):
    def setUp(self):
        self.module = "github.com/qiangli/sh/v3"
        self.commit = "a" * 40
        self.version = "v3.0.0-20261103000000-" + self.commit[:12]
        self.download = {"Path": self.module, "Version": self.version,
                         "Origin": {"VCS": "git", "URL": "https://github.com/qiangli/sh", "Hash": self.commit}}

    def test_full_revision(self):
        self.assertEqual(pins.full_revision(self.module, self.version, self.download), self.commit)

    def test_wrong_revision_origin_or_identity_rejected(self):
        for field, value in [("Hash", "b" * 40), ("Hash", "a" * 12),
                             ("VCS", "hg"), ("URL", "https://example.com/sh")]:
            with self.subTest(field=field, value=value):
                record = dict(self.download, Origin=dict(self.download["Origin"], **{field: value}))
                with self.assertRaises(ValueError):
                    pins.full_revision(self.module, self.version, record)
        with self.assertRaises(ValueError):
            pins.full_revision(self.module, self.version, dict(self.download, Path="example.com/sh"))

    def test_effective_replacements_and_idempotence(self):
        source = (pins.ROOT / "internal/agentos/src_mount.go").read_text()
        requests = []
        by_path = {m[4] or m[3]: m[3] for m in pins.PIN.finditer(source)}

        def query(args):
            requests.append(args)
            if "@" not in args[-1]:
                path = args[-1]
                module = by_path[path]
                return {"Path": path, "Version": "v1.0.0", "Replace": {"Path": module, "Version": self.version}}
            module, version = args[-1].split("@")
            repository = pins.re.sub(r"/v[0-9]+$", "", module)
            return {"Path": module, "Version": version, "Origin": {
                "VCS": "git", "URL": "https://" + repository, "Hash": self.commit}}

        updated = pins.refresh(source, query)
        self.assertTrue(all(m[5] == self.commit for m in pins.PIN.finditer(updated)))
        self.assertEqual(pins.refresh(updated, query), updated)
        self.assertTrue(any(args[-1] == "mvdan.cc/sh/v3" for args in requests))

    def test_stale_check_refuses_without_writing(self):
        source = (pins.ROOT / "internal/agentos/src_mount.go").read_text()
        by_path = {m[4] or m[3]: m[3] for m in pins.PIN.finditer(source)}

        def metadata(command, **kwargs):
            selected = command[-1]
            if "@" not in selected:
                return json.dumps({"Replace": {"Path": by_path[selected], "Version": self.version}})
            module, version = selected.split("@")
            return json.dumps({"Path": module, "Version": version, "Origin": {
                "VCS": "git", "URL": "https://" + pins.re.sub(r"/v[0-9]+$", "", module), "Hash": self.commit}})

        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            target = root / "internal/agentos/src_mount.go"
            target.parent.mkdir(parents=True)
            target.write_text(source)
            with mock.patch.object(pins, "ROOT", root), mock.patch("sys.argv", ["update-src-pins.py"]), mock.patch.object(pins.subprocess, "check_output", side_effect=metadata):
                with self.assertRaisesRegex(SystemExit, "source pins are stale"):
                    pins.main()
            self.assertEqual(target.read_text(), source)

    def test_local_or_wrong_fork_refused(self):
        source = (pins.ROOT / "internal/agentos/src_mount.go").read_text()
        for linked in [{"Path": "../local"}, {"Path": "example.com/other", "Version": self.version}]:
            with self.subTest(linked=linked), self.assertRaises(ValueError):
                pins.refresh(source, lambda args: {"Replace": linked})
        with self.assertRaises(ValueError):
            pins.refresh("", lambda args: self.fail("query must not run"))


if __name__ == "__main__":
    unittest.main()
