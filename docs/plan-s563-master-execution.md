# Sprint 563 master execution plan: product UAT through Klatch group chat

Review date: 2026-11-03. Manager: `claude-opus5.5-s563` (conductor lease
taken 2026-11-03). Develops Sprint 563 under `plan-clean-baseline-campaign.md`.
Draft for operator review: no story text has been changed yet.

## Main goal

**Any Bashy user can run group chat for the people they invite, from a clean
Bashy install.** A host owner installs Klatch on their own outpost as a Bashy
service, publishes it through their satellite, and every cloudbox member they
share it with opens it, signs in once with their cloudbox identity and is
already connected with the rest of the team. Klatch is a Bashy feature for
all Bashy users, not a deployment for one group.

Two product properties are part of the goal:

- **Who can join.** Any cloudbox user can be invited: a member needs only a
  cloudbox account and a browser to join the chat on the host owner's
  satellite — no satellite, outpost or Bashy of their own. The host that
  runs Klatch always has outpost installed and paired.
- **Software-shapeable.** Software shaping is a Bashy feature for every Bashy
  user and every Bashy app, not something only a satellite owner can do: any
  user changes an app's source on their own host, builds and tests it there,
  runs it in place of the pinned release and can return to the release.
  That general workflow is **deferred to Sprint 564** (F14). Sprint 563
  only keeps Klatch shapeable-ready: the pinned release rebuilds from its
  pinned source on the host through Bashy's existing build path, and the
  manual elephant shaping is handed over as that sprint's first worked
  example.

**Real-user UAT.** The 2026–2027 Presidential SI Challenge group (`psic2027`)
is the first real cohort: its members use it from school, home and the
library, starting by Saturday 2026-11-07. Their use is the UAT evidence; any
cohort-specific setup (who is invited, topic names) is test data, not scope.

Each step a host owner or invited member takes is a journey with a defined
input and expected outcome. Klatch's own vocabulary is host owner, member and
team; role words of a particular cohort (teacher, student, class) never
appear in the product. Every defect met on the way is fixed at its
owning layer (C here, A/B via 561/562, satellite issues by support ticket)
and the affected journey is rerun.

Production target: the features this UAT needs — single sign-on with the
trusted cloudbox identity, team mode, native Windows — ship in the **product**
(the Klatch fork's release and Bashy's pinned tool and service), and Klatch
runs **natively** on the outpost. The elephant container and its host-scoped
shaping branch are the test vehicle and the weekend fallback, not the end
state.

Out of scope: release publication, shared-install replacement, cloudbox
(satellite) code changes — the satellite is proprietary, so its gaps become
support tickets, never shaping.

## Current state (2026-11-03)

- Card: backlog → manager assigned; 0/4 stories closed; PRIORITY-FIRST.
- Live: Klatch 1.2.5 + elephant shaping (single sign-on, team mode) runs in
  a podman container on `elephant` (native Windows, x86_64), published
  through the `zoo` satellite with login + trusted cloudbox identity.
  Shaping commits `469031ce`, `0fe385ad` on klatch branch
  `shape/elephant-sso` exist only on elephant.
- 2026-11-03: the `klatch` app entry had vanished from elephant's outpost
  config (cause unknown); restored and secret refreshed. Browser sign-in not
  yet re-verified.
- C3 fix ready: inbox refusal for a registered agent with several live
  instances, branch `s563/c3-inbox-registered-agent` (`51998d67`), not merged.

## Journey matrix (C1)

Each journey has one expected outcome and one or more failure checks.
Platforms: **W** = elephant (native Windows), **M** = macOS arm64 (dragon),
**L** = a claimed Linux test host. Satellite: `zoo`; J8 adds `zoo-aws`.

| ID | Journey | Expected outcome | Failure checks | Platforms |
| --- | --- | --- | --- | --- |
| J1 | Clean install: `bashy` + `outpost`, pair to satellite | host listed on the satellite; `outpost status` active | wrong invite refused | W, M, L |
| J2 | Install Klatch natively as a Bashy service from the pinned, signed release | service running on loopback 7100; `/admin/status` 200; signature verified | tampered archive refused | M, L now; W after F2/F3 |
| J3 | Provide Klatch's media dependencies through Bashy only (`ffmpeg`, ImageMagick, `sh`) | image/video thumbnails produced | missing tool reported, never silent | W, M, L |
| J4 | Publish via outpost with login + trusted identity | app reachable only through the satellite; survives an outpost restart | anonymous 401; app entry still present after restart (F1) | W, M |
| J5 | Host owner setup: one Bashy command bootstraps the admin, sets the domain, enables single sign-on and team mode | admin claimed; sign-ups closed; team topic exists; no Python or manual API calls | second bootstrap refused | W, M, L |
| J6 | Invited member with only a cloudbox account (no satellite, outpost or Bashy): added as a member + app shared → open tile on the owner's satellite → single sign-on → chat in team topic | lands in chat with no Klatch login; sees teammates; messages delivered both ways | unshared member refused; forged/unsigned identity refused | W (UAT cohort browsers at school/home) |
| J7 | Host reboot | Klatch and its publication come back without manual steps | — | W, M |
| J8 | Move the host from `zoo` to `zoo-aws` | app re-registers; same accounts/chats; domain updated | — | W |
| J9 | Agent coordination used to run this sprint (`bashy mb`, `inbox`, `sprint`) | managers read their mail; sprint commands finish promptly | registered identity never refused (F9) | M |
| J10 | Shapeable-ready: rebuild the pinned Klatch release from its pinned source on the host with Bashy's existing build path | rebuilt server and web bundle match the pinned source commit; recorded build identity | build from a source that does not match the pin is refused | W, M, L |

J6 with the UAT cohort is the weekend gate. J2–J6 and J10 on the product
release (no host-scoped shaping), native on every platform, is the production
gate. The general shaping workflow is not a 563 gate (see F14).

Automated happy paths (11-04):

- J6 in the browser: `tests/klatch-e2e/run.py` drives the owner's real
  Chrome through the browser hub (sign-in gate, single sign-on into the
  session, topics listed, post and read back; exit 2 = BLOCKED). PASS on
  elephant via zoo, 11-04 11:27.
- Server side: klatch `TestVerifyOutpostIdentity`, `TestSetSSOAccess`,
  `TestSSOTeam` (all pairs connected, topic membership) and
  `TestSSOTeamMessaging` (member posts, another member reads it), in
  `shape/elephant-sso` `c5cac3f9`.
- Negative paths: unsigned or forged identity refused (401), anonymous
  access gated by the satellite, disabled account refused.

## Priorities (operator, 2026-11-03)

The card runs PRIORITY-FIRST; invite and chat come before everything else.

| Priority | Work |
| --- | --- |
| P0 | Invite/chat: J4, J6, J7 on elephant for the UAT cohort; F1 (app entry vanished); T3 load check; F9 merge so managers can coordinate |
| P1 | Invite/chat in the product: single sign-on, team mode and named owner released in the Klatch fork and pinned by Bashy (F8); J5 setup command (F4); J1 |
| P2 | Native Windows (F2, F3); J2/J3 native; J8 satellite move; J10 shapeable-ready |
| P3 | F5–F7, F10–F13 unless they block P0/P1 |
| Deferred | General software shaping → Sprint 564 (F14); satellite-less single-node outpost by default, joining and switching satellites → Sprint 565 (J1, J8) |

## Story chain

| Order | Story | Gate to start | Exit evidence |
| --- | --- | --- | --- |
| C1 `883c9048b024` | journey matrix above, with inputs/expected outcomes and prerequisites | now | this table agreed by operator; card text updated |
| C2 `86c744779ff1` | early smoke: run J1–J9 as far as they go, keep raw failures | C1 | per-cell result or blocker; every defect routed (table below) |
| C3 `f04323bd2836` | fix C-owned defects, consume A/B fixes, rerun affected journeys | per-finding baseline from C2 | red/green per fix; affected journey rerun |
| C4 `00480e8f2c3c` | final UAT from clean install on frozen candidates | C3 + stable 561/562 candidates | every cell passes; limitations documented |

## Phase 0 — weekend UAT start (by Fri 2026-11-06)

The elephant container stays; goal is J6 working for the UAT cohort, with
every problem they hit recorded as a finding against the product.

1. Re-verify browser single sign-on after the 11-03 app restore (operator).
2. Diagnose F1 (vanished app entry) before the cohort starts. No custom
   watchdog: outpost's own OpenTelemetry export is enabled on elephant
   (user environment `OTEL_EXPORTER_OTLP_ENDPOINT=127.0.0.1:31416`, the local
   `bashy otel serve` gRPC port; `OTEL_SERVICE_NAME=outpost`), effective at
   the next outpost restart, so a later config change is searchable in the
   local VictoriaLogs. Until then, reinstall by hand if the entry is lost.
3. Team-topic owner: the host owner's choice of account (named-owner setting
   `KLATCH_SSO_TEAM_OWNER`; product setting, prototyped on elephant first).
4. Load check: several members at once from one network (satellite 429 risk).
5. J7 on elephant: reboot, confirm podman machine + container + publication
   return (claim elephant first; other sprints use it).
6. Operator invites the cohort and shares the app; dry run with two accounts.

## Phase 1 — product (C3, after the weekend)

1. Land single sign-on, team mode and the named owner in the Klatch fork with
   their tests; cut a release; bump Bashy's pinned tool and service (F8).
2. Replace the Python provisioner with a Bashy command (F4, J5).
3. Native Windows: pure-Go SQLite build and Bashy-provided media tools
   (F2, F3); run J2–J7 native on W, M, L.
4. Shapeable-ready (J10): confirm the pinned Klatch release rebuilds from
   its pinned source on W, M and L through Bashy's existing build path; hand
   the elephant shaping record to Sprint 564 (F14).
5. Re-run the cohort's journeys on the product release, then retire the
   elephant shaping branch and container.

## Defect routing (C2/C3)

| ID | Finding | Evidence | Owner |
| --- | --- | --- | --- |
| F1 | **Confirmed root cause.** The 12 h cluster-token rotation (`vknode` refresher → `persistClusterCredential`, `cmd/outpost/main.go:2908`) saved the daemon's boot-time `FileConfig` snapshot over `agent.json`, erasing every app added since boot. Reproduced live: 11-04 00:03:06 `vknode: refresh ok`, and `agent.json` lost the `klatch` entry (the only difference from `agent.json.1`); same at 11-03 00:03 and 12:03. No CLI call involved (`cli-trace.log`). Fix: outpost `e4d3e6a` (branch `s563/f1-cluster-credential-keeps-config`) applies the credential to the file as it is now, like the host-cert refresh; red/green tests in `cmd/outpost`. Until a fixed outpost runs on elephant, klatch must be re-added after each rotation (next ~12:03 and 00:03) | `~/.config/matrix/agent.json*`, `~/klatch/f1-agent.json.{before,after}-0003`, `daemon.log` on elephant | C3 (outpost) — P0 |
| F16 | Bashy on native Windows intermittently reports an external program as "command not found" (`curl.exe`, `where.exe`) although `command -v` resolves it; seen in pipelines | elephant session 11-03 | A3 (561) — shell exec/lookup |
| F21 | Team join is slow under a crowd: 8 simultaneous first visits take 12–15 s (serialized team join connects each newcomer with every member, O(n) handshakes each). Correct (all 56 pairs connected, `TestSSOConcurrentFirstVisits` 5/5, klatch `24294689`); repeat visits are fast | concurrent test on elephant 11-04 | C3 (Klatch, P1 performance) |
| F19 | Klatch web client: Enter does not send a message (adds a line); only the send icon sends. Members may expect Enter to send | J6 browser run 11-04 | C3 (Klatch web, P2) |
| F20 | Klatch opened SQLite with no busy timeout, and the SSO first visit ran a read-then-write transaction: concurrent sign-ins failed with "database is locked" (2 of 5 test runs). Fixed in klatch `c5cac3f9` (`_busy_timeout=5000`, write-only sign-in transaction, config reset fix); `_txlock=immediate` rejected (broke TestContactSync). Deployed on elephant as `klatch:1.2.5-sso-team2` 11-04 (rollback container `klatch-team-pre-lockfix`, backup `backup-klatch-data-pre-lockfix.tar`) | SSO tests 6/6 separate runs | C3 — fixed |
| F18 | `bashy browser hub --port N` advertised but rejected (top-level parser takes GNU-interspersed flags); hub ignored `--live-port` and always bound 58082. Fixed: hub listens on `--live-port` (yoke `e57567cf`, red/green `TestHubListensOnLivePort`); new opt-in `browser-hub` service unit so the owner or an agent starts it from the outpost admin (yoke `68a20e27`, story C3b `0bb55463319c`). Ships with the next Bashy yoke pin bump (S561 freeze now lifted; verify the current consumer pin) | live run on dragon 11-04 | C3 — fixed |
| F17 | Sprint goal links kept the pre-migration story repo path (`~/projects/poc/bashy`, renamed `bashy.stale-20261103`), so 563's stories showed as dangling; relinked to `umbrella/bashy` with `sprint goal link` | `sprint show 563` 11-03 | C3 (migration follow-up) |
| F2 | Klatch has no `windows/amd64` build (cgo SQLite driver) | release assets linux/darwin only | C3 (klatch build) |
| F3 | Transform scripts need `/bin/sh`, `ffmpeg`, ImageMagick | `transform/*.sh` | C3 (provisioning) |
| F4 | Klatch provisioner needs `python3`; absent on elephant | `examples/klatch/provision.py` | C3 (bashy command) |
| F5 | **Confirmed by J7 on puppy (11-06).** After a Windows reboot the podman machine is listed "Currently running" but its API refuses connections (`127.0.0.1:50081`) until `podman machine stop/start`; outpost and its app entry came back on their own | J7 reboot 09:54, puppy | C3 (Bashy podman service) — P1 |
| F25 | Restart policy `always` is not honored when the podman machine starts: the Klatch container stayed Exited after the VM restart; only `podman start` brought it back (then 200). Native Klatch under Bashy's supervisor avoids this; container apps need the podman service to restart `always` containers at machine start | J7 on puppy 11-06 | C3 — P1 |
| F24 | puppy's Bashy-managed podman machine (WSL, rootful, user-mode networking off) does not forward published container ports to Windows localhost; Klatch published via the VM address `172.17.254.194` (unchanged across the reboot) | J7 setup on puppy 11-06 | C3 (Bashy podman service) — P2 |
| F23 | `outpost ssh exec` ends long-running commands silently (exit 0, no output); a `podman build` was cut off. Workaround: start detached and poll a log | puppy 11-06 | C3 (outpost) — P2 |
| F22 | `outpost ssh exec` joins arguments into one shell line without quoting: `~`, `$VAR`, backslashes, `{{…}}`, quotes and parentheses are mangled. Workaround: direct commands with forward-slash paths, or `powershell -EncodedCommand` | puppy 11-06 | C3 (outpost) — P2 |
| F6 | Piping stdin into Bashy's SSH shell on Windows hangs | session 11-01 | C3 |
| F7 | Bashy built-in `git`: no `checkout -B B START`, no `-q` | session 11-01 | C3 |
| F8 | **Released 11-06.** SSO + team mode merged in the Klatch fork (PR #1, `474bebae`), tagged `v1.2.6`, signed build `bashy-v1.2.6-1` (run 42263928634, cosign verified, 5 archives), Bashy build pin `8c3cd6d0`, yoke fleet pin `54262614` (yoke main `698d3161`), Bashy yoke pin `ea21154c`. Remaining: a Bashy release carrying it (rc.27; rc.25/rc.26 intentionally unused) and switching elephant from the shaped container to the released native service | was `shape/elephant-sso` | C3 — released; rollout pending |
| F9 | `inbox --as` refuses registered agent with 2+ live instances | red/green test; blocks `conductor:561` mail | C3 — fixed `51998d67`, merge pending |
| F10 | `sprint take` 4m34s (3.5 min CPU); `sprint show` > 90 s | timed 11-03 | C3 |
| F11 | **Fixed 11-06** (`ea21154c` yoke, `62f5106b` coreutils; all four src pins match go.mod). Was: `TestSrcPinsMatchLinkedModules`: yoke src pin out of sync | fails on `main` | C3 (migration follow-up) |
| F12 | `TestRunSignaledIsNonLossy` fails on `main` | fails on `main` | A3 (561) — to confirm with 561 |
| F13 | `TestCommandDocSourceNames` fails on `main` | fails on `main` | C3 |
| F15 | Outpost `sshd` unit restarts every few seconds on Windows (`firewall: show rule "outpost sshd": exit status 1`); `daemon.log` has grown to 3.4 GB | `daemon.log` on elephant | C3 (outpost) |
| F14 | No supported way for a Bashy user to shape an app on their host: elephant's shaping was manual (host git, two podman builds, image swap, secret copy) | session 11-01/11-02 | Sprint 564 (deferred, backlog); 563 keeps J10 only |
| T1 | Satellite forwards `admin` for every sharee; no custom roles | cloudbox `host_proxy.go:199` | support ticket |
| T2 | New app takes up to 60 s to become reachable | poller interval | support ticket |
| T3 | Anonymous rate limit (429) may hit many members behind one IP | provisioning 11-01 | support ticket (if reproduced in J6) |

Findings stay grouped in C3; a separate story only for independent ownership.

## Coordination

- 561 (A, `conductor:561`, held by `claude-opus5.5`): F12 and any shell or
  coreutils cause; F9 currently blocks its mailbox — workaround posted on the
  561 thread.
- 562 (B, `claude-opus5.5-s562`): any fence/language cause; shared-code
  fixes (sh, bashsharp, bashy, yoke) announced on its thread before landing.
- Test hosts: claim elephant/dragon/Linux host before reboots or long runs.

## Cost and risk

- **Weekend:** F1 is the main risk; a silent loss of the app entry takes the
  chat down for everyone. Container is the fallback for native work.
- **Native Windows (F2/F3):** a pure-Go SQLite driver swap plus Bashy-provided
  media tools; moderate, reviewed before implementation.
- **F8:** moving the elephant shaping into the Klatch fork makes it the
  default for every Bashy user; review the security path (signature check,
  account creation on first visit) before the release.

## Open decisions (recommendations)

1. Merge `51998d67` to `main`: yes. Replacing the installed `bashy`: operator
   call, needed for F9 to help 561 now.
2. Team-topic owner: a product setting the host owner sets (named owner);
   until satellite roles exist (T1), first sign-in remains the default.
3. Native Windows Klatch and the product release of single sign-on and team
   mode: after the weekend, target C4.
4. Who can join (J6): confirmed so far only for members whose cloudbox account
   signs in on the owner's satellite. If members' accounts live on a
   different cloudbox than the owner's satellite, that identity path is a
   satellite capability — check in C2, ticket if missing (no shaping).

## Stopping rules

- Deadline hit = retained incomplete evidence and diagnosis, not retries,
  skips or timeout increases.
- No change to the live UAT deployment without a backup and a kept
  rollback container.
- Satellite gaps go to tickets; no satellite shaping.
