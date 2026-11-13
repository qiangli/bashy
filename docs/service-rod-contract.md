# Service rod contract (v1) — song, image, animate, narrate

Sprint 627 M1. A **rod** is one provider of an open service role: a
registered command that declares `provides:` and a `route:`. The service
verbs (`bashy song`, `bashy image`, `bashy animate`, `bashy narrate`, `bashy transcribe`, …) are
the one door body `examples/service-door/service-door.bsh`, registered by
`bashy-defaults apply` with the role as the door's first fixed argument. The
door selects a rod with `bashy providers select`, hands it the request, and
emits one `bashy-svc-v1` envelope. Every route (`oss`, `browser`, `api`) of a
role takes the same request and produces the same envelope, so a caller
(`film.bsh`, Sprint 627 M7) changes only `--route` or `--provider` between a
draft and a final.

## Roles and verbs

| Verb | Role(s) | Request fields (beyond the common ones) | Outputs |
|---|---|---|---|
| `song` | `song` | `prompt` (style), `lyrics` (text) or `lyrics_file`, `duration_s`, `instrumental` (bool), `language` | one audio file (`audio/wav` preferred, `audio/mpeg` allowed) |
| `image` | `image.generate` | `prompt`, `negative`, `width`, `height`, `count`, `reference_images` (paths; character consistency) | `count` images (`image/png`) |
| `image --role image.edit` | `image.edit` | `prompt`, `image` (path), `mask` (path), `reference_images` | edited images (`image/png`) |
| `animate` | `animate` | `image` (path, the performer), `audio` (path, drives lip sync), `prompt`, `duration_s`, `width`, `height`, `fps` | one video (`video/mp4`) |
| `narrate` | `narrate` | `text`, `voice`, `language`, `reference_audio` (path; own or synthetic voice only) | one audio file (`audio/wav`) |
| `transcribe` | `transcribe` | `audio` (path), `language`, `format` (`txt` or `srt`) | one transcript file (`application/x-subrip` or `text/plain`) |

Common request fields: `"schema_version": "bashy-svc-request-v1"`, `seed`
(integer; a rod without seeding ignores it), and `title`. Paths in a request
are absolute or relative to the request file. Unknown fields are ignored by
v1 rods; a rod must refuse a different `schema_version` major. The request is
the file named on the command line (`bashy song req.json`); without one a rod
reads its operands (`bashy narrate "Hello"`).

`--role` picks a sibling role of the verb's service only (`image` →
`image.edit`); a role of another service is refused with exit 2.

## Handoff: door → rod

The door runs the rod as `bashy ROD [operands…]` (a vendor tool command as
`bashy tool cmd run TOOL:NAME …`) with:

| Env | Value |
|---|---|
| `BASHY_SVC_ROLE` | the selected role |
| `BASHY_SVC_ROUTE` | `oss`, `vendor`, `browser` or `api` |
| `BASHY_SVC_REQUEST` | absolute path of the request JSON, or empty |
| `BASHY_SVC_OUT` | absolute artifact directory (`--out DIR`, else a fresh temp dir that is removed if left empty) |
| `BASHY_SVC_YES` | `1` when the caller passed `--yes`, else `0` |

A rod writes every artifact into `BASHY_SVC_OUT`. It may print a rod result
on stdout:

```json
{"schema_version":"bashy-svc-rod-v1",
 "outputs":[{"path":"take.mp4","media_type":"video/mp4","note":"still image with a slow pan"}],
 "text":"one line for the caller"}
```

`path` may be relative to `BASHY_SVC_OUT`; `media_type` defaults from the
extension. Use `note` to label a degraded output. For example, an `animate`
draft without a lip-sync host (spec, Venue) must say it is a still image with a
slow pan. Without a rod result, the door reports every file the rod added to
`BASHY_SVC_OUT` and uses stdout as `text`. Exit 0 is success; any other exit
is `outcome: error`.

## Envelope: door → caller

```json
{"schema_version":"bashy-svc-v1","role":"song","provider":"song-acestep","route":"oss",
 "outcome":"success","outputs":[{"path":"/abs/out/song.wav","media_type":"audio/wav"}],
 "text":"","cost_class":"oss","elapsed":41}
```

`outcome` is `success`, `error`, `queued` (`--async`), `dry-run` (selection
only; no rod runs) or `unavailable`. Output paths are absolute.

## Effects and consent

Every rod declares its effects (atlas vocabulary). By route:

| Route | Declare | Notes |
|---|---|---|
| `oss` | `read,write,exec`, plus `net` if weights download on first use | weights only after a disk-headroom preflight, never at install |
| `browser` | `read,write,exec,net`; `spend` when a generation uses paid credits; `send` when it posts or publishes | claim `chrome-browser` first |
| `api` | `read,write,net,spend` | key from the vault; `send` if it publishes |

The selector does not choose a rod that declares `spend` or `send` unless the
caller passes `--yes`. The door then reports it as unavailable with the hint
"rerun with --yes", and no rod runs. A rod run directly (`bashy ROD`, not
through the door) must check `BASHY_SVC_YES=1` itself before it spends or
posts.

## Unavailable providers and install hints

A rod declares its dependencies in `requires:` (space-time grammar, e.g.
`os=darwin,linux has=uv`). When selection fails, the door lists every
candidate that was rejected, with its reason and one install line. It prints
this list on stderr and as `unavailable` in the envelope, then exits 126:

```json
"unavailable":[{"provider":"song-acestep","route":"oss","reason":"has=uv: absent","install":"brew install uv"}]
```

The install line comes from the rod. `ROD --doctor --json` must answer
without side effects, even when its `requires:` fails:

```json
{"schema_version":"bashy-svc-doctor-v1","available":false,
 "missing":[{"need":"has=uv","install":"brew install uv"}]}
```

Give the install line for the running OS, as `media doctor` does. Some
rejections do not come from a dependency, so the door writes those hints
itself:

| Rejection | Hint |
|---|---|
| a vendor CLI is absent | `bashy tool show TOOL` |
| a route mismatch | `--route` / `--provider` |
| no rod provides the role | how to register one |

## Registering a rod

```sh
bashy commands add song-acestep \
  --set exec.0=bashy --set exec.1=--bashsharp --set exec.2=/path/to/song-acestep.bsh \
  --set provides.0=song --set route=oss --set 'requires=has=uv' \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
bashy providers song            # candidates and why each is rejected
bashy song --dry-run req.json   # which rod would run
```

Shipped rods (M2 oss, M4 browser, M5 api) add their line to
`DEFAULT_COMMANDS` in `examples/defaults/bashy-defaults.bsh`, as the verbs
themselves do.

## Verification

`GOWORK=off go -C examples/service-door test ./...` runs the door under a
freshly built bashy and the real `bashy providers` selector. It uses a scratch
command ring and covers:

- the handoff and discovered outputs;
- rod-result outputs and notes;
- the install hint for a missing dependency;
- the no-provider hint;
- spend refused without `--yes`, `--dry-run`, and `--yes`;
- `--role`.

`go -C examples/defaults test ./...` checks that `song` and `animate` are
registered with their roles.
