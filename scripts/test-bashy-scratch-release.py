#!/usr/bin/env python3
"""Offline contract tests for the release-published bashy scratch image."""

import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[1]


class ScratchImageReleaseContractTest(unittest.TestCase):
    def test_release_publishes_and_gates_the_public_multiarch_image(self):
        workflow = (ROOT / ".github/workflows/release.yml").read_text()
        for contract in (
            "make --no-print-directory build-bashy-scratch",
            "platforms: linux/amd64,linux/arm64",
            "ghcr.io/qiangli/bashy-scratch:${{ steps.identity.outputs.commit8 }}",
            "ghcr.io/qiangli/bashy-scratch:${{ github.ref_name }}",
            "subject-digest: ${{ steps.push.outputs.digest }}",
            "push-to-registry: true",
            'DOCKER_CONFIG="$anon" docker pull',
        ):
            self.assertIn(contract, workflow)
        self.assertRegex(
            workflow,
            r"needs: \[darwin, goreleaser, windows-sign, otel, scratch-image\]",
        )

    def test_stable_promotion_retags_the_exact_candidate_manifest(self):
        workflow = (ROOT / ".github/workflows/promote.yml").read_text()
        for contract in (
            "packages: write",
            'source_raw=$(DOCKER_CONFIG="$anon" docker buildx imagetools inspect --raw "$IMAGE:$commit8")',
            'candidate_raw=$(DOCKER_CONFIG="$anon" docker buildx imagetools inspect --raw "$IMAGE:$DEV")',
            'docker buildx imagetools create --tag "$IMAGE:$VER" "$IMAGE:$commit8"',
            'stable_raw=$(DOCKER_CONFIG="$anon" docker buildx imagetools inspect --raw "$IMAGE:$VER")',
            'DOCKER_CONFIG="$anon" docker pull --platform linux/amd64 "$IMAGE:$VER"',
        ):
            self.assertIn(contract, workflow)
        self.assertGreaterEqual(workflow.count('test "$candidate_raw" = "$source_raw"'), 1)
        self.assertGreaterEqual(workflow.count('test "$stable_raw" = "$source_raw"'), 1)

    def test_image_is_from_scratch_with_only_bashy_as_payload(self):
        containerfile = (ROOT / "tools/bashy-scratch/Containerfile").read_text()
        self.assertRegex(containerfile, r"(?m)^FROM scratch$")
        copies = re.findall(r"(?m)^COPY\s+(.+)$", containerfile)
        self.assertEqual(copies, ["bashy-linux-${TARGETARCH} /bashy"])
        self.assertIn(
            'org.opencontainers.image.licenses="BSD-3-Clause"', containerfile
        )
        self.assertIn('ENTRYPOINT ["/bashy"]', containerfile)

    def test_release_binaries_report_the_full_source_commit(self):
        config = (ROOT / ".goreleaser.yaml").read_text()
        stamp = "github.com/qiangli/bashy/internal/cli.buildID={{ .FullCommit }}"
        self.assertGreaterEqual(config.count(stamp), 2)
        darwin = (ROOT / "scripts/build-native-darwin-release.sh").read_text()
        self.assertIn("source_commit=$(git rev-parse HEAD)", darwin)
        self.assertIn("internal/cli.buildID=$source_commit", darwin)


if __name__ == "__main__":
    unittest.main()
