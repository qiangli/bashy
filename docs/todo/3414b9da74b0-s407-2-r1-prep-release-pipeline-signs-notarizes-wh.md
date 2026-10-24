---
id: 3414b9da74b0
kind: feature
title: 'S407.2 R1 prep: release pipeline signs + notarizes when credentials exist (macOS Developer ID + notarytool, Windows Azure Artifact Signing), skips loudly when absent; verify scripts'
seq: 1946
status: todo
priority: p1
labels:
    - release
    - signing
created: 2026-10-24T22:57:39.758015Z
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
---

Repo: bashy (public; no hostnames, no secrets, no proprietary references). Part of R1 98bc74b37f07, which stays open until real certificates sign a published release.

Decided (s407-manager, 2026-10-24): macOS = Apple Developer ID Application certificate of the organization dhnt, Inc. + notarization with `xcrun notarytool` (App Store Connect API key). Windows = Azure Artifact Signing (formerly Trusted Signing), public-trust, via the official GitHub Action with OIDC (azure/login federated credential), no hardware token. Credentials do not exist yet.

Do:
1. In .github/workflows/release.yml (and promote.yml only if it re-packages bytes; promote must copy tested bytes unchanged), add signing steps for the Darwin binaries (bashy, outpost, bash, sh; codesign --options runtime --timestamp, then notarytool submit --wait on a zip; stapling is not possible for bare Mach-O, document that) and the Windows .exe binaries, BEFORE archives, checksums and SHA256 sidecars are computed, so checksums cover signed bytes.
2. Gate every signing step on the presence of its secrets/vars (names: APPLE_DEVELOPER_ID_P12_BASE64, APPLE_DEVELOPER_ID_P12_PASSWORD, APPLE_NOTARY_KEY_ID, APPLE_NOTARY_ISSUER_ID, APPLE_NOTARY_KEY_P8; AZURE_CLIENT_ID, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID, ARTIFACT_SIGNING_ENDPOINT, ARTIFACT_SIGNING_ACCOUNT, ARTIFACT_SIGNING_PROFILE). When absent the job prints a single 'UNSIGNED: <reason>' notice and continues (dev tags must still publish); add a release-notes/manifest field signed=true|false per platform so nothing claims signing by absence of evidence.
3. Add scripts/verify-signatures.sh (macOS: codesign --verify --strict + spctl --assess --type execute where applicable; Windows: Get-AuthenticodeSignature via a .ps1 or signtool verify /pa) that a QA lane runs against downloaded published bytes and that fails closed when signed=true but verification fails.
4. Document the secret names and the one-time setup in docs/release-runbook.md (short section).
Acceptance: actionlint/yamllint clean; a dry run with no secrets shows UNSIGNED notices and an otherwise unchanged asset set (27 assets, same names); unit or script test for the manifest signed flag; no change to promote's byte-copy guarantee. Commit with Sprint/Story/Story-ID trailers; do not push tags.
