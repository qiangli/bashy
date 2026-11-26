# form: the spec release

A spec release of `form` is a folder of its own that holds exactly these
files, and nothing of the reference build:

| File in the spec release | Taken from this folder |
|---|---|
| `CONTRACT.md` | `CONTRACT.md` |
| `SKILL.md` | `SKILL.md` |
| `features.yaml` | `features.yaml` |
| `examples/` | `examples/` |
| `dag.md` | `spec/dag.md` |

`spec/dag.md` is the lifecycle of the app as a spec: steps that run with `sh`
and need neither bashy nor this reference build. An implementation makes
those steps work and does not change the file. The `dag.md` beside
`CONTRACT.md` in this folder is the lifecycle of the reference build, whose
steps run through bashy.

A spec release must pass `conform spec check` on its own.
