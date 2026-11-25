---
id: 64ef2626b13e
kind: bug
title: 'form: registered command rejects every flag (--def, --title, --key)'
seq: 3311
status: todo
priority: p1
labels:
    - desk
created: 2026-11-25T10:17:38.006874Z
sprint: 721
sprint_id: 69fc2c51-8faf-58e7-83ac-f27007837ae7
sprint_title: 'Shapeware pilot on form: spec v0 layout, one-step launch'
---

Symptom (2026-11-25, dev box): `bashy form add --def F.json --title T` exits 2 with 'bashy: form: unknown flag --def'. Running `bashy --bashsharp examples/desk/form/form.bsh add --def ...` works.

Root cause (code inspected): examples/desk/form/register.sh registers args.positionals (verb enum, id) but declares no args.flags. registeredSchema (internal/agentos/registered.go) copies that schema as-is, and the interp binder (sh/interp/command_schema.go:56) rejects any flag the schema does not declare. Every flag-taking form verb fails through the registered command. examples/desk/chart/register.sh has the same pattern (positionals, no flags); check it too.

Classification: product defect (registration schema), not interp. Fix where it lives: declare form's flags in register.sh, or omit args.positionals so the schema is absent (nil = no validation).

Verify: `bashy form add --def examples/desk/form/examples/contact.json --title T` prints an id; `bashy form list` still works; desk unit tests.

## Addendum 2026-11-25: the same session hit three more defects on the form path

Seen while launching a form for the umbrella Sprint 712 review. Root causes are not yet investigated, so these are recorded here, not fixed.
1. **The source fallback is broken.** `bashy --bashsharp examples/desk/form/form.bsh add …` (also the registered command's own `exec`) rebuilds the server when the sources are newer than the cache. The rebuild resolves `examples/desk/kit v0.5.0` and `examples/desk/chart v0.1.3` and fails with "unknown revision" from the local Go module proxy. This is probably the desk-module defect tracked in umbrella Sprint 719. Workaround: run the cached `~/.cache/bashy/form/form -addr 127.0.0.1:18107 …` directly.
2. **The owner key is not resolved automatically** when the cached binary runs directly. `wait` answered "Join this form first" on a form that binary had just created. Passing `FORM_KEY` works.
3. **`wait` stops after a 5-minute default** that `-h` never mentions (`server/cli.go` `--timeout`).

Baseline and context: pilot 9643d77f, addendum (e).

## Addendum 2026-11-25 (b): forms land in another app's store (shared fixed address)

Seen: the form client defaults to `127.0.0.1:18107`. On the dev box that address was held by a **custom app's own form server** (`team-signup`, started 11-22 from its custom-app folder with its own `-data`). Every `add`/`show`/`wait` in this session went to that server, so five unrelated review forms, including operator answers, were stored inside `team-signup`'s data (9 forms in its store). The client never checks which app or store it is talking to. Two of those forms stayed `pending` with no answers after the operator submitted. Whether that is linked is unknown; not reproduced yet.

Expected: a form-based app either uses its own address or the client refuses (or warns) when the server at the address is not the default form store.

Workaround used: start a private server `form serve -data DIR -addr 127.0.0.1:18117` and pass `-data DIR -addr …` to every client call.

Root cause not investigated (root cause first, before any fix; Sprint 721 story).
