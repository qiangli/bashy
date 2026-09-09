---
id: 77ffe6335682
kind: task
title: Wire shared Go source mode through Bashy execution and transpilation
seq: 251
status: done
priority: p0
created: 2026-09-09T03:37:44.377922Z
assignee: qiangli
sprint: 118
closed: 2026-09-09T04:54:28.946718Z
---

Implement Bashy explicit --source=go input dispatch and --check semantic-only using new sh/gosource API. Own bashy/internal/cli and internal/agentos/transpile.go plus associated tests/docs only. sh worker owns gosource Source{Name string;Data []byte}, Options{RunMain bool}, Parse(reader,name,opts)/Load([]Source,opts) returning Program{File *syntax.File,Package,...}; API skeleton expected shortly, inspect workspace /Users/qiangli/.bashy/weave/sh-7e2e7b65/workspaces/issue-28 or canonical after integration. --bashpp --source=go original.go interprets existing AST with main/init; --check validates but executes nothing, disallowed Classic/POSIX selection; transpile --bashpp --source=go input -o output --map path maps original positions. Handle original single/multiple files and module directories as needed. Do not reimplement Go source parsing. Explicit malformed Go cannot fall into shell dispatch. Coordinate initial API assumptions in a file/readme before sh lands. Keep existing --bashpp script interface. For private sibling build setup ask manager; do not rewrite canonical go.mod.

Sprint118 execution assignment from sprint118-manager. Read /Users/qiangli/projects/poc/dhnt/docs/sprint-118-master-execution-plan.md. Work only in your isolated weave workspace. Original upstream Go bytes unchanged; no source adaptations or native-Go whole-program delegation. No subagents. Commit named files with Sprint: #118, Story and Story-ID trailers; do not push, close stories or declare sprint gates complete. Manager independently verifies/integrates. Use GOMAXPROCS=2 and Go -p 2 for focused tests; no broad heavy suites. Shared tools/corpus library is being implemented separately; do not edit it. Report missing support honestly and leave failures visible. Read canonical /Users/qiangli/projects/poc/dhnt/sh as needed. Existing product does not yet support source=go; prepare harness contract now, never forge passes.

Review and acceptance,2026-09-08:
Merged e11e6ad. Canonical default Makefile build and make test passed;72CLI probes including original module import from assets-only runtime cwd passed. Canonical120 focused Go profile and33 Bashsharp cases preserved. Shared GoSource flags, package checks, original source maps and output collision handling reviewed. Full Tour/GbE/official Go acceptance remains open; standalone sibling pins updated to published canonical revisions.
