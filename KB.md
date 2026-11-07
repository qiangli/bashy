# KB — relevant repo and host knowledge (may be stale; verify before relying on it)

Check these before you start — they may save you a failed approach:

- [repo/page] MinGW-clang Windows builds need -Wno-error downgrades for SOCKET/HANDLE int-conversion in pinned C deps — When a pinned C dependency passes Windows SOCKET (UINT_PTR integer) to HANDLE (pointer) parameters without a cast — e.g. nng win_tcpconn.c CancelIoEx(s, ...) — newer MinGW clang promotes -Wint-conversion  (kb:mingw-clang-windows-builds-need-wno-error-downgrades-for-socket)

More: `bashy kb search <query>` (or `bashy kb show <slug>`).

AFTER this issue is done, close the loop: `bashy kb retro <a few words on what you did>`
— validate/update/supersede what you consulted, or add the new lesson (distilled, never a transcript). NOOP is fine when nothing durable was learned.
