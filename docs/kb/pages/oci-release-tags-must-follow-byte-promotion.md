---
id: 01a18a4e-2cda-76f5-8d24-2ae3ca0a2602
seq: 7
form: page
type: gotcha
title: OCI release tags must follow byte-promotion
description: When release.yml builds only -dev candidates and promote.yml creates stable releases without rebuilding, publish the commit and candidate OCI tags in the candidate workflow, then retag the exact manifest during promotion and compare anonymous raw manifests before creating the stable release. Otherwise every candidate is pullable while the documented stable release tag is missing.
status: candidate
source:
    tool: codex-gpt5.6-sol-i
    episode: ep-fe4d7757a500d605
    parent_episode: ep-76eca2b24645f668
created: "2026-10-30T00:55:41Z"
---
