---
id: aa4a12f785ee
kind: task
title: 'sh CI: darwin test binary hijacks fd 8 and crashes the Go runtime signal loop'
seq: 203
status: done
priority: p0
created: 2026-09-04T05:36:36.368259Z
sprint: 101
closed: 2026-09-06T08:12:15.290034Z
---

TestRunnerTerminalExecInheritedSparseFD did unix.Dup2(pty, 8) on the TEST PROCESS. On darwin the Go runtime's signal note is a pipe allocated at the first signal.Notify (sigNoteSetup); when it lands on fd 8 the hijack makes the blocked read in sigNoteSleep return a byte no sigsend queued, leaving sig.state at sigReceiving, so signal_recv throws 'inconsistent state' and kills the test binary. Proven with a standalone reproducer: identical crash signature. All three observed CI crashes landed at this same site. Place fd 8 in the CHILD via closed-slot ExtraFiles instead.
