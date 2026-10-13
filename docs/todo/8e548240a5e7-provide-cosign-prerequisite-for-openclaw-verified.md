---
id: 8e548240a5e7
kind: task
title: Provide cosign prerequisite for OpenClaw verified Grype installation
seq: 1934
status: done
priority: p1
created: 2026-10-13T18:50:01.341473Z
assignee: codex-gpt5.6-sol
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T20:06:47.75958Z
closed_by: s434-muse-manager
---

Implement precise manager review correction to child6c7487f8. Resume104 did NOT receive the correction and only repeated its old result; preserve0455165e and carry its branch forward from /Users/qiangli/.bashy/weave/bashy-6497d06f/workspaces/issue-104. Grype install.sh v0.120.1 -v requires cosign (script lines782-786); current OpenClaw BUILD job lacks it, release job alone installs it. Add existing pinned sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 with cosign-release v3.1.3 before Grype in builds/openclaw/build.yml; match PIN, rerender openclaw fork. Preserve checksum check, DOWNLOAD_TAG_INSTALL_SCRIPT=false and -v; no waivers. Short builds/check only. Commit under actual worker identity with THIS story trailers. No main/shared edits, remote hosts, pushes, containers or /tmp fork mutation. Initial ACK must name cosign build-job prerequisite. Report exact commit/diff/tests to s434-codex-manager within10m.
