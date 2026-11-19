---
id: 01a1f309-e75c-76c6-b2dc-d39bed4e0c4d
seq: 33
form: page
type: gotcha
title: browser login with --success-url and --cookie captures the URL, not the secret
description: 'WHEN an agent captures a session cookie, token or password-equivalent with bashy browser login (headed solo or --mode live) into bashy secret set: give ONLY --cookie NAME or --token-selector CSS. With --success-url too, the URL wins and the vault silently stores the landing URL. Verify the stored value''s shape (length, dots, not http) without printing it; prefer --mode live when the human is already signed in'
tags:
    - browser
    - login
    - secrets
    - vault
    - cookie
    - gotcha
status: candidate
evidence: yoke cmds/browser/browser.go DetectLoginCompletion checks SuccessURL first and returns only the URL; reproduced 2026-11-19 capturing a step-up session (38-char URL stored); --cookie alone stored a 284-char JWT
source:
    tool: claude
created: "2026-11-19T09:01:14Z"
---

## The gotcha

`bashy browser login URL --success-url PATH --cookie NAME` prints the **URL**, not the
cookie. Completion is checked in this order: success URL, then token selector, then
cookie. The first match ends the wait. A success-URL match carries no secret, so the
printer falls back to the URL.

Piped into `bashy secret set`, the vault then silently holds something like
`https://host/landing`. Every later call fails as "unauthenticated", far from the cause.

Tracked as a bug (umbrella todo `a7dbe3e4`). Until it is fixed, follow the rule below.

## Do this instead

Give **only** the flag for the value you want: `--cookie NAME` for a session cookie, or
`--token-selector CSS` for a token the page displays. Use `--success-url` alone only when
the URL itself is the answer.

```bash
# The human signs in; the secret goes straight to the vault and never reaches the terminal.
bashy browser --headed --timeout 10m login "$LOGIN_URL" --cookie "$COOKIE_NAME" \
  | tr -d '\n' | bashy secret set "$SECRET_NAME"

# Verify its SHAPE before trusting it. Print properties, never the value.
bashy secret exec "$SECRET_NAME" -- bash -c '
  v=${!1}; echo "len=${#v} dots=$(tr -cd . <<<"$v" | wc -c) url=$([[ $v == http* ]] && echo yes || echo no)"' _ "$SECRET_NAME"
# A JWT session: hundreds of characters, 2 dots, url=no. If url=yes, you captured the redirect.
# Delete it (bashy secret rm NAME) and repeat with the one right flag.
```

## Choosing the browser mode

- **`--mode live`** drives the person's real Chrome through `bashy browser hub` and the
  extension, so an existing sign-in or SSO is reused. Use it when the human is already
  signed in there; set it up once with `bashy browser setup live`.
- **`--headed`** (solo, the default mode) opens a private, throwaway profile. The human
  must sign in from scratch, and the profile is discarded afterwards. If the capture
  fails, they sign in **again**. That makes getting the flags right the first time worth
  it.
- Never ask the human to paste a secret into the chat. Use `browser login` piped into
  `secret set`, or `bashy ask --name NAME --stdout | bashy secret set NAME`, which prompts
  them directly.

## Related

A step-up or admin session is often a cookie set by a dedicated login path, then a
redirect to an admin landing page. Capture the cookie (`--cookie`); don't wait for the
landing URL.
