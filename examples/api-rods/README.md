# Replicate API media rods (Sprint 627 M5)

`song-replicate`, `image-replicate` (`image.generate` and `image.edit`), and
`animate-replicate` implement the [service rod contract](../../docs/service-rod-contract.md).
They are registered by `bashy-defaults apply` with route `api` and a declared
`spend` effect. The same bootstrap imports `replicate.yaml`, whose five
commands use `mode: http`: four prediction creations and one prediction read.

The tool's `token_ref: replicate` resolves a host vault binding, conventionally
`REPLICATE_API_KEY=@replicate` in the host's `secrets.map`. No key is accepted
in a request, written to a repo file, or printed by the rod. The caller's
explicit `--yes` is required to select a paid rod; a direct rod invocation
checks `BASHY_SVC_YES=1` before it creates a prediction. A service door dry
run selects the paid route for display without dispatching a rod or calling
HTTP and reports `cost_class: api`.

The selected models are [MiniMax Music 2.5](https://replicate.com/minimax/music-2.5/api/schema)
for vocal songs, [FLUX Schnell](https://replicate.com/black-forest-labs/flux-schnell/api/schema)
for image generation, [FLUX Kontext Pro](https://replicate.com/black-forest-labs/flux-kontext-pro/api/schema)
for image edits and reference guided generation, and
[OmniHuman](https://replicate.com/bytedance/omni-human/api/schema) for an image
animated by an audio clip. Their inputs and output shapes are model specific;
the rod maps the common request onto them. Image generation supports 1–4 PNG
outputs. Image edits and reference guided generation support one PNG. Masks
are refused. OmniHuman clips are limited to 15 seconds; longer performances
must be split. The animation model determines output dimensions and frame
rate. The song model generates vocals; `instrumental: true` is refused.

Local image and audio inputs become data URLs inside a private 0600 temporary
JSON file, capped at 12 MB per input and 32 MiB for the transport body. The
typed HTTP runner reads that file without putting its contents in argv.
Prediction creation and polling are bounded, and only completed predictions
yield an artifact. The output URL must use `https://replicate.delivery/`; the
download has no API bearer. `ffprobe` must decode each downloaded file before
the rod reports success.

Verification is offline: `GOWORK=off go -C examples/api-rods test ./...` uses
fixture prediction responses and local media bytes. The `pkg/toolcmd` test
stubs the HTTP transport and checks the bearer, JSON body and escaped poll
path. No paid API call is part of those gates. An operator approved live paid
smoke remains a separate check.
