# Author a transcription source or decoder

The editable [`pipeline.bsh`](../examples/transcribe/pipeline.bsh) calls four
ordinary registered commands: acquire, decode, infer and export. A new source
or decoder is an executable that reads one v1 JSON request path from argv and
writes one v1 JSON result to stdout. Register its command record in the normal
local ring; the pipeline and Bashy binary do not change. The complete field
contract is in [transcribe-contract.md](transcribe-contract.md).

## Set up and run the seed pipeline

Install the admitted native Bashy binary, seed helper, decoder, inference binary
and model for your host. Follow the exact build and registration instructions in
[the seed adapter README](../examples/transcribe/adapters/README.md), and use
[`transcribe.md`](../skills/bashy/commands/transcribe.md) to register the one
pipeline body and its skill face. Run model setup deliberately before going
offline. The seed helper verifies configured executable hashes at invocation;
inference reads a verified cached model and does not fetch it implicitly.

For an already admitted WAV setup, a local run looks like this:

```sh
bashy transcribe-model-setup base
bashy transcribe --source transcribe-local --decoder transcribe-wav \
  --provider transcribe-whisper --exporter transcribe-export \
  --model base --language auto --format txt --output /private/speech.txt \
  /private/speech.wav
```

Use `--format srt` or `--format vtt` and a fresh matching output path for the
subtitle exports. The existing output is never overwritten. After setup,
repeat from the local media path with network access disabled externally and
record that denial separately from the transcript result. A source that needs
the network, such as YouTube, cannot perform a fresh acquisition offline;
replay the downloaded local media instead.

## Walk through two new adapters

[`verify-http-fixture-extension.sh`](../examples/transcribe/verify-http-fixture-extension.sh)
is a complete, disposable example. It creates a loopback page containing a
media reference and a small FPCM file. Its `fixture-site` operation really
fetches the page and media; `fixture-fpcm` validates the media bytes and emits
PCM. The fixture infer operation checks those exact PCM bytes before returning
labelled fixture text. It is a registration and data-flow demonstration, not
speech recognition or native inference evidence.

1. Copy the two operations in `fixture-stage.sh` from that example into your
   own executable. Accept the JSON request file as an argv element. The acquire
   request carries `input` and `work_dir`; return an existing local media path
   in `artifacts`. The decode request carries that artifact and `work_dir`;
   return normalized audio plus `media.duration_ms`, `sample_rate_hz` and
   `channels`. Keep only owned intermediates under `work_dir`. The pipeline
   removes that directory on success, failure and cancellation.
2. Register each executable with its open role, request applicability, host
   requirements and effects. This example uses one executable with two
   operations; `/absolute/path/fixture-stage.sh` is a placeholder for the
   copied file. Give the source a URL pattern and the decoder an artifact-path
   pattern so automatic selection does not claim unrelated media:

   ```sh
   bashy commands add fixture-site-source \
     --set exec.0=/bin/sh --set exec.1=/absolute/path/fixture-stage.sh \
     --set exec.2=fixture-site --set provides.0=transcribe.acquire \
     --set accepts.0.path=/input \
     --set 'accepts.0.pattern=^http://127\.0\.0\.1:[0-9]+/' \
     --set 'requires=has=curl' \
     --set os.0=linux --set os.1=darwin \
     --set effects.0=read --set effects.1=write \
     --set effects.2=exec --set effects.3=net
   bashy commands add fixture-fpcm-decoder \
     --set exec.0=/bin/sh --set exec.1=/absolute/path/fixture-stage.sh \
     --set exec.2=fixture-fpcm --set provides.0=transcribe.decode \
     --set accepts.0.path=/artifacts/0/path \
     --set 'accepts.0.pattern=\.fpcm$' \
     --set 'requires=has=python3' \
     --set os.0=linux --set os.1=darwin \
     --set effects.0=read --set effects.1=write --set effects.2=exec
   ```

   Declare every effect the implementation uses, including `exec` if it
   launches another program, and add `os`/`requires` constraints only for
   coordinates the executable supports. Registration and declared platform
   support do not prove native execution. `commands show NAME --yaml` displays
   the effective record; `commands set NAME ...` creates a local override.
3. Inspect selection with v1 request JSON before invoking the whole pipeline:

   ```sh
   bashy transcribe select acquire /private/acquire-request.json
   bashy transcribe select decode /private/decode-request.json
   bashy transcribe select decode --provider fixture-fpcm-decoder \
     /private/decode-request.json
   ```

   `select` reports every matching name if there is ambiguity. Pass
   `--source` or `--decoder` to the pipeline to choose explicitly; the named
   command still has to match its role, applicability and host requirements.
   An absent dependency produces a selection error rather than a fallback
   success. A stage failure must exit nonzero and return an `error` object with
   no success artifacts or segments. Treat input paths and URLs as argv data,
   never as shell source.
4. Run the example's end-to-end fixture check with an S503-capable Bashy binary:

   ```sh
   BASHY_BIN=/absolute/path/bashy BASHY_TRANSCRIBE_NATIVE=1 \
     sh examples/transcribe/verify-http-fixture-extension.sh
   ```

   It checks real HTTP acquisition, real FPCM decoding, automatic and explicit
   selection, ambiguity, unavailable-provider refusal, a local pipeline
   override, script/skill output parity, and a valid skill receipt. Its output
   text is `fixture media decoded` and is intentionally synthetic. For real
   TXT/SRT/VTT, run a speech-bearing fixture through the registered source and
   decoder with an admitted inference provider, then inspect each export and
   the skill/function receipts as in the
   [native gate](../examples/transcribe/verify-controlled-native.py).

## Dependency and license boundary

Record the source, version, SHA-256, license and build provenance for the exact
adapter executable, decoder, inference tool and model used. A digest establishes
identity, not redistribution permission or native support. The optional FFmpeg
record is separately admitted and is not installed by seed registration. The
YouTube source uses an independently pinned yt-dlp runtime; its source license
does not settle bundled executable, Python, JavaScript runtime or model terms.
Follow [the dependency setup guide](../examples/transcribe/dependencies/README.md)
and the project's [licensing policy](licensing-supply-chain-policy.md) before
shipping an adapter or downloaded tool. Keep acquired media, model weights and
transcripts outside public source control.
