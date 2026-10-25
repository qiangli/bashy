"""Pure guard tests for the evidence-only QuickJS native pilot."""

import importlib.util
import io
from pathlib import Path
import struct
import tarfile
import tempfile
import unittest
import zipfile


MODULE = Path(__file__).with_name("s503_quickjs_native_pilot.py")
SPEC = importlib.util.spec_from_file_location("s503_quickjs_native_pilot", MODULE)
PILOT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PILOT)


class PilotGuards(unittest.TestCase):
    def test_native_configure_uses_absolute_link_map_and_available_arm_generator(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for target in ("linux/amd64", "windows/arm64"):
                args, link_map = PILOT.qjs_configure_args(
                    root / "source", root / "work", Path("evidence/quickjs-pilot"), target)
                self.assertTrue(link_map.is_absolute())
                self.assertIn(str(link_map), " ".join(args))
                if target == "windows/arm64":
                    self.assertEqual(args[args.index("-G") + 1], "Visual Studio 18 2026")
                    self.assertEqual(args[args.index("-A") + 1], "ARM64")

    def test_link_records_require_native_input_coverage(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            link_map = root / "qjs-link.map"
            build_log = root / "build.log"
            build_log.write_text("Searching C:\\sdk\\ucrt.lib\n", encoding="utf-8")
            link_map.write_text("Linker script and memory map\nLOAD qjs.o\nLOAD libqjs.a\n",
                                encoding="utf-8")
            PILOT.verify_link_records(link_map, build_log, "linux/amd64")
            link_map.write_text("qjs\nPublics by Value\n 0001:0000 main qjs.obj\n",
                                encoding="utf-8")
            PILOT.verify_link_records(link_map, build_log, "windows/arm64")
            build_log.write_text("no library inventory", encoding="utf-8")
            with self.assertRaisesRegex(RuntimeError, "library search record"):
                PILOT.verify_link_records(link_map, build_log, "windows/arm64")

    def test_fetch_requires_exact_size_and_sha(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = root / "input"
            output = root / "output"
            source.write_bytes(b"pinned-source")
            PILOT.fetch_exact(source.as_uri(), output, 13,
                              PILOT.sha256_bytes(b"pinned-source"))
            self.assertEqual(output.read_bytes(), b"pinned-source")
            with self.assertRaisesRegex(RuntimeError, "SHA-256 drift"):
                PILOT.fetch_exact(source.as_uri(), output, 13, "0" * 64)

    def test_tar_rejects_traversal_and_links(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for kind in ("traversal", "symlink"):
                archive = root / (kind + ".tar.gz")
                with tarfile.open(archive, "w:gz") as output:
                    for number in range(21):
                        payload = b"x"
                        member = tarfile.TarInfo(PILOT.SOURCE_PREFIX + "/safe-%02d" % number)
                        member.size = len(payload)
                        output.addfile(member, io.BytesIO(payload))
                    if kind == "traversal":
                        bad = tarfile.TarInfo(PILOT.SOURCE_PREFIX + "/../escape")
                        bad.size = 1
                        output.addfile(bad, io.BytesIO(b"x"))
                    else:
                        bad = tarfile.TarInfo(PILOT.SOURCE_PREFIX + "/escape")
                        bad.type = tarfile.SYMTYPE
                        bad.linkname = "../escape"
                        output.addfile(bad)
                with self.assertRaisesRegex(RuntimeError, "unsafe archive path|link or special"):
                    PILOT.unpack_source(archive, root / (kind + "-out"))
                self.assertFalse((root / "escape").exists())

    def test_machine_headers_reject_wrong_architecture(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "program"
            elf = bytearray(64)
            elf[:5] = b"\x7fELF\x02"
            struct.pack_into("<H", elf, 18, 62)
            path.write_bytes(elf)
            self.assertEqual(PILOT.machine_from_binary(path), "linux/amd64")
            struct.pack_into("<H", elf, 18, 183)
            path.write_bytes(elf)
            self.assertEqual(PILOT.machine_from_binary(path), "linux/arm64")
            pe = bytearray(128)
            pe[:2] = b"MZ"
            struct.pack_into("<I", pe, 0x3c, 80)
            pe[80:84] = b"PE\0\0"
            struct.pack_into("<H", pe, 84, 0xaa64)
            path.write_bytes(pe)
            self.assertEqual(PILOT.machine_from_binary(path), "windows/arm64")
            struct.pack_into("<H", pe, 84, 0x8664)
            path.write_bytes(pe)
            self.assertEqual(PILOT.machine_from_binary(path), "windows/amd64")

    def test_rejects_unreviewed_ejs_member(self):
        with tempfile.TemporaryDirectory() as temporary:
            archive = Path(temporary) / "yt-dlp.pyz"
            with zipfile.ZipFile(archive, "w") as output:
                output.writestr("yt_dlp_ejs/_version.py", "__version__ = version = '0.8.0'")
                for name in PILOT.EJS_SHA256:
                    output.writestr(name, "tampered")
            with self.assertRaisesRegex(RuntimeError, "SHA-256 drift"):
                PILOT.read_pinned_ejs(archive)


if __name__ == "__main__":
    unittest.main()
