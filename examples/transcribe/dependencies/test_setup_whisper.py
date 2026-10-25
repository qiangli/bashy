"""Pure tests for explicit whisper.cpp source setup; no native build runs."""

import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import build_whisper
import setup_whisper


class SourceSetupTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.cache = Path(self.temp.name) / "cache"
        self.cli = build_whisper.get_default_coordination_cli_path("linux", self.cache)
        self.cli.parent.mkdir(parents=True)
        self.cli.write_bytes(b"reviewed native binary")
        self.digest = build_whisper.compute_sha256(self.cli)
        self.report = {
            "status": "built_and_cached", "platform": "linux/amd64",
            "runner_label": "ubuntu-24.04", "whisper_version": "1.9.5",
            "source_archive_sha256": build_whisper.PINNED_SOURCE_SHA256,
            "source_archive_bytes": build_whisper.PINNED_SOURCE_BYTES,
            "cpu_only": True, "portable_cpu": True, "openmp_disabled": True, "gpl_free": True,
            "build_flags": ["-DGGML_NATIVE=OFF"],
            "cached_executable_path": str(self.cli),
            "executable_sha256": self.digest, "executable_bytes": self.cli.stat().st_size,
        }
        self.target = patch.object(build_whisper, "detect_target", return_value=("linux", "amd64", "ubuntu-24.04"))
        self.target.start()
        self.addCleanup(self.target.stop)

    def test_receipt_records_exact_runtime_coordinates_atomically(self):
        receipt = setup_whisper.record_source_build(self.report, self.cache)
        self.assertEqual(receipt["registration"], {
            "WHISPER_PATH": str(self.cli.resolve()), "WHISPER_SHA256": self.digest,
        })
        path = self.cli.parent / setup_whisper.RECEIPT_NAME
        self.assertEqual(json.loads(path.read_text()), receipt)
        self.assertFalse(list(path.parent.glob(".whisper-setup-*")))

    def test_tamper_and_false_report_do_not_publish_receipt(self):
        for mutation in (
            {"executable_sha256": "0" * 64},
            {"executable_bytes": 1},
            {"platform": "linux/arm64"},
            {"portable_cpu": False},
            {"build_flags": []},
            {"source_archive_sha256": "0" * 64},
            {"gpl_free": False},
            {"cached_executable_path": None},
        ):
            with self.subTest(mutation=mutation), self.assertRaises(RuntimeError):
                setup_whisper.record_source_build({**self.report, **mutation}, self.cache)
            self.assertFalse((self.cli.parent / setup_whisper.RECEIPT_NAME).exists())
        self.cli.write_bytes(b"x" * self.report["executable_bytes"])
        with self.assertRaisesRegex(RuntimeError, "SHA-256 differs"):
            setup_whisper.record_source_build(self.report, self.cache)

    def test_setup_calls_only_pinned_recipe_on_explicit_invocation(self):
        work = Path(self.temp.name) / "work"
        archive = Path(self.temp.name) / "source.tar.gz"
        with patch.object(build_whisper, "build_whisper", return_value=self.report) as recipe:
            receipt = setup_whisper.setup(work, self.cache, archive, True, 2)
        recipe.assert_called_once_with(
            work_dir=work, cache_dir=self.cache, source_archive=archive,
            skip_download=True, parallel=2,
        )
        self.assertEqual(receipt["executable_sha256"], self.digest)

    def test_existing_receipt_reuses_cached_binary_without_build(self):
        receipt = setup_whisper.record_source_build(self.report, self.cache)
        with patch.object(build_whisper, "build_whisper", side_effect=AssertionError("unexpected build")):
            self.assertEqual(setup_whisper.verify_existing(self.cache), receipt)

    def test_existing_receipt_rejects_changed_binary_or_target(self):
        receipt = setup_whisper.record_source_build(self.report, self.cache)
        self.cli.write_bytes(b"x" * self.report["executable_bytes"])
        with self.assertRaisesRegex(RuntimeError, "executable differs"):
            setup_whisper.verify_existing(self.cache)
        self.cli.write_bytes(b"reviewed native binary")
        receipt["platform"] = "linux/arm64"
        (self.cli.parent / setup_whisper.RECEIPT_NAME).write_text(json.dumps(receipt))
        with self.assertRaisesRegex(RuntimeError, "target and source pin"):
            setup_whisper.verify_existing(self.cache)


if __name__ == "__main__":
    unittest.main()
