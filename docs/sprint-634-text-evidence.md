# Sprint 634: recorded local text drafting — evidence

Story #3232 / `759ea0f92839`; agent `codex-gpt6-astra`.
Dependency `aad065039174` was already merged at checkout HEAD `d3d8a641`.
Implementation sequence: [text plan](sprint-634-text-plan.md); umbrella plan:
`docs/sprint-634-master-execution-plan.md`.

## Delivered behavior

- `translate-ollama` now provides both `translate` and `text`. One shared
  doctor, staged-tag check and HTTP client; pinned default `qwen3:8b`, no
  pulls. Translation retains temperature 0. Drafting takes request temperature
  (default 0.7), disables thinking and requests a kind-specific JSON schema.
- `film write DIR --route oss --provider-text NAME --kind
  lyrics|shots|prompts|all` reads `brief.md` and updates only prose fields in
  the existing storyboard. Shot indices and character ids are checked before
  each patch is applied. Omitted objects and nonselected fields remain intact.
- `text/NN/` retains the brief, requests, raw model responses, before/after
  storyboard snapshots, stages and manifest. Each stage records provider,
  route, cost class, elapsed seconds and `fields_written`.
- Draft manifests copy the latest text manifest as `text_provenance`.
  Hand edits remain valid; the recorded text storyboard hash is retained,
  not silently replaced by the current draft's hash.
- Native Windows verification exposed two existing portability blockers:
  the service door now converts shell `/c/...` paths to native `c:/...`
  before external/native consumers open them, and the shared request reader
  uses jq environment bindings instead of Python/unsupported `--arg`.

## Targeted fixture gate (development host; no model runs)

The examples are nested Go modules. An untracked temporary Go workspace
included only the repository root, `examples/film` and `examples/oss-rods`.
Command:

```sh
GOWORK=/tmp/s634-text.go.work OSS_AVAIL_KB_OVERRIDE=104857600 \
  go test ./examples/film/ ./examples/oss-rods/ -count=1
```

The disk override applies only to fixture tests: the existing image fixture
preflight requires 35 GiB even though it does not load weights. No live run
used this override.

Coverage includes deterministic text output from the request; three recorded
text stages and six written fields; a second shots-only write preserving
hand-edited lyrics; sparse patches preserving omitted objects and production
settings; invalid indices leaving the storyboard untouched with an error
manifest; latest text provenance copied into a subsequent draft; the shared
Ollama client with custom/default temperatures, thinking off, response schema,
and no pull endpoint. Existing translation and media fixture cases also run.

Final gate: **PASS** — `examples/film` 189.338 s; `examples/oss-rods`
33.338 s, exit 0. `git diff --check` also passed. A current Windows amd64
binary was cross-built with `CGO_ENABLED=0 GOOS=windows GOARCH=amd64
GOWORK=off go build ./cmd/bashy` for the native run.

## Live run (Windows test host, 2026-11-13 UTC)

The Windows test host was claimed before use and released afterwards. All
access used the direct `bashy outpost ssh USER@HOST -- ...` form. Work used
an isolated story directory and command rings; existing review projects and
model weights were not edited. The installed host binary predated `route`
registration, so a current Windows build was placed in the isolated directory.
Native jq/sha256sum applet hardlinks supplied PATH-based prerequisite probes.
No paid route, `--yes`, model pull, package install or Podman workload was run.

Native Ollama answered its doctor with `available:true` and listed the
already-staged `qwen3:8b` tag, Q4_K_M, digest:
`500a1f067a9f782620b40bee6f7b0c89e17ae61f686b92c24933e4ca4b2b8b41`.

The brief described “Morning by the sea”: a hopeful sunrise and an original
adult singer in a blue coat, two lyric lines, two shots and portrait prompts.
The successful `film write` was **text/05**, at approximately 21:21 UTC:

| Stage | Provider | Route / cost class | Elapsed | Written fields |
|---|---|---|---:|---|
| text.lyrics | translate-ollama | oss / oss | 3 s | lyrics |
| text.shots | translate-ollama | oss / oss | 5 s | shots[0].description, shots[1].description |
| text.prompts | translate-ollama | oss / oss | 11 s | shots[0].prompt, shots[1].prompt, characters[0].prompt |

Overall elapsed: **21 s**, outcome **success**. The model wrote the lyric
lines “Sunlight breaks” and “Ocean breathes”; no agent rewrote the output.
The brief's four-word-per-line preference was not satisfied: counts are hints,
not a claim of semantic or artistic validation. An earlier unstructured
prompt response had an incorrect wrapper and was rejected before application;
the final run used the response schema. Earlier failed native setup attempts
remain in text/01–04 as failure provenance.

Then `film draft DIR --route oss --no-qa --no-shot-numbers` ran on the same
Windows test host using its staged WSL media stack and native image inference.
Only `song.duration_s` was manually changed from 6 to 10 to meet ACE-Step's
minimum; locally drafted text was unchanged. The draft copied text/05's
provenance without rejecting this subsequent production-setting edit.

**drafts/01: success, 108 s.**

| Stage | Provider | Route / cost class | Elapsed |
|---|---|---|---:|
| song | song-acestep | oss / oss | 36 s |
| portrait:lead | image-klein (native inference) | oss / oss | 38 s |
| transcribe | transcribe-whisperx | oss / oss | 19 s |
| animate:1, animate:2 | film-still-pan | local / local | 0 s recorded each |

No OSS animate rod was available: both still-pan fallbacks are explicitly
recorded as degraded. No vendor animation ran. QA was disabled. WhisperX
transcribed a different sung phrase and anchored 0 of 2 lyric lines, so timing
was interpolated; this proves the local pipeline and provenance, not lyrical
fidelity or lip sync. Contact-sheet inspection confirmed both still shots.

`ffprobe` verified H.264 video at **320×180**, AAC audio, a subtitle stream,
and duration **12.066667 s**. Film, contact sheet, look sheet and subtitles
are nonempty. Native Ollama was unloaded after writing; the draft's own
ACE-Step process group stopped on exit, and a process check confirmed it was
gone before the host claim was released.

## Artifact identities

Artifacts remain under the isolated story directory on the Windows test host
(`proj/text/05` and `proj/drafts/01`), with local verification copies outside
the repository. Paths here identify roles only.

| Artifact | SHA-256 |
|---|---|
| text/05/manifest.json | `0003568e1ba628d5908c90cc3084557640cbfd184c2a7b6506b406dea4c01d2f` |
| text/05 storyboard hash (also copied into draft provenance) | `1b2c383b584187f3035972e021c0e44592bcfd29e813b8f1eb65bdf3ef6538f0` |
| drafts/01/manifest.json | `6ae4765ec5294a5f313e920c1b36a44a04c91d981c651ddb83af90103cad08a4` |
| drafts/01/film.mp4 (310012 bytes) | `3dd71dd41e08cf0e62542ddb64460380829936d8854f834a66a93586bf0ee301` |
| drafts/01/contact.png | `bb74f2d58ee07dccb88365aaf51c9114eb10620478dfff56351f3a8ece3d2a3a` |
| drafts/01/looksheet.png | `7a89b633958dc1369a55f6b5097c582417aa7db42a23facc5d2eae7458baef8c` |
| drafts/01/subs.ass | `021e6d297792db0aa6d5095064517da2024736378af83c359d488419c9b98951` |
