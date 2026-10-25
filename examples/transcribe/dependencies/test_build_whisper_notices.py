"""Focused offline tests for the Whisper notice evidence gate."""

import copy
import hashlib
import io
import json
import shutil
import tarfile
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace

import build_whisper_notices as notices


def sha(data):
    return hashlib.sha256(data).hexdigest()


class NoticeBundleTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        shutil.copytree(notices.ROOT / "sources", self.root / "sources")
        self.pins = json.loads(notices.PINS.read_text())
        license_bytes = (self.root / "sources" / "whisper-MIT.txt").read_bytes()
        self.archive = self.root / "source.tar.gz"
        with tarfile.open(self.archive, "w:gz") as tar:
            item = tarfile.TarInfo(self.pins["whisper"]["license_member"])
            item.size = len(license_bytes)
            tar.addfile(item, io.BytesIO(license_bytes))
        source = self.archive.read_bytes()
        self.pins["whisper"].update(archive_sha256=sha(source), archive_bytes=len(source))
        self.pins_path = self.root / "source-pins.json"
        self.pins_path.write_text(json.dumps(self.pins))
        self.binary = self.root / "whisper-cli.exe"
        self.binary.write_bytes(b"PE test binary")
        self.map = self.root / "link.map"
        self.map.write_bytes(b"loaded test objects")
        self.inventory = {
            "binary_name": self.binary.name, "binary_sha256": sha(self.binary.read_bytes()),
            "map_sha256": sha(self.map.read_bytes()), "map_bytes": self.map.stat().st_size,
            "archive_members": [{"archive": name, "member": "sample.o"} for name in sorted(
                notices.PROJECT_ARCHIVES | notices.RUNTIME_ARCHIVES | {"libclang_rt.builtins-x86_64.a"})],
            "unclassified_archives": sorted(notices.PROJECT_ARCHIVES - {"libwhisper.a"}),
            "map_objects_without_archive_match": sorted(notices.STARTUP_OBJECTS),
        }
        self.inventory_path = self.root / "inventory.json"
        self.inventory_path.write_text(json.dumps(self.inventory))
        asset = self.pins["windows_toolchain"]["assets"]["amd64"]
        self.toolchain = {"platform": "windows/amd64", "version": "20260616",
                          "source_url": asset["url"], "archive_sha256": asset["archive_sha256"],
                          "archive_bytes": asset["archive_bytes"],
                          "toolchain_root": "C:\\llvm-mingw"}
        self.toolchain_path = self.root / "toolchain.json"
        self.toolchain_path.write_text(json.dumps(self.toolchain))
        self.report = {"platform": "windows/amd64", "whisper_version": "1.9.5",
                       "source_archive_sha256": sha(source), "source_archive_bytes": len(source),
                       "product_admitted": False, "status": "built_and_cached",
                       "executable_sha256": sha(self.binary.read_bytes()),
                       "executable_bytes": self.binary.stat().st_size,
                       "compiler_path": "C:\\llvm-mingw\\bin\\clang.exe",
                       "cxx_compiler_path": "C:\\llvm-mingw\\bin\\clang++.exe",
                       "link_map_inventory": self.inventory}
        self.report_path = self.root / "report.json"
        self.save_report()

    def save_report(self):
        self.report_path.write_text(json.dumps(self.report))

    def args(self, output):
        return SimpleNamespace(source_archive=self.archive, binary=self.binary,
                               build_report=self.report_path, output_dir=self.root / output,
                               toolchain_report=self.toolchain_path,
                               inventory=self.inventory_path, link_map=self.map, review=None)

    def test_windows_bundle_deterministic_and_bound_to_binary(self):
        first = notices.generate(self.args("one"), self.pins_path)
        second = notices.generate(self.args("two"), self.pins_path)
        self.assertEqual(first, second)
        self.assertEqual((self.root / "one" / "THIRD_PARTY_NOTICES.txt").read_bytes(),
                         (self.root / "two" / "THIRD_PARTY_NOTICES.txt").read_bytes())
        self.assertEqual(len(first["notices"]), 8)
        self.assertEqual(first["status"], "candidate_product_not_admitted")
        self.assertEqual(first["evidence"]["map_sha256"], sha(self.map.read_bytes()))

    def test_tampered_binary_and_notice_rejected(self):
        self.binary.write_bytes(b"changed")
        with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
            notices.generate(self.args("bad-binary"), self.pins_path)
        self.binary.write_bytes(b"PE test binary")
        (self.root / "sources" / "mingw-w64-runtime.txt").write_text("changed")
        with self.assertRaisesRegex(ValueError, "SHA-256 mismatch"):
            notices.generate(self.args("bad-notice"), self.pins_path)

    def test_unknown_static_member_and_map_rejected(self):
        altered = copy.deepcopy(self.inventory)
        altered["archive_members"].append({"archive": "libgpl.a", "member": "x.o"})
        self.inventory_path.write_text(json.dumps(altered))
        self.report["link_map_inventory"] = altered
        self.save_report()
        with self.assertRaisesRegex(ValueError, "unexpected or missing linked archive"):
            notices.generate(self.args("bad-member"), self.pins_path)
        self.map.write_bytes(b"tampered map")
        with self.assertRaisesRegex(ValueError, "link map mismatch"):
            notices.generate(self.args("bad-map"), self.pins_path)

    def test_wrong_toolchain_asset_rejected(self):
        self.toolchain["archive_sha256"] = "0" * 64
        self.toolchain_path.write_text(json.dumps(self.toolchain))
        with self.assertRaisesRegex(ValueError, "toolchain pin mismatch"):
            notices.generate(self.args("bad-toolchain"), self.pins_path)

    def test_linux_remains_open_for_static_gcc_runtime(self):
        self.binary.rename(self.root / "whisper-cli")
        self.binary = self.root / "whisper-cli"
        self.report["platform"] = "linux/amd64"
        self.save_report()
        result = notices.generate(self.args("linux"), self.pins_path)
        self.assertEqual(len(result["notices"]), 1)
        self.assertTrue(any("GPLv3 GCC Runtime Library Exception" in gap for gap in result["open_gates"]))


if __name__ == "__main__":
    unittest.main()
