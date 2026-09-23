# Bashsharp Archive (`.bar`)

A Bashsharp Archive is a runnable app bundle. The official extension is `.bar`;
the payload is a regular ZIP or TAR archive, optionally gzip-compressed. Bashy
also accepts `.zip`, `.tar`, `.tar.gz`, `.tgz`, and `.gz` containers.

The archive root must contain `dag.md`, and the DAG must declare a `main`
target. That `dag.md` plus `main` is the bundle marker; no other manifest is
required. `bashy run` checks this marker before publishing the extraction to
the cache.

```sh
bashy run app.bar
bashy run app.zip
```

Each archive content version is extracted to
`~/.bashy/cache/bars/<bundle>/<sha256>/`. Identical bytes reuse the existing
directory. Changed bytes get a new directory, so an active run never sees its
files replaced. Set `BASHY_HOME` to relocate this cache with the rest of
Bashy's user state.

The launcher executes `dag.md`'s `main` target with the extracted directory as
the working directory. Extra arguments after the archive are available as
JSON in `BASHY_BAR_ARGS_JSON`. Archives are bounded in compressed size,
expanded size, and entry count. Absolute paths, parent traversal, duplicate
file paths, links, and special files are rejected during extraction.

A DAG file or directory can also run directly, without archiving:

```sh
bashy run ./app/dag.md
bashy run ./app --target deploy
```

For direct DAGs, `--target NAME` selects a target. Without it, Bashy selects
`main` if present and otherwise lets the DAG runner use its configured default.
Direct DAGs run from their own directory and receive extra arguments as JSON in
`BASHY_DAG_ARGS_JSON`.
