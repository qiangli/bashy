#!/usr/bin/env python3
"""Release-package contract tests that require no GoReleaser installation."""

import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


class LinuxPackageContractTest(unittest.TestCase):
    def test_nfpm_packages_all_linux_products_in_each_required_format(self):
        config = (ROOT / ".goreleaser.yaml").read_text()
        self.assertRegex(config, r"(?m)^nfpms:\s*$")
        self.assertRegex(config, r"(?s)nfpms:.*?ids:\s*\[bashy, bash, sh, outpost\]")
        self.assertRegex(config, r"(?s)nfpms:.*?formats:\s*\n\s*- deb\s*\n\s*- rpm\s*\n\s*- apk")
        self.assertRegex(config, r"(?s)nfpms:.*?license:\s*[\"']?BSD-3-Clause AND MIT")
        self.assertRegex(config, r"(?s)nfpms:.*?postinstall:\s*./scripts/linux-package-postinstall.sh")

    def test_postinstall_never_starts_or_enables_a_service(self):
        script = (ROOT / "scripts/linux-package-postinstall.sh").read_text()
        forbidden = r"\b(systemctl|service|rc-service|start|enable)\b"
        self.assertIsNone(re.search(forbidden, script, re.IGNORECASE), script)

    def test_release_workflow_lists_and_checks_package_contents(self):
        workflow = (ROOT / ".github/workflows/release.yml").read_text()
        self.assertIn("./scripts/verify-linux-packages.sh dist", workflow)
        verifier = (ROOT / "scripts/verify-linux-packages.sh").read_text()
        for command in ("dpkg-deb --contents", "rpm -qlp", "tar -tzf"):
            self.assertIn(command, verifier)
        for binary in ("bashy", "bash", "sh", "outpost"):
            self.assertIn(f"usr/lib/bashy/bin/{binary}", verifier)
        self.assertIn("forbidden='usr/bin/bash usr/bin/sh bin/bash bin/sh'", verifier)

    def test_packages_never_own_the_distribution_shells(self):
        config = (ROOT / ".goreleaser.yaml").read_text()
        self.assertRegex(config, r"(?s)nfpms:.*?bindir:\s*/usr/lib/bashy/bin")
        self.assertNotRegex(config, r"(?s)nfpms:.*?bindir:\s*/usr/bin\s*$")
        for binary in ("bashy", "outpost"):
            self.assertRegex(config, rf"(?s)src:\s*/usr/lib/bashy/bin/{binary}\s*\n\s*dst:\s*/usr/bin/{binary}\s*\n\s*type:\s*symlink")


class PublicReleaseClaimsTest(unittest.TestCase):
    def test_readme_distinguishes_darwin_release_from_cgo_free_builds(self):
        readme = (ROOT / "README.md").read_text()
        darwin = (ROOT / "scripts/build-native-darwin-release.sh").read_text()
        self.assertIn("CGO_ENABLED=1", darwin)
        self.assertNotIn("one static binary — no CGo", readme)
        self.assertIn("pre-Go C constructor", readme)
        self.assertIn("Darwin", readme)

    def test_current_yash_claims_link_evidence_instead_of_a_percentage(self):
        for name in ("README.md", "docs/TODO.md", "docs/conformance-statement.md",
                     "docs/bashy-v1.0.0-readiness.md", "docs/verify-conformance-subcommands.md"):
            with self.subTest(file=name):
                text = (ROOT / name).read_text()
                self.assertIn("yash-chunks.json", text)
                self.assertNotRegex(text, r"(?i)yash[^\n]*[0-9]+\s*%")

    def test_current_job_control_claims_do_not_repeat_retired_unix_limits(self):
        retired = ("some interactive job-control behavior remains incomplete",
                   "notifications are non-functional", "can't own the controlling")
        for name in ("README.md", "docs/conformance-statement.md",
                     "docs/vsc-pcts-readiness.md", "docs/bashy-v1.0.0-readiness.md"):
            with self.subTest(file=name):
                text = (ROOT / name).read_text()
                for claim in retired:
                    self.assertNotIn(claim, text)
                self.assertIn("ExtraFiles", text)

    def test_transpile_toolchain_claim_matches_module(self):
        module = (ROOT / "go.mod").read_text()
        doc = (ROOT / "docs/transpile.md").read_text()
        version = re.search(r"(?m)^go (\S+)$", module).group(1)
        toolchain = re.search(r"(?m)^toolchain (\S+)$", module).group(1)
        self.assertIn(f"`go {version}`", doc)
        self.assertIn(f"`toolchain {toolchain}`", doc)
        self.assertNotIn("Go 1.26.5 compatibility floor", doc)


if __name__ == "__main__":
    unittest.main()
