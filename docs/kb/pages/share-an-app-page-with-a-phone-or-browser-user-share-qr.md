---
id: 01a1c7fd-6f37-7134-83c1-b66bda29e3c1
seq: 22
form: page
type: runbook
title: Share an app page with a phone or browser user (share qr)
description: 'Hand a URL to someone on a phone or another browser: bashy share qr prints the link and a terminal QR code for ANY URL, optionally a PNG — WHEN an owner wants a member or guest to open a published app page without typing it, or anyone needs to move a link from a terminal to a device (keywords: share, qr, QR code, phone, link, app page, outpost apps add, member invite, shapeable app, png)'
tags:
    - share
    - qr
    - apps
    - outpost
    - phone
status: candidate
evidence: '2026-11-10 S604 C2: verbs checked against bashy member invite --help, bashy outpost apps add --help and bashy app new --help on the current release; share qr sample run from the candidate build (yoke pkg/share)'
source:
    tool: claude
created: "2026-11-11T00:23:57Z"
---

## Who shares

Any bashy user with a URL: owner, member or guest. `share qr` publishes nothing and talks to no service; it prints the link on the first line and a QR code under it, so a phone camera opens the same page the terminal shows.

```sh
bashy share qr "$URL"                   # URL, then the QR code
bashy share qr "$URL" --png page.png    # also a PNG, for a slide or a chat
bashy share qr "$URL" --json            # {"url":..., "png":...}
```

## Typical owner path

1. Put the page where the phone can reach it. An app on this host is published through the paired outpost, sign-in required:

   ```sh
   bashy outpost apps add "$APP" --port "$PORT" --require-login
   ```

   Its satellite URL is `$SATELLITE_URL/matrix/h/$HOST_NAME/app/$APP/` (details in the outpost setup runbook).
2. If the page needs sign-in and the person has no account on the satellite yet, invite them (owner only): `bashy member invite EMAIL`. They sign in with their own account.
3. Hand over the link: `bashy share qr "$URL"` and let them scan it.

## What the phone user sees

The page, in the phone's browser: nothing to install. If the page requires sign-in, the satellite asks for THEIR account first, then shows the page.

## Shaping hook

A bashy app URL lands on the owner's shapeable app page: what the owner shapes with `bashy app new NAME` and the Genie button is exactly what a scan opens at that moment, so a kept change is what the next scan shows.

## Gotchas

- `share qr` refuses a bare `host/path`; give the scheme (`https://...`).
- The QR carries only the URL. It is not a login and not a pairing code (`bashy app pair` is the pairing path).
