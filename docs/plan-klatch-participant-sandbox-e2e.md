# Plan: Klatch participant sandbox E2E (Sprint 597, story f5b524878fb0)

Umbrella plan of record: `docs/sprint-597-master-execution-plan.md` item 3 —
extend the existing public `tests/klatch-e2e` harness *and its sandbox
lifecycle* to exercise the participant CLI, sealed messaging and attachments,
and the canonical Bash# wrapper.

## Architecture (manager correction, 2026-11-08)

`tests/klatch-e2e/run.py` owns the sandbox. `participant.py` is an imported
journey helper only — it never builds, starts or stops a server.

```
run.py --suite participant --topology none
  preflight (node, helper)              -> BLOCKED, no container
  default_start_server                  -> probe URL (occupied => BLOCKED)
                                           bashy oci run -d -p 127.0.0.1:URLPORT:CPORT IMAGE
                                           wait_for_health
  participant.run_journey(args, url)    -> provision (refuses a claimed store),
                                           login, seal, contacts, conversation,
                                           messages, attachments, watch,
                                           wrapper, logout
  default_stop_server                   -> bashy oci rm -f (always, once started)
  evidence record (suite=participant)
```

- Only topology `none` is accepted for the participant suite: the journey
  claims the server's admin and creates accounts, so it may only run against
  a disposable container. Every other topology is BLOCKED without contacting
  a server.
- Accounts are disposable: they live in the container's fresh store and are
  dropped by `oci rm -f`.
- `Containerfile.server` builds a lean server-only image (pure Go,
  `CGO_ENABLED=0`) from a Klatch tree; it listens on 7100. On a host where
  that port is already in use, pick a free high local port:
  `--url http://127.0.0.1:FREE/ --container-port 7100`. The harness only ever
  talks to the container it started; it never targets an existing service.
- POSIX-only checks (0600 session/download files, SIGTERM clean exit) are
  recorded in `skipped_checks` on Windows, never counted as passed.

## Gate (shared test host only, via `bashy outpost ssh exec`)

1. `bashy python -m unittest test_run test_participant test_roster` in `tests/klatch-e2e`.
2. Build the image: `bashy oci build -f tests/klatch-e2e/Containerfile.server
   -t localhost/klatch-e2e:REV KLATCH_TREE`.
3. Build the helper at the staged helper commit: `bashy npm ci && bashy npm run
   build` in `app/client/cli` (needs `app/sdk` in the tree).
4. Run `run.py --suite participant` with `--image`, `--helper`, `--helper-rev`,
   `--node`, `--wrapper` (the canonical `examples/klatch/klatch.bsh` from the
   wrapper story) and `--evidence`; require `result: PASS`, `server_stop: done`
   and no leftover `klatch-e2e-*` container.
5. Run `run.py --suite roster` with `--image` (a build from a klatch tree at
   `5334662c` or later; see #Roster suite) and `--evidence`; require
   `result: PASS`, `server_stop: done` and no leftover `klatch-e2e-*`
   container.

## Roster suite (Sprint 619 e2e-c, story 41c78ac63f30)

`run.py --suite roster --topology none` reuses the participant sandbox
lifecycle (disposable `bashy oci run` container, fresh store, health wait,
teardown by `oci rm -f`) with two differences: the container is started with
`-e KLATCH_SSO_SECRET=<secret>` (from `--sso-secret` /
`KLATCH_E2E_SSO_SECRET`, generated fresh when unset; the same value signs
the journey's outpost identities), and the journey is `roster.py`, which
needs neither Node nor the helper — only the server.

Steps (all dependent; any failure or block stops the run):

- `setup` — refuse a claimed store, probe the roster endpoint, bootstrap the
  admin through `examples/klatch/provision.py` (the same client
  `bashy klatch sync` uses) and set the empty federation domain.
- `push` — `PUT /admin/sso/roster` for owner + one member
  (`{members: 2, created: 2, enabled: 0, disabled: 0}`), then the identical
  push again (`{members: 2, created: 0, enabled: 0, disabled: 0}`).
- `signin` — the outpost single sign-on the server implements
  (`POST /account/sso` with the HMAC-signed identity headers, then the
  one-time code through `PUT /account/access`): owner and member sign in
  with distinct guids, an unlisted email is refused (403) and an unsigned
  request is refused (401).
- `team` — the Team channel is owned by the roster owner (the owner's own
  channel listing carries subject `Team`, the member's does not) and the
  roster meshed the members into mutual connected contacts.
- `remove` — roster with the owner only
  (`{members: 1, created: 0, enabled: 0, disabled: 1}`), the member's
  contacts are dropped, its sign-in is refused, and the disable itself is
  proven by re-adding the member (`{members: 2, enabled: 1}` — `enabled`
  only counts accounts that existed disabled), after which it signs in
  again.
- `federation` — an external-node contact is refused with the empty domain:
  a fully signed identity DataMessage carrying a foreign node (built in
  `roster.py` with the standard library only — RSA PKCS#1 v1.5 over the
  SHA-256 digest, exactly what the server's `ReadDataMessage` verifies)
  answers 405, as does a node-less identity whose guid the store has never
  seen.

**Image selection (the released 1.2.7 image predates the endpoint).**
`PUT /admin/sso/roster` shipped in klatch `5334662c`; the pinned v1.2.7
release (`builds/klatch`, commit `febff05b`) does not carry it. The suite
therefore probes the endpoint first (a PUT with an invalid token answers
401 when the route exists) and is BLOCKED — never a pass, never a silent
skip — with the build instructions when the image answers 404. Select a
local build exactly as the participant suite does: `--image`, else
`KLATCH_E2E_IMAGE`, else the first local image whose name mentions klatch:

```sh
bashy oci build -f tests/klatch-e2e/Containerfile.server \
    -t localhost/klatch-e2e:5334662c /path/to/klatch-tree
python3 run.py --suite roster \
    --image localhost/klatch-e2e:5334662c --evidence roster.json
```

Offline coverage (no container): `test_roster.py` unit-tests the counts,
header HMAC, refusal classifiers and the DER/RSA builders, and drives the
real `Journey` and steps against `FakeKlatch`, an in-process server
faithful to the roster contract (the same pattern as
`examples/klatch/test_provision.py`).

## Measured on the shared test host, 2026-11-08

All runs: isolated scratch directory on the shared test host (native Windows
side), bashy `9187e1f0`, Node v22.23.2, a free high local port mapped to
container port 7100, server image built by `Containerfile.server` from the
Klatch tree at `8a12a2c9` (`net/` is byte-identical at `5cf927d1`, so the
image is unchanged for the final helper). Every run below started its own
`klatch-e2e-*` container, ran the journey against it, removed it with
`oci rm -f` (`server_stop: done`) and left 0 `klatch-e2e-*` containers.

Unit: `bashy python -m unittest test_run test_participant` — 83 tests OK
(1 POSIX-mode skip) at `0a3bfe1c`; 84 OK (1 skip) with the `helper_dist_sha256`
test added in the follow-up commit.

| Run | Helper | Wrapper (`klatch.bsh` sha256) | Result |
|---|---|---|---|
| accept-2 | `8a12a2c9` | `099c5e09` (`6a3edd81…15e4f`) | **FAIL** — `conversation` `HTTP_404` |
| diag-1 | `8a12a2c9` + one-line scratch patch | `099c5e09` | PASS — diagnostic only, not acceptance |
| final-1 | `5cf927d1` (fresh `npm ci && npm run build`) | `374d29cc` (`7e1fd493…2694`) | **FAIL** 9/10 — `wrapper` |
| h5cf-w099-1 | `5cf927d1` | `099c5e09` | **PASS** 10/10, 49 checks, 57 CLI calls |
| **final-2** | `5cf927d1` (`helper_dist_sha256` `811e6d3f…85b5d`) | `c97d3052` (`6076ef47…a15c`) | **PASS** 10/10, 49 checks, 57 CLI calls — acceptance |

Skipped on Windows in every run (never counted as passed): `session_0600`,
`download_private`, `sigterm_clean_exit`.

- accept-2 root cause (helper): `scope()` in `app/client/cli/src/commands.ts`
  built the contact token from the caller's own guid; the SDK uses the
  contact's `profile.guid`, so `ParamContactToken` found no card. Fixed by
  helper commit `5cf927d1`; its built `dist/commands.js` differs from the
  `8a12a2c9` build while `dist/main.js` is identical — which is why evidence
  now also records `helper_dist_sha256` over the whole `dist/` tree.
- final-1 root cause (wrapper): the `374d29cc` hardening's credential-name
  match `*seal*` also refuses the non-secret boolean `sealed:=false` on
  `conversations-create` (`SECRET_VIA_ARGV`, exit 2). Owner: wrapper story;
  the journey is not changed to avoid it.
- Server finding (no server change made): `GetSessionDetail`/`ParamAgentToken`
  use GORM `Find`, which never yields `ErrRecordNotFound`, so a revoked app
  token gets 200 with an empty session instead of 401. The journey's logout
  check accepts a refusal or an answer that no longer resolves the account.

**Final gate passed.** final-2 ran this branch's harness at `c495662c` (unit
84 OK, 1 skip, in the same scratch) with the committed helper `5cf927d1` and
the committed final wrapper `c97d3052` (whose `sealed` exemption fixed
final-1): `result: PASS`, every step `pass`, `server_stop: done`, teardown
done, 0 leftover `klatch-e2e-*` containers; no pre-existing service on the
host was contacted or changed.
