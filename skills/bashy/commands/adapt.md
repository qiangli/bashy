---
name: adapt
description: Apply declared native common-option mappings to a foreign command, refusing unsupported requests before execution.
---

# adapt

`bashy adapt [COMMON-OPTIONS] -- CMD ARGS...` matches an exact argv prefix
against fleet `adapter` records; the longest prefix wins. No shell string is
constructed. Native stdout, stderr, stdin and executed exit statuses pass through.
No executable is downloaded or provisioned.

Trigger word: `bashy adapt --OPTION -- CMD` gives any foreign command the common options.

```
bashy adapt --null -- git status
bashy adapt --dry-run -- git clean -fd
bashy adapt --json -- docker ps
bashy adapt --json -- kubectl get pods
bashy adapt --explain -- apt install jq
bashy adapter list
bashy adapter show git-clean --yaml
bashy adapter add ./my-adapter.yaml
bashy adapter set my-adapter --set summary='My command'
```

Rows live in fleet's embedded/shared/local rings. Use `adapter add`, `set`,
`rm`, `edit`, `show`, and `schema` to manage them. `BASHY_ADAPTERS_PATH`
mounts shared directories; `BASHY_ADAPTERS_DIR` selects the local store.
A local same-name record replaces its lower-ring definition. Ambiguous equal
prefixes and invalid catalogs refuse; they never select an arbitrary row.

```yaml
kind: adapter
name: my-list
match: [my-tool, list]
json: [--output=json]
preview_level: unsupported
effects: [fs.read, proc.spawn]
```

Missing/null mappings mean unsupported (except `fields`, which uses generic JSON projection); `[]` explicitly declares that the
native invocation already has the requested behavior. Mapping values are argv
arrays. `{1}` expands a valued option without shell interpolation. Declaration
keys accept both separators; argv values are never normalized. Options are
appended, and a target `--` terminator refuses a nonempty mapping rather than
turning a dry-run switch into an operand. Global options before a subcommand
need their own explicit row; adapt does not guess how foreign flags parse.

Unsupported wrapper requests exit 125 with a reason before running the target.
`--no-input` never implies yes/force or closes stdin. `--explain` never executes;
its first implementation shows the row, effects and preview level and omits
user argv/option values that might contain credentials. It does not claim a
complete foreign argument schema.

Curated native mappings include Git status/ls-files NUL output, Git clean
preview, Docker ps JSON, kubectl get JSON/apply server preview, rsync preview,
and apt install/remove simulation. Preview effects and support depend on the
installed client and explicit argv. The base rows conservatively describe
possible effects; they grant no authority. Make dry-run is unsupported because
`make -n` may execute recipes. Rsync NUL output is unsupported because `-0`
controls input. Tar/unzip expose native listing templates to the provider API;
listing is not added to the closed common CLI option table.

Native output streams directly with no capture or claim of secret scrubbing.
`json: 'lens: cmd:NAME'` preflights the published S3 lens route table before
execution and converts through its API. Missing parsers exit 125; S3 currently
publishes no `cmd:` rows. Adapt supplies no private parser.

`--json --fields=a,b` prefers a declared native `fields` mapping, otherwise
projects literal top-level keys from JSON objects, arrays of objects, or NDJSON.
Missing keys are omitted; number literals and nested values retain precision.
Generic conversion/projection buffers at most 8 MiB of input and accepts at most
8 MiB of converted output. Overflow drains stdout, emits no partial converted
result, and reports a value-free diagnostic. Malformed output and conversion
failures also preserve the executed child's exit status, including zero; callers
must inspect diagnostics. Wrapper diagnostics omit parser errors and raw values;
explicit stdout data and child stderr are not automatically scrubbed.

`--since` refuses until S19 supplies the shared cursor binding/validation seam.
No private cursor is used.
