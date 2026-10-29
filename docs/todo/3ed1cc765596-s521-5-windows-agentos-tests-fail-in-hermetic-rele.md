---
id: 3ed1cc765596
kind: bug
title: S521.5 Windows agentos tests fail in hermetic release-preflight (CI skips them on bare clones)
seq: 2818
status: todo
priority: p0
labels:
    - known-failures
created: 2026-10-28T06:18:22.261574Z
sprint: 521
sprint_id: 8215f52e-1496-5a0f-b6f6-729bf3056119
sprint_title: 'Fleet upgrade fix release + ivy deploy: ship outpost upgrade-ordering fix (cd8b98c4) as bashy rc.22, move every host incl. ivy (lern, lj2ivy) onto it once, then promotions auto-flow'
---

From S514 hermetic preflight (bashy eff3625e, rc.20): ./internal/agentos on the Windows peer failed TestGenieRecipeGeneratesCallerWorkspaceAndModel (274s, timeout class), TestInboxHookSeededDeliveryEnvelope, TestSprintInboxAckCommitsRenderedRoleBatchOnly, then "panic: test timed out after 10m" (sbom_test.go:221, TestReleaseSbomCLI building ./cmd/bashy). CI windows-latest is green only because these skip without umbrella siblings. The original log was cleaned up: reproduce first.

## Brief (S521 manager, 2026-10-28)

Goal: make ./internal/agentos pass on Windows under the release-preflight hermetic lane, so S521.1 fast-loop step 2 (`bashy dag release-preflight`) can go green. Gates bashy rc.22.

1. Reproduce on the Windows peer `windows` in ~/.bashy/release-hosts (noviwin1, port 2222; remote shell is bashy). Remote execution ONLY via `outpost ssh` / `outpost scp` (never the OpenSSH client). Copy the hermetic env setup of the windows lane in umbrella script/bashy-release-preflight.sh (scrubbed BASHY_*/CLAUDE_* env, throwaway HOME/APPDATA/LOCALAPPDATA/TMP, GOPATH/GOCACHE/GOMODCACHE pinned, Git usr\bin on PATH, umbrella sibling checkouts present). Cross-compile the test binary locally (`GOOS=windows go test -c -o agentos.test.exe ./internal/agentos`), upload it, and run it there with `-test.run` for the three tests, then the whole package with `-test.timeout 30m` to get honest timings. Keep scratch under %USERPROFILE%/s521-5/ and remove it at the end.
2. Root-cause each failure. Fix the product or test bug (Windows path/quoting, file locking, line endings, missing tool, timeout). Do NOT skip a test on Windows only to get green. If a test is legitimately too slow for the 10m package budget (genie: go run + ycode build; sbom: full bashy build), make it fast (prebuild once, reuse the binary) rather than raising the timeout blindly.
3. Red/green: show each test failing before and passing after on the Windows peer, and passing on darwin locally (`go test ./internal/agentos -run '<names>'`).
4. Commit in bashy with Sprint/Story trailers; do not bump pins or release.

Evidence: put the exact commands, before/after test output (the summary lines) and timings in the commit message body or in docs/sprint-521/s521-5-windows-agentos.md (umbrella; the manager will commit it). Never put real host names in bashy (OSS) files: say "Windows peer".
Host etiquette: noviwin1 also runs other lanes' agents; keep at most one test process running at a time; stop on finishing.
