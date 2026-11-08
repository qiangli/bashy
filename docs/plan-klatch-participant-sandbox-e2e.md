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

1. `bashy python -m unittest test_run test_participant` in `tests/klatch-e2e`.
2. Build the image: `bashy oci build -f tests/klatch-e2e/Containerfile.server
   -t localhost/klatch-e2e:REV KLATCH_TREE`.
3. Build the helper at the staged helper commit: `bashy npm ci && bashy npm run
   build` in `app/client/cli` (needs `app/sdk` in the tree).
4. Run `run.py --suite participant` with `--image`, `--helper`, `--helper-rev`,
   `--node`, `--wrapper` (the canonical `examples/klatch/klatch.bsh` from the
   wrapper story) and `--evidence`; require `result: PASS`, `server_stop: done`
   and no leftover `klatch-e2e-*` container.

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
