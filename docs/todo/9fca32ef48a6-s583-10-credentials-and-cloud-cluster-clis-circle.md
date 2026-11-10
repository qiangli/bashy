---
id: 9fca32ef48a6
kind: feature
title: 'S583.10 Credentials and cloud/cluster CLIs: circle-bound accounts (yoke/pkg/accounts) and profile selection'
seq: 3141
status: todo
priority: p3
labels:
    - boundary
    - credentials
    - boundary-later
created: 2026-11-06T21:45:56.692567Z
---

Credentials and tier-5/6 CLIs. Reuse yoke/pkg/accounts (Account.Connections[].Profile and Account.Links) bound to the circles each account may serve (from S583.3) rather than maintaining a parallel credential inventory in circles.yaml. Registry Entry gains a circle-aware ExecEnv (yoke/external/registry/registry.go): the active circle selects AWS_PROFILE, AZ_PROFILE, GLAB_PROFILE, DKS_PROFILE, kubeconfig context, and gcloud/doctl/gws/tofu/gitea credentials from the circle's bound account; using a profile or `--account` bound to another circle fails closed (consistent with aws/auth.go no-default rule). `secret --account` vault selection is checked against the active circle.
Depends on: S583.2, S583.3.
Acceptance: aws with a family-bound profile inside a team repo refused; correct profile auto-selected per circle; kubectl/helm DKS_PROFILE likewise.
