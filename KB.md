# KB — relevant repo and host knowledge (may be stale; verify before relying on it)

Check these before you start — they may save you a failed approach:

- [host/page] Never run the foreign uutils conformance suite on a host — Host-crash guardrail WHEN running bashy/coreutils/uutils conformance: infinite-device and root-equivalent recursive cases can OOM or walk the host root; use disposable hard-capped containment and quarantine known cases. Do not run the full foreign uutils suite  (kb:never-run-the-foreign-uutils-conformance-suite-on-a-host)

More: `bashy kb search <query>` (or `bashy kb show <slug>`).

AFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`
— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.
