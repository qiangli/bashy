#!/usr/bin/env python3
"""Offline contract tests for the release-published bashy scratch image."""

import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


class ScratchImageReleaseContractTest(unittest.TestCase):
    def test_release_retains_scratch_binaries_without_registry_publish(self):
        workflow = (ROOT / ".github/workflows/release.yml").read_text()
        self.assertNotIn("scratch-image:", workflow)
        self.assertNotIn("ghcr.io/qiangli/bashy-scratch", workflow)
        assets = (ROOT / "scripts/verify-release-asset-set.sh").read_text()
        self.assertIn("bashy-scratch-linux-amd64", assets)
        self.assertIn("bashy-scratch-linux-arm64", assets)

    def test_stable_promotion_does_not_republish_registry_images(self):
        workflow = (ROOT / ".github/workflows/promote.yml").read_text()
        self.assertNotIn("docker buildx imagetools", workflow)
        self.assertNotIn("packages: write", workflow)
        self.assertNotIn("ghcr.io/qiangli/bashy-scratch", workflow)
        self.assertIn("dist/bashy-*", workflow)

    def test_image_is_from_scratch_with_only_bashy_as_payload(self):
        containerfile = (ROOT / "tools/bashy-scratch/Containerfile").read_text()
        self.assertRegex(containerfile, r"(?m)^FROM scratch$")
        copies = re.findall(r"(?m)^COPY\s+(.+)$", containerfile)
        self.assertEqual(copies, ["bashy-linux-${TARGETARCH} /bashy"])
        self.assertIn(
            'org.opencontainers.image.licenses="BSD-3-Clause"', containerfile
        )
        self.assertIn('ENTRYPOINT ["/bashy"]', containerfile)

    def test_native_binary_reports_the_full_source_commit(self):
        darwin = (ROOT / "scripts/build-native-darwin-release.sh").read_text()
        self.assertIn("source_commit=$(git rev-parse HEAD)", darwin)
        self.assertIn("github.com/bashsharp/bashsharp/cli.buildID=$source_commit", darwin)


if __name__ == "__main__":
    unittest.main()
