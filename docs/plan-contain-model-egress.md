# Plan: contained agent workers keep their model egress (Sprint 421, Story 1906)

**Bug.** A genie worker whose process tree sits under `bashy contain --net deny`
(macOS Seatbelt, `contain_darwin.go`) fails every model call with
`dial tcp ...:443: connect: operation not permitted`, retries, exits, and weave
relaunches it. Reproduced with
`bashy contain --net deny -- bashy genie -m glm-5.3 "reply with exactly: OK"`:
the same error and the same z.ai addresses as yoke run 105. Containment reaches a
worker by inheritance: genie's `incomplete-preflight` remedy tells a manager to
wrap unprovable commands in `@contain(net: "deny")`, and every child of that
scope (including a `weave start` worker) keeps the kernel profile. Neither the
fleet registry's launch argv for tool genie (`bashy genie -m {model} {prompt}`),
the weave booth, nor the genie.bar `chat` target applies containment itself.

**Constraint.** Seatbelt filters accept only `*` or `localhost` as a host
(`(remote tcp "api.z.ai:443")` is a syntax error), and provider addresses rotate,
so the profile cannot name the provider.

**Fix.** `bashy contain` (native, darwin) runs a CONNECT-only proxy outside the
sandbox on a loopback port. The profile allows that one port and the model door's
port; the proxy refuses every endpoint not on the allowlist (registry API-model
`base_url`s + `BASHY_CONTAIN_ALLOW_HOST`). The child gets `HTTPS_PROXY` & co.
Direct dials, other ports and other hosts stay denied by the kernel/proxy.
`BASHY_CONTAIN_MODEL_EGRESS=off` restores the fully offline profile.

**Not done.** Linux (network namespace) still has no model egress; weave still
relaunches on a deterministic network denial (yoke `pkg/weave`, separate repo).
