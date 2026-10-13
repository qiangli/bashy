# Bashy release runbook

Before publishing release notes, apply the [public-claim checks](public-claims-evidence.md).
Use the actual candidate tag from `bashy --version`, Bash compatibility 5.3,
and the Go 1.27 / toolchain go1.27.2 build coordinate; never substitute a
historical report's version or percentage.

The candidate release is built once from a `vX.Y.Z-dev` tag. Promotion reuses
those tested bytes; it does not rebuild them. The repository knowledge record
`kb:release-bashy` owns the full candidate, native-QA and promotion sequence.

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

## Install-channel manifests

After promotion, download all `bashy-<os>-<arch>` archives plus
`checksums.txt`, then generate the Homebrew, Scoop and Winget manifests:

```sh
python3 scripts/generate-install-channels.py vX.Y.Z dist channels
```

The generator verifies every archive against `checksums.txt`. Do not publish a
manifest generated from candidate (`-dev`) bytes under a stable version.

### Winget validation and submission

Validation is a native Windows gate. Copy the generated `channels/winget`
directory to `noviwin1.local` and run in PowerShell:

```powershell
winget validate C:\path\to\winget
```

Record the command output with the release evidence. Validation is not
submission. With the operator present, copy the three files into the fork of
`microsoft/winget-pkgs` at
`manifests/q/qiangli/bashy/<version>/`, rerun `winget validate` on that final
directory, commit and push one package-version change, and open a pull request
to `microsoft/winget-pkgs`. Never create or submit that public pull request
without explicit operator approval.
