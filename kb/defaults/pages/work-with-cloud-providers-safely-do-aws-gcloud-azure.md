---
id: 01a1d2d2-b384-7bbf-81da-153373098347
seq: 26
form: page
type: runbook
title: 'Work with cloud providers safely: one model for DigitalOcean, AWS, Google Cloud and Azure'
description: 'Cross-cloud policy and procedure for agents: scoped automation identity via secret exec, interactive-login (root) caveat, least-privilege policies, GPU quota requests, budgets that EXCLUDE credits, explicit --yes spend, tagged self-ending compute, CLI over console — WHEN about to use any cloud account, request a GPU or quota, set a spend cap, or start work on a new provider such as gcloud or Azure (keywords: cloud, aws, digitalocean, gcloud, gcp, azure, quota, gpu, budget, credits, spend cap, iam, service account, service principal, aws login, root, spot, terminate, least privilege)'
tags:
    - cloud
    - aws
    - digitalocean
    - gcloud
    - azure
    - budget
    - quota
    - gpu
    - iam
status: candidate
evidence: '2026-11-13: AWS verified live — automation-user quota read and GPU quota request (support case), budget created with credits excluded and alerts, scoped managed policy added after the inline-policy 2048-byte cap was hit, aws login observed to borrow a root console session then logged out; DO rules from the 2026-11-07 access runbook; gcloud/Azure sections are unverified shape'
source:
    tool: claude
created: "2026-11-13T02:53:06Z"
---

One safety model for every cloud: the same nine rules apply to
DigitalOcean (do), AWS, Google Cloud (gcloud) and Azure. Only the commands
differ. The per-provider sections below are the concrete commands. DO and
AWS are verified. gcloud and Azure are written in the same shape for when
credits arrive, and are marked UNVERIFIED until a live run proves them.

Credential plumbing and the DO/AWS API calls for listing and deleting
resources live in [[kb:access-digitalocean-and-aws-with-vault-held-credentials]].
This page is the policy and the cross-cloud procedure.

## The nine rules (all providers)

1. **The agent runs as a scoped automation identity, never as the human
   console identity.** Keys live in the vault and are projected into one
   command at a time:
   `bashy secret exec KEY... -- <cli> ...`. Never write credential files
   or CLI profiles for the automation identity.
2. **Interactive login borrows whoever is signed in to the console.**
   AWS `aws login`, `gcloud auth login` and `az login` give the CLI the
   console user's identity. That may be the account ROOT or owner. Use it
   only for a one-off bootstrap the automation identity cannot do (for
   example granting it a missing permission), confirm who you are first
   (`aws sts get-caller-identity`, `gcloud auth list`,
   `az account show`), then log out (`aws logout --profile P`,
   `gcloud auth revoke`, `az logout`).
3. **Grow the automation identity's permissions with small, named,
   least-privilege policies.** Scope them by resource name or ARN
   (e.g. only `budget/sprint-*`, only the GPU quota codes) and by tag
   conditions for compute. Prefer managed or attached policies: AWS caps
   inline user policies at 2048 bytes in total, which an already-scoped
   user hits quickly. Never widen a policy from inside a task without the
   operator's go-ahead.
4. **Check quota before planning a GPU.** New accounts usually have zero
   GPU quota. Requesting more is a support case, approved in hours to days
   and sometimes closed without a raise; add a one-line justification
   (workload, single instance, budget-capped). No quota means no launch,
   so plan a fallback.
5. **Cap spend with a provider budget that EXCLUDES credits.** Budgets
   count credits by default, so with credits on the account the net cost
   stays at $0 and the alarm never fires. Name sprint budgets
   `sprint-<id>-...`, set alerts at about 80% and 100% of actual spend
   plus 100% of forecast, and confirm the recipient email (new recipients
   must verify before alerts are delivered). Credits are a reserve, not a
   spending target. The operator sets the cap; agents stop at about 80% of
   it.
6. **Every paid action is explicit.** Anything that spends or sends
   declares that effect and needs `--yes`. `--dry-run` prints the selection
   and the cost class without calling the provider. A refusal says why and
   names the escape hatch; it never skips silently.
7. **Compute is born labelled and self-ending.**
   - Tag it at launch (`sprint=<id>`, `owner=<agent>`).
   - Prefer spot/preemptible capacity for batch work.
   - Set shutdown to terminate, and add an in-guest shutdown timer.
   - Record start, stop and cost in the evidence.
   - Terminate as soon as the job ends.
   - Delete only IDs your own create returned. Never delete by tag, age,
     pattern or "all except".
8. **Protected resources are never touched.** The operator keeps their
   list in the private host kb, not in a public page.
9. **Claim before you use.** Take `bashy claim host:<name>` or
   `bashy claim chrome-browser` for any shared host, account console or
   browser session, and release it with `bashy claim release <name>`.
   (`bashy release` is the software-release verb, not the claim release.)

## Prefer the CLI over the console

Drive the provider's own CLI under `bashy secret exec`, or `bashy infra` /
`bashy aws` where a verb exists. Use the web console through
`bashy browser --mode live` only for steps the CLI identity cannot do.
Consoles are slower, change their layout, can block page scripts (CSP), and
act as the human. If the CLI identity lacks a permission, fix the permission
once (rules 2 and 3) rather than working around it in the browser.

## DigitalOcean (verified)

- One API token per team. The token picks the team.
- On hosts with a saved `doctl` context, doctl may ignore the token. Use
  curl or `bashy infra`; see the access page.
- Billing control is per team: keep disposable droplets in an ephemeral
  team and destroy them after use.
- GPU droplets need account enablement and are billed hourly. Same rules 4
  to 7.

## AWS (verified 2026-11-13)

Read who you are, then check the GPU quota (on-demand and spot G/VT):

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- aws sts get-caller-identity
for q in L-DB2E81BA L-3819A6DF; do
  bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
    aws service-quotas get-service-quota --region "$AWS_REGION" \
      --service-code ec2 --quota-code "$q" --query 'Quota.[QuotaName,Value]' --output text
done
```

Request a GPU quota (vCPUs) and follow the case:

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  aws service-quotas request-service-quota-increase --region "$AWS_REGION" \
    --service-code ec2 --quota-code L-DB2E81BA --desired-value 8
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  aws service-quotas list-requested-service-quota-change-history-by-quota \
    --region "$AWS_REGION" --service-code ec2 --quota-code L-DB2E81BA \
    --query 'RequestedQuotas[0].[Status,CaseId]' --output text
```

Create a sprint budget with credits excluded. The budget JSON sets
`CostTypes.IncludeCredit=false`, and the notifications file holds 80/100
ACTUAL plus 100 FORECASTED, each with an EMAIL subscriber:

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  aws budgets create-budget --account-id "$AWS_ACCOUNT_ID" \
    --budget file://budget.json --notifications-with-subscribers file://notifications.json
```

GPU instances with enough VRAM for local video/diffusion models. Prices
are approximate on-demand rates; check `pricing:GetProducts` or the console
before quoting.

| Instance | GPU | VRAM | ~USD/h | Note |
|---|---|---|---|---|
| g6.xlarge | L4 | 24 GB | ~0.8 | 5B-class models; 14B needs FP8/offload |
| g5.xlarge | A10G | 24 GB | ~1.0 | same class, a bit faster |
| g6e.xlarge | L40S | 48 GB | ~1.9 | 14B-class video models without offload |

Gotchas:
- `bashy infra provision aws` refuses GPU shapes by design. GPU work is a
  separate, explicitly approved, tag-scoped EC2 path under rules 4 to 7.
- `aws login` (CLI 2.32+) borrows the console session. If that session is
  root, the CLI holds root until `aws logout`.
- A stopped Lightsail instance and an unattached static IP still bill.

## Google Cloud (UNVERIFIED: shape only)

Equivalents to prove when credits arrive:
- **Automation identity:** a service account with narrow roles.
  `gcloud auth activate-service-account --key-file=-`, with the key from
  the vault on stdin. Prefer workload identity or short-lived tokens over
  long-lived keys.
- **Quota:** the GPU quota is `GPUS_ALL_REGIONS` plus per-region
  `NVIDIA_*_GPUS`, often 0 on new projects. Check with
  `gcloud compute project-info describe` /
  `gcloud compute regions describe REGION`, and request through the quotas
  page or API.
- **Budget:** a Cloud Billing budget per billing account. Exclude credits
  (`creditTypesTreatment=EXCLUDE_ALL_CREDITS`) and set thresholds 0.8 / 1.0
  plus forecast: `gcloud billing budgets create ...`.
- **Compute:** labels instead of tags. Spot VMs:
  `--provisioning-model=SPOT --instance-termination-action=DELETE`,
  with `--max-run-duration` as the self-ending timer.

## Azure (UNVERIFIED: shape only)

Equivalents to prove when credits arrive:
- **Automation identity:** a service principal with a narrow role
  assignment on one resource group.
  `az login --service-principal -u APP_ID --tenant TENANT --password @-`,
  with the secret from the vault on stdin.
- **Quota:** per-region, per-VM-family vCPU quota (e.g.
  `standardNCADSA100v4Family`), usually 0 for GPU families.
  Check with `az vm list-usage --location REGION`; request through the
  quota API or a support request.
- **Budget:** a Consumption budget on the subscription or resource group
  (`az consumption budget create`) with 80/100% notifications. Check how
  credits (e.g. sponsorship or startup credits) are counted before relying
  on it.
- **Compute:** tags. Spot VMs (`--priority Spot --eviction-policy Delete`)
  plus an auto-shutdown schedule (`az vm auto-shutdown`). Delete the whole
  sprint resource group when done.

## Verify before you trust this page

For each provider, a live run needs a scoped identity read
(`get-caller-identity`-style), a quota read, a budget created with credits
excluded and its alert email confirmed, and a tagged
launch → terminate of the smallest instance under the cap. Record the date
and outcome in this page's evidence. Mark gcloud and Azure verified only
after that run.
