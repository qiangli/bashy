# Bashy release runbook

## Frozen local security evidence

Release security checks run once on the committed frozen candidate in the
locked official build image through `scripts/local-release-gate.sh`, with
`GOWORK=off`. License and reachable-vulnerability failures block the receipt;
report-only mode is disabled. The receipt includes the exact source commit,
root module locks and builder lock, resolved transitive module graphs and
module lock digests, retained report digests, and the scanner's actual database
modification time. The vulnerability database must be at most seven days old.

After reviewing the passing reports, the release manager records the receipt
as a Git note keyed by the candidate's exact commit:

```sh
git notes --ref=refs/notes/bashy-local-gates add -F bin/local-release-gate/evidence.json "$COMMIT"
git push origin refs/notes/bashy-local-gates
git rev-parse refs/notes/bashy-local-gates
```

Dispatch `release.yml` on the exact candidate tag with `evidence_commit` set to
that reviewed notes commit. This immutable approval input prevents a later or
untrusted update to the floating notes ref from becoming release authority.
Keeping evidence outside the source commit avoids a self-referential commit hash.
GitHub's publish-only preflight fetches that ref once, requires the approved notes commit,
and verifies the exact tagged source and receipt. GitHub does not rerun the
license or vulnerability scans. A missing, stale, mismatched or failing receipt
blocks publication. Upload the reviewed local distribution as a draft targeted
to the full source commit before dispatch; include its `release-assets.json`
manifest inside the receipt so preflight binds every uploaded asset digest.
Artifact SBOM, signing, provenance and digest checks remain
release requirements. Signing credentials are supplied only to the trusted local
native signing steps; GitHub verifies the resulting bytes and declared signing
status without rebuilding or re-signing them.

macOS releases support Apple Silicon only. The public GHCR scratch image is no
longer published or byte-promoted. Install Bashy and run `bashy self image` to
create the local FROM-scratch image using checksum-verified
`bashy-scratch-linux-{amd64,arm64}` release assets, which remain published.


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
The local native macOS signing step imports the certificate into a temporary keychain, signs
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
local gate: `scripts/govulncheck-release.sh` scans the pinned Bashy, Outpost and
Yoke modules and release tag profiles. Its reachable-vulnerability result is
blocking when producing the local security receipt. Package- and module-level
findings remain in the retained reports.

As measured on the **v1.0.0-rc.27** candidate
(`9187e1f0b5d8294fd83d97d24437bc05eb840a8d`) there are **4** reachable
findings — GO-2026-6860 and GO-2026-6866 in `golang.org/x/net` (fixed in
0.62.0), GO-2026-6865 in `klauspost/compress` (fixed in 1.20.2), and
GO-2026-6809 in the standard library's `crypto/x509` (fixed in **go1.27.3**).
All four are report-only for that release and are fixed on `main` by the
dependency bumps plus a toolchain bump; see
`docs/releases/v1.0.0-rc.27-known-issues.md` row G14. A reachable
standard-library finding is always fixed by raising the `toolchain` line
across the umbrella, never by an exception. Run the check locally from an
umbrella checkout before proposing a candidate tag.

## License gate

`scripts/verify-license-gate.sh` (`make license-gate`) verifies that bashy's
full shipped Go dependency graph (cmd/bashy, cmd/bash, cmd/sh, outpost) plus
vendored/third_party directories contains only permissive licenses.

The release check is **blocking** in the local frozen-candidate gate.
`scripts/verify-license-gate.sh` checks the pinned shipped graph; the receipt
retains report digests and preflight verifies the reviewed evidence. `scripts/license-review.json` has zero exceptions; a new
non-permissive or unknown code license fails the release before builds begin.

## Install-channel manifests (NOT SUPPORTED — future work, skipped)

**Operator decision (2026-11-06, v1.0.0-rc.27): the Homebrew, Scoop and winget
install channels are not yet supported.** The whole of this section is
**future work**: post-promotion channel generation and publication are
**skipped**, and no step below is part of the promotion sequence today. The
supported install paths are the `install.sh` / `install.ps1` scripts and the
release archives (plus the Linux `.deb`/`.rpm`/`.apk` packages, which ship as
ordinary GitHub release assets and are not a channel).

`scripts/generate-install-channels.py` is **retained but unused** — it is kept
for the future signed track and must not be deleted. Do not publish a tap,
bucket or winget package from its output until an operator reverses this
decision, and do not advertise a package-manager install in the release notes,
README or `docs/setup.md` while it stands.

The rest of this section records the procedure for when the channels are
enabled. For reference, the generator would be invoked after promotion against
all `bashy-<os>-<arch>` archives plus `checksums.txt`:

```sh
python3 scripts/generate-install-channels.py vX.Y.Z dist channels   # future work; not run today
```

The generator verifies every archive against `checksums.txt`. Do not publish a
manifest generated from candidate (`-dev`) bytes under a stable version; manifests
must only be published from promoted release bytes.

Channel manifests identify publisher as **DHNT Inc.** and use homepage **`https://dhnt.io`**.

### Homebrew tap publication (future work — not supported, skipped)

Homebrew formulae set no quarantine flag, allowing installation of macOS binaries
without Gatekeeper prompts. Formulae are hosted in the company GitHub org repo
`github.com/dhnt/homebrew-tap`:

1. Copy `channels/Formula/bashy.rb` into the `homebrew-tap` repository under `Formula/bashy.rb`.
2. Commit and push to `dhnt/homebrew-tap`.
3. Users install via:
   ```sh
   brew install dhnt/tap/bashy
   ```

### Scoop bucket publication (future work — not supported, skipped)

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


### Winget validation and submission (future work — not supported, skipped; v1.0.x signed track)

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

## Linux packages (supported)

Linux releases ship `.deb`, `.rpm`, and `.apk` packages alongside the tarballs
directly in the GitHub release assets. These are **not** an install channel and
are **not** affected by the channel decision above — they need no tap, bucket or
manifest publication step.

## Promoting a release candidate (`vX.Y.Z-rc.N`)

`promote.yml` accepts `vX.Y.Z` and `vX.Y.Z-rc.N` (a `-dev` suffix is stripped:
the candidate `vX.Y.Z-rc.N-dev` promotes to `vX.Y.Z-rc.N`). Bare-tag pushes still
match only `vX.Y.Z`; an rc is promoted by `workflow_dispatch` with
`version=vX.Y.Z-rc.N`. **Install channels are not yet supported, so every
channel is skipped**: a promotion publishes the release assets and
`checksums.txt` only, and the post-promotion channel steps above are not run.
The tooling still accepts an rc version string verbatim for when the channels
are enabled.

**`tier_verdict` must be an https URL pinned to a commit** (for example a
`raw.githubusercontent.com/<org>/<repo>/<commit-sha>/...` URL). `promote.yml`
checks out the candidate commit before the eligibility step, so a repo-relative
path resolves inside the candidate tree, not the tree that holds the verdict
record; the verdict is therefore not found (or is the wrong file). A URL not
pinned to a commit can change under the gate and is not evidence.

Because the workflow uses the candidate commit's release tools, a fix to the
promote scripts only takes effect for candidates built after it lands.

## Shaping lifecycle smoke (release gate)

Every release candidate must pass the model-free Hello World shaping smoke
against the **built candidate binary** (not a repo-local `go run`, not an older
installed one). It blocks the release on failure:

```sh
bashy dag test-shaping-smoke test-shaping-smoke-regression   # BASHY_SMOKE_BIN=/path/to/candidate (default bin/bashy)
```

`test-shaping-smoke` (`scripts/test-shaping-hello-world.sh`) registers a temp
copy of `examples/shaping/hello-world` in a disposable `HOME`/`BASHY_HOME`,
starts a console of its own (`--bind lan --pair`, so the host needs a LAN
address), pairs two principals and asserts, in order: `register.sh`,
`shape wish --from` a prepared workspace, show, diff, verify (the app's real
`checks/smoke.bsh`, evidence file checked), preview serving "Hello, Shaping!",
approve, a second principal still seeing "Hello, World!", the browser
checkpoint `TestDOMExternalShapeCheckpoint` (`-tags verifydom`, needs Chrome;
`SHAPING_SMOKE_SKIP_DOM=1` skips it and is not a pass of that step), undo
restoring "Hello, World!", and submit recording the candidate without
activating it. `test-shaping-smoke-regression` makes the app's smoke check
expect the old greeting and passes only if the smoke then fails.

Part 2 (two-host promotion round trip: outgoing, push to an org remote,
incoming/pull on a second host, undo, push upstream) runs on the fleet e2e
harness and is required for releases that touch shaping, promotion or the shape
CLI. Run `bashy dag test-shaping-promotion` with `BASHY_SMOKE_BIN` naming the
candidate, `SHAPING_PROMOTION_IMAGE` naming an already cached Linux image,
and `SHAPING_PROMOTION_EVIDENCE` naming a new private JSON evidence file.
The runner uses two disposable containers with separate state and actual Git
transport, verifies the pulled side-slot candidate before approval, checks undo,
and checks the exact proposal ref on the upstream remote. Retain binary/image
identities and command results. First qualification also runs the runner with
`--break-check org-ref`; this must fail rather than count as passing coverage.

Before each release, also run `bashy dag test-shaping-real-agent` once on a
claimed test host against that candidate. Opt in with `BASHY_REAL_AGENT_E2E=1`,
select the cheapest probed working agent in `SHAPING_SMOKE_AGENT=TOOL:MODEL`,
and record billing mode and cap in `SHAPING_SMOKE_BUDGET_NOTE`. The default
generation timeout is 600 seconds; there are no automatic retries. This gate
uses the production delegate rather than a prepared source edit, the real app
verifier, browser checks, second-principal isolation and undo. Preserve the
private artifacts with `SHAPING_SMOKE_KEEP=1` and record their references and
candidate digest in the release packet. Setup is documented in
`examples/shaping/hello-world/e2e-matrix.md`.

A missing, failed or timed-out real-agent run blocks release unless the operator
records an explicit override and reason in that release packet. An agent exit
status alone, a browser skip or a passing run on an older binary is insufficient.
