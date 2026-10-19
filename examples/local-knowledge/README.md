# Local Knowledge — kb-ui

This example preserves kg-ui's offline monitor, explorer and source-preview
interface against Bashy's shared document index. It needs no kg daemon or store.

Build from the Bashy repository:

```sh
go build -o bin/kb-ui ./examples/local-knowledge/kb-ui
bin/kb-ui --bashy bashy --addr 127.0.0.1:6674
```

Open the loopback URL, select **monitor**, and add a directory in **indexed roots**.
`~` explicitly selects the current user's home. No root is selected automatically.
The add action explicitly initializes the shared index if necessary. Root scope,
exclusion reasons, processed counts, pause/resume and forget use that same index.
An existing index is rediscovered after UI restart.

For continuous updates, initialize and enable the existing service:

```sh
bashy search index init
bashy service enable index
bashy supervisord --only index
```

The supervisor runs in the foreground; use the normal host service setup for
persistent operation. Its index unit is disabled by default. `BASHY_HOME` selects
an isolated state home for both commands and the UI. Source files remain read-only.

Search defaults to **auto**, with an explicitly labeled lexical fallback. Semantic
and hybrid modes require a configured local model door and an explicit embedding
model, enabled with `bashy search index model MODEL`. No model is downloaded and no
cloud provider is selected automatically. Curated kb pages remain separate;
`bashy kb recall QUERY --ring index` reads indexed sources.

Previews return scrubbed indexed text through the shared source API. They cannot
open arbitrary files, bypass current exclusions, download cloud placeholders or
serve unindexed binary files. Preview responses are plain text with a restrictive
content policy. File links retain source provenance.

Graph enrichment, the legacy job queue and disk telemetry are explicitly
unavailable in this adapter. Their offline interface assets are retained for the
Local Knowledge consumer; missing backend features never become synthetic data.

Resource limits are visible in `bashy search index status --json`. The default
corpus cap is 1,000 sources and 4 MiB of extracted text, with a separate shared
retrieval cap of 1,000 embedding chunks. Use a narrower root when a cap is reached.
Configure promotion/decay and file/root limits with `bashy search index config`.

The example DAG exposes build, test, smoke and run. Its smoke uses synthetic
inputs and must never point at the operator's real home. Tests use the real shared
command implementation in isolated temporary state; native installed-artifact and
browser checks are recorded by the owning sprint.

See [ATTRIBUTION.md](ATTRIBUTION.md) for the UI's source and retained licenses.
