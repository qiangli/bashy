# form — agent-defined forms, human-filled on the loopback page

F1 of Sprint 537 (`bashy desk`). An agent defines a form in JSON or YAML;
the human fills it on the loopback page built from the
[desk kit](../kit/README.md) field components; the agent reads answers back
as JSON keyed by field id. This engine is the base for replyform (F2) and
questionnaire (F3).

## Quick round-trip

```bash
bashy form add --def ./examples/contact.json --title "Contact preferences"
# form-9f3a… http://127.0.0.1:18107/?id=form-9f3a… pending
# ownerKey: … (keep private)

bashy form open form-9f3a…        # id, URL, status only — never content
# human fills the page (Save = partial, Submit = complete)

bashy form show form-9f3a… --json  # definition + answers keyed by field id
bashy form wait form-9f3a…        # blocks until answered (or closed/timeout)
```

## Replyforms

Hand a web form back to the human without losing the agent's proposals:

```bash
bashy browser extract > extract.json
bashy form reply --extract extract.json --values values.json --refusals refusals.json
# --extract - reads the extract result from stdin.
bashy form show form-9f3a… --json --key "$FORM_KEY"
```

The extract result's `data` carries `elements` (as a JSON object, or string in transport form).
Only input/select/textarea fields are included (button, submit, reset, image,
and hidden inputs are skipped). Rows use `stable_id`, falling back to the
verbatim `selector`; frame-qualified selectors remain unchanged. Labels
fall back to `name`, then `attrs.placeholder`, then the row key.
Indexes are never used.

Both optional files are JSON objects keyed by that same row key:

```json
{"name-id":"Ada","email-id":{"value":"ada@example.test","status":"filled-by-agent"}}
```

```json
{"ssn-id":{"outcome_class":"refused","reason":"field:label-ssn","content":"","error":""},
 "bio-id":{"outcome_class":"failed","reason":"fill:detached","content":"","error":""}}
```

A string proposal starts as `pending`; an object can specify `pending` or
`filled-by-agent`. Refusals override proposals and produce `needs-human`;
failed fills produce `failed`. Only the guard's reason code is retained:
proposal values, extracted attribute values, result content and errors are
never stored or displayed for these rows. Password/secret-named inputs are
also reduced to a needs-human row. Unknown proposal/refusal keys are rejected
so a mismatched identity cannot silently bypass redaction.

Each safe proposal has the kit CopyButton (with its “Copied” state). The
human marks rows done and saves; Submit requires every remaining row done.
`show --json` returns `answers` keyed by stable ID (or selector), each containing
`stable_id`, `selector`, `label`, `status`, `done`, and optional `value` /
`reason`. Completion is a separate boolean so the original fill status
remains visible. Replyforms use the F1 item store, owner key and sharing
contract. Their proposals are immutable; the human answer API accepts only
boolean completion marks, including for sensitive rows. `open` stays
contentless. There is no in-page browser overlay.

## Questionnaires

`ask` creates the questionnaire mode on the same engine. It accepts the same
definition schema, validates every supplied field (including secret refusal),
then assigns stable question ids in display order:

```bash
bashy form ask --def ./questions.json --title "Release decision"
# form-9f3a… http://127.0.0.1:18107/?id=form-9f3a… pending
# ownerKey: … (keep private)

# Later, append only the add round; the page now shows Q1..Qn+2.
bashy form ask form-9f3a… --key "$FORM_KEY" --def ./followup.json

bashy form open form-9f3a…        # id, URL, status only
bashy form show form-9f3a… --json  # answers keyed by q1..qn, each with status
bashy form wait form-9f3a… --json
```

Question labels render as `Q1`, `Q2`, … on one persistent page. Follow-up
rounds append `qN+1` onward without resending or renumbering earlier questions.
Partial saves merge, so the human can answer in any order and reload safely.
For questionnaires, `show` and `wait` return entries such as
`"q1":{"status":"answered","answer":"yes"}` and
`"q2":{"status":"pending"}`. Choice questions retain the definition's
recommended mark. A follow-up round returns an already-completed questionnaire
to `pending` until it is submitted again.

## Multiple respondents and polls

Set `respondents: many` beside `fields` in a JSON or YAML definition. This
works with `form add` and `form ask`; omitting it keeps the existing `one`
respondent behavior.

```yaml
title: Lunch?
respondents: many
fields:
  - id: vote
    label: Lunch?
    kind: choice
    required: true
    options: [a, b, c]
```

`form add --poll 'Lunch?' --options a,b,c` creates this poll. `form share ID
--key "$FORM_KEY"` returns a reusable link. Each browser keeps its own
response under a participant cookie; submitting again replaces its earlier
vote. Partial saves remain editable and do not count until submitted.

`form show ID --json --key "$FORM_KEY"` returns `results.respondents` (answers
and completion state) and `results.aggregate` (tallies for choice, multi,
yes-no; lists for text), plus a submitted-response `count`. The owner can poll
for live results. Respondents can see their own answers; `--results-visible`
also lets them see results. `--anonymous` records only opaque participant IDs.
`--close-at 2026-10-31T17:00:00Z` closes at that time; `form close ID` closes
immediately. Closed forms reject saves and re-votes.

## Definition schema (reusable: F2 and F3 build on this)

A definition is a JSON array of fields, or `{"title":…, "fields":[…]}`.
YAML accepts the same shape (block mappings, `-` lists, inline `[a, b]`,
quoted scalars, `true`/`false`; see `examples/contact.yaml`):

| Key | Meaning |
|---|---|
| `respondents` | Top-level `one` (default) or `many` |
| `id` | Field id, `^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`; answers are keyed by it |
| `label` | Human label (required) |
| `kind` | `text` \| `textarea` \| `choice` \| `multi` \| `yes-no` \| `file` |
| `required` | Must be filled before a complete submit |
| `options` | Choice/multi option list (required there, forbidden elsewhere) |
| `recommended` | The one suggested option of a choice/multi; the page marks it |
| `help` | Optional hint shown under the field |

Validation rejects unknown kinds, option misuse, duplicate or ill-formed
ids, a `recommended` outside its options, and — before anything is
stored — secret-typed (`"kind": "secret"`) or secret-named (`db_password`,
`api-key`, …) fields with an error that points at `bashy ask`/vault.

## Answer store (reusable)

Each item persists at `$BASHY_HOME/desk/form/items/<id>.json` (atomic
temp-file + fsync + rename; a failed save is never acknowledged):

```json
{
  "version": 1, "id": "form-9f3a…", "title": "Contact preferences",
  "status": "pending", "revision": 7,
  "definition": [ {"id": "contact", "kind": "choice", "options": ["email", "phone", "post"], "recommended": "email"} ],
  "answers": {"contact": "email", "topics": ["news"], "subscribe": "yes"}
}
```

Answer shapes: strings for text/textarea/choice/file, string arrays for
multi, `"yes"`/`"no"` for yes-no (common spellings and booleans are
normalised). File answers carry `{name, size}` metadata; bytes never flow
through the page. Every Save persists, so partial answers survive reload;
a complete submit with all required fields filled moves the form
`pending → answered`. The agent door (`show --json`, `wait`) polls
`pending`/`answered`. The owner key gates `show`/`set`/`share`/`close`;
the human page posts answers under its participant cookie.

## Contract

Desk seven (`add|open|show|set|list|share|close`, `--json`) plus `ask` and `wait`,
all over the same `/api/...` operations; `open` prints id, URL, status
only; loopback bind (`127.0.0.1:18107` default); stable id + loopback URL;
contentless `list`; single-use share invites; `set` after `close` is 409.
Go module `form-server` pulls the kit with
`require desk-kit v0.0.0` + `replace desk-kit => ../kit`.

## Layout

- `form.bsh` — Bash# front (skill face + verb delegation), `form.sh` — compat shim
- `server/` — pure-Go server + CLI (`schema.go` reusable core, `main.go` doors + page, `cli.go`, `form_test.go`)
- `examples/` — `contact.json` / `contact.yaml` (identical definitions)
- `register.sh`, `dag.md`, `browser-test.mjs`

## Verify

```bash
cd examples/desk/form && GOWORK=off go test ./...
```

No podman, no browser here: unit + contract only. `browser-test.mjs`
runs on a test host with Playwright + a built binary (see its header).
