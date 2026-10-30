---
id: 67f698960f5e
kind: bug
title: S537.F2b replyform must accept browser extract data as a JSON OBJECT (CLI --json shape), not only a string
seq: 2963
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T18:36:12.662225Z
weave: 367
assignee: agy-gemini3.8-flash
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T18:59:48.521105Z
closed_by: s537-manager
---

Found in sprint (s536-manager correction mb:11025): `bashy browser extract --json` emits data as an OBJECT {viewport,elements} (wire.Result.MarshalJSON decodes structured JSON); only transport internals carry a string. examples/desk/form/server/reply.go buildReply declares Data string and rejects the object form; reply_test.go even asserts {"data":{"elements":[]}} is invalid. Fix: decode data as json.RawMessage; object -> use directly; string -> tolerate as transport form (single parse, no double-parse). Keep opaque stable_id and verbatim selector unchanged. Red/green: object-form extract (testdata copy of extract.json with data as object) builds the same rows as the string form; update the 'object data' validation case to a missing-elements object. Scope: examples/desk/form only.
