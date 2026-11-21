# examples/ adapter inventory

Sprint #677 · Story #607 (`e0a18974cc3b`). One row per `examples/` entry:
**converted** (a standalone adapter entry exists and is proven), **kept**
(pure script — stays a registered command), or **reason** (why no adapter).

## What "adapter" means here

An example app is a Go `package main` with its own nested module. Its
adapter is `<app>.bsh` beside it: a Bash# entry that builds that module alone
(`GOWORK=off`, `bashy go` fallback) into `$BASHY_ADAPTER_CACHE` and `exec`s the
binary with argv, stdio, env and exit status unchanged. Examples are not
imported by the `bashy` binary (`go list ./examples/...` in the root module is
only `transcribe/adapters`), so no `agentos.go` case points at them — the
registered-command path (`register.sh`, `bashy-defaults apply`) is the front
door and is unchanged.

Independence proof: every Go `main` package under `examples/` carries
`standalone_deps_test.go` (`TestStandaloneDeps`). It runs
`GOWORK=off go list -deps .` and fails if the closure contains another bashy
package, a sibling bashy module, or a yoke/coreutils command package.
Fast (< 3 s each). Run it with
`cd <dir> && GOWORK=off GOTOOLCHAIN=go1.27.4 go test -run TestStandaloneDeps .`

## Go apps (standalone `main` packages) — 19

| Main package | Disposition | Adapter / reason |
|---|---|---|
| `brainstorm/server` | converted (pre-existing adapter) | `brainstorm/brainstorm.bsh` builds `server/` GOWORK=off; deps test added |
| `calendar` | converted (pre-existing) | `calendar/calendar.bsh`; deps test added |
| `clip/server` | converted (pre-existing) | `clip/clip.bsh`; deps test added |
| `desk/chart` | converted (pre-existing) | `desk/chart/chart.bsh`; deps test added |
| `desk/form/server` | converted (pre-existing) | `desk/form/form.bsh`; deps test added |
| `desk/notepad` | converted (pre-existing) | `desk/notepad/notepad.bsh`; deps test added |
| `desk/viewer` | converted (pre-existing) | `desk/viewer/viewer.bsh`; deps test added |
| `site/generator`, `site/sample/generator` | converted (pre-existing) | `site/site.bsh` runs `GOWORK=off go run ./generator`; deps tests added |
| `shaping/website/template` | converted (pre-existing) | `shaping/website/website.bsh` rod + `shaped.json`; deps test added |
| `session/server` | converted (pre-existing) | `session/server/run.sh` builds+execs (`serve`/`join`/`stop`/`build`); deps test added |
| `local-knowledge/kb-ui` | **converted (new)** | `local-knowledge/kb-ui/kb-ui.bsh`; smoke: `--help` through the adapter |
| `tray` | **converted (new)** | `tray/tray.bsh` (CGO on — webview); smoke: prints its own usage, exit 1 |
| `transcribe/webm-opus` | **converted (new)** | `transcribe/webm-opus/webm-opus.bsh`; smoke: no-arg usage envelope, exit 2 |
| `transcribe/adapters` | **converted (new)** | `transcribe/adapters/transcribe-adapter.bsh` (builds `./examples/transcribe/adapters` in the root module, `CGO_ENABLED=0`); smoke: usage, exit 2. Deps test allows only the shared `internal/transcribecontract` + `internal/transcribeseed`, never `internal/agentos` |
| `table/shape-adapter` | reason | shaped-app server: the console builds `shaped.json` `server.main`; no separate command surface. Deps test added |
| `whiteboard/server` | reason | same — `shaped.json` `server.main=server`; deps test added |
| `desk/kit/cmd/atlas` | reason | register-time helper (`go run ./cmd/atlas` inside `register.sh`), not a user command. Deps test added |
| `desk/kit/internal/fixture` | reason | test fixture app with its own `register.sh`/`memo.sh`. Deps test added |
| `board/board-console` | reason | generated at `board install` from `main.go.tmpl`; the checked-in file is an empty placeholder that keeps the module buildable |
| `manifests/gomod` | reason | `//go:build gomodfence` fixture source for the manifest-fence demo, not an app |

## Front doors

| Command | Disposition | Reason |
|---|---|---|
| `bashy transcribe` (`agentos.go` case, `internal/agentos/transcribe*.go`) | reason — not switched | The front door is not an `examples/` program: it reads the layered command registry in-process (`registeredLookup`/`registeredCommands` with shadowing, `BindCommandSchema`) to select and run providers. A standalone adapter would need a registry API at the process boundary first; changing it would alter selection/exit-code behaviour. Left in `agentos.go` untouched (no cross-story conflict). The example *helpers* it dispatches to (`transcribe-adapter`, `transcribe-webm-opus`) are converted above. |

## Pure-script and data examples (kept as registered commands / docs)

| Entry | Disposition | Note |
|---|---|---|
| `agentic` | kept | three `.bsh` demos run with `bashy --bashsharp` |
| `api-rods` | kept | `replicate-media.bsh` rod (+ `replicate.yaml`); Go module is tests only |
| `board` | kept | `board.bsh` + python API + shaped-app registration; no Go `main` of its own (see `board-console`) |
| `cast` | kept | `cast.bsh` embeds Go as a `~~~go` fence; module is library + tests |
| `chrome` | kept | `chrome.bsh` (`chrome`, `chrome-open` in defaults); module is tests only |
| `cli` | kept | YAML CLI adapter `main.bsh` per tool (codex, hermes-agent, mini-swe-agent, openclaw, opencode, ycode) |
| `collab-save` | kept | one `.bsh` + json |
| `dag` | data | `dag.md` project front doors (18 repos), no code |
| `defaults` | kept | `bashy-defaults.bsh` (registers the defaults); module is tests only |
| `desk` (root) | data | shaped-app tests + `kit`/`chart`/`form`/`notepad`/`viewer` listed above |
| `desktop`, `dks` | kept | single `.bsh` skills |
| `film` | kept | `film.bsh`, `lipsync.bsh`; module is tests only |
| `fleet` | data | model/tool/agent definition YAML |
| `founder-bootcamp`, `google`, `kb-catalog`, `klatch`, `matrix-chat` | kept | `.bsh` + `register.sh` (python helpers where present) |
| `launcher` | data | plain HTML |
| `llm-usage` | kept | `usage-alarm.bsh` + viewer template |
| `manifests` | data | fence fixtures per toolchain (`gomod` source noted above) |
| `media`, `record`, `video`, `vlog` | kept | `.bsh`/`.sh` skill-scripts; Go files are tests only |
| `notes` | kept | `notes.bsh` + shaped registration over a pinned third-party app |
| `onboard`, `outpost-profile`, `satellite-member`, `satellite-provision` | kept | `.bsh` (defaults) ; modules are tests only |
| `oss-rods`, `service-door` | kept | rod `.bsh` per provider; modules are tests only |
| `quickstart` | kept | Bash# teaching scripts with `.expected` outputs |
| `restore-board.py` | kept | stand-alone python utility |
| `runners` | kept | fence-runner `.bsh` examples |
| `shaping`, `shaping-host` | kept | `hello-world` rod and `website` rod/template (above); `shaping-host.bsh` |
| `local-knowledge`, `tray`, `transcribe`, `session`, `site`, `table`, `whiteboard`, `brainstorm`, `calendar`, `clip` | see Go apps | their scripts (`pipeline.bsh`, `session.bsh`, …) remain registered commands |

## Verification record (GOWORK=off, GOTOOLCHAIN=go1.27.4)

* Every example module builds alone; "no main packages" for library/test-only
  modules is not an error.
* `TestStandaloneDeps` passes in all 19 `main` directories and fails on a
  negative control (copied into `optional/arch`, it reports the coreutils
  command package).
* Adapter smokes above were run through `bashy --bashsharp <adapter>.bsh` with
  a clean `BASHY_ADAPTER_CACHE`.
