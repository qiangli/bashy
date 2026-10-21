# EVIDENCE S350 SBOM (story 476b77ff9e24)

Command: `scripts/release-sbom.sh [outdir]` (SKIP_ENGINES=1 to skip the engine build). Wired into release.yml job license-gate.

## licensegate
- default (-tags fb_archives, bashy+bash+sh+outpost): 366 modules scanned, 0 unreviewed findings, exit 0
- `-tags bashy_engines,fb_archives ./cmd/bashy`: 348 modules scanned, 0 findings, exit 0

## SBOM packages (SPDX 2.3, root + deps), darwin/arm64 host build
- bashy: 235 · bashy-engines: 348 · outpost: 252
- `release sbom --dist DIR --verify`: VERIFIED 3 artifacts

## manifest.json (artifact -> sha256 -> SBOM)
```json
{
  "artifacts": [
    {
      "artifact": "bashy",
      "sha256": "6940a1304cf671405abe2e8f9872fdadc51eac61822dfb9ba339bcce9359d812",
      "sbom": "bashy.spdx.json",
      "packages": 235
    },
    {
      "artifact": "bashy-engines",
      "sha256": "23184b7e24e88b74e836b94a35c86063a6f395eef726fb6ab4fbda7e2167979c",
      "sbom": "bashy-engines.spdx.json",
      "packages": 348
    },
    {
      "artifact": "outpost",
      "sha256": "4998cb4d0a0afa630e3c1ed79167050a653d8bbaccf4dea31dced68767844f17",
      "sbom": "outpost.spdx.json",
      "packages": 252
    }
  ]
}
```

## Fixes
- FindLicense now uses licensegate's LICENSE|LICENCE|COPYING[.-]suffix rule (LICENSE-APACHE/LICENSE-MIT: qiangli/yamux, yamux/v5 were UNKNOWN in outpost's SBOM).
- resolveModuleLicenseDir resolves local-path replaces and nested package paths (the umbrella-only UNKNOWN go-billy / yoke/pkg/provider case).
- RED->GREEN: TestReleaseSbomMatrixVerify (manifest + --verify + digest drift); outpost yamux UNKNOWN found and fixed by the gate.

## Inventories
- docs/license-inventory.md (generated), docs/runtime-acquisition-inventory.md, THIRD_PARTY_LICENSES.md, docs/licensing-supply-chain-policy.md §5

Not run here: umbrella go.work red repro (clone is standalone); engine build with embed_* blobs.
