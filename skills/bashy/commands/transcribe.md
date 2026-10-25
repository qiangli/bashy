---
name: transcribe
description: 'Compose registered acquisition, decode, inference and export adapters for one finite file or URL. The single editable Bash# body is examples/transcribe/pipeline.bsh; register it as transcribe-pipeline before running.'
metadata:
  requires: "has=bashy"
  step-porota: 'test -n "$BASHY_TRANSCRIBE_INPUT" && test -n "$BASHY_TRANSCRIBE_RESULT" && bashy transcribe --source "$BASHY_TRANSCRIBE_SOURCE" --decoder "$BASHY_TRANSCRIBE_DECODER" --provider "$BASHY_TRANSCRIBE_PROVIDER" --exporter "$BASHY_TRANSCRIBE_EXPORTER" --model "${BASHY_TRANSCRIBE_MODEL:-default}" --language "${BASHY_TRANSCRIBE_LANGUAGE:-auto}" --format "${BASHY_TRANSCRIBE_FORMAT:-txt}" --output "$BASHY_TRANSCRIBE_OUTPUT" "$BASHY_TRANSCRIBE_INPUT" > "$BASHY_TRANSCRIBE_RESULT"'
  check-tests: 'test -s "$BASHY_TRANSCRIBE_RESULT" && test -s "$(cat "$BASHY_TRANSCRIBE_RESULT")"'
---

# Transcribe one local media input or URL

This skill's executable step calls `bashy transcribe`. That front door invokes
the registered `transcribe-pipeline` command, whose only implementation is
[`pipeline.bsh`](../../../examples/transcribe/pipeline.bsh). Run the script
directly with `bashy --bashsharp` to use the code-outer orientation. This
skill face is embedded through bashy's single `bashy` skill; copy it to a
local skill ring when editing the prose. The same Bash# file runs in both
orientations.
The script's `--skill` flag prints its code-facing `~~~skill` guidance.

## Requirements and discovery

- `bashy commands show transcribe-pipeline` identifies the registered body;
  `bashy src show transcribe-pipeline` points to its editable source file.
- `bashy commands list` discovers stage commands. Each selected stage must
  implement the v1 JSON contract in `docs/transcribe-contract.md`.
- This unit needs bashy's portable `jq` applet and a writable temporary
  directory. Its cap is read, write, destroy (owned temporary cleanup), and
  network. The Bash# body also declares exec because adapters are commands.
  Adapter-specific dependencies and effects remain in their registered records.
- Stage selection calls `bashy transcribe select STAGE REQUEST.json`;
  an explicit name is sent as `--provider NAME` to the same selector and
  takes precedence. S503.2 supplies declared applicability, ambiguity
  errors and local overrides. Until S503.2 is integrated, the fixture shim
  is the only selector; the production command cannot run.
  S503.4 and S503.6 supply real inference and verified dependencies.
- The pipeline passes optional `work_dir` in acquire, decode and infer
  requests. Adapters should put owned intermediates there so the unit removes
  them on success, failure or cancellation. Export writes to the pipeline's
  adjacent temporary output; the unit publishes it with an exclusive hard
  link only after validation and refuses an existing final path.

## Run

```bash
bashy commands add transcribe-pipeline \
  --set exec.0=bashy --set exec.1=--bashsharp \
  --set "exec.2=$PWD/examples/transcribe/pipeline.bsh" \
  --set effects.0=read --set effects.1=write \
  --set effects.2=destroy --set effects.3=exec --set effects.4=net

bashy transcribe --source SOURCE --decoder DECODER \
  --provider INFERENCE --exporter EXPORTER input.wav

export BASHY_TRANSCRIBE_INPUT=input.wav
export BASHY_TRANSCRIBE_RESULT="$PWD/transcript-result.txt"
export BASHY_TRANSCRIBE_OUTPUT="$PWD/input.txt"
export BASHY_TRANSCRIBE_SOURCE=SOURCE BASHY_TRANSCRIBE_DECODER=DECODER
export BASHY_TRANSCRIBE_PROVIDER=INFERENCE BASHY_TRANSCRIBE_EXPORTER=EXPORTER
bashy skill run transcribe
```

The skill receipt is the existing `unit.Attestation` projection, stored in
the skill ledger. The Bash# function writes its own receipt to the same
ledger for either entry point; its identity, checks and effects come from the
one body. A failed step cannot produce a valid skill receipt. The command
record declares the unprojected Bash# effects; the skill face carries their
read/write/destroy/net projection. `bashy src show transcribe-pipeline`
exposes the editable body; `bashy commands set transcribe-pipeline --set
exec.2=/absolute/local/pipeline.bsh` composes a local variant without core
dispatch edits. `bashy skill show transcribe` discovers the prose face.

## Verification

`sh examples/transcribe/verify-stub.sh` registers fixture adapters, runs
the script and skill orientations, compares output bytes and function receipts,
checks the skill identity, contract, effect cap and receipt, and proves a
local command source override. With a candidate binary that contains S503.1,
set `BASHY_BIN=/absolute/path/to/bashy BASHY_TRANSCRIBE_NATIVE=1` to use
its real front door, registered `provides` roles and selector. That mode
checks `src show` before and after a local override, automatic selection,
ambiguity and explicit precedence. The default test shim supplies a minimal
front door and selector for an older installed binary so the unit can be
developed before S503.2 integrates.
Fixture text says `fixture only`; it does not prove transcription or S503.2's
real applicability rules. S503.4/5/6/8 own media quality and native evidence.
