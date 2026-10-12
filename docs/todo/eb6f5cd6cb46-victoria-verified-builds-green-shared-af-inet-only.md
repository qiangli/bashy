---
id: eb6f5cd6cb46
kind: bug
title: 'Victoria verified builds green: shared AF_INET-only no-egress verdict, bump to latest upstream tags, security-only overrides (no waivers)'
seq: 1897
status: done
priority: p1
labels:
    - supply-chain
created: 2026-10-12T21:45:34.795997Z
weave: 60
assignee: claude-opus5
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-12T22:41:29.796282Z
closed_by: s434-manager
---

Victoria fork CI failed on all three first runs (qiangli/victoria-metrics 38311645575, victoria-logs 38311666036, victoria-traces 38311680149): pin OK; govulncheck and the Linux no-egress gate failed. Conductor diagnosis + decisions (binding):

1. no-egress: the failure is the AF_UNIX (glibc nscd/resolver socket) miscount already fixed for tuwunel in bashy fb22eaf9 (builds/tuwunel/files/no-egress-test.sh, the sa_family case: only AF_INET/AF_INET6 connects to non-loopback count; AF_UNIX is local IPC). Factor that strace verdict into ONE shared copy under builds/_shared (sourced or rendered into each fork's bashy-build/), use it from victoria-logs/metrics/traces; keep tuwunel's behaviour identical (re-render it if you move its logic; openclaw/tuwunel fork/ diffs must be explained).
2. govulncheck: 12 reachable = 8 stdlib (fixed in go1.26.6) + golang.org/x/text 0.39, x/net 0.55, grpc 1.83.1, otel 1.45. Bump pins to latest stable upstream tags: VictoriaMetrics v1.154.0, VictoriaLogs v1.53.0, VictoriaTraces v0.12.0 (verify each tag commit via git ls-remote, LICENSE Apache-2.0 at tag, review the build-flag/ldflags diff, update tool.env + READMEs + GO_TOOLCHAIN). Anything still reachable after the bump: a security-only `go get module@fixed` override step (declared in tool.env, e.g. GO_SECURITY_OVERRIDES), with a gate asserting ONLY those modules change in go.mod/go.sum. NO waivers for reachable vulns (vuln-ignore stays for unreachable only). If the tag's go.mod toolchain predates the stdlib fix, allow GOTOOLCHAIN = newest patch of the same minor as a recorded security override (setup.yml gate must accept exactly that declared override and record it in the release notes/PIN).
3. Gate: builds/render.sh for all tools + builds/check.sh exit 0 (shellcheck/actionlint). Commit with trailers, then sprint submit with the commit SHA and gate output. Do NOT push to qiangli/* forks or tag; the manager pushes. Do not edit yoke (the yoke version flip is a separate story after green CI).
