---
id: f0261ec7cae0
kind: bug
title: 'S525.A5w-img bashy: publish ghcr.io/qiangli/bashy-scratch:<commit8> for every release (A5w-oci default sandbox image is unpullable)'
seq: 2845
status: assigned
priority: p0
labels:
    - app
    - release
created: 2026-10-30T00:35:41.681295Z
weave: 347
assignee: codex-gpt5.6-sol
sprint: 525
sprint_id: 9a9a4443-02c8-53a3-b0bd-96bd67980643
sprint_title: 'Software Shaping foundation: verified personal features and same-host promotion'
---

Part of S525.A5w (goal a5w). yoke contain.DefaultSandboxImage resolves a bare/argument-less @sandbox() island to ghcr.io/qiangli/bashy-scratch:<running release's commit8>, documented as 'the FROM-scratch image bashy self image builds, published every release'. Live check 2026-10-29 on elephant: the registry holds only tags v1.0.0-rc.15-dev and c119fd68 (ghcr tags/list); b5bf7a47 (rc.22) and the doc examples b84caaed/756b2c24/13d140c2 are 'manifest unknown'. So on every current release a zero-setup @sandbox() island fails to pull. bashy CI only runs 'make verify-bashy-scratch' (.github/workflows/test.yml); no workflow pushes the image. Fix (small): the release workflow builds the existing build-bashy-scratch / build-image target (linux amd64+arm64, multi-arch manifest) and pushes ghcr.io/qiangli/bashy-scratch:<commit8 of the release binary's self-reported commit> (and the release tag), public, permissive-licence image (FROM scratch, bashy only), signed like the release assets; a release gate fails if the tag is not pullable anonymously. Dev builds keep $BASHY_SANDBOX_IMAGE as the override. Proof: anonymous 'bashy oci pull ghcr.io/qiangli/bashy-scratch:<commit8>' for the next release on Linux + Windows. Live island proof already done with the published c119fd68 tag: bashy 6ce19453 'bashy oci run --rm --network=none ... uname' -> Linux, on elephant native Windows (machine auto-created) and WSL.

CARRY-FORWARD 2026-10-29 ~00:5xZ: the first run (bashy #346, codex) built the change and was then lost; its complete work is commit b0008572, kept at refs/salvage/abandoned-346 in the bashy repo (release.yml +96, test.yml, .goreleaser.yaml, docs/airgap-image.md, scripts/build-native-darwin-release.sh, scripts/test-bashy-scratch-release.py, tools/bashy-scratch/Containerfile). START by cherry-picking it (git cherry-pick b0008572; drop its KB.md hunk if it conflicts), review it, finish, commit with trailers incl. 'Agent: <you>'. Its focused checks (release-policy tests, actionlint, ELF/eligibility audits of both scratch artifacts) were green. DO NOT run 'bashy gate' or 'make test*' in the bashy repo on this dev host: bashy's gate is the FULL suite (go test ./internal/... ./tools/... && make test-bash), forbidden here; the manager stopped it once already.
