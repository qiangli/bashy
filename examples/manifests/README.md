# Manifest fences — a script carries its project manifest

Each directory is one toolchain bashy provisions: the sources sit beside a
`build.bsh` whose fence *is* the manifest — `~~~cargo`, `~~~pyproject`,
`~~~gomod`, `~~~cmake`, `~~~makefile`, `~~~package` — and whose alias exposes
the toolchain's verbs with their declared effects. Run one from anywhere:

```sh
bashy awd examples/manifests/cargo -- bashy --bashsharp examples/manifests/cargo/build.bsh
```

`awd` chooses the directory; the verbs run there, the manifest, dependency
caches and outputs live under the fence root in the user cache, and the
directory stays byte-identical (`git status` sees nothing). `gomod/build.bsh`
carries both a `~~~gomod` manifest and a `~~~go` code fence: the manifest is
the code fence's module, in a directory with no `go.mod`.

`make smoke-dag-manifests` runs all six against the installed binary in a
scratch copy of each directory and asserts the copy is unchanged. The
conventions per tool (why cargo shadows `src`, why go uses `-overlay`, why
npm sees `INIT_CWD`) are in `bashsharp/docs/fenced-text-blocks-plan.md`
§Manifest fences.
