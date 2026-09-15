---
id: 425e3a99ff00
kind: task
title: S183.6 — product integration and installed Python import smoke
seq: 284
status: assigned
priority: p1
created: 2026-09-15T00:07:55.542386Z
assignee: codex-gpt-5.5
sprint: 183
---

Wire only the minimal Bashy product integration required for S183 direct Python imports and EnvironmentPlan selection. Do not add a new top-level env command. Prove scripts without Python never probe it, diagnostics identify selected interpreter/environment, CI=true make test passes, and an installed binary runs both unchanged-checkout probes. Record exact interpreter/package versions for closure. Depends on S183.5. Sprint: #183.
