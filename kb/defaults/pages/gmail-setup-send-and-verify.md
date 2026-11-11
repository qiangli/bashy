---
id: 01a1cce0-e6e6-796a-b249-60a05af06e42
seq: 25
form: page
type: runbook
title: 'Gmail with Bashy: set up, send, and verify'
description: Connect Gmail through the Google Workspace example command and verify a test message — WHEN any agentic tool needs Gmail OAuth setup, gws discovery, gmail-send, or delivery checks.
tags:
    - gmail
    - google
    - oauth
    - commands
status: candidate
evidence: 'Live setup completed: Gmail-only API enablement, Desktop OAuth login, wrapper credential import, send, and message metadata verification. Genericized from observed behavior; provider UI may change.'
source:
    tool: codex
    episode: ep-93caeef33a1fed4b
created: "2026-11-11T23:10:53Z"
updated: "2026-11-11T23:11:01Z"
---

Any agentic tool with shell access can use Bashy for Gmail. `google` is a custom Bash# command wrapping the managed `gws` CLI, not a built-in Gmail integration.

1. Fetch the release source with `bashy src fetch bashy`. If its `examples/google` directory is present, run its `register.sh` through Bashy, then read `bashy google --help`. Check existing authentication before creating anything.
2. Establish the intended Google account, Cloud project, and test recipient. Run `bashy gws auth setup --project PROJECT` interactively to select Gmail alone. Configure a Desktop OAuth client; for an External app in Testing, save the account as a test user and verify it appears before login.
3. Import the downloaded client JSON with `bashy google auth-import CLIENT_JSON account=ACCOUNT scopes=gmail --no-login`. Set `GOOGLE_WORKSPACE_CLI_CONFIG_DIR` to the per-account directory reported by import. Run `bashy gws auth login --scopes SCOPE_URLS` with explicit `gmail.send` and `gmail.readonly` scope URLs for sending and verification.
4. The wrapper expects an authorized-user JSON file. Export with `gws auth export --unmasked` directly into an owner-only temporary file, then use `google auth-import` on that file. Never print tokens or paste secrets into arguments, chat, or public notes; remove temporary credential files afterward.

If the wrapper reports `GWS_NOT_FOUND` although `bashy gws` works, its native subprocess cannot resolve Bashy's managed command. Locate the installed executable and configure `GWS_BIN` through `bashy commands set google --set "env.0=GWS_BIN=GWS_PATH"` (choose an unused environment slot). Recheck that binding after upgrades; do not hardcode another machine's cache path.

Send only the authorized test:

```sh
bashy google gmail-send account=ACCOUNT to=RECIPIENT subject='Bashy test' body='Gmail setup test.'
bashy google raw gmail users messages get account=ACCOUNT userId=me id=MESSAGE_ID format=metadata
```

Retain the returned message ID. Verify actual From, To, Subject and labels: `SENT` confirms sending; `INBOX` confirms arrival for a self-test. Gmail can use a default sender alias rather than the authenticated address. Inspect that result and report it accurately; avoid duplicate sends when verification is uncertain.
