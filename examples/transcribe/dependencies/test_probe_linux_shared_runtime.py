"""Local-only provenance checks; no Whisper build or Linux runtime needed."""

import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import probe_linux_shared_runtime as probe


class GlibcStartupReceiptsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.objects = []
        for name in probe.GLIBC_STARTUP_NAMES:
            path = self.root / "usr/lib/x86_64-linux-gnu" / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(name.encode())
            self.objects.append(str(path))
        copyright_path = self.root / "usr/share/doc/libc6-dev/copyright"
        copyright_path.parent.mkdir(parents=True)
        copyright_path.write_text("GNU C Library; Lesser General Public License\n")

    @staticmethod
    def package_receipt(path):
        is_copyright = path.name == "copyright"
        return {"path": str(path), "sha256": probe.pinned.compute_sha256(path),
                "package": "libc6:amd64" if is_copyright else "libc6-dev:amd64",
                "package_version": "2.39-0ubuntu8"}

    @staticmethod
    def source_receipt(package):
        return {"package": package, "package_version": "2.39-0ubuntu8",
                "source_package": "glibc", "source_package_version": "2.39-0ubuntu8"}

    def collect(self, objects=None):
        with patch.object(probe, "package_receipt", side_effect=self.package_receipt), \
             patch.object(probe, "dpkg_source_receipt", side_effect=self.source_receipt):
            return probe.glibc_startup_receipts(
                self.objects if objects is None else objects, "amd64", self.root)

    def test_binds_all_three_files_and_package_copyright(self):
        result = self.collect()
        self.assertEqual(set(result["objects"]), set(probe.GLIBC_STARTUP_NAMES))
        self.assertEqual(result["source_package"], "glibc")
        self.assertEqual(result["copyright"]["package"], "libc6:amd64")
        self.assertEqual(result["license_review_status"],
                         "open; file-level LGPL and linked-byte review required")
        self.assertFalse(result["product_admitted"])

    def test_missing_or_duplicate_object_fails(self):
        with self.assertRaisesRegex(RuntimeError, "incomplete"):
            self.collect(self.objects[:-1])
        with self.assertRaisesRegex(RuntimeError, "duplicate"):
            self.collect(self.objects + self.objects[:1])

    def test_wrong_owner_fails(self):
        def wrong_owner(path):
            result = self.package_receipt(path)
            if path.name == "Scrt1.o":
                result["package"] = "libgcc-13-dev:amd64"
            return result

        with patch.object(probe, "package_receipt", side_effect=wrong_owner):
            with self.assertRaisesRegex(RuntimeError, "unexpected glibc startup owner"):
                probe.glibc_startup_receipts(self.objects, "amd64", self.root)

    def test_copyright_mismatch_fails(self):
        def wrong_source(package):
            result = self.source_receipt(package)
            if package == "libc6:amd64":
                result["source_package_version"] = "other-version"
            return result

        with patch.object(probe, "package_receipt", side_effect=self.package_receipt), \
             patch.object(probe, "dpkg_source_receipt", side_effect=wrong_source):
            with self.assertRaisesRegex(RuntimeError, "copyright package version differs"):
                probe.glibc_startup_receipts(self.objects, "amd64", self.root)

    def test_package_receipt_rejects_ambiguous_owner(self):
        path = Path(self.objects[0]).resolve()
        with patch.object(probe, "run_checked",
                          return_value=f"libc6-dev:amd64, other:amd64: {path}\n"):
            with self.assertRaisesRegex(RuntimeError, "ambiguous package owner"):
                probe.package_receipt(path)

    def test_source_metadata_requires_glibc_and_installed_package(self):
        with patch.object(probe, "run_checked",
                          return_value="libc6-dev:amd64\t2.39\tglibc\t2.39\tinstalled"):
            self.assertEqual(probe.dpkg_source_receipt("libc6-dev:amd64"),
                             {"package": "libc6-dev:amd64", "package_version": "2.39",
                              "source_package": "glibc", "source_package_version": "2.39"})
        for metadata in ("libc6-dev:amd64\t2.39\tother\t2.39\tinstalled",
                         "libc6-dev:amd64\t2.39\tglibc\t2.39\tnot-installed"):
            with patch.object(probe, "run_checked", return_value=metadata):
                with self.assertRaisesRegex(RuntimeError, "unexpected glibc package"):
                    probe.dpkg_source_receipt("libc6-dev:amd64")


if __name__ == "__main__":
    unittest.main()
