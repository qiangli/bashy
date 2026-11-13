---
id: 01a1b809-095b-737a-83b6-489296912de7
seq: 9
form: page
type: runbook
title: Access DigitalOcean and AWS with vault-held credentials (curl for DO, secret exec for AWS)
description: 'How to reach the DigitalOcean and AWS APIs with credentials kept in the bashy vault, why doctl can act on the wrong DO team, and the AWS budget guard — WHEN listing, creating or deleting droplets or Lightsail resources, or a DO call returns 403 or the wrong team''s droplets (keywords: digitalocean, doctl, droplet, team token, curl, 403, aws, lightsail, secret exec, budget, dhnt-monthly-cap, static ip, billing)'
tags:
    - cloud
    - digitalocean
    - aws
    - doctl
    - lightsail
    - secrets
status: candidate
evidence: '2026-11-07: DO curl list/create/delete run end to end with a team token while doctl with the same token listed only the saved context''s team; AWS secret exec used for a live Lightsail satellite provision; budget rules from yoke/docs/s522-2-aws-lightsail-adapter.md'
source:
    tool: claude
created: "2026-11-07T22:02:42Z"
updated: "2026-11-07T22:03:51Z"
---

Cross-cloud policy (identity, quota, budgets that exclude credits, GPU, gcloud/Azure): [[kb:work-with-cloud-providers-safely-do-aws-gcloud-azure]].

## Variables used below

```sh
DO_TOKEN_SECRET=<vault name of the team's DO token>   # one token per DO team
AWS_REGION=us-west-2
```

## DigitalOcean

- A DO API token belongs to exactly one team. Pick the team by picking the token; there is no team flag or context to switch.
- Gotcha: on a host with a saved doctl context, `doctl` and `bashy doctl` were observed to ignore `--access-token`/`-t` and `DIGITALOCEAN_ACCESS_TOKEN` and act on the saved context's team. Symptoms: every token lists the same droplets, and creates return 403 although the token is fine. Test the exact token with curl before concluding it lacks scope.

List (read-only identity check: shows the token's team):

```sh
bashy secret exec "$DO_TOKEN_SECRET=T" -- /bin/sh -c \
  'curl -s -H "Authorization: Bearer $T" "https://api.digitalocean.com/v2/droplets?per_page=50"'
```

Create (body in a JSON file; `user_data` carries cloud-init):

```sh
bashy secret exec "$DO_TOKEN_SECRET=T" -- /bin/sh -c \
  'curl -s -X POST -H "Authorization: Bearer $T" -H "Content-Type: application/json" \
   --data @droplet.json https://api.digitalocean.com/v2/droplets'
```

Delete only by the ID your own create returned, after resolving that ID back to the name you created:

```sh
DROPLET_ID=<id from your create>
bashy secret exec "$DO_TOKEN_SECRET=T" -- /bin/sh -c \
  'curl -s -X DELETE -H "Authorization: Bearer $T" "https://api.digitalocean.com/v2/droplets/$1"' sh "$DROPLET_ID"
```

Rules:
- Never print token values; compare lengths or hash prefixes.
- Never delete by tag, age, pattern or "all except". Keep disposable test droplets in a separate team and destroy them after use. Production droplets the operator marks protected are never touched (the list lives in the operator's private host kb, not here).
- Without an account-registered SSH key, DO expires root's password and non-interactive SSH fails ("Password change required but no TTY"). Put the key in cloud-init with `chpasswd: {expire: false}`, and in runcmd `passwd -d root` and `chage -M 99999 root`.
- `bashy infra ...` verbs do not go through doctl: they read `DIGITALOCEAN_ACCESS_TOKEN` (or `--account <alias>`), so `bashy secret exec "$DO_TOKEN_SECRET=DIGITALOCEAN_ACCESS_TOKEN" -- bashy infra ...` selects the team.

## AWS

Credentials come from the vault for one command at a time:

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  bashy infra list --provider aws --region "$AWS_REGION"
```

(`bashy aws ...` is the general front door under the same `secret exec`.)

Budget guard in `bashy infra provision aws` (runs on create AND on `--dry-run`):
- bundle monthly price + month-to-date spend BEFORE credits must fit the monthly cap: the limit of the AWS Budget named by `--budget-name` (default `dhnt-monthly-cap`), or `--budget-cap`, else $50;
- the bundle alone must fit the per-instance ceiling (`--budget-per-instance`, default $24);
- GPU plans are refused by shape, whatever the price;
- spend comes from AWS Budgets, falling back to Cost Explorer (which answers nothing on a brand-new account); losing both fails closed.

Billing traps: a stopped Lightsail instance still bills monthly; an unattached Lightsail static IP bills. Full design: `yoke/docs/s522-2-aws-lightsail-adapter.md`.

## Candidate Bash# script

`do-api.bsh` — `list | create FILE | delete ID` over the curl calls above, token name from `$DO_TOKEN_SECRET`, refusing a configured protected-ID list. Not written yet.
