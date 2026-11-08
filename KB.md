# KB — relevant repo and host knowledge (may be stale; verify before relying on it)

Check these before you start — they may save you a failed approach:

- [repo/page] Derive no-egress allowlists from the pinned test's full port set — When a sandbox profile or strace verdict allowlists loopback ports for a pinned test binary, enumerate every bind/connect in that test's source at the pinned commit, not the subset remembered from its description. Prima's ring test was allowed 5  (kb:derive-no-egress-allowlists-from-the-pinned-test-s-full-port-set)

More: `bashy kb search <query>` (or `bashy kb show <slug>`).

AFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`
— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.
