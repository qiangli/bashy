# Prima verified build — Sprint 436 / #2029

Story `17ba375e3318`. The operator's 2026-10-13 decision admits
`qiangli/prima.cpp` as first-party MIT source, including the nng ring patches.
The official third-party upstream-release requirement is inapplicable; the
other supply-chain gates remain mandatory.

1. Review source after `1ea4707ea4`, pin the first-party tag and full commit.
2. Add the CMake kind and generate a four-platform hosted workflow.
3. Audit the actual compiler inputs from all four builds, dependency licenses
   and vulnerabilities; publish SBOM, notices and audit reports.
4. Require bounded inference and ring traffic checks before keyless signing.
5. Deliver the exact raw-asset/SHA/Sigstore contract to the conductor and
   consumer worker. Keep consumers closed until hosted CI and real verification
   pass. The worker commits on its current branch; the conductor installs and
   publishes the generated tree.

The earlier `539f6b2d` inspection was reviewed for source, dependency and
consumer findings. Its blocked manifest and official-tag requirement are
superseded by `builds/prima.cpp/tool.env` and the operator decision. Do not
cherry-pick its stale admission workflow or manifest on top of this delivery.

The umbrella policy exception is separately assigned under
`f095c0b50f09`; recorded worker commit `48d53363f`. Binmgr preparation is
separately assigned under `90ea12d9ef68`, worker commit `8d9dbad1`.
The conductor owns integration and CI, including any follow-up fixes prompted
by the hosted evidence. No dev-host native build, model exercise or container
is part of this resumed worker's validation.
