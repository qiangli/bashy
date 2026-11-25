---
id: 64ef2626b13e
kind: bug
title: 'form: registered command rejects every flag (--def, --title, --key)'
seq: 3311
status: todo
priority: p2
labels:
    - desk
created: 2026-11-25T10:17:38.006874Z
---

Symptom (2026-11-25, dev box): `bashy form add --def F.json --title T` exits 2 with 'bashy: form: unknown flag --def'. Running `bashy --bashsharp examples/desk/form/form.bsh add --def ...` works.

Root cause (code inspected): examples/desk/form/register.sh registers args.positionals (verb enum, id) but declares no args.flags. registeredSchema (internal/agentos/registered.go) copies that schema as-is, and the interp binder (sh/interp/command_schema.go:56) rejects any flag the schema does not declare. Every flag-taking form verb fails through the registered command. examples/desk/chart/register.sh has the same pattern (positionals, no flags); check it too.

Classification: product defect (registration schema), not interp. Fix where it lives: declare form's flags in register.sh, or omit args.positionals so the schema is absent (nil = no validation).

Verify: `bashy form add --def examples/desk/form/examples/contact.json --title T` prints an id; `bashy form list` still works; desk unit tests.
