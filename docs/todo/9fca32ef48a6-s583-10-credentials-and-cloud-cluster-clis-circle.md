---
id: 9fca32ef48a6
kind: feature
title: 'S583.10 Credentials and cloud/cluster CLIs: circle-bound accounts and profile selection'
seq: 3141
status: todo
priority: p1
labels:
    - boundary
    - credentials
created: 2026-11-06T21:45:56.692567Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Credentials and tier-5/6 CLIs. Accounts bound to the circles they may serve (from S583.3). Registry Entry gains a circle-aware ExecEnv (yoke/external/registry/registry.go): the active circle selects AWS_PROFILE, AZ_PROFILE, GLAB_PROFILE, DKS_PROFILE, kubeconfig context, gcloud/doctl/gws/tofu/gitea credentials; using a profile bound to another circle fails closed (consistent with aws/auth.go no-default rule). secret --account vault selection checked against the circle.
Depends on: S583.2, S583.3.
Acceptance: aws with a home-bound profile inside an org repo refused; correct profile auto-selected per circle; kubectl/helm DKS_PROFILE likewise.
