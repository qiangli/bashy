---
id: 77ed1861b0c8
kind: bug
title: 'Remote install upgrade rewrites a running bashy: ''text file busy'' on Linux'
seq: 3164
status: done
priority: p1
labels:
    - install
    - linux
created: 2026-11-07T08:56:29.028611Z
closed: 2026-11-07T10:27:06.17911Z
---

Found 2026-11-07 in the make test lane on a rootful Linux gate (bashy 35a450b3): internal/cli TestRemoteInstallFake_FreshIdempotentAndUpgrade fails at remote_test.go:147 with 'open .../.bashy/remote/bin/bashy: text file busy'. The upgrade step opens the installed bashy for writing while that binary is executing; Linux refuses with ETXTBSY. The other two remote-install tests pass. Fix: the installer must write the new binary to a temporary file in the same directory and rename it over the target (an atomic replace that is safe while the old binary runs), never open the target for writing in place. Check the Windows path too, where a running exe cannot be replaced (rename aside first). Add a test that upgrades while the old binary is running.

Verified 2026-11-07 on the Windows test host gate machine (targeted, with network): TestRemoteInstallFake_* all PASS after the atomic replace (cc649725). Fixed on bashy main.
