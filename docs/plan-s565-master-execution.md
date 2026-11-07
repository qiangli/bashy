# Sprint 565 master execution plan: simultaneous satellites

Review date: 2026-11-07. Manager: `claude-opus5.5-s565`. Target: ready
before Sunday 2026-11-08. Re-scoped by the operator on 2026-11-07.

## Goal

One host's apps (Klatch first) are reachable through several satellites
at the same time. A member of any of those satellites signs in with that
satellite's identity and joins the same Klatch team.

## Approach (KISS)

**One outpost process per extra satellite**, not a multi-satellite daemon.
Today `agent.json` holds one satellite (address, token, access token, host
name, auth URL, ticket key) plus the app list, and the daemon runs one
tunnel. Making the daemon multi-satellite would need one local listener
per satellite (identity is trusted per satellite) and per-satellite
credential refresh, heartbeat and host-cert handling. A separate process
already has all of that.

What already exists:

- Config and cache roots follow `XDG_CONFIG_HOME` / `XDG_CACHE_HOME`
  (`outpost/internal/agent/conf/paths.go`); the admin port follows
  `OUTPOST_ADMIN_ADDR`.
- Sprint 497: `register` with a non-default config dir leaves the host's
  OS service alone; a non-default instance does not self-upgrade a shared
  binary; `OUTPOST_SUPERVISED` is stripped from child sessions.
- Sprint 522.7: `outpost-profile add <name> --name <host>` pairs one
  machine with several satellites under a host name per satellite.
- No extra proxy: every instance publishes an app entry pointing at the
  same local backend (Klatch on `127.0.0.1:7100`) with the same SSO secret,
  so Klatch verifies the signed identity from either instance.

## Stories

| Order | Story | Exit evidence |
| --- | --- | --- |
| S1 `a009204f` | Spike: second instance next to the main one, unpaired then paired; list collisions and the setting that avoids each | findings recorded on the story |
| O2 `4dfaaef18f4a` | `outpost-profile up/down/status`; S2b only if S1 finds a collision that config cannot turn off | red/green tests in `examples/outpost-profile` |
| O3 `030a968b059b` | Klatch through zoo and zoo-aws at once on M, L and native W; down one, other keeps working; Windows reboot | raw logs and identities per platform |

## Limits (MVP)

- Cluster mode, the ollama pool, peer mesh and LAN ssh run on the main
  instance only.
- The same email from two satellites is one Klatch account.
- The satellite-less single-node outpost (O1 `f7459c010953`) is deferred
  (operator, 2026-11-07); it stays a standalone p3 todo.

## Risks

- A seeded service unit or listener in the second instance that no
  setting turns off: S1 finds it first; the fix is the smallest outpost
  change with a red/green test.
- Bashy service units passing the instance environment on Windows.

## Stopping rules

- Never change the main outpost's config or OS service during S1.
- A deadline miss is recorded as retained evidence and a diagnosis, not
  retries or skipped checks.
- Satellite gaps become support tickets; no satellite shaping.
