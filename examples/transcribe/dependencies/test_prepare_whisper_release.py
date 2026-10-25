import hashlib
import importlib.util
import json
from pathlib import Path
import tarfile
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
        pins = json.loads(release.NOTICE_PINS.read_text())
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
            if os_name in ("windows", "linux"):
                link_map = folder / "whisper-cli.link.map"
                link_map.write_text("lld map fixture\n")
                inventory = {"binary_sha256": sha, "map_sha256": release.digest(link_map),
                             "map_bytes": link_map.stat().st_size, "binary_name": binary.name,
                             "schema": "s503-linux-link-map-v1" if os_name == "linux" else "s503-windows-link-map-v1",
                             "archive_members": [{"archive": "libx.a", "member": "x.o"}],
                             "runtime_archive_members": [{"archive": "libx.a", "member": "x.o"}]}
                (folder / "whisper-link-inventory.json").write_text(json.dumps(inventory))
                report["link_map_inventory"] = inventory
            if os_name == "windows":
                asset = pins["windows_toolchain"]["assets"][arch]
                (folder / "toolchain-report.json").write_text(json.dumps({
                    "platform": f"{os_name}/{arch}", "source_url": asset["url"],
                    "archive_sha256": asset["archive_sha256"], "archive_bytes": asset["archive_bytes"]}))
            (folder / "build-report.json").write_text(json.dumps(report))
            (folder / "runner-identity.json").write_text(json.dumps({
                "runner_label": label, "actual_arch": arch,
                "actual_os": {"linux": "Linux", "darwin": "macOS", "windows": "Windows"}[os_name],
                "translated": "0", "commit_sha": COMMIT}))
            notice_dir = folder / "whisper-notices"
            licenses = notice_dir / "LICENSES"
            licenses.mkdir(parents=True)
            selected = [name for name, row in pins["notices"].items()
                        if row["targets"] == "all" or (os_name == "windows" and row["targets"] == "windows")]
            sections = ["Whisper executable third-party notices\n",
                        "Candidate notice bundle; product admission and license review remain open.\n"]
            notice_rows = []
            for name in selected:
                pin = pins["notices"][name]
                data = (release.NOTICE_PINS.parent / "sources" / name).read_bytes()
                (licenses / name).write_bytes(data)
                sections.append(f"\n===== {name} =====\nSource: {pin['url']}\nSHA-256: {pin['sha256']}\n\n")
                sections.append(data.decode("utf-8"))
                if not sections[-1].endswith("\n"):
                    sections.append("\n")
                notice_rows.append({"file": f"LICENSES/{name}", "source_url": pin["url"],
                                    "sha256": pin["sha256"]})
            gaps = ["Map linked Whisper and ggml objects to exact source files."]
            if os_name == "windows":
                gaps.extend(["Resolve Cephes license uncertainty.", "Verify source-to-prebuilt llvm-mingw provenance."])
            elif os_name == "linux":
                gaps.append("Review GPLv3 GCC Runtime Library Exception.")
            else:
                gaps.append("Verify macOS linked system framework notices.")
            sections.append("\nOpen review gates:\n" + "".join(f"- {gap}\n" for gap in gaps))
            bundle = notice_dir / "THIRD_PARTY_NOTICES.txt"
            bundle.write_text("".join(sections))
            evidence = {"build_report_sha256": release.digest(folder / "build-report.json")}
            if os_name in ("windows", "linux"):
                evidence.update({"inventory_sha256": release.digest(folder / "whisper-link-inventory.json"),
                                 "map_sha256": release.digest(folder / "whisper-cli.link.map")})
            if os_name == "windows":
                evidence.update({"toolchain_report_sha256": release.digest(folder / "toolchain-report.json"),
                                 "toolchain_asset": pins["windows_toolchain"]["assets"][arch]})
            (notice_dir / "NOTICE_MANIFEST.json").write_text(json.dumps({
                "schema": "s503-whisper-notice-bundle-v1", "status": "candidate_product_not_admitted",
                "target": f"{os_name}/{arch}", "binary_sha256": sha,
                "binary_bytes": binary.stat().st_size, "source_archive_sha256": release.SOURCE_SHA256,
                "source_pins_sha256": release.digest(release.NOTICE_PINS),
                "bundle_sha256": release.digest(bundle), "evidence": evidence,
                "notices": notice_rows, "open_gates": gaps}))

    def test_exact_six_assets_notice_manifest_and_sums(self):
        release.stage(self.artifacts, self.output, COMMIT)
        manifest = json.loads((self.output / "whisper-release-manifest.json").read_text())
        self.assertEqual(len(manifest["targets"]), 6)
        self.assertEqual(len(manifest["notice_targets"]), 6)
        self.assertFalse(any(t["product_admitted"] for t in manifest["targets"]))
        rows = (self.output / "SHA256SUMS").read_text().splitlines()
        self.assertEqual(len(rows), 21)
        for row in rows:
            sha, name = row.split("  ")
            self.assertEqual(sha, hashlib.sha256((self.output / name).read_bytes()).hexdigest())
        self.assertIn("windows/amd64", (self.output / "WHISPER-NOTICES.txt").read_text())
        self.assertIn("MinGW-w64 runtime licensing", (self.output / "WHISPER-NOTICES-windows-amd64.txt").read_text())
        with tarfile.open(self.output / "whisper-build-evidence.tar.gz") as archive:
            names = archive.getnames()
        self.assertEqual(len(names), len(set(names)))
        self.assertIn("s503-native-build-windows-2025/whisper-notices/LICENSES/mingw-w64-runtime.txt", names)
        self.assertIn("s503-native-build-ubuntu-24.04/whisper-cli.link.map", names)
        self.assertIn("s503-native-build-ubuntu-24.04-arm/whisper-link-inventory.json", names)

    def test_publication_requires_admission(self):
        with self.assertRaisesRegex(ValueError, "product admission"):
            release.stage(self.artifacts, self.output, COMMIT, require_admitted=True)

    def test_windows_linker_map_must_match_binary(self):
        path = self.artifacts / "s503-native-build-windows-2025" / "whisper-cli.link.map"
        path.write_text("changed\n")
        with self.assertRaisesRegex(ValueError, "linker evidence"):
            release.stage(self.artifacts, self.output, COMMIT)

    def test_linux_linker_map_must_match_notice_manifest(self):
        path = self.artifacts / "s503-native-build-ubuntu-24.04" / "whisper-cli.link.map"
        path.write_text("changed\n")
        with self.assertRaisesRegex(ValueError, "linker evidence"):
            release.stage(self.artifacts, self.output, COMMIT)

    def test_all_six_targets_required(self):
        path = self.artifacts / "s503-native-build-macos-15" / "build-report.json"
        path.unlink()
        with self.assertRaises(FileNotFoundError):
            release.stage(self.artifacts, self.output, COMMIT)

    def test_missing_or_tampered_target_notice_rejected(self):
        notice = self.artifacts / "s503-native-build-windows-2025" / "whisper-notices" / "LICENSES" / "mingw-w64-runtime.txt"
        notice.write_text("tampered")
        with self.assertRaisesRegex(ValueError, "notice source mismatch"):
            release.stage(self.artifacts, self.output, COMMIT)

    def test_binary_bound_notice_manifest_required(self):
        manifest = self.artifacts / "s503-native-build-macos-15" / "whisper-notices" / "NOTICE_MANIFEST.json"
        data = json.loads(manifest.read_text())
        data["binary_sha256"] = "0" * 64
        manifest.write_text(json.dumps(data))
        with self.assertRaisesRegex(ValueError, "notice manifest differs"):
            release.stage(self.artifacts, self.output, COMMIT)

    def test_mandatory_linux_license_gate_required(self):
        folder = self.artifacts / "s503-native-build-ubuntu-24.04" / "whisper-notices"
        manifest = folder / "NOTICE_MANIFEST.json"
        bundle = folder / "THIRD_PARTY_NOTICES.txt"
        data = json.loads(manifest.read_text())
        data["open_gates"] = ["Map linked Whisper and ggml objects to exact source files."]
        bundle.write_text(bundle.read_text().replace("- Review GPLv3 GCC Runtime Library Exception.\n", ""))
        data["bundle_sha256"] = release.digest(bundle)
        manifest.write_text(json.dumps(data))
        with self.assertRaisesRegex(ValueError, "mandatory notice review gate"):
            release.stage(self.artifacts, self.output, COMMIT)


if __name__ == "__main__":
    unittest.main()
