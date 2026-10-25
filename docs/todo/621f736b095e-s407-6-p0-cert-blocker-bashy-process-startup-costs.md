---
id: 621f736b095e
kind: bug
title: 'S407.6 P0 cert blocker: bashy process startup costs ~67 ms (go-runewidth v0.0.27 init builds full-width LUTs, 39 ms); every utility and background job pays it, VSC awk set exceeds 600 s'
seq: 1949
status: todo
priority: p0
labels:
    - posix-cert
    - performance
created: 2026-10-25T03:56:21.936137Z
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
---

Found 2026-10-25 by the S407.3 replay on v1.0.0-rc.14-dev (bashy 62c83441) on Ubuntu 24.04 x86_64 (s-2vcpu-4gb). The awk set still exceeds its 600 s cap AFTER the S407.4 fd fix (historical 173-229 s on ba62cd91, 2026-10-06).

Measurements on the cert host (uid vsctester):
- 100x '/bin/true & wait $!': bashy sh 6.79 s (68 ms each) vs dash 0.12 s. Subshell 100x 0.07 s; pipeline 100x 0.28 s (fine).
- 'bashy -c true': 76.6 ms per process vs a 9.7 ms runuser baseline; '--version' 91 ms.
- GODEBUG=inittrace=1: total package init 46.5 ms over 335 packages; github.com/mattn/go-runewidth init = 39 ms (next: x509roots/fallback 3.0 ms, internal/platform 1.8, yoke/pkg/fleet 1.0, yoke/pkg/atlas 0.8).
- go-runewidth v0.0.27 init() calls initStrictWidthLUT() (65536 runes x 2 runeWidthNoLUT) and fills eastAsianWidth0; v0.0.24 (bashy ba62cd91 pin) has no strict LUT. v0.0.27 arrived with bashy 262669e1 (2026-10-07, charm.land TUI modules for ycode; required by charm.land/bubbles/v2 v2.2.1 and ycode).
- On the cert host every utility (wc, xargs, awk, ...) is a shim into the same bashy binary and every background job re-execs bashy as --bashy-job-carrier, so the init cost multiplies across the whole suite.

Do:
1. Red first: a test/benchmark gate on process startup, e.g. a test that runs the built binary with GODEBUG=inittrace=1 and fails if any single package init exceeds ~5 ms or total init exceeds ~15 ms (Linux), and a scripted '100x (/bin/true & wait)' timing.
2. Fix at the root: make the runewidth tables lazy (sync.Once on first width query) via a permissive-licensed fork under the existing yoke/external/<name> pattern + versioned replace in bashy (and siblings that link it), OR remove the dependency from the shell/utility path. Keep bubbles/ycode TUI behaviour identical (width results unchanged; add a parity test over a sample of runes incl. East Asian ambiguous).
3. Check the remaining top init costs and trim anything > 1 ms that runs on every shell/utility exec if cheap.
4. Report before/after: init trace total, 'bashy -c true' ms, '100x & wait' seconds, on Linux.
Acceptance: Linux startup within ~2x of ba62cd91-era (target total init < 10 ms), test-bash 86/86 on Linux, runewidth parity test green. Sprint/Story/Story-ID trailers. Pin/replace changes in yoke/coreutils go through their own repos (producers first); list exact SHAs for the conductor.
