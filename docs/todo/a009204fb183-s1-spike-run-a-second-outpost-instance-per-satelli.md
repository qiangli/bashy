---
id: a009204fb183
kind: spike
title: 'S1 Spike: run a second outpost instance per satellite next to the main one'
seq: 3163
status: done
priority: p0
labels:
    - outpost
created: 2026-11-07T08:41:45.677628Z
assignee: claude-opus5.5-s565
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: Satellite-less single-node outpost by default; join and switch between satellites
closed: 2026-11-07T08:50:36.240861Z
closed_by: claude-opus5.5-s565
---

Run a second outpost with its own XDG_CONFIG_HOME/XDG_CACHE_HOME and OUTPOST_ADMIN_ADDR next to the host's main outpost, first unpaired and then paired with a second satellite. Publish Klatch through it as an app that points at the same local backend with the same SSO secret. List every collision (LAN sshd 2222, seeded service units, discovery, mesh, ollama, upgrade puller, OS service) and whether an existing config toggle turns it off. Exit: a written list of findings with the config that avoids each collision, or the smallest outpost change needed (S2b). Runs from a scratch directory on the dev box; never touches the main outpost's config or service. KISS/MVP: reuse existing code and harnesses; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

## Findings (2026-11-07, macOS dev box, outpost 5be964e built from the rc.27 pin)

Ran next to the host's main outpost (installed rc.16-dev, paired with zoo), first unpaired, then with a copy of this host's existing zoo-aws profile. The main outpost's config, its service store (~/.config/bashy/services) and its LaunchAgent were unchanged; its zoo tunnel and its apps/meet/otel-stack services kept running.

Recipe that isolates a second instance, with no outpost code change (S2b not needed):
1. Environment: XDG_CONFIG_HOME and XDG_CACHE_HOME set to the instance root; **BASHY_FLEET_DIR set to the instance's own store** (the service-unit store defaults to ~/.config/bashy and ignores XDG, so without it the instance shares the main instance's units); OUTPOST_ADMIN_ADDR set to a free loopback port (17778 used); OUTPOST_SUPERVISED unset.
2. agent.json: ssh_enabled, mesh_enabled, desktop_enabled, clipboard_enabled, shell_enabled and discovery_enabled set to false; cluster block removed.
3. Instance service store: disable sshd (the seeded unit is enabled on lan:2222 and collides), **shard (it starts the peer mesh even with mesh_enabled=false)**, ollama, ollama-pool, warm, files, podman, otel, apps, meet and otel-stack (`BASHY_FLEET_DIR=<store> bashy service disable <unit>`). Disabling units in the instance store did not affect the main instance's units.
With that, the instance binds only its admin port and its local app listener, logs in to the satellite (frp "login to server success") and publishes the apps added to it.

Publishing: `outpost apps add s1probe --target http://127.0.0.1:7199/ --require-login` through the instance. Anonymous requests through zoo-aws got the satellite login page and never reached the backend (0 backend hits). The signed-in positive path is left to O3 (needs a browser session).

Harmless leftovers (warnings only): storeroute cannot bind 127.0.0.1:31890 (the main instance serves it); repeated "service monitor link failed ... not activated by this monitor"; selfcheck warns that the instance has no ssh host key.

Open for O2/O3: credential refresh must persist to the instance's live agent.json (O2: the live file belongs to the profile, and `use` refuses a profile that is running as an instance); verify that the S497 guard declines self-upgrade for an instance on a release build (the spike's dev version was declined as non-semver); Bashy service unit environment passthrough on Windows; the signed-in member journey.
Spike instance, probe app and probe server removed afterwards. Raw logs are in the manager's scratch directory (not committed: they contain host details).
