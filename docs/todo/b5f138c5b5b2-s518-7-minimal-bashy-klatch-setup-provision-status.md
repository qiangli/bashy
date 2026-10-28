---
id: b5f138c5b5b2
kind: feature
title: S518.7 minimal bashy klatch setup/provision/status command
seq: 2815
status: done
priority: p0
labels:
    - klatch
created: 2026-10-28T05:42:55.900235Z
due: 2026-10-31T00:00:00Z
assignee: claude-sonnet5.5
sprint: 518
sprint_id: bbe9153b-b299-5be8-8ef8-215ad002deb9
sprint_title: 'Klatch MVP: rebranded Databag chat server + web + federation on one satellite (replaces Matrix), due Sat 10-31'
closed: 2026-10-28T08:31:29.205884Z
closed_by: s518-manager
---

S518.7 REWORK (bug found by S518.10 live E2E): provisioned contacts are only "confirmed", never "connected", so group/DM notifications never reach members (bob/carol never see alice's group; DB notified_channel stays 0).
Fix examples/klatch/provision.py (bashy repo) so provisioning completes the SAME connect handshake the web client does (verified live in umbrella docs/sprint-518/k8-federation.md "Public HTTPS transcript"):
  for each owner<->member pair after both identity cards exist (POST /contact/cards both ways):
  1. PUT {A}/contact/cards/<A-card-for-B>/status?agent=<A-agent> body "connecting"
  2. GET {A}/contact/cards/<A-card-for-B>/openMessage?agent=<A-agent>  -> signed open message
  3. PUT {B}/contact/openMessage  body = that message  -> status "requested"
  4. PUT {B}/contact/cards/<B-card-for-A>/status?agent=<B-agent> body "connecting"
  5. GET {B}/contact/cards/<B-card-for-A>/openMessage?agent=<B-agent>
  6. PUT {A}/contact/openMessage body = that message -> status "connected"
  7. PUT {B}/contact/cards/<B-card-for-A>/status?agent=<B-agent>&token=<token from step 6 response or card detail>&viewRevision=..&articleRevision=..&channelRevision=..&profileRevision=.. body "connected" (read the exact field names from the k8 doc and the server handlers net/server/internal/api_setCardStatus.go / api_setOpenMessage.go in github.com/qiangli/klatch)
  until GET /contact/cards on both sides shows status=connected. Idempotent: skip pairs already connected.
Also: send writes with 'Origin: <scheme://host of base_url>' and logins with 'Credentials: Basic' (front strips Authorization) so the provisioner also works against a public base_url.
Add `reset-passwords CONFIG HANDLE...`: rotate the listed users' passwords using the server's admin account-reset flow (find it in net/server: admin access token + account reset/auth token endpoints) and the vault refs in CONFIG (generate new values with `bashy secret gen | bashy secret set REF`; never print). Unit tests with the fake server for the handshake (incl. idempotent rerun) and reset.
Then LIVE on the deploy host (novidesign; ACCESS via `bashy claim list | grep s518`; scratch ~/s518-klatch): copy the new provision.py, run reset-passwords for bob and carol (their old passwords leaked into a worker log), re-run provision against node A (http://127.0.0.1:7100) and prove alice<->bob and alice<->carol are status=connected and that bob's GET /content/channels?agent=<bob> now lists group "S518 test". Never print secrets.
Verification: unit tests pass (counts); live transcript (status codes, no secrets, no hostnames in bashy commit messages) in the commit message.

== RULES (all S518 workers) ==
- Your cwd is your isolated workspace (a full git clone). Stay inside it; relative paths only; never follow git remotes. Do not push. Do not switch branches. git add files BY NAME. Keep the bashy-injected BASHY_AGENT and the commit trailers weave gives you.
- Plan of record: umbrella docs/sprint-518-master-execution-plan.md (Klatch = our rebranded fork of Databag, github.com/qiangli/klatch, pinned to upstream main 45343338582b394f4f5fdb4b7e271ea1dc8dd5fd; deadline Sat 2026-10-31 PDT).
- Dragon (this machine) is for editing + short unit tests only: no podman/docker, no full builds of large trees, keep disk use small (delete node_modules you created before finishing).
- Public repos (klatch fork, bashy, yoke): NEVER write real hostnames, IPs, user names or emails into files or commit messages. Hosts come from env/config only.
- Remote Linux access when your story needs it: read `bashy claim list | grep s518` — the host:alien:s518-klatch intent carries the ACCESS line (outpost ssh ...). Use only /opt/s518-klatch on that host, loopback ports 7100/7101 (7000 is taken). Never touch cloudbox, DKS, k3s or the 80/443 front there.
- Credit: Klatch is based on Databag by Roland Osborne (github.com/balzack/databag), Apache-2.0. Keep upstream LICENSE and copyright headers.
- Done = the story's verification passes and is committed. State EXACT measured results in the commit message. If stuck after 3 serious attempts, commit what works plus a BLOCKERS.md with diagnosis, then exit 0. Headless: exit cleanly (exit 0) when done. COMMIT YOUR WORK.
