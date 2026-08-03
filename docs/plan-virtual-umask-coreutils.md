# Virtual umask propagation for coreutils

Goal: make both Bashy's in-process userland and the lean certification shell's
external coreutils obey each Runner's current virtual `umask` without relying
on a long-lived process-global mask.

1. Expose the current mask narrowly on `interp.HandlerContext`.
2. Carry it into coreutils `tool.RunContext`; create new files restrictively and
   apply the masked mode through the open descriptor.
3. Keep external commands on the existing locked start boundary, where the
   child inherits the virtual mask and the parent immediately restores its own.
4. Prove default, `077`, and certification `006` modes through both dispatch
   paths, then run focused tests and cross-platform builds.
