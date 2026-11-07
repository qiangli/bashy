# KB — relevant repo and host knowledge (may be stale; verify before relying on it)

Check these before you start — they may save you a failed approach:

- [repo/page] Windows gates must hand native exes cygpath-converted paths — When a Git Bash gate script invokes a native Windows executable (python.exe, cmake.exe, a compiled WFP guard): convert every path that binary opens, filters, or spawns with cygpath -m. MSYS auto-converts slash paths heuristically, but app-id filtering, Cr  (kb:windows-gates-must-hand-native-exes-cygpath-converted-paths)

More: `bashy kb search <query>` (or `bashy kb show <slug>`).

AFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`
— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.
