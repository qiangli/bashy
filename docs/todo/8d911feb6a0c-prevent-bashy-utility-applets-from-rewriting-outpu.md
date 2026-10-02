---
id: 8d911feb6a0c
kind: bug
title: Prevent Bashy utility applets from rewriting output-parent environment
seq: 400
status: assigned
priority: p0
created: 2026-10-02T23:12:18.745077Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Profile D sh_12 TP717 regressed in the one-file route: two child env snapshots differ only in BASHY_OUTPUT_PARENT, each set to the applet process PID. AgentOS output_reduce.go currently sets this variable from package init for every alias including coreutils env. Move the PID export to shell startup while preserving nested Bashy Stage 0 output reduction; add a one-file env applet regression and focused gates. This is a new fixable finding from the full diagnostic; keep live run and approval pins unchanged.
