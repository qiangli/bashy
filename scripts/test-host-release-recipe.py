#!/usr/bin/env python3
"""S451.5a release contract: the host build must be distinct and signed."""

from pathlib import Path
import unittest


ROOT = Path(__file__).resolve().parents[1]


class HostReleaseRecipeTest(unittest.TestCase):
    def test_engine_host_asset_is_distinct_from_lean_release(self):
        workflow_path = ROOT / ".github/workflows/host-release.yml"
        self.assertTrue(workflow_path.exists(), "missing separate host release workflow")
        workflow = workflow_path.read_text()
        for value in ("source_sha", "BASHY_ENGINES: '1'", "CGO_ENABLED: '1'", "arch: amd64", "arch: arm64", "build-host-release.sh"):
            with self.subTest(value=value):
                self.assertTrue(value in workflow, f"host workflow lacks {value}")
        builder = (ROOT / "scripts/build-host-release.sh").read_text()
        self.assertIn("-tags bashy_engines,embed_runner,bashy_host", builder)
        recipe = (ROOT / ".goreleaser.yaml").read_text()
        lean = recipe.split("  - id: bashy\n", 1)[1].split("\n  - id:", 1)[0]
        self.assertIn("CGO_ENABLED=0", lean)
        self.assertNotIn("bashy_engines", lean)

    def test_host_asset_has_separate_manifest_and_signature_gate(self):
        workflow_path = ROOT / ".github/workflows/host-release.yml"
        self.assertTrue(workflow_path.exists(), "missing separate host release workflow")
        workflow = workflow_path.read_text()
        for value in ("cosign sign-blob", "cosign verify-blob", "host-asset-manifest", "tools/verifyhost"):
            with self.subTest(value=value):
                self.assertTrue(value in workflow, f"host workflow lacks {value}")
        builder = (ROOT / "scripts/build-host-release.sh").read_text()
        self.assertIn("generate-release-sbom.sh", builder)
        verifier = (ROOT / "tools/verifyhost/main.go").read_text()
        self.assertIn("binmgr.VerifySignedBlob", verifier)


if __name__ == "__main__":
    unittest.main()
