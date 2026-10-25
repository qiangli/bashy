"""Fail-closed native identity and retained-byte guards for six-target research."""

import importlib.util
import os
from pathlib import Path
import struct
import sys
import tempfile
import unittest
from unittest import mock


MODULE = Path(__file__).with_name("s503_quickjs_six_target.py")
sys.path.insert(0, str(MODULE.parent))
SPEC = importlib.util.spec_from_file_location("s503_quickjs_six_target", MODULE)
SIX = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SIX)


class SixTargetGuards(unittest.TestCase):
    def test_only_quickjs_target_objects_or_output_archives_are_retained(self):
        self.assertTrue(SIX.is_quickjs_output(Path("CMakeFiles/qjs.dir/quickjs.c.o")))
        self.assertTrue(SIX.is_quickjs_output(Path("qjs_exe.dir/Release/qjs.obj")))
        self.assertTrue(SIX.is_quickjs_output(Path("libqjs.a")))
        self.assertTrue(SIX.is_quickjs_output(Path("Release/qjs.lib")))
        self.assertFalse(SIX.is_quickjs_output(Path("CMakeFiles/CompilerIdC/probe.obj")))
        self.assertFalse(SIX.is_quickjs_output(Path("sdk/qjs.lib")))
        self.assertFalse(SIX.is_quickjs_output(Path("vcruntime.lib")))

    def test_all_six_configs_request_native_architecture_and_absolute_map(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.assertEqual(len(SIX.RUNNERS), 6)
            for target in SIX.RUNNERS:
                with self.subTest(target=target):
                    args, link_map = SIX.qjs_configure_args(
                        root / "source", root / "work", root / "evidence", target)
                    self.assertTrue(link_map.is_absolute())
                    self.assertIn(str(link_map), " ".join(args))
                    self.assertIn("-DBUILD_SHARED_LIBS=OFF", args)
                    if target.startswith("darwin/"):
                        arch = "x86_64" if target.endswith("amd64") else "arm64"
                        self.assertIn("-DCMAKE_OSX_ARCHITECTURES=" + arch, args)
                    elif target.startswith("windows/"):
                        self.assertEqual(args[args.index("-A") + 1],
                                         "x64" if target.endswith("amd64") else "ARM64")

    def test_runner_identity_refuses_emulation_or_wrong_host(self):
        for target, expected in SIX.RUNNERS.items():
            with self.subTest(target=target):
                with mock.patch.dict(os.environ,
                                     {"RUNNER_OS": expected[0], "RUNNER_ARCH": expected[1]}):
                    self.assertEqual(SIX.check_runner(target)["target"], target)
                with mock.patch.dict(os.environ,
                                     {"RUNNER_OS": expected[0], "RUNNER_ARCH": "WRONG"}):
                    with self.assertRaisesRegex(RuntimeError, "runner identity mismatch"):
                        SIX.check_runner(target)

    def test_macho_header_requires_exact_native_cpu(self):
        with tempfile.TemporaryDirectory() as temporary:
            binary = Path(temporary) / "qjs"
            for cpu, expected in ((0x01000007, "darwin/amd64"),
                                  (0x0100000c, "darwin/arm64")):
                binary.write_bytes(b"\xcf\xfa\xed\xfe" + struct.pack("<I", cpu) + b"\0" * 24)
                self.assertEqual(SIX.machine_from_binary(binary), expected)
            binary.write_bytes(b"\xcf\xfa\xed\xfe")
            with self.assertRaisesRegex(RuntimeError, "truncated Mach-O"):
                SIX.machine_from_binary(binary)

    def test_macos_map_must_name_objects_and_symbols(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "qjs-link.map"
            path.write_text("# Object files:\n[  1] qjs.o\n# Symbols:\n", encoding="utf-8")
            SIX.verify_link_records(path, Path(temporary) / "build.log", "darwin/arm64")
            path.write_text("# Object files:\n[  1] qjs.o\n", encoding="utf-8")
            with self.assertRaisesRegex(RuntimeError, "symbol sections"):
                SIX.verify_link_records(path, Path(temporary) / "build.log", "darwin/amd64")

    def test_retain_exact_binary_archive_objects_and_reject_symlink(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            source.mkdir()
            (source / "LICENSE").write_text("MIT", encoding="utf-8")
            archive = root / "quickjs-source.tar.gz"
            archive.write_bytes(b"source-bytes")
            binary = root / "qjs"
            binary.write_bytes(b"native-binary")
            build = root / "build"
            (build / "CMakeFiles" / "qjs.dir").mkdir(parents=True)
            obj = build / "CMakeFiles" / "qjs.dir" / "qjs.c.o"
            obj.write_bytes(b"object-bytes")
            (build / "CMakeFiles" / "CompilerIdC").mkdir(parents=True)
            (build / "CMakeFiles" / "CompilerIdC" / "CMakeCCompilerId.o").write_bytes(
                b"compiler-probe")
            evidence = root / "evidence"
            inventory, notices = SIX.copy_review_bytes(
                archive, source, binary, build, evidence)
            self.assertEqual({i["path"] for i in inventory},
                             {"source/quickjs-source.tar.gz", "native/qjs",
                              "objects/CMakeFiles/qjs.dir/qjs.c.o"})
            for item in inventory:
                self.assertEqual(SIX.pilot.sha256_file(evidence / item["path"]), item["sha256"])
            self.assertEqual(notices[0]["path"], "LICENSE")
            (build / "CMakeFiles" / "qjs.dir" / "link.o").symlink_to(obj)
            with self.assertRaisesRegex(RuntimeError, "symlink"):
                SIX.copy_review_bytes(archive, source, binary, build, evidence)

    def test_retained_byte_cap_fails_closed(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "source"
            source.mkdir()
            (source / "LICENSE").write_text("MIT", encoding="utf-8")
            archive = root / "quickjs-source.tar.gz"
            archive.write_bytes(b"archive")
            binary = root / "qjs"
            binary.write_bytes(b"binary")
            build = root / "build"
            (build / "CMakeFiles" / "qjs.dir").mkdir(parents=True)
            (build / "CMakeFiles" / "qjs.dir" / "qjs.o").write_bytes(b"object")
            with mock.patch.object(SIX, "MAX_RETAINED_BYTES", 10):
                with self.assertRaisesRegex(RuntimeError, "exceed cap"):
                    SIX.copy_review_bytes(archive, source, binary, build, root / "evidence")

    def test_external_inputs_keep_paths_hashes_and_no_provider_bytes(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            build = root / "build"
            build.mkdir()
            provider = root / "provider.lib"
            provider.write_bytes(b"host-sdk-placeholder")
            link_map = root / "map"
            link_map.write_text("", encoding="utf-8")
            imports = root / "imports"
            imports.write_text("", encoding="utf-8")
            build_log = root / "build.log"
            build_log.write_text("Searching " + str(provider) + ":\n", encoding="utf-8")
            records = SIX.external_link_inputs(link_map, build_log, imports,
                                               build, "windows/amd64")
            self.assertEqual(len(records), 1)
            self.assertEqual(records[0]["link_path"], str(provider))
            self.assertEqual(records[0]["sha256"], SIX.pilot.sha256_file(provider))
            self.assertIn("Microsoft", records[0]["provider_candidate"])
            self.assertIn("Windows SDK", records[0]["notice_candidate"])
            self.assertEqual(records[0]["notice_review"], "PENDING")


if __name__ == "__main__":
    unittest.main()
