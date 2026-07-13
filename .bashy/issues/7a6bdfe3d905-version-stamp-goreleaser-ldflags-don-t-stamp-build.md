---
id: 7a6bdfe3d905
kind: bug
title: 'version-stamp: GoReleaser ldflags don''t stamp buildID; dirty-check ignores untracked files'
status: open
reporter: qiangli
created: 2026-07-13T00:04:05.125351Z
---

Judge (claude-opus) revise findings on merged run #13 (version stamp, commit fbd8b69 in f3f9555) — follow-ups; core feature works and is gated 86/86 with correct version string:

MAJOR: The release build path (GoReleaser .goreleaser ldflags) still stamps only cli.bashVersion, NOT cli.buildID — so every DOWNLOADED release/snapshot binary reports an empty runtime.build and a bare --version line, while 'make dist' binaries of the same commit carry the id. An agent calling 'bashy context --json' on a shipped artifact gets nothing. Add the buildID -X to the goreleaser ldflags (and any release workflow that stamps).

MINOR: the dirty check (git diff --quiet + git diff --cached --quiet, replicated across Makefile/bashy/dag.md/self.go) ignores UNTRACKED files, so a tree with new never-added files stamps a clean SHA that doesn't match the source. Use 'git status --porcelain' emptiness instead.

MINOR: selfBuildID and the runtime.build context field are untested — the new version_test.go only covers bashVersionLine() string assembly, not the tag-vs-SHA fallback or -dirty suffix (the parts with real failure modes, duplicated across six build paths).
