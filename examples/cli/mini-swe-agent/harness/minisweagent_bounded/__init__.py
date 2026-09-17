"""Bounded, self-contained rewrite of the mini-swe-agent harness core.

This package is a MINIMAL, stdlib-only projection of the mini-swe-agent control
flow: a system/instance prompt, a step loop that queries a model for a bash
command, executes it as an independent stateless Bashy process, feeds the
observation back, and stops on an explicit submit marker, a resource limit, a
repeated format error, or a user interruption. It is the LOCAL implementation
both entrypoints run — `main.bpp` (shell) and the ycode-gated YAML bridge — not
a wrapper around an installed upstream `mini`.

Substantive behavior preserved from upstream (see README.md manifest):
  - the Default/Interactive agent step loop and its exit-status taxonomy
    (Submitted / LimitsExceeded / TimeExceeded / RepeatedFormatError /
    UserInterruption),
  - the interactive subset: confirm/yolo/human modes, `/c /y /u /h /m` controls,
    Ctrl-C interruption, confirm-on-exit, fail-closed non-interactive behavior,
  - the submit protocol, output truncation, and the linear trajectory shape
    ("mini-swe-agent-1.1"), persisted on success and failure.

Models: a deterministic offline `ReplayModel` and a minimal stdlib
OpenAI-compatible `LiveModel` (pointed at a loopback fake provider in tests).

Deliberately NOT ported (out of scope): litellm/provider SDKs,
docker/singularity/swerex environments, jinja2 templating, pydantic config
models, rich/prompt_toolkit console decoration, and trajectory storage
integrations. Every action's execution capability is Bashy only.
"""

# Upstream mini-swe-agent version this bounded rewrite was derived from.
UPSTREAM_VERSION = "2.4.6"
# Identity of this bounded rewrite (independent of the upstream version).
BOUNDED_VERSION = "0.1.0"
TRAJECTORY_FORMAT = "mini-swe-agent-1.1"

__all__ = ["UPSTREAM_VERSION", "BOUNDED_VERSION", "TRAJECTORY_FORMAT"]
