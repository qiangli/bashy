---
id: 46669697bbbd
kind: bug
title: test-startup-linux must measure bsh, not the full bashy product (P5 removed the bashy_cert_base profile it used to build)
seq: 3302
status: done
priority: p1
labels:
    - bashy
    - dag
    - release
created: 2026-11-22T14:32:53.683499Z
weave: 136
assignee: claude-fable5.1
sprint: 692
closed: 2026-11-22T14:41:34.531186Z
closed_by: claude-fable5.1
---

Found by P5 aa693f4a (bashy 2d7f52ad). dag.md test-startup-linux built cmd/bashy with -tags bashy_cert,bashy_cert_base,osusergo and asserted package-init time (total <= 15 ms, max <= 5 ms). P5 removed that profile, so the target now builds the full product, which measured 19-45 ms init on the dev box: the rc.40 candidate gate will go red for a known cause. DECISION (conductor 2026-11-22): the startup budget belongs to the certified thin shell. Change the target to build and measure bsh (bashsharp/cmd/bsh, GOWORK=off, its own module, pinned sibling) with the same thresholds and the same scripts/test-startup-linux.sh; do NOT loosen the thresholds; drop the inert bashy_cert_base tag mentions. Unit-level check: the target's build command resolves and the script runs on linux in the official image (targeted run on a claimed gate-host lane only if a Linux check is needed; otherwise the candidate gate proves it). TEST RULE (operator 2026-11-22): build + smallest targeted unit tests only, then submit; the one full run is the rc.40 candidate gate 458628c0. Start on current origin main; Sprint/Story/Story-ID + Agent trailers; bashy commit+push first, then the conductor bumps the umbrella pin. Owner codex, one run with the sibling stories filed at the same time (claim each).

RUN SCOPE (conductor 2026-11-22): this ONE codex run delivers three stories, one commit each: 46669697bbbd (startup check on bsh), de8e2cc0d87b (yoke/dag capacity services), 744d93b6177e (custom/kubectl Windows env). Claim all three on Sprint 692 before touching them; matching Story/Story-ID trailers per commit.
