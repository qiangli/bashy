---
id: 0642298c7d43
kind: task
title: R1 Promote rc.25 after explicit operator approval
seq: 3151
status: todo
priority: p1
labels:
    - release
created: 2026-11-06T21:50:00.657162Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Blocked on explicit operator approval of the gated candidate. Promote the same tested bytes (never rebuild) via promote.yml with the commit-pinned tier verdict; spot-check sha256 and the bashy-scratch digest against the -dev build; scripts/generate-install-channels.py and a native winget validate; bashy mb announcement. Not v1.0.0: no v1.0.0 before official POSIX certification.

## Promotion receipts (manager, 2026-11-07)

- Operator approval: the 2026-11-06 go-ahead ("tag -dev, gate, then promote") with the candidate name v1.0.0-rc.27. rc.25 and rc.26 are intentionally unused.
- Candidate: bashy 9187e1f0b5d8294fd83d97d24437bc05eb840a8d. Pre-release v1.0.0-rc.27-dev from release.yml run 42327125592 (39 assets).
- Tier verdict: release/tier-verdicts/9187e1f0.json committed at bashy 970ff717 (dhnt.tier-smoke/v1, native, PASS on darwin/linux/windows; releaseeligibility PASS). The umbrella identity adfec991 is published as umbrella tag candidate/bashy-v1.0.0-rc.27.
- QA refs: refs/qa/v1.0.0-rc.27/{linux,darwin,windows} -> 9187e1f0.
- Bare tag: v1.0.0-rc.27 -> 9187e1f0.
- promote.yml run 42339120565: success.
- Spot check:
  - promoted checksums.txt is byte-identical to the -dev one (38 entries);
  - bashy-darwin-arm64.tar.gz verifies against it;
  - bashy-scratch manifest byte-promoted from :9187e1f0 to :v1.0.0-rc.27 (digest sha256:c25b4ed03abc26c55ed05edb951690a38e17273f71c0ed3cc0602d5aae67d9cb);
  - releases/latest = v1.0.0-rc.27.
- Signing: darwin.signed=false windows.signed=false (no Apple/Azure secrets), the same as rc.23 and rc.24.
- Remaining:
  - Homebrew tap and Scoop bucket updates from the install-channels artifact;
  - winget validate on the Windows host (the winget-pkgs PR needs separate operator approval);
  - mb announcement.
