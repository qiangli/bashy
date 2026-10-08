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


if __name__ == "__main__":
    unittest.main()
