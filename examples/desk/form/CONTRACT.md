# Form CLI Contract (`CONTRACT.md`)

Certification-style behavioural specification for `form` (`schema_version`: `"bashy-form-v1"`).

- Placeholders `{ID}`, `{PORT}`, `{TIME}`, and `{KEY}` bind on first match and apply consistently across subsequent steps.
- `--json` outputs are compared structurally after normalizing loopback addresses to `localhost`; `"..."` (`"...": "..."` on objects or `"..."` on scalars) marks open implementation-varying fields.
- A `ui` block fetches the page with `GET` and compares its accessible inventory (fields by id, label, kind, required, options; visible button labels) with the expected JSON; markup and styling are implementation freedoms.
- Plain-text output is checked only where explicitly given in a console block; when omitted, human-facing text formatting is a documented implementation freedom and only the exit code is required.

### F-01
Create and open a form from `examples/contact.json` in one command, returning a `pending` envelope and one-time `ownerKey`.
```console
$ form add --id intake --def examples/contact.json --title "Contact preferences" --open --json
{"id":"{ID}","inviteURL":"...","opened":true,"openedURL":"...","ownerKey":"{KEY}","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id={ID}"}
```
exit: 0

### F-02
Re-surface a form with `form open --json --fields` without exposing form definition or answers.
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
{"id":"release-poll","ownerKey":"...","schema_version":"bashy-form-v1","status":"pending","url":"http://localhost:{PORT}/?id=release-poll"}
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
