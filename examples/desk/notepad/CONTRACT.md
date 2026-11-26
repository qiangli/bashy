# Notepad CLI Contract (`CONTRACT.md`)

Certification-style behavioural specification for `notepad` (`schema_version`: `"bashy-notepad-v1"`). Spec v1.0 (2026-11-26): every rule below is demonstrated by at least one assertion or acceptance line, a rule that names a set defines the set, and the service has a defined stop.

## How assertions are judged

- Placeholders `{ID}`, `{PORT}`, `{TIME}` and `{KEY}` bind on first match and apply consistently across later steps.
- `--json` outputs are compared structurally after normalizing loopback addresses to `localhost`. `"..."` marks an open value: `"key": "..."` means any value, and `"...": "..."` in an object means that object may carry **extra keys**. An object without `"...": "..."` is **closed**: exactly the keys shown.
- The page is checked by fetching it with `GET` and looking for its button text (U-01). Markup, styling and label association are implementation freedoms (see UI rules).
- Plain text is checked only where a console block gives it. Otherwise only the exit code is required.
- Assertions run in order against one server and one data directory. N-01 starts the service and N-36 stops it, so a run leaves no server behind.

## Output rules

- **Envelope.** Every `--json` output is one JSON object on one line, and it always includes `"schema_version": "bashy-notepad-v1"`.
- **`--fields K1,K2,...`** works on every verb that prints JSON (`add`, `open`, `show`, `set`, `list`, `share`, `close`, `export`, `history`, `undo`, `restore`). It returns exactly the named keys plus `schema_version` (N-03, N-04, N-08, N-20, N-29, N-32).
- **Open vs closed.** Create output (`add`) is open: an implementation may add keys (N-02). `--fields` projections are closed. `open` is closed because it must never expose the title, the body or the fields (N-04). `show` is closed: exactly the keys shown in N-06.
- **Errors.** With `--json`, an error is one JSON object `{"error": {"code": ..., "message": ...}, "schema_version": "bashy-notepad-v1"}` (N-23). Without `--json`, errors go to the output as `error: <message>` (N-22, N-26, N-27). Exit codes: `1` when the server refuses a request for a state reason (a duplicate id, a missing note, a closed note), `2` for usage errors (an invalid id, an unknown verb, an unsupported export format), and `0` on success.

## Definitions (input)

- **Note id.** A note id is 1 to 64 characters: the first character is a letter or a digit, and every later character is a letter, a digit, `-` or `_`. The decision rule is that pattern and nothing else. `note-1` and `file-txt` are accepted (N-02, N-10); `bad id!` is refused because it contains a space and `!` (N-27).
- **Verbs.** The verb set is exactly: `serve`, `add`, `open`, `show`, `set`, `list`, `share`, `close`, `join`, `export`, `history`, `undo`, `restore`, `stop`. Any other verb is a usage error (N-28). `join` prints the invite URL for a note and is not asserted here; joining happens on the page.
- **`--addr HOST:PORT`** selects the loopback address of this user's notepad server (default `127.0.0.1:18095`). **`--data DIR`** selects the folder that holds the stored notes (default: the user's desk data folder). Both work on every verb, in any position, with a separate value or with `=` joined to the flag (N-01 to N-36).
- **`-f FILE`, `--file FILE`** reads note content from a file; `-f -` reads it from standard input. **`--format FORMAT`** names the content format when the file name cannot. **`--body TEXT`** gives the content inline and **`--title TEXT`** gives the title inline. When `-f` names a file and no `--title` is given, the title is the file's base name without its extension (N-10).
- **Content formats.** The format set is exactly: `txt`, `md`, `html`, `json`, `pdf`. The format is decided by the file extension (`.txt`, `.md` and `.markdown`, `.html` and `.htm`, `.json`, `.pdf`) or by `--format`, and otherwise by the content: a leading `%PDF-` means `pdf`, a leading `{` or `[` that parses as JSON means `json`, a leading `<!doctype html` or `<html` means `html`, and anything else is `txt`. A `txt` or `md` file becomes the body unchanged. A `json` file becomes the body pretty-printed with two-space indent (N-13). An `html` file becomes Markdown: a heading becomes a `#` heading and a link keeps its target (N-15). A `pdf` file becomes its extracted text (N-19).
- **Statuses.** A note's status is one of the closed set `pending`, `needs-human`, `filled-by-agent`, `answered`, `failed`, `closed`. A note created with no body is `pending` (N-03); a note created or set with a non-empty body is `filled-by-agent` (N-02, N-07); a closed note is `closed` (N-30). This spec asserts only the three statuses in that sentence.
- **Revisions.** Every body write records a revision: the first body write of a note is `r1`, the second is `r2`, and the revision of the nth body write is the letter `r` followed by the decimal number n. `history` lists them, `undo` reactivates the previous revision, and `restore ID REV` reactivates a named one (N-32, N-33).
- **Export formats.** The export format set for `--to` is exactly: `pdf`, `png`, `html`, `txt`. The spellings `htm`, `text`, `md` and `markdown` are accepted and mean `html`, `txt`, `txt` and `txt`. Any other value is a usage error before the server is asked (N-21). `export` writes the rendered note to a file and reports its `format` (N-20).
- **Secret fields are refused before storage.** A stored field is one entry of the `fields` array that the JSON API accepts on `add` and `set`; the CLI verbs in this contract do not send fields, so the refusal is asserted through the API (N-23, N-24, N-25). A field is secret when its `name`, ignoring case, contains `password`, `passwd`, `secret` or `credential`, or one of the pairs `api`+`key`, `access`+`token`, `private`+`key` with the two words joined directly or by one `_` or `-`. A field whose `type` is `secret` is refused the same way. So `api_key` is secret (N-23) and a field of type `secret` is secret (N-24), while `keyboard` is not: it contains none of the listed words and no listed pair (N-25). This is the whole rule.

## Service rules

- **The server must be running before any other verb works.** No verb starts it. When no server answers at `--addr`, a verb fails with an error that names the address; the exact exit code of that failure is a known issue (KI-1) and is not asserted.
- **`notepad serve`** runs this user's notepad server in the foreground on the loopback address given by `--addr`, storing notes under `--data`. It answers `GET /healthz` once it is ready (N-01).
- **`notepad stop`** stops the server that the most recent `serve` in this folder started, whatever shell started it. It exits 0 whether or not a server was running (N-34, N-35). What a signal does to a foreground server, and what happens when `serve` is started while a server is already running at the same address, are implementation freedoms; the lifecycle (`dag.md`) therefore stops before it serves.
- **Stored notes are kept across a stop.** The next `serve` starts the server again on the same `--data` folder (N-36), and a note stored before the stop is read back unchanged (N-36).

## UI rules (the page at the note's `url`)

- The page has one text control for the title, one textarea for the body, and one file control for importing a file. The visible text beside them reads `Title`, `Note content (Markdown)` and `Import file (TXT, MD, HTML, JSON, PDF) auto-convert`. In the reference those texts are not programmatically associated with their controls (KI-3); an implementation may associate them, and this contract does not check the association.
- The page has the buttons "Save Note" and "New Note", layout buttons "Edit", "Split" and "View", and a "Copy text" button (U-01).
- Saving from the page stores the title and body, and the CLI then reads the result. See "Page interaction" below.

### N-01
Start this user's notepad server in the background and wait until its health endpoint answers.
```console
$ notepad stop >/dev/null 2>&1; rm -rf .npdata; (notepad --addr 127.0.0.1:18195 --data ./.npdata serve >.npserve.log 2>&1 &); for i in $(seq 1 50); do curl -sf http://127.0.0.1:18195/healthz >/dev/null && break; sleep 0.2; done
$ notepad --addr 127.0.0.1:18195 --data ./.npdata list --json
{"items":[],"schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-02
Create a note with a title and a body in one command, returning a `filled-by-agent` envelope and a one-time `ownerKey`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id note-1 --title "Probe One" --body "Hello probe world" --json
{"active":"r1","id":"note-1","ownerKey":"{KEY}","schema_version":"bashy-notepad-v1","status":"filled-by-agent","url":"http://localhost:{PORT}/?id=note-1","...":"..."}
```
exit: 0

### N-03
Create an empty note: its status is `pending`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id note-empty --json --fields id,status
{"id":"note-empty","schema_version":"bashy-notepad-v1","status":"pending"}
```
exit: 0

### N-04
Re-surface a note with `notepad open --json --fields` without exposing the title, the body or the fields (closed by design).
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata open note-1 --json --fields id,status,url
{"id":"note-1","schema_version":"bashy-notepad-v1","status":"filled-by-agent","url":"http://localhost:{PORT}/?id=note-1"}
```
exit: 0

### N-05
Re-surface a note in plain text: id, URL and status only, never the body.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata open note-1
note-1 http://localhost:{PORT}/?id=note-1 filled-by-agent
```
exit: 0

### U-01
The note page served at the note's URL carries its Save Note button.
```console
$ curl -s "http://127.0.0.1:18195/?id=note-1" | grep -c "Save Note"
1
```
exit: 0

### N-06
Read back the stored title and body of a note via `notepad show --json`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show note-1 --json
{"active":"r1","body":"Hello probe world","fields":null,"id":"note-1","revision":1,"schema_version":"bashy-notepad-v1","status":"filled-by-agent","title":"Probe One","url":"http://localhost:{PORT}/?id=note-1"}
```
exit: 0

### N-07
Replace a note's body with `notepad set`: a new revision becomes active.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata set note-1 --body "Updated body text" --json --fields id,status,active
{"active":"r2","id":"note-1","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-08
Read back the replaced body and the unchanged title.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show note-1 --json --fields title,body
{"body":"Updated body text","schema_version":"bashy-notepad-v1","title":"Probe One"}
```
exit: 0

### N-09
List the stored notes in id order, without bodies.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata list --json
{"items":[{"id":"note-1","status":"filled-by-agent","url":"http://localhost:{PORT}/?id=note-1"},{"id":"note-empty","status":"pending","url":"http://localhost:{PORT}/?id=note-empty"}],"schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-10
Create a note from `examples/meeting.txt`: the title is the file's base name and the body is the file unchanged.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id file-txt -f examples/meeting.txt --json --fields id,status
{"id":"file-txt","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-11
The text file's content is the note's body, unchanged.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show file-txt --json --fields title,body
{"body":"Hello from a text file\nsecond line\n","schema_version":"bashy-notepad-v1","title":"meeting"}
```
exit: 0

### N-12
Create a note from `examples/data.json`: the body is the JSON pretty-printed with two-space indent.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id file-json -f examples/data.json --json --fields id,status
{"id":"file-json","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-13
The JSON example is stored pretty-printed.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show file-json --json --fields body
{"body":"{\n  \"name\": \"alice\",\n  \"age\": 30\n}","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-14
Create a note from `examples/page.html`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id file-html -f examples/page.html --json --fields id,status
{"id":"file-html","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-15
The HTML example is stored as Markdown with its heading and link target kept.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show file-html --json --fields body
{"body":"# Imported Title\n\nA paragraph with [a link](https://example.com).","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-16
Create a note from `examples/notes.md`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id file-md -f examples/notes.md --json --fields id,status
{"id":"file-md","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-17
The Markdown example is stored unchanged.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show file-md --json --fields body
{"body":"# Markdown Title\n\nSome **bold** text.\n","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-18
Create a note from `examples/paper.pdf`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id file-pdf -f examples/paper.pdf --json --fields id,status
{"id":"file-pdf","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-19
The PDF example's body holds the paper's text.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show file-pdf --json --fields body
{"body":"## Page 1\n\nImported Paper\nText extracted from the paper.","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-20
Export a note as plain text: the answer reports the format that was written.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata export note-1 --to txt --json --fields id,format
{"format":"txt","id":"note-1","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-21
Refuse an export format outside the defined set, before the server is asked.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata export note-1 --to docx
error: unsupported export format "docx" (supported: pdf, png, html, txt)
```
exit: 2

### N-22
Refuse to create a note whose id is already taken.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata add --id note-1
error: A note with that id already exists
```
exit: 1

### N-23
Refuse a secret-named field before storage, through the JSON API.
```console
$ curl -s -X POST http://127.0.0.1:18195/api/action -H "Content-Type: application/json" -d '{"action":"add","id":"sec-1","fields":[{"name":"api_key","type":"text","value":"x"}]}'
{"error":{"code":"invalid_arg","message":"Secret fields are refused here; credentials stay with bashy ask/vault"},"schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-24
Refuse a field whose type is `secret`, whatever its name.
```console
$ curl -s -X POST http://127.0.0.1:18195/api/action -H "Content-Type: application/json" -d '{"action":"add","id":"sec-2","fields":[{"name":"q","type":"secret","value":"x"}]}'
{"error":{"code":"invalid_arg","message":"Secret fields are refused here; credentials stay with bashy ask/vault"},"schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-25
Accept a field name that only looks similar: the rule is the list above, nothing more.
```console
$ curl -s -X POST http://127.0.0.1:18195/api/action -H "Content-Type: application/json" -d '{"action":"add","id":"sec-ok","fields":[{"name":"keyboard","type":"text","value":"x"}]}'
{"active":"r1","id":"sec-ok",...,"status":"pending",...}
```
exit: 0

### N-26
Read a note that does not exist: an error, exit 1.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show nope-404
error: Note not found. Create it with notepad add first.
```
exit: 1

### N-27
Refuse an id outside the defined pattern: a usage error, exit 2.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show 'bad id!'
error: use 1-64 letters, digits, - or _ for an id
```
exit: 2

### N-28
Refuse an unknown verb: a usage error, exit 2.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata frobnicate
error: unknown verb "frobnicate"
```
exit: 2

### N-29
Mint a share invite for a note.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata share note-1 --json --fields id,status
{"id":"note-1","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

### N-30
Close a note: its status becomes `closed`.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata close note-empty --json --fields id,status
{"id":"note-empty","schema_version":"bashy-notepad-v1","status":"closed"}
```
exit: 0

### N-31
A closed note refuses a new body.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata set note-empty --body x
error: This note is closed
```
exit: 1

### N-32
List a note's revisions: the second body write is the active revision.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata history note-1 --json --fields id,active
{"active":"r2","id":"note-1","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-33
Undo the last body write: the first revision is active again.
```console
$ notepad --addr 127.0.0.1:18195 --data ./.npdata undo note-1 --json --fields id,active
{"active":"r1","id":"note-1","schema_version":"bashy-notepad-v1"}
```
exit: 0

### N-34
Stop the service. `notepad stop` exits 0.
```console
$ notepad stop
```
exit: 0

### N-35
Stop again with no server running: still exit 0.
```console
$ notepad stop
```
exit: 0

### N-36
Restart the server on the same data folder, read the note stored before the stop (its body is the undone first revision), and stop the service again.
```console
$ (notepad --addr 127.0.0.1:18195 --data ./.npdata serve >.npserve.log 2>&1 &); for i in $(seq 1 50); do curl -sf http://127.0.0.1:18195/healthz >/dev/null && break; sleep 0.2; done; notepad --addr 127.0.0.1:18195 --data ./.npdata show note-1 --json --fields id,status,body; notepad stop >/dev/null
{"body":"Hello probe world","id":"note-1","schema_version":"bashy-notepad-v1","status":"filled-by-agent"}
```
exit: 0

## Page interaction (documented; executable as I-01 once conform supports `interact`)

A person edits the page and saves it. The CLI then reads the result:

```text
open   http://localhost:{PORT}/?id=note-1
fill   "Title" = Probe One
fill   "Note content (Markdown)" = Edited on the page
press  "Save Note"
$ notepad --addr 127.0.0.1:18195 --data ./.npdata show note-1 --json --fields title,body
{"body":"Edited on the page","schema_version":"bashy-notepad-v1","title":"Probe One"}
```

## Venue

A rebuilt `notepad` must run on: a 64-bit Linux or macOS host with a POSIX `sh`, `curl` on the PATH for the API assertions, and loopback networking only. Building the reference needs Go 1.27.4. Converting `html` and `pdf` input in the reference shells out to `bashy lens`, so `bashy` must be on the PATH for those two formats (KI-2). Nothing here needs a display, a network beyond loopback, or an account.

## Known issues

- KI-1 [workaround] feature:serve-and-stop — when no server answers, a verb prints a long dial error and exits with code 255 instead of a short error and exit 1 -> start the server first (`serve`, or the lifecycle `serve` step) and only then run other verbs
  retry: 1
  cause: proven (the CLI returns -1 for an unreachable server, which the shell reports as 255)
  expires: 2027-01-31
- KI-3 [workaround] feature:create-and-open — the page's field texts (Title, Note content, Import file) sit beside their controls without a programmatic label association, so assistive tech and label-based automation cannot find the controls by label -> drive the page by control name (title, body, importFile) or use the CLI
  retry: 1
  cause: proven (the shared desk kit renders the label and the control as siblings with no for, wrapping, or aria association)
  expires: 2027-01-31
- KI-2 [workaround] feature:import-formats — importing an `html` or `pdf` file fails when `bashy` is not on the PATH, because the reference converts those formats with `bashy lens` -> keep `bashy` on the PATH, or import the same content as `txt`, `md` or `json`
  retry: 1
  cause: proven (the converter runs `bashy lens -f html|pdf -t md`; the README's claim of a pure-Go PDF extractor does not match the code or the module list)
  expires: 2027-01-31

## Open questions for the owner

- Q1: The Go binary has no `stop` verb and no verb starts the server. This spec puts `stop` in the launcher that the lifecycle builds (the way the form wrapper stops its server) and asserts it (N-34, N-35). Should a future version grow a `stop` verb in the binary itself, and should `add` start the server the way `form add` does?
- Q2: Should the page's layout and copy buttons (Edit, Split, View, Copy text) be part of every implementation's page, or only the Title and Note content controls with Save Note and New Note?
- Q3: Export to `pdf` and `png` is asserted only by the reported `format`, never by the written bytes. Should a later spec pin any property of those files?
