---
id: d91599c253c5
kind: feature
title: 'Release vuln gate: explicit logged operator override (rc.39-dev publish)'
seq: 3279
status: done
priority: p0
labels:
    - release
created: 2026-11-21T07:37:41.087122Z
assignee: claude-opus5.5
sprint: 656
sprint_id: b220aade-5c96-5625-9a68-a0b7619c418a
sprint_title: RC38 release pipeline and QA follow-ups
---

Operator decision 2026-11-21: the local release vulnerability gate gets an explicit, logged operator override so rc.39-dev can publish with the known advisories disclosed. Base: bashy main 4f0ce3d3 (tag v1.0.0-rc.39-dev = 732eb770). Tooling-only change; no product code.

1. scripts/govulncheck-release.sh: when RELEASE_VULN_OVERRIDE (reason text) AND RELEASE_VULN_OVERRIDE_BY (role, e.g. "operator") are both non-empty, unwaived REACHABLE findings do not fail the run. Print one line "govulncheck-release: OVERRIDE by <by>: <reason>" plus the overridden ids, and write ${GOVULNCHECK_OUT}/../vuln-override.json (NOT inside the govulncheck/ report dir, collect globs it): {"reason","by","overridden":{"<scope name>":["GO-…",…]}}. Exactly one of the two env vars set = hard error. Scanner failures (exit other than 0/3, no report) still fail regardless. Without the env vars, behaviour is byte-for-byte unchanged.
2. scripts/local-release-evidence.py collect: if report_dir/vuln-override.json exists, record govulncheck.verdict="override" and govulncheck.override={reason,by,overridden,sha256 of the file}; else unchanged "pass". verify(): accept verdict "pass", or "override" only when reason/by are non-empty strings and overridden is a non-empty map of lists of GO-/stdlib OSV ids. License verdict must still be "pass".
3. release.yml preflight: if the receipt verdict is override, echo the override (by, reason, ids) into $GITHUB_STEP_SUMMARY. Keep everything else.
4. Tests: extend scripts/test-govulncheck-release.py and scripts/test-local-release-evidence.py — override pass, half-set env fails, scanner failure still fails under override, verify rejects override with empty reason. Run them (python3) on the dev box; nothing heavy, no Chrome.
5. docs/release-runbook.md: short "Operator override" paragraph under the vulnerability gate: when used, the draft release notes must list the overridden ids and the reason; retire by fixing deps.
Commit trailers: Sprint #656 + this story. Do not push; DONE on the board with branch+commit.
