"""Policy checks for the evidence-only Linux musl linker inventory."""

import tempfile
import unittest
from pathlib import Path

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

    def inspect(self, path: Path, retain=False):
        path.write_bytes(b"input")
        self.map_file.write_text(
            "VMA LMA Size Align Out In Symbol\n" +
            f"    1000 1000 10 4 {path}(member.o):(.text)\n")
        return probe.check_link_inputs(self.map_file, str(path) + "\n",
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

    def test_host_archive_is_refused(self):
        host = self.root / "host"
        host.mkdir()
        with self.assertRaisesRegex(RuntimeError, "outside verified"):
            self.inspect(host / "libc.a")

    def test_single_undefined_symbol_is_refused(self):
        probe.assert_no_undefined_symbols("$ nm -u whisper-cli\n")
        with self.assertRaisesRegex(RuntimeError, "undefined symbols"):
            probe.assert_no_undefined_symbols("$ nm -u whisper-cli\n                 U _Unwind_Resume\n")


if __name__ == "__main__":
    unittest.main()
