# Bashy release runbook

Before publishing release notes, apply the [public-claim checks](public-claims-evidence.md).
Use the actual candidate tag from `bashy --version`, Bash compatibility 5.3,
and the Go 1.27 / toolchain go1.27.2 build coordinate; never substitute a
historical report's version or percentage.

The candidate release is built once from a `vX.Y.Z-dev` tag. Promotion reuses
those tested bytes; it does not rebuild them. The repository knowledge record
`kb:release-bashy` owns the full candidate, native-QA and promotion sequence.

## Platform signing setup

Release candidates still publish when credentials are absent. The workflow emits
one `UNSIGNED: <reason>` notice per affected platform and writes
`Signing: darwin.signed=true|false windows.signed=true|false` in the candidate notes. Promotion
copies this status line and every tested asset byte unchanged. A missing flag is
a QA failure, never evidence of signing. The source release currently has 39
assets; signing adds no asset names.

For macOS, enroll dhnt, Inc. in the Apple Developer Program, issue a
**Developer ID Application** certificate, export its `.p12`, and create an App
Store Connect API key with notarization access. Set these GitHub Actions
secrets: `APPLE_DEVELOPER_ID_P12_BASE64` (base64 of the `.p12`),
`APPLE_DEVELOPER_ID_P12_PASSWORD`, `APPLE_NOTARY_KEY_ID`,
`APPLE_NOTARY_ISSUER_ID`, and `APPLE_NOTARY_KEY_P8` (the private key text).
The native macOS job imports the certificate into a temporary keychain, signs
`bashy`, `outpost`, `bash`, and `sh` with hardened runtime and a timestamp,
then submits a temporary ZIP through `xcrun notarytool submit --wait` before
building the release tarballs. Bare Mach-O files cannot carry a stapled ticket;
Gatekeeper may need network access to check Apple's notary service.

For Windows, create an Azure Artifact Signing Public Trust account and
certificate profile, grant the federated app the **Artifact Signing Certificate
Profile Signer** role, and add a GitHub OIDC federated credential scoped to this
release workflow. Set GitHub Actions secrets `AZURE_CLIENT_ID`,
`AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` and repository variables
`ARTIFACT_SIGNING_ENDPOINT`, `ARTIFACT_SIGNING_ACCOUNT`,
`ARTIFACT_SIGNING_PROFILE`. The Windows job uses `azure/login` OIDC and the
official Azure Artifact Signing action. It signs all four PE files for each
architecture, then assembles the final ZIPs and raw outpost files. The publish
job computes SHA-256 sidecars and the complete checksum manifest after signing.
Run `scripts/verify-signatures.sh DIST darwin|windows true|false` on a native
QA runner against downloaded published assets; the release workflow does this
for each candidate.

## Vulnerability gate

`make govulncheck` (`scripts/govulncheck-release.sh`) is part of the release
workflow: the `govulncheck` job runs it after resolving the pinned siblings,
uploads the JSON reports, and prints the summary. Both build jobs need the
completed check. It builds the pinned
`golang.org/x/vuln/cmd/govulncheck` (v1.9.0) with the toolchain named in
`go.mod`, then scans bashy, `../outpost` and `../yoke` in source mode, each
with `GOWORK=off` so the scan sees the versions a release links.

The release step is temporarily **report-only**: its
`continue-on-error: true` setting lets the release proceed while the 66 known
reachable third-party findings are removed by dependency bumps (bashy 18,
outpost 22, yoke 26). To make the check blocking, change that one value in
`.github/workflows/release.yml` to `false`. The checker itself always returns
nonzero for reachable findings, so this switch does not hide or reinterpret
the result. Package- and module-level findings are printed and retained too;
nothing is waived. Zero reachable standard-library findings are expected. Any
future reachable standard-library finding is fixed by raising the `toolchain`
line across the umbrella, never by an exception. Run the check locally from an
umbrella checkout before proposing a candidate tag.

## License gate

`scripts/verify-license-gate.sh` (`make license-gate`) verifies that bashy's
full shipped Go dependency graph (cmd/bashy, cmd/bash, cmd/sh, outpost) plus
vendored/third_party directories contains only permissive licenses.

The release check is **blocking**: native macOS and GoReleaser builds depend on
the `license-gate` job in `.github/workflows/release.yml`. It runs
`scripts/verify-license-gate.sh` over the pinned shipped graph and uploads its
report artifact. `scripts/license-review.json` has zero exceptions; a new
non-permissive or unknown code license fails the release before builds begin.

## Install-channel manifests

After promotion, download all `bashy-<os>-<arch>` archives plus
`checksums.txt`, then generate the Homebrew, Scoop and Winget manifests:

```sh
python3 scripts/generate-install-channels.py vX.Y.Z dist channels
```

The generator verifies every archive against `checksums.txt`. Do not publish a
manifest generated from candidate (`-dev`) bytes under a stable version; manifests
must only be published from promoted release bytes (e.g. current stable `v0.32.0`,
and regenerated from promoted bytes at `v1.0.0`).

Channel manifests identify publisher as **DHNT Inc.** and use homepage **`https://dhnt.io`**.

### Homebrew tap publication

Homebrew formulae set no quarantine flag, allowing installation of macOS binaries
without Gatekeeper prompts. Formulae are hosted in the company GitHub org repo
`github.com/dhnt/homebrew-tap`:

1. Copy `channels/Formula/bashy.rb` into the `homebrew-tap` repository under `Formula/bashy.rb`.
2. Commit and push to `dhnt/homebrew-tap`.
3. Users install via:
   ```sh
   brew install dhnt/tap/bashy
   ```

### Scoop bucket publication

Scoop sets no Mark-of-the-Web on downloaded files, allowing installation of Windows
binaries without SmartScreen unknown-publisher prompts. Manifests are hosted in the
company GitHub org repo `github.com/dhnt/scoop-bucket`:

1. Copy `channels/bucket/bashy.json` into the `scoop-bucket` repository under `bucket/bashy.json`.
2. Commit and push to `dhnt/scoop-bucket`.
3. Users add the bucket and install via:
   ```powershell
   scoop bucket add dhnt https://github.com/dhnt/scoop-bucket
   scoop install dhnt/bashy
   ```

### Linux packages

Linux releases ship `.deb`, `.rpm`, and `.apk` packages alongside tarballs directly
in the GitHub release assets.

### Winget validation and submission (v1.0.x signed track)

The generator creates `DHNT.bashy` manifests (`PackageIdentifier: DHNT.bashy`,
`Publisher: DHNT Inc.`). Per the 2026-10-25 decision, Winget submission requires
code-signed binaries and is handled under S504.6. Manifests are generated at release
time but submitted only after Authenticode signing.

Validation is a native Windows gate. Copy the generated `channels/winget`
directory to `windows-host` and run in PowerShell:

```powershell
winget validate C:\path\to\winget
```

Record the command output with the release evidence. Validation is not
submission. With the operator present, copy the three files into the fork of
`microsoft/winget-pkgs` at
`manifests/d/DHNT/bashy/<version>/`, rerun `winget validate` on that final
directory, commit and push one package-version change, and open a pull request
to `microsoft/winget-pkgs`. Never create or submit that public pull request
without explicit operator approval.
