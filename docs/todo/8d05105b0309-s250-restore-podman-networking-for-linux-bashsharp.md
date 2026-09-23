---
id: 8d05105b0309
kind: bug
title: S250 restore Podman networking for Linux BashSharp Tour
seq: 322
status: assigned
priority: p1
created: 2026-09-23T09:51:28.178157Z
weave: 16
assignee: qiangli
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

The BashSharp Tour on a Linux droplet ran 38 pass/2 fail on an older authenticated candidate: advanced/dockerfile and advanced/k8s both fail because the provisioned Podman cannot find netavark. First rerun these cases on the current Bashy candidate with the same host setup. If still red, fix the shared Podman/network provisioning cause and rerun the complete BashSharp Tour; preserve fixtures. Private Sprint 250 Story #676 holds the raw evidence.
