---
id: 3f46d757ac67
kind: feature
title: Provision C# toolchain through binmgr
seq: 386
status: todo
priority: p2
labels:
    - toolchains
    - licensing
created: 2026-10-01T17:31:31.224687Z
sprint: 350
sprint_id: 406ae7fe-78aa-5ddc-9508-6e4dbf105b2a
sprint_title: Bashy release SBOM and license boundary
---

Goal
Give a bare Bashy host a managed C# build/run toolchain without linking the SDK into Bashy or requiring a preinstalled dotnet command.

Source reference
- https://github.com/dotnet/roslyn is the MIT-licensed C# compiler source. Identify the Roslyn compiler version inside the selected .NET SDK archive and trace it to its source release; use the SDK's included compiler rather than downloading an unrelated second compiler. The SDK archive's own LICENSE and third-party notices remain authoritative for the downloaded bytes.

Implementation contract
- Add a binmgr-backed provisioner for a pinned .NET SDK release, with explicit supported OS/architecture assets, verified digest, cache-first resolution, and a useful unsupported-platform or download error. Provide `bashy dotnet` as the transparent CLI front door and make `csharp`/`dotnet` available to Bash# toolchain resolution and `bashy check --prepare`; a discoverable `bashy csharp` spelling may delegate to the same SDK rather than creating a second cache or version policy.
- The managed SDK must support `dotnet --info`, `dotnet new console`, `dotnet build`, and `dotnet run` from a clean machine, with child environment and MSYS/native path conversion handled on Windows. Document version override and offline/cache behavior using the existing toolchain conventions.
- Inspect the LICENSE and third-party notices in the *actual pinned release archives* for every supported platform. Official .NET product distributions have different stated license terms on Windows versus Linux/macOS; do not infer Windows terms from the dotnet/sdk source repo's MIT license. Choose only assets whose terms satisfy the explicit runtime-download policy, or leave that platform unsupported with a clear reason. Record the SDK and any bundled non-permissive or proprietary components in Sprint #350's runtime-acquisition inventory, never as code linked into Bashy.
- Add command-atlas/platform declarations, help and tests for cache hit, verified fetch, missing asset/digest, version override, CLI passthrough, and Bash# selection. Run the existing relevant cross-platform gates and one clean-machine C# compile/run smoke per supported OS.

Acceptance
- A supported host can run a C# console project through Bashy with no system .NET install, using one verified cached SDK.
- The release SBOM remains scoped to Bashy's own distributed bytes; the .NET SDK appears in the separate runtime-download inventory with platform-specific license evidence and no unqualified permissive claim.
