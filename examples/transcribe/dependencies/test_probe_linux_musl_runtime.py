"""Policy checks for the evidence-only Linux musl linker inventory."""

import tempfile
import unittest
import os
import stat
from pathlib import Path
from unittest.mock import patch

import probe_linux_musl_runtime as probe


class LinkInputPolicyTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.build = self.root / "build"
        self.zig = self.root / "zig"
        self.cache = self.root / "cache"
        for directory in (self.build, self.zig, self.cache):
            directory.mkdir()
        self.map_file = self.root / "link.map"

    def inspect(self, path: Path, retain=False, member="member.o"):
        path.write_bytes(b"input")
        self.map_file.write_text(
            "VMA LMA Size Align Out In Symbol\n" +
            f"    1000 1000 10 4 {path}({member}):(.text)\n")
        link_log = f"ld.lld -o {self.build}/bin/whisper-cli {path}\n"
        return probe.check_link_inputs(self.map_file, link_log,
                                       self.build, self.zig, self.cache,
                                       self.root / "evidence" / "inputs" if retain else None)

    def test_musl_archive_has_source_anchor(self):
        result = self.inspect(self.cache / "libc.a", retain=True)
        self.assertEqual(result["inputs"][0]["license_anchor"],
                         "lib/libc/musl/COPYRIGHT")
        self.assertEqual(result["inputs"][0]["members"], ["member.o"])
        self.assertEqual((self.root / "evidence" /
                          result["inputs"][0]["retained_path"]).read_bytes(), b"input")

    def test_gcc_archive_is_refused(self):
        with self.assertRaisesRegex(RuntimeError, "GCC runtime"):
            self.inspect(self.cache / "libgcc.a")

    def test_unclassified_archive_is_refused(self):
        with self.assertRaisesRegex(RuntimeError, "unclassified"):
            self.inspect(self.cache / "libunknown.a")

    def test_zigc_requires_the_reviewed_member_and_retains_source_identity(self):
        archive = self.cache / "libzigc.a"
        with self.assertRaisesRegex(RuntimeError, "unreviewed libzigc map members"):
            self.inspect(archive)
        listed = probe.subprocess.CompletedProcess(["ar", "t", str(archive)], 0,
                                                    "libzigc_zcu.o\n", "")
        with patch.object(probe.shutil, "which", return_value="/usr/bin/ar"), \
             patch.object(probe.subprocess, "run", return_value=listed):
            result = self.inspect(archive, retain=True,
                                  member=str(self.cache / "libzigc_zcu.o"))
        item = result["inputs"][0]
        self.assertEqual(item["category"], "zig-libc-compat")
        self.assertEqual(item["license_anchor"], "LICENSE")
        self.assertEqual(item["archive_members"], ["libzigc_zcu.o"])
        self.assertEqual(set(item["source_anchors"]), set(probe.ZIGC_SOURCES))
        self.assertEqual((self.root / "evidence" / item["retained_path"]).read_bytes(), b"input")
        listed.stdout = "other.o\n"
        with patch.object(probe.shutil, "which", return_value="/usr/bin/ar"), \
             patch.object(probe.subprocess, "run", return_value=listed):
            with self.assertRaisesRegex(RuntimeError, "unreviewed libzigc archive members"):
                self.inspect(archive, member="libzigc_zcu.o")

    def test_host_archive_is_refused(self):
        host = self.root / "host"
        host.mkdir()
        with self.assertRaisesRegex(RuntimeError, "outside verified"):
            self.inspect(host / "libc.a")

    def test_single_undefined_symbol_is_refused(self):
        probe.assert_no_undefined_symbols("$ nm -u whisper-cli\n")
        with self.assertRaisesRegex(RuntimeError, "undefined symbols"):
            probe.assert_no_undefined_symbols("$ nm -u whisper-cli\n                 U _Unwind_Resume\n")

    def test_only_local_hidden_musl_dynamic_marker_is_allowed(self):
        nm = "$ nm -u whisper-cli\n                 U _DYNAMIC\n"
        symbols = "$ readelf -s whisper-cli\n     2: 0000000000000000     0 NOTYPE  LOCAL  HIDDEN   UND _DYNAMIC\n"
        self.assertEqual(probe.assert_no_undefined_symbols(nm, symbols),
                         "musl-static-local-hidden-_DYNAMIC")
        with self.assertRaisesRegex(RuntimeError, "exact local-hidden"):
            probe.assert_no_undefined_symbols(nm, symbols.replace("HIDDEN", "DEFAULT"))
        with self.assertRaisesRegex(RuntimeError, "undefined symbols"):
            probe.assert_no_undefined_symbols(nm + "                 U _Unwind_Resume\n", symbols)
        with self.assertRaisesRegex(RuntimeError, "absent from nm"):
            probe.assert_no_undefined_symbols("$ nm -u whisper-cli\n", symbols)

    def test_retained_cli_keeps_executable_mode_and_exact_bytes(self):
        source = self.build / "whisper-cli"
        retained = self.root / "evidence" / "whisper-cli"
        retained.parent.mkdir()
        sentinel = retained.parent / "notice.txt"
        source.write_bytes(b"\x7fELF\x00example")
        source.chmod(0o751)
        sentinel.write_bytes(b"notice")
        mode = probe.retain_executable(source, retained)
        self.assertEqual(mode, 0o751)
        self.assertEqual(retained.read_bytes(), source.read_bytes())
        self.assertEqual(stat.S_IMODE(retained.stat().st_mode), 0o751)
        self.assertTrue(os.access(retained, os.X_OK))
        self.assertEqual(stat.S_IMODE(source.stat().st_mode), 0o751)
        self.assertEqual(sentinel.read_bytes(), b"notice")

    def test_supported_print_map_is_extracted_from_final_build_log(self):
        source = (
            "$ cmake --build ...\n"
            "ld.lld -o /tmp/bin/whisper-cli /tmp/libc.a\n"
            "VMA LMA Size Align Out In Symbol\n"
            "1000 1000 10 4 .text\n"
            "1000 1000 10 4 /tmp/libc.a(start.o):(.text)\n"
            "1010 1010 8 4 .data\n"
            "1010 1010 8 4 /tmp/libc.a(start.o):(.data)\n"
            "ninja: build complete\n"
        )
        probe.extract_printed_map(source, self.map_file)
        extracted = self.map_file.read_text()
        self.assertIn("/tmp/libc.a(start.o)", extracted)
        self.assertNotIn("ninja: build complete", extracted)
        with self.assertRaisesRegex(RuntimeError, "one final LLD"):
            probe.extract_printed_map("no map", self.map_file)
        with self.assertRaisesRegex(RuntimeError, "found 2"):
            probe.extract_printed_map(source + source, self.map_file)

    def test_link_command_must_identify_direct_inputs_without_library_aliases(self):
        preflight = "ld.lld -o /tmp/flag-preflight /tmp/start.o\n"
        command, inputs = probe.verbose_link_inputs(preflight, "flag-preflight")
        self.assertEqual(command, preflight.strip())
        self.assertEqual(inputs, {"/tmp/start.o"})
        with self.assertRaisesRegex(RuntimeError, "unresolved -l"):
            probe.verbose_link_inputs("ld.lld -o /tmp/bin/whisper-cli -lstdc++ /tmp/start.o\n")
        with self.assertRaisesRegex(RuntimeError, "no object or archive inputs"):
            probe.verbose_link_inputs("ld.lld -o /tmp/bin/whisper-cli\n")


if __name__ == "__main__":
    unittest.main()
