---
id: b2702397da1c
kind: task
title: 'tar: --help omits -x, which misled a QA design into assuming extract is unavailable'
seq: 2
status: todo
priority: p3
created: 2026-07-22T06:01:47.444776Z
---

**The original premise was wrong: `tar -x` works.** Verified 2026-07-22 by
round-tripping an archive on two builds — the PATH bashy (`5.3.0(1)-bashy-dev`,
`fd9150f`) and a build from current source:

```
bashy tar -czf a.tgz -C src f.txt     # ok
bashy tar -xzf a.tgz -C out           # ok -> out/f.txt
```

This confirms the earlier note on this task: `cmds/tar/tar.go:49` registers
`extract`/`-x`, and `TestCreateListExtractRoundtrip` covers it. There is no
dispatch, PATH-resolution or build-tag gap. Extract works on the pure-bashy
path.

## The actual defect

`tar --help` prints only the create and list forms:

```
Usage: tar -c [-zv] -f ARCHIVE [-C DIR] FILE...
   or: tar -t [-zv] -f ARCHIVE [MEMBER...]
```

`-x` is implemented but undocumented in its own usage output. That is the whole
bug, and it is a small fix — add the extract form to the usage string.

## Why a help-text gap is worth a task

It already caused a real design error. While wiring the shared CI graph, the
missing `-x` line was read as "extract is not available on a pure-bashy host",
and two artifacts were written around a constraint that does not exist:

- `bashy:ci.dag.md` — `qa-smoke` executes a raw downloaded binary, with a
  comment steering archive-publishing repos away from an execute-and-probe
  smoke.
- `ycode:DAG.md` — the `qa` target's smoke was scoped around not extracting.

Both should be revisited now that extract is known to work: an archive-publishing
repo *can* extract and run the real binary, which is a materially stronger gate
than checksum-plus-member-listing. Until that happens, the QA contract still
tacitly assumes raw binaries for the strongest smoke — a workaround that
outlived the (imagined) reason for it.

Worth noting the raw-binary preference is not purely a QA artifact: outpost's
fleet self-upgrade downloads and execs a raw binary (the worker stages
`<binary>.upgrading`, probes it, then atomically swaps), so raw binaries remain
the right shape *there* regardless of what tar supports.

## Suggested work

1. Add the `-x` form to `tar`'s usage output (the fix).
2. Consider a usage/flag drift check — a registered flag absent from `--help` is
   invisible to both humans and agents, and agents in particular treat `--help`
   as the capability contract. This one silently narrowed a design.
3. Revisit `ci.dag.md` `qa-smoke` and `ycode:DAG.md` to extract-and-probe.
