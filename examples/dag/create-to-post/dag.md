---
name: create-to-post
description: Offline Wave 2 fixture for explicit image transfer and feed publication
type: dag
default: transfer
vars:
  BASHY_CLI ?= bashy
  SOURCE ?= blender-output.png
  STORAGE_ROOT ?= .create-to-post/storage
  SINK_ROOT ?= .create-to-post/feed.git
  EXPECTED_SHA256 ?= 91ea15f875c3130683837d4a6538d1fbbcc3be69e22580531addbc06acb898e5
---

# Create to post: fixture only

`blender-output.png` is a pre-created one-pixel PNG standing in for a Blender
render. It was not produced by a live Blender invocation. The fixture uses a
local storage directory and a local bare Git feed sink configured separately
as `fixture` with `bashy feed sink add github fixture "$SINK_ROOT"`.
It uses no account, credential, network destination, LLM, or resident process.

Run `bashy dag -f dag.md -n verify` to preview the ordered steps. A preview
does not transfer or publish. The default target `transfer` only copies the
artifact. Publication requires the explicit `verify` (or `publish`) target:
`bashy dag -f dag.md -B verify`. Set `SOURCE` to the absolute fixture path and
`STORAGE_ROOT` and `SINK_ROOT` to private local directories. The test supplies
an offline rclone command stand-in for the external binary; a live rclone or
Blender operation and a real feed destination remain untested.

## Tasks

### source

Check the exact pre-created bytes before any write.
Effects: read

```bash
set -euo pipefail
test -f "$SOURCE"
sum=$("$BASHY_CLI" sha256sum "$SOURCE")
test "${sum%% *}" = "$EXPECTED_SHA256"
test -d "$STORAGE_ROOT"
test -d "$SINK_ROOT"
```

### transfer

Copy through the existing `bashy rclone copyto` surface to fixture-backed
local storage and check the stored bytes before publication.
Requires: source
Effects: read, write, exec

```bash
set -euo pipefail
"$BASHY_CLI" rclone copyto "$SOURCE" "$STORAGE_ROOT/blender-output.png"
sum=$("$BASHY_CLI" sha256sum "$STORAGE_ROOT/blender-output.png")
test "${sum%% *}" = "$EXPECTED_SHA256"
```

### publish

Explicitly publish to the one local fixture sink with a fixed excerpt.
`--no-index` prevents registry traffic. The body records the fixture provenance;
the signed attachment records its checksum.
Requires: transfer
Effects: read, write, exec

```bash
set -euo pipefail
test "$("$BASHY_CLI" feed sink list)" = "$(printf 'fixture\tgithub\t%s' "$SINK_ROOT")"
sum=$("$BASHY_CLI" sha256sum "$STORAGE_ROOT/blender-output.png")
test "${sum%% *}" = "$EXPECTED_SHA256"
printf 'Fixture provenance: pre-created Blender output stand-in; sha256 %s.\n' "$EXPECTED_SHA256" > "$STORAGE_ROOT/post-body.md"
"$BASHY_CLI" feed post "$STORAGE_ROOT/post-body.md" \
  --headline "Blender fixture image" \
  --dek "A pre-created image fixture transferred to local storage" \
  --attach "$STORAGE_ROOT/blender-output.png" \
  --path posts/fixture.md --no-index
```

### verify

Read the local sink and check that its published attachment is byte-identical.
Requires: publish
Effects: read, exec

```bash
set -euo pipefail
test "$(git --git-dir="$SINK_ROOT" show main:posts/fixture.md | head -1)" = '---'
sum=$(git --git-dir="$SINK_ROOT" show main:posts/fixture/blender-output.png | "$BASHY_CLI" sha256sum)
test "${sum%% *}" = "$EXPECTED_SHA256"
```
