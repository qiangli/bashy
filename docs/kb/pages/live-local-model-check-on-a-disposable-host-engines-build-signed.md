---
id: 01a1c781-7582-7b81-abf9-a00d0d666739
seq: 21
form: page
type: gotcha
title: 'Live local-model check on a disposable host: engines build + signed runner, and the droplet SSH key'
description: 'A plain cross-built bashy has no inference engine (the door reports ollama-local present=false and answers 404 for every model); a live local-model check needs -tags bashy_engines,embed_runner with the signed runner staged by scripts/fetch-signed-runner.sh OS ARCH (CGO_ENABLED=0 cross build then works). A DigitalOcean droplet create must pass the account-registered ssh key id (ssh_keys) or root''s expired password blocks non-interactive SSH even with cloud-init keys. WHEN proving an unpriced/local-model path on a test host that is not the dev box (keywords: ollama-local, engines build, embed_runner, signed runner, droplet, ssh_keys, password expired, S607 F4)'
status: candidate
source:
    tool: claude
created: "2026-11-10T22:08:32Z"
---
