"""Bounded, self-contained, offline rewrite of the mini-swe-agent harness core.

This package is a MINIMAL, stdlib-only projection of the mini-swe-agent control
flow: a system/instance prompt, a step loop that queries a model for a bash
command, executes it in a local environment, feeds the observation back, and
stops on an explicit submit marker or a resource limit. It exists so the bounded
agent-harness lifecycle can run deterministically OFFLINE (no network, no model
provider, no third-party dependencies) and emit a normalized result envelope for
a future benchmark harness.

Substantive behavior preserved from upstream (see README.md manifest):
  - the DefaultAgent step loop and its exit-status taxonomy
    (Submitted / LimitsExceeded / TimeExceeded / RepeatedFormatError),
  - the LocalEnvironment submit protocol (a command whose stdout begins with
    COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT and returns 0 submits the remainder),
  - the trajectory serialization shape ("mini-swe-agent-1.1").

Deliberately NOT ported (out of scope for the bounded offline harness): litellm
and other model providers, docker/singularity/swerex environments, the
interactive confirmation UX, jinja2 templating, pydantic config models, and
trajectory storage integrations.
"""

# Upstream mini-swe-agent version this bounded rewrite was derived from.
UPSTREAM_VERSION = "2.4.6"
# Identity of this bounded rewrite (independent of the upstream version).
BOUNDED_VERSION = "0.1.0"
TRAJECTORY_FORMAT = "mini-swe-agent-1.1"

__all__ = ["UPSTREAM_VERSION", "BOUNDED_VERSION", "TRAJECTORY_FORMAT"]
