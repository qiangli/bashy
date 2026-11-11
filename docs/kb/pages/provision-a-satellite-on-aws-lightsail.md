---
id: 01a1b809-631b-7bb8-a651-27e1f1900eaa
seq: 10
form: page
type: runbook
title: Provision a satellite on AWS Lightsail (check, register, dry run, provision, sign in, tear down)
description: 'Copy-runnable sequence to stand up a new satellite on AWS Lightsail with bashy infra provision aws, sign in as owner, mint pairing codes and tear it down without billing leftovers — WHEN creating, re-creating or destroying a cloud satellite on AWS (keywords: satellite, provision, lightsail, aws, infra provision, enrollment, static ip, sslip, BASHY_FLEET_TOKEN, invite code, destroy, api/v1/satellites)'
tags:
    - satellite
    - aws
    - lightsail
    - provision
    - infra
status: candidate
evidence: '2026-11-07: steps 1-6 run live for a new satellite (register 201, dry run, provision, health 200 about a minute after boot, owner console sign-in, invite code from Hosts); step 7 per bashy infra destroy --help and yoke/docs/s522-2-aws-lightsail-adapter.md, not re-run in this session'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
updated: "2026-11-07T22:03:51Z"
---

## Variables

```sh
SATELLITE_ID=<new satellite name>
MASTER_URL=<master base URL>                 # infra provision defaults to $BASHY_CLOUDBOX_URL, else the official portal
OWNER_TOKEN_SECRET=<vault name of an owner bearer with the satellites scope>
AWS_REGION=us-west-2
```

## Steps

1. Check the master's satellite list first. The master never deletes satellite rows, so a name may already be taken by an old satellite:

```sh
bashy secret exec "$OWNER_TOKEN_SECRET=T" -- /bin/sh -c \
  'curl -s -H "Authorization: Bearer $T" "$1/api/v1/satellites"' sh "$MASTER_URL"
```

2. Register the name (expect `201`):

```sh
bashy secret exec "$OWNER_TOKEN_SECRET=T" -- /bin/sh -c \
  'curl -s -o /dev/null -w "%{http_code}\n" -X POST -H "Authorization: Bearer $T" \
   -H "Content-Type: application/json" -d "{\"name\":\"$2\",\"endpoint\":\"\"}" \
   "$1/api/v1/satellites"' sh "$MASTER_URL" "$SATELLITE_ID"
```

3. Dry run: prints the request, the size verdict and the budget verdict; creates nothing:

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  bashy infra provision aws --satellite "$SATELLITE_ID" \
  --region "$AWS_REGION" --size small_3_0 --image ubuntu_24_04 --dry-run
```

4. Live: the same with the owner bearer in `BASHY_FLEET_TOKEN` (add `--master "$MASTER_URL"` when it is not the default):

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY "$OWNER_TOKEN_SECRET=BASHY_FLEET_TOKEN" -- \
  bashy infra provision aws --satellite "$SATELLITE_ID" \
  --region "$AWS_REGION" --size small_3_0 --image ubuntu_24_04
```

It reserves the static IP `<id>-ip` first, gets the enrollment bundle rendered for `https://<a-b-c-d>.sslip.io` (dashed address, one wildcard label), creates the instance, applies the satellite firewall (no internet-facing sshd) and attaches the address. The endpoint is printed; it answers HTTP 200 about a minute after boot. Re-running with the same `--satellite` is idempotent.

5. Sign in as the owner from the master's Satellites page, then **Open console**. The satellite's own landing page has no sign-in.

6. Pairing codes for hosts: owner console, Hosts, **Generate invite code**. `bashy satellite invite --satellite "$SATELLITE_URL" --name "$HOST_NAME"` needs a bearer that can be exchanged for the `host:invite-mint` scope; an owner token without it is refused.

7. Inspect and tear down:

```sh
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  bashy infra inspect aws --name "$SATELLITE_ID" --region "$AWS_REGION"
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  bashy infra destroy aws --name "$SATELLITE_ID" --region "$AWS_REGION" --yes
bashy secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- \
  bashy infra list --provider aws --region "$AWS_REGION"     # prove nothing is left
```

Destroy releases the static IP and the staged first-boot script too, and is safe to run after a failed create. Stopping is not enough: a stopped Lightsail instance still bills, as does an unattached static IP. The satellite row at the master stays (see step 1).

## Notes

- Budget guard, region pinning and billing traps: see the page on accessing DigitalOcean and AWS, and `yoke/docs/s522-2-aws-lightsail-adapter.md`.
- Rehearse satellite flows before paying for a cloud host: a cloudbox dev-mode sandbox can run master + satellite + hosts end to end locally (the operator's harness for this is private; the approach is generic).
- For DigitalOcean see the DO satellite page (not verified live).

## Candidate Bash# script

`satellite-provision.bsh` (examples/satellite-provision; default command `bashy satellite-provision`) — `check | register | plan | up | invite | share-llm | down` over steps 1-4, 6 and 7 plus `bashy llm share`, with the variables above as flags (environment fallback), waiting for the endpoint's 200 after `up`. Mutating verbs refuse without `--yes`; `--dry-run` prints the calls. Read `bashy satellite-provision --skill` first.
