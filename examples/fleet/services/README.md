# Fleet service recipes

These are third-party `kind: service` declarations for the generic bashy
service registry.  They are not built-ins and do not need a service-specific
supervisor, toggle, or exposure path.

Add one with `bashy service add -f FILE`, then configure its vault references
and enable it with `bashy service enable NAME`.

- **codync-host** (`codync-host.yaml`): Codync ACP host for paired messenger
  input and agent control. Pinned download of the official GitHub release
  (`leepokai/Codync` v2.13.2) verified per-platform, supervised in the
  foreground by `bashy supervisord`, exposed to the mesh.
- **openclaw-gateway** (`openclaw-gateway.yaml`): OpenClaw gateway for
  messenger ingress and the bashy LLM door. Uses a pinned OCI image digest
  (`ghcr.io/openclaw/openclaw@sha256:d0ded1dd76939b2bf4d67ef2d13247b8b160aa5666331d4a0b0e58811182cbb8`)
  invoked via a generic OCI launcher (`podman`) in the foreground, preserving
  the full Node dependency tree without bespoke package managers, declared HTTP
  health `/health` on port 18789, app tunnel elevation and mesh exposure, and
  vault secret references for bot tokens.

Both recipes are intentionally **unverified**.  Do not stamp them verified
until a live S16 monitor and S18 exposure run has retained its gateway logs.
