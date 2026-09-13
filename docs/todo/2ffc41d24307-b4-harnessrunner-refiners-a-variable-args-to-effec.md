---
id: 2ffc41d24307
kind: task
title: 'B4 harnessrunner refiners: (a) variable args to effect-pure builtins prove no governed effect; (b) kb front-door subverb refiner (read-only subverbs complete)'
seq: 269
status: assigned
priority: p1
created: 2026-09-13T03:06:58.828246Z
weave: 11
assignee: qiangli
sprint: 164
---

Sprint 163 Y1/Y2 residue. Today pkg/harnessrunner/compiler.go compileCall marks ANY call with a non-static argument 'dynamicCommand' (staticWord returns <dynamic> for every expansion) and, for atlas commands, marks every non-pure command 'effectRefinement' ('only a pure applet is effect-complete'). Consequences: (1) 'printf %s "$YCODE_IN_X"' is incomplete evidence, so ycode's bashy.run typed-input mechanism is denied under the real executor; (2) 'bashy kb context --for ... --json' — fully literal — is incomplete because the kb atlas row carries [read write], so the sprint 163 kb pipe is wired but never executes under the real boundary (ycode degrades to an honest abstained envelope). Both are refiners in this package; no atlas change, no ycode change.

(a) PURE-BUILTIN ARGUMENT REFINER. In compileCall: if argv[0] is a static word naming a builtin whose builtinEffects are ALL atlas.EffPure (printf, echo, true, false, :), then dynamic ARGUMENTS add no governed effect: record the CommandFact with the argv as parsed (dynamic words rendered '<dynamic>'), keep intent.Complete, do NOT markUnsupported. A dynamic COMMAND NAME stays unsupported. 'read' and 'pwd' are not pure and keep today's behaviour. Redirections are unchanged (a dynamic redirect target is still unsupported). Tests: printf/echo with $VAR and "${VAR}" args compile Complete with one command fact; 'printf' with a dynamic name ("$CMD" x) stays incomplete; 'echo hi > $F' stays incomplete (redirection); digest covers the composed script unchanged.

(b) KB SUBVERB REFINER. A command-specific refiner table keyed by front-door verb, seeded with 'kb' (the structure must admit other verbs later — 'graph', 'todo' — without touching compileCall): for argv = kb <subverb> …: read-only subverbs {context, search, show, list, backlinks, doctor, recall, log, index, sources} → replace the atlas maximum [read write] with ONE exact EffRead effect whose Target is the resolved kb store (repo docs/kb under intent.Cwd's git root when in a repo, else the host store — resolve through coreutils pkg/scope exactly as pkg/kb does; include the agent ring dir when --rings names agent and YCODE_DATA_DIR is set) and mark the fact Complete; write subverbs {add, note, update, supersede, validate, observe, transfer, retro} → EffWrite (plus EffRead) on the same resolved store, Complete (policy then decides under the ceiling); any other subverb (or no subverb) keeps today's effectRefinement-incomplete. Flags are static words already; a dynamic flag VALUE (e.g. --for "$X") is allowed for read-only subverbs (it cannot change the store) and stays unsupported for write subverbs (it could name a ring). Tests: 'bashy kb context --for x --rings repo,host --budget 700 --json' → Complete, one EffRead exact on the repo store path; same with --for "$TASK" → Complete; 'bashy kb note add --candidate --ring agent --title t --body b' → Complete with EffWrite; 'bashy kb note add --ring "$R" …' → incomplete; 'bashy kb frobnicate' → incomplete; 'bashy graph impact x' unchanged (still incomplete) — proves the table is per-verb.

Gate: go test ./pkg/harnessrunner/... ./internal/agentos/... green; go vet; a manifest/contract golden that pins the refiner names (kind values) if the package has one. Files: pkg/harnessrunner/compiler.go, pkg/harnessrunner/refine_builtin.go + refine_kb.go (new), tests. Depends on: nothing (kb verbs are on coreutils main 70faebdd, pinned).
