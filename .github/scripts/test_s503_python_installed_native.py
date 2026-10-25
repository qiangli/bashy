"""Focused notice provenance checks for the installed-Python research helper."""

import importlib.util
from pathlib import Path
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("s503-python-installed-native.py")
spec = importlib.util.spec_from_file_location("s503_python_installed_native", SCRIPT)
research = importlib.util.module_from_spec(spec)
spec.loader.exec_module(research)


class NoticeInventoryTest(unittest.TestCase):
    def test_preinstalled_notice_delta_and_vendored_candidate(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "framework"
            evidence = Path(directory) / "evidence"
            evidence.mkdir()
            license_file = root / "LICENSE.txt"
            vendored_file = root / "pip/_vendor/cachecontrol/LICENSE.txt"
            removed_file = root / "old/COPYING"
            for path, body in ((license_file, "old Python license"),
                               (vendored_file, "vendored license"),
                               (removed_file, "old notice")):
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(body)

            before = research.notice_inventory([root], evidence, phase="before")
            self.assertEqual(before["third_party_notice_candidates"], 1)
            self.assertTrue(before["all_files_retained"])

            license_file.write_text("new Python license")
            removed_file.unlink()
            new_file = root / "new/NOTICE.md"
            new_file.parent.mkdir()
            new_file.write_text("new notice")
            after = research.notice_inventory([root], evidence)
            delta = research.classify_notice_delta(before, after)

            self.assertEqual(after["third_party_notice_candidates"], 2)
            self.assertEqual(delta["counts"], {
                "unchanged_from_preinstall": 1,
                "changed_from_preinstall": 1,
                "new_path_after_install": 1,
                "removed_after_install": 1,
            })
            rows = {Path(row["path"]): row for row in after["files"]}
            self.assertEqual(rows[vendored_file]["change_from_preinstall"],
                             "unchanged_from_preinstall")
            self.assertEqual(rows[license_file]["preinstall_sha256"],
                             next(row["sha256"] for row in before["files"]
                                  if row["path"] == str(license_file)))
            self.assertEqual(Path(delta["removed"][0]["path"]), removed_file)
            self.assertIn("not proven installer payload", delta["attribution"])

    def test_fresh_prefix_has_empty_baseline(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory) / "new-python"
            evidence = Path(directory) / "evidence"
            evidence.mkdir()
            before = research.notice_inventory([root], evidence, phase="before")
            self.assertEqual(before["files"], [])
            license_file = root / "LICENSE.txt"
            license_file.parent.mkdir()
            license_file.write_text("Python license")
            after = research.notice_inventory([root], evidence)
            delta = research.classify_notice_delta(before, after)
            self.assertEqual(delta["counts"]["new_path_after_install"], 1)


if __name__ == "__main__":
    unittest.main()
