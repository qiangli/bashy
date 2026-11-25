---
name: form
description: Lifecycle of the form shapeware app (source, build, test, install, run). Use when asked to fetch, build, test, install or run form.
type: dag
default: run
---

# form: lifecycle

Every step is exact and non-interactive. Run one from this folder with
`bashy dag -f dag.md <step>` (or `bashy awd PATH -- bashy dag -f dag.md <step>`).
Without bashy, read the step's code block. `$BASHY` is set by `bashy dag`.

## Tasks

### source
Where the source is. This folder is the complete source of release 0.1.0; nothing to fetch.
Effects: read

```bash
echo "source: this folder (form 0.1.0); nothing to fetch"
```

### build
Verify the pinned trusted-core digest in `shaped.json`, ensure the cached core server binary is ready, and parse-check `form.bsh`.
Effects: read, write, exec

```bash
"$BASHY" --bashsharp --check ./form.bsh
"$BASHY" --bashsharp ./form.bsh --build-core
```

### test
Run the shapeware acceptance checks (`PASS`/`FAIL`/`TBD` lines; exit 1 on any `FAIL`).
Effects: read, write, exec, net

```bash
"$BASHY" --bashsharp ./checks/acceptance.bsh
```

### install
Install `form` as a registered bashy command, shapeware skill, and shaped app.
Effects: read, write, exec

```bash
"$BASHY" ./register.sh
```

### run
Run `form` with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='add --def examples/contact.json --title "Intake" --open'`. Default: `list`.
Effects: read, write, exec, net

```bash
mapfile -d '' -t argv < <(printf '%s' "${ARGS:-list}" | xargs printf '%s\0')
"$BASHY" --bashsharp ./form.bsh "${argv[@]}"
```

### serve
Start the foreground loopback form server (default 127.0.0.1:18107; custom `-data` uses its own port).
Effects: read, write, exec, net

```bash
"$BASHY" --bashsharp ./form.bsh serve
```

### show
Read the registered executable body and skill face.
Effects: read

```bash
"$BASHY" commands show form
"$BASHY" skill show form
```
