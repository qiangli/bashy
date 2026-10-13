---
id: a60d93b43aa5
kind: bug
title: 'builds: separate audit-tool toolchain, EOL-minor security override; gitea/zot/kopia overrides + kopia server env'
seq: 1902
status: done
priority: p1
labels:
    - supply-chain
created: 2026-10-13T00:35:02.202465Z
weave: 68
assignee: claude-opus5
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T02:17:15.208022Z
closed_by: s434-manager
---

Follow-up to 88f90c8f (lands first; rebase on it). bashy builds/ only; you own builds/_shared for this story.
1. _shared audit tools toolchain: install govulncheck + cyclonedx-gomod with a separate pinned TOOL toolchain (GOTOOLCHAIN=go1.27.2 for `go install` only), independent of the tool's build toolchain; govulncheck still analyzes with the module's own go line / effective toolchain. kopia v0.23.1 (go1.25.11) failed because govulncheck v1.9.0 needs go>=1.26 (qiangli/kopia run 38326642419).
2. EOL minor: Go 1.25 gets no security fixes now that 1.27 shipped. Allow GO_TOOLCHAIN_SECURITY_OVERRIDE to move to the oldest SUPPORTED minor (1.26.x latest patch) when the tag's minor is unsupported; the gate must still refuse a downgrade and refuse jumping when the tag minor IS supported; record it in PIN + release notes as "EOL-minor security override".
3. Declarations (latest patches: go1.27.2, go1.26.9; golang.org/x/net fixed v0.60.0):
   - gitea v28.1.0: GO_TOOLCHAIN_SECURITY_OVERRIDE to newest patch of its minor + GO_SECURITY_OVERRIDES golang.org/x/net@v0.60.0 (qiangli/gitea run 38326618659: 4 builds green, audit 12 reachable stdlib + 5 x/net).
   - zot v2.1.22 (go1.27.0): audit failed in qiangli/zot run 38326659679 - run govulncheck locally at go1.27.2 and add exactly the needed overrides.
   - kopia v0.23.1: EOL-minor override to go1.26.9 + whatever govulncheck needs; files/no-tracking.env: KOPIA_CHECK_FOR_UPDATES=false (exact env name from cli/update_check.go at the tag) and NO --no-check-for-updates on `server start` (repository-only flag; it made the no-egress gate fail). Locally verify the server start argv used by files/no-egress-test.sh is accepted (kopia server start --help at the tag, or go run).
   Run govulncheck locally per tool at the effective toolchain: 0 reachable required; NO waivers.
Gate: builds/check.sh exit 0. Commit with trailers, sprint submit with SHA + per-tool govulncheck summary. Do NOT push to qiangli/* or tag (manager pushes).

Conductor addendum (kopia, exact locations): builds/kopia/files/no-egress-test.sh lines ~73/75 pass --no-check-for-updates to `kopia server start` (and the rendered fork/bashy-build copy); README, files/no-tracking.env, files/otel.env and the PIN RELEASE_NOTE still describe that flag. Replace everywhere with the env switch KOPIA_CHECK_FOR_UPDATES=false (as yoke a089a6db does: --otlp-trace stays as a global flag before the subcommand), then re-render. `grep -rn no-check-for-updates builds/kopia` must return nothing.

Base: bashy main 24680fb5 (88f90c8f landed: GO_SECURITY_OVERRIDES step + gate, UPSTREAM_TAG_FORM, non-GitHub forge; fail-fast:false). Key names: GO_TOOLCHAIN_SECURITY_OVERRIDE, GO_SECURITY_OVERRIDES.

Conductor addendum (zot, qiangli/zot run 38326659679: 4 builds green, audit refused): stdlib (go1.27.2) + golang.org/x/net v0.60.0 + otel 1.45.0 are fixable via overrides. Three have NO fix: GO-2026-4919 (trivy), GO-2026-6225 (docker-credential-acr-env), GO-2026-5932 (x/crypto openpgp). Try first: build zot with ONLY the extension build tags bashy needs (zot gates extensions by build tags; trivy = search/CVE scanning, acr creds = sync; check what yoke external/zot configures - yoke disables optional outbound extensions) and re-run govulncheck with the same tags. If the no-fix IDs become unreachable, ship that minimal build and document the tag set in README + PIN. If any no-fix ID stays reachable: record zot as blocked-on-upstream with the IDs (same as headlamp) - CI keeps refusing to sign; report it in sprint submit (the manager files the yoke disable).
