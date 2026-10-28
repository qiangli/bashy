---
id: d89389e9f180
kind: bug
title: S514.R12 release tooling accepts rc versions so rc.20 can be promoted (generate-install-channels + promote path)
seq: 2749
status: done
priority: p0
created: 2026-10-27T20:57:04.463472Z
assignee: claude-sonnet5.5
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
closed: 2026-10-27T21:01:27.199052Z
closed_by: s514-manager
---

Operator decision 2026-10-27: fix the release tooling so an rc can be promoted, ship it in rc.20, promote rc.20 (rc.19 stays a pre-release).

Facts: promote.yml run 40561979947 (version=v1.0.0-rc.19) passed QA refs, eligibility (signed assets + tier verdict via https URL) and signed-manifest verification, then failed in 'generate install channel manifests': scripts/generate-install-channels.py rejects the tag (regex `v\d+\.\d+\.\d+(?:-dev)?`, 'expected vX.Y.Z or vX.Y.Z-dev'). The formula test regex (line ~53) and its delete_suffix("-dev") (line ~56) assume the same shape. promote.yml checks out the CANDIDATE commit for release tools, so the fix must be in the rc.20 commit itself.

Do:
1. Accept `vX.Y.Z` and `vX.Y.Z-rc.N` (and their `-dev` candidates) everywhere the promote path parses a version: generate-install-channels.py (tag check, Homebrew formula version/test regex, scoop/manifest version fields), scripts/register-product-release.sh, the promote.yml 'normalize version' step, and any other script promote.yml calls after eligibility. Read promote.yml end to end; list each step's version assumption in the commit message.
2. Decide the channel semantics minimally: an rc promotion publishes the same channel manifests with the rc version string (Homebrew/scoop accept semver prerelease strings); no new channel. If a channel cannot take an rc version, say so and skip only that channel with an explicit log line (never silently).
3. Tests: extend the existing script tests (or add a focused one) proving v1.0.0-rc.19 and v1.0.0 both produce valid manifests, and junk tags still fail.
4. Dry-run the promote path locally against the published v1.0.0-rc.19-dev assets (download to a scratch dir on a peer or Dragon scratch, run generate-install-channels.py v1.0.0-rc.19 dist channels) and paste the produced manifest names in the commit/notes.
5. Also document in docs (release runbook section in this repo if one exists) that `tier_verdict` must be an https URL pinned to a commit, because promote.yml checks out the candidate first.
No release, no tag, no dispatch. Commit with provenance trailers and submit.
