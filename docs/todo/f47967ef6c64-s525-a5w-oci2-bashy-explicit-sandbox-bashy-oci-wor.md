---
id: f47967ef6c64
kind: bug
title: 'S525.A5w-oci2 bashy: explicit @sandbox()/bashy oci works out of the box on native Windows (machine created + started on first use)'
seq: 2844
status: done
priority: p0
labels:
    - windows
    - app
created: 2026-10-30T00:22:12.28074Z
weave: 345
assignee: claude-opus5.5
sprint: 525
sprint_id: 9a9a4443-02c8-53a3-b0bd-96bd67980643
sprint_title: 'Software Shaping foundation: verified personal features and same-host promotion'
closed: 2026-10-30T00:35:59.910679Z
closed_by: s525-manager
---

Part of S525.A5w (goal a5w, umbrella 944522763e8f), operator standing rule 2026-10-29: bashy features work the same on every OS with zero user setup; an explicit sandbox request uses bashy oci and must work out of the box on Windows, macOS and Linux; a platform where it fails is a bug. Shaping itself never needs a sandbox; this is ONLY for an app's explicit @sandbox() island (yoke contain.BashyOCI -> 'bashy oci run ...'), never for ordinary/genie commands (that design was rejected for S531.D3w; do not reintroduce it). Bug: on a fresh native-Windows host, 'bashy oci run' fails 125 with 'bashy podman is not running (create its VM once: bashy podman machine init && bashy podman machine start)'. Two gaps: (1) the oci/sandbox/docker/podman engine alias (internal/agentos/engines_common.go engineAlias) passes run/pull/etc. straight to podman without ensuring the engine, unlike text_fences.go and contain_container.go which call ensurePodmanMachineHook; (2) ensurePodmanMachineWith (internal/agentos/podman_machine.go) only probes/starts/restarts and never runs 'machine init' when no bashy machine exists. Fix (keep small, reuse the existing helper): for engine subcommands that need a running engine (run, create, pull, exec, ps, images, ...; NOT the 'machine' subcommands themselves), call ensurePodmanMachineHook first on non-Linux; extend ensurePodmanMachineWith so that when start fails because no machine exists it runs 'machine init' for bashy's own isolated machine (Windows: WSL2 provider) then start, then probe; serialise concurrent first calls (existing mutex is per-process; add a cross-process lock file under BASHY_HOME); keep the clear error if init/start truly cannot succeed (fail closed, never run the island unsandboxed). Tests: table over the injected runner (no machine -> init+start; stopped -> start; running -> nothing; broken -> clear error), plus engine-alias routing; NEVER start podman/VMs on Dragon (fake seam only). Live proof by the manager on elephant native Windows (fresh/stopped machine) + WSL: an app unit declaring @sandbox() runs inside the island; bare @sandbox never runs unsandboxed. Gate: go test -run 'Podman|Engine|Oci' ./internal/agentos + GOOS=windows go vet ./... .
