# TaaS A/O/C end-to-end gate (Sprint 739 K5)

Commands and folders as a service, as typed tool calls, in one disposable
container. `run.py` has the shape of `tests/group-shaping-e2e/run.py`: it
checks the locked Ubuntu base image and runs it through `bashy oci` with
networking off. It mounts this directory at `/fixture` and the group-shaping
fixture at `/group-shaping`, whose `check`, `ready` and `run` helpers
`sandbox.py` imports.

Roles: the container host is **C**, the offerer, with its door
(`bashy llm serve --no-engine`, port 24556) and the real outpost. **A** and
**O** are members of the `support` group in the fake satellite's grant copy
(cloudboxfake-style accounts). They reach C's `offer-NAME` apps through the
front with synthetic `X-Periscope-User` headers, and outpost signs the stamp.
A second door with its own `BASHY_HOME` (port 24600) plays O's owned host.
It serves the peer path and the symmetry check, where O offers `files-in` to C.

No LLM and no network. A fake OpenAI-compatible server is registered as fleet
model `fake-model`, used for `llm` and as agent `helper`'s model. The `codex`
stub here is the agent binary. `transcribe` is a registered script command
with a typed `--input` flag. It writes `out.txt` and prints JSON with
`seconds`.

Checks, each printed as an `ok - ...` line with its evidence:

1. `bashy llm call` and the agent tool run locally on C.
2. C offers `fake-model` (model), `transcribe` (tool, `--limit 1`) and
   `helper` (agent, `--token-cap 64`). Each prints its `bashy remote call`
   line. Offering `cat` (internal) or `untyped` (no args schema) is refused.
3. A and O call the llm offer through the front. A transcribes
   `bashy://o/files-in/in.wav` and gets a `files-offers` artifact URI. O runs
   `bashy remote call c transcribe` as a peer and streams the artifact with a
   Range request (206).
4. An agent call above the offer cap is refused. A call within the cap stays
   under it, or the adapter refuses a cap it cannot enforce. The log records
   which happened.
5. T1 (`TAAS_T1=1`): `bashy offer usage --json` on C and
   `bashy remote usage --json` on O agree per tool: tokens for llm and the
   agent, seconds for transcribe, bytes for files. No record has a price.
6. T2 (`TAAS_T2=1`): `bashy remote add c transcribe` and
   `bashy remote add c files-docs` appear typed in `bashy mcp tools` and work.
   After the offer's definition changes, the mounted tool refuses with 409.
7. Refusals: a non-member and an anonymous caller (401/403), path escapes
   (`..` and a symlink out of the folder), a command not offered on the app
   (403), a peer calling a non-offered command (403), A's second transcribe
   (429), and a read after `bashy files unshare` (403). Sharing HOME without
   `--force` is refused.
8. Symmetry: O shares `in` with C as a peer, and C reads `in.wav`.

Folders: A reads C's managed `docs` folder in place. A also reads an outside
`--path` share through the typed `files-NAME` tool.

## Run on a claimed gate lane

The inputs are the group-shaping gate's: `GROUP_SHAPING_BIN` (bashy, outpost,
delegate), `GROUP_SHAPING_PYTHON` and `GROUP_SHAPING_GATE=1`; see
`tests/group-shaping-e2e/README.md`. The run targets less than 5 minutes.
`TAAS_TIMEOUT` (default 420 s) bounds the container. `TAAS_WAIT` (default
15 s) bounds readiness waits. No wait is a fixed sleep.

```sh
cd bashy
python3 tests/taas-e2e/run.py --evidence /path/to/private-evidence/taas-run1.log
TAAS_T1=1 TAAS_T2=1 python3 tests/taas-e2e/run.py --evidence /path/to/private-evidence/taas-run2.log
```

`run.py` passes `TAAS_T1`, `TAAS_T2` and `TAAS_WAIT` into the container with `-e`.
