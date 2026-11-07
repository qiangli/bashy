# KB — relevant repo and host knowledge (may be stale; verify before relying on it)

Check these before you start — they may save you a failed approach:

- [repo/page] OCI release tags must follow byte-promotion — When release.yml builds only -dev candidates and promote.yml creates stable releases without rebuilding, publish the commit and candidate OCI tags in the candidate workflow, then retag the exact manifest during promotion and compare anonymous raw manifests before creating the stable rel  (kb:oci-release-tags-must-follow-byte-promotion)

More: `bashy kb search <query>` (or `bashy kb show <slug>`).

AFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`
— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.
