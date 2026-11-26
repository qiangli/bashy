# Form CLI Contract (`CONTRACT.md`)

Certification-style behavioural specification for `form` (`schema_version`: `"bashy-form-v1"`). Spec v2 (2026-11-25): every rule below is demonstrated by at least one assertion or acceptance line.

## How assertions are judged

- Placeholders `{ID}`, `{PORT}`, `{TIME}` and `{KEY}` bind on first match and apply consistently across later steps.
- `--json` outputs are compared structurally after normalizing loopback addresses to `localhost`. `"..."` marks an open value: `"key": "..."` means any value, and `"...": "..."` in an object means that object may carry **extra keys**. An object without `"...": "..."` is **closed**: exactly the keys shown.
- A `ui` block fetches the page with `GET` and compares its **accessible inventory** (see UI rules) with the expected JSON. Markup and styling are implementation freedoms.
- Plain text is checked only where a console block gives it. Otherwise only the exit code is required.

## Output rules

- **Envelope.** Every `--json` output is one JSON object on one line, and it always includes `"schema_version": "bashy-form-v1"`.
- **`--fields K1,K2,...`** works on every verb (`add`, `open`, `show`, `answer`, `wait`). It returns exactly the named keys plus `schema_version` (F-02, F-03, F-04, F-10, F-11).
- **Open vs closed.** Create outputs (`add`) are open: an implementation may add keys (F-01, F-07). `--fields` projections are closed. `open` is closed because it must never expose the definition or the answers (F-02).
- **Errors** go to stderr as `error: <message>` with exit 1 (F-06, F-13).

## Definitions (input)

- `--def FILE` reads a JSON (`.json`) or YAML (`.yaml`/`.yml`) file holding an object `{"title": ..., "fields": [...]}`. `examples/contact.json` and `examples/contact.yaml` are the same form; the YAML uses block mappings and flow lists such as `options: [email, phone, post]` (F-01, F-10).
- `--definition JSON` takes inline JSON: a bare array of fields (F-06, F-12) or the same object as a file.
- A field is `id`, `label` and `kind`, plus optional `required`, `options` (for `choice` and `multi`), `recommended` and `help`. `kind` is one of `text`, `textarea`, `choice`, `multi`, `yes-no` or `file`. Any other kind is an error (F-13).
- Secret-typed or secret-named fields (`api_key`, `db_password`, ...) are refused before storage (F-06).
- `--title` overrides the file's title.

## UI rules (the page at the form's `url`)

- Each field has **one** control and a **programmatic label** whose text is the field label: `<label for>`, a wrapping `<label>`, or a `fieldset` with a `legend`, `aria-label` or `aria-labelledby`.
- Required fields are marked (`required` or `aria-required="true"`).
- `choice` is a select or a radio group, and `multi` is a multi-select or a checkbox group. Each option's accessible name is the option text, never the field label.
- `yes-no` is a yes/no select, a yes/no radio pair, or a single checkbox (checked means yes).
- The page has the buttons "Save (partial)" and "Submit answers". Submitting with every required field filled stores the answers, and `status` becomes `answered`. Saving partially keeps the answers across reloads. See "Page interaction" below.

### F-01
Create and open a form from `examples/contact.json` in one command, returning a `pending` envelope and one-time `ownerKey`.
```console
$ form add --id intake --def examples/contact.json --title "Contact preferences" --open --json
{"id":"{ID}","inviteURL":"...","opened":true,"openedURL":"...","ownerKey":"{KEY}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}","...":"..."}
```
exit: 0

### F-02
Re-surface a form with `form open --json --fields` without exposing the definition or answers (closed by design).
```console
$ form open {ID} --json --fields id,status,url
{"id":"{ID}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
```
exit: 0

### U-01
The form page served at the form's URL shows every defined field (label, kind, required, options) and the "Save (partial)" and "Submit answers" buttons.
```ui
GET http://localhost:{PORT}/?id={ID}
{"fields":[{"id":"name","label":"Full name","kind":"text","required":true},{"id":"bio","label":"Short bio","kind":"textarea"},{"id":"contact","label":"Preferred contact","kind":"choice","required":true,"options":["email","phone","post"]},{"id":"topics","label":"Topics","kind":"multi","options":["news","events","offers"]},{"id":"subscribe","label":"Subscribe?","kind":"yes-no","required":true},{"id":"avatar","label":"Avatar file","kind":"file"}],"buttons":["Save (partial)","Submit answers"]}
```

### F-03
Submit complete respondent answers via `form answer --complete` and transition status to `answered`.
```console
$ form answer {ID} --answer name=Ada --answer contact=email --answer subscribe=yes --complete --json --fields id,status
{"id":"{ID}","schema_version":"bashy-form-v1","status":"answered"}
```
exit: 0

### F-04
Wait for an answered form with automatic local owner-key resolution and an explicit `--timeout`.
```console
$ form wait {ID} --timeout 5s --json --fields id,status
{"id":"{ID}","schema_version":"bashy-form-v1","status":"answered"}
```
exit: 0

### F-05
Read back the stored definition in declared field order and the normalized answers via `form show --json`.
```console
$ form show {ID} --json
{"answers":{"contact":"email","name":"Ada","subscribe":"yes"},"definition":[{"id":"name","kind":"text","label":"Full name","required":true},{"help":"Two or three sentences.","id":"bio","kind":"textarea","label":"Short bio"},{"id":"contact","kind":"choice","label":"Preferred contact","options":["email","phone","post"],"recommended":"email","required":true},{"id":"topics","kind":"multi","label":"Topics","options":["news","events","offers"],"recommended":"news"},{"id":"subscribe","kind":"yes-no","label":"Subscribe?","required":true},{"id":"avatar","kind":"file","label":"Avatar file"}],"id":"{ID}","schema_version":"bashy-form-v1","status":"answered","title":"Contact preferences","url":"http://localhost:{PORT}/?id={ID}","...":"..."}
```
exit: 0

### F-06
Refuse secret-typed or secret-named fields before storage so credentials never enter the form store.
```console
$ form add --id bad-secret --definition '[{"id":"api_key","label":"API key","kind":"text"}]' --json
error: Secret fields are refused here...
```
exit: 1

### F-07
Create a multi-respondent poll with an RFC3339 `--close-at` timestamp.
```console
$ form add --id release-poll --poll "Ship release?" --options "yes,hold" --close-at 2099-12-31T23:59:59Z --json
{"id":"release-poll","ownerKey":"...","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id=release-poll","...":"..."}
```
exit: 0

### F-08
Round-trip the poll's `close_at` timestamp `{TIME}` and `respondents: "many"` mode in `form show --json`.
```console
$ form show release-poll --json
{"close_at":"{TIME}","definition":[{"id":"vote","kind":"choice","label":"Ship release?","options":["yes","hold"],"required":true}],"id":"release-poll","respondents":"many","schema_version":"bashy-form-v1","status":"pending","title":"Ship release?","url":"http://localhost:{PORT}/?id=release-poll","...":"..."}
```
exit: 0

### F-09
List forms in plain text (human formatting is an open freedom; exit status must be zero).
```console
$ form list
```
exit: 0

### F-10
Create a form from the YAML definition (block mappings, flow lists), with `--fields`.
```console
$ form add --id intake-yaml --def examples/contact.yaml --title "Contact preferences" --json --fields id,status
{"id":"intake-yaml","schema_version":"bashy-form-v1","status":"pending"}
```
exit: 0

### F-11
The YAML form stores the same definition as the JSON example, in declared order.
```console
$ form show intake-yaml --json --fields definition
{"definition":[{"id":"name","kind":"text","label":"Full name","required":true},{"help":"Two or three sentences.","id":"bio","kind":"textarea","label":"Short bio"},{"id":"contact","kind":"choice","label":"Preferred contact","options":["email","phone","post"],"recommended":"email","required":true},{"id":"topics","kind":"multi","label":"Topics","options":["news","events","offers"],"recommended":"news"},{"id":"subscribe","kind":"yes-no","label":"Subscribe?","required":true},{"id":"avatar","kind":"file","label":"Avatar file"}],"schema_version":"bashy-form-v1"}
```
exit: 0

### F-12
Create a form from an inline `--definition` array of fields.
```console
$ form add --id inline-1 --definition '[{"id":"q","label":"Question","kind":"text"}]' --json --fields id,status
{"id":"inline-1","schema_version":"bashy-form-v1","status":"pending"}
```
exit: 0

### F-13
Refuse an unknown field kind.
```console
$ form add --id bad-kind --definition '[{"id":"q","label":"Q","kind":"slider"}]' --json
error: field "q": unknown kind "slider"...
```
exit: 1

## Page interaction (documented; executable as I-01 once conform supports `interact`)

A person fills the page by label and submits it. The CLI then reads the result:

```text
open   http://localhost:{PORT}/?id={ID}            (a new form from examples/contact.json)
fill   "Full name" = Ada
choose "Preferred contact" = email
choose "Subscribe?" = yes
press  "Submit answers"
$ form show {ID} --json --fields status,answers
{"answers":{"contact":"email","name":"Ada","subscribe":"yes"},"schema_version":"bashy-form-v1","status":"answered"}
```

