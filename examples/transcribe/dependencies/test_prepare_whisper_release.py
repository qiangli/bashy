import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest


SCRIPT = Path(__file__).with_name("prepare_whisper_release.py")
SPEC = importlib.util.spec_from_file_location("prepare_whisper_release", SCRIPT)
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)
COMMIT = "a" * 40


class PrepareWhisperReleaseTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        root = Path(self.temp.name)
        self.artifacts = root / "artifacts"
        self.output = root / "dist"
        self.notices = root / "WHISPER-NOTICES.txt"
        self.notices.write_text("Reviewed notice fixture\n")
        for label, (os_name, arch) in release.TARGETS.items():
            folder = self.artifacts / f"s503-native-build-{label}"
            folder.mkdir(parents=True)
            binary = folder / ("whisper-cli.exe" if os_name == "windows" else "whisper-cli")
            binary.write_bytes(f"{label} executable".encode())
            sha = release.digest(binary)
            report = {"platform": f"{os_name}/{arch}", "runner_label": label,
                      "whisper_version": "1.9.5", "source_archive_sha256": release.SOURCE_SHA256,
                      "status": "built_and_cached", "cpu_only": True, "portable_cpu": True,
                      "openmp_disabled": True, "gpl_free": True, "product_admitted": False,
                      "executable_sha256": sha, "executable_bytes": binary.stat().st_size}
            if os_name == "windows":
                link_map = folder / "whisper-cli.link.map"
                link_map.write_text("lld map fixture\n")
                inventory = {"binary_sha256": sha, "map_sha256": release.digest(link_map),
                             "archive_members": [{"archive": "libx.a", "member": "x.o"}]}
                (folder / "whisper-link-inventory.json").write_text(json.dumps(inventory))
                report["link_map_inventory"] = inventory
            (folder / "build-report.json").write_text(json.dumps(report))
            (folder / "runner-identity.json").write_text(json.dumps({
                "runner_label": label, "actual_arch": arch,
                "actual_os": {"linux": "Linux", "darwin": "macOS", "windows": "Windows"}[os_name],
                "translated": "0", "commit_sha": COMMIT}))

    def test_exact_six_assets_notice_manifest_and_sums(self):
        release.stage(self.artifacts, self.notices, self.output, COMMIT)
        manifest = json.loads((self.output / "whisper-release-manifest.json").read_text())
        self.assertEqual(len(manifest["targets"]), 6)
        self.assertFalse(any(t["product_admitted"] for t in manifest["targets"]))
        rows = (self.output / "SHA256SUMS").read_text().splitlines()
        self.assertEqual(len(rows), 9)
        for row in rows:
            sha, name = row.split("  ")
            self.assertEqual(sha, hashlib.sha256((self.output / name).read_bytes()).hexdigest())

    def test_publication_requires_admission(self):
        with self.assertRaisesRegex(ValueError, "product admission"):
            release.stage(self.artifacts, self.notices, self.output, COMMIT, require_admitted=True)

    def test_windows_linker_map_must_match_binary(self):
        path = self.artifacts / "s503-native-build-windows-2025" / "whisper-cli.link.map"
        path.write_text("changed\n")
        with self.assertRaisesRegex(ValueError, "linker evidence"):
            release.stage(self.artifacts, self.notices, self.output, COMMIT)

    def test_all_six_targets_required(self):
        path = self.artifacts / "s503-native-build-macos-15" / "build-report.json"
        path.unlink()
        with self.assertRaises(FileNotFoundError):
            release.stage(self.artifacts, self.notices, self.output, COMMIT)


if __name__ == "__main__":
    unittest.main()
