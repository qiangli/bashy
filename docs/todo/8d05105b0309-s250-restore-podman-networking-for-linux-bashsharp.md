---
id: 8d05105b0309
kind: bug
title: S250 restore Podman networking for Linux BashSharp Tour
seq: 322
status: done
priority: p1
created: 2026-09-23T09:51:28.178157Z
weave: 16
assignee: qiangli
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T10:28:13.887892Z
closed_by: codex-s250
---

The BashSharp Tour on a Linux droplet ran 38 pass/2 fail on an older authenticated candidate: advanced/dockerfile and advanced/k8s both fail because the provisioned Podman cannot find netavark. First rerun these cases on the current Bashy candidate with the same host setup. If still red, fix the shared Podman/network provisioning cause and rerun the complete BashSharp Tour; preserve fixtures. Private Sprint 250 Story #676 holds the raw evidence.

Delivery: `bashy@485422e` appends the standard Linux sbin directories to the Podman child PATH while retaining caller precedence. Netavark could start but could not find the host firewall tools under the minimal validation PATH. Both affected cases matched their pinned transcripts after the fix; the complete unchanged Linux BashSharp Tour passed 40/40 with no failures or skips. Full Tour log SHA-256: `dc6409172236a3e5ade20d45dd4eb572bc6d96478a84c4174e09fa346650c41c`.
