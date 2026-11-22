---
id: 92b00a7d4e7e
kind: feature
title: Stream screen and tab audio over Outpost
seq: 3293
status: todo
priority: p1
labels:
    - demo-queue
    - deferred-hardware
created: 2026-11-21T23:44:29.494536Z
---

Build the standalone Bashy example at examples/demo-queue using Brainstorm's Bash# entrypoint pattern, a small Go HTTP server, and embedded browser assets. Provide host and join commands, one private invite per roster entry, and temporary outpost mesh expose/listen forwards that are cleaned up on orderly shutdown. Keep organizer controls and projector media on a separate loopback listener. The host prints control and fullscreen projector URLs. The join command prompts for an invite and prints a local presenter URL. Reusing an invite restores the roster entry.

Support current desktop Google Chrome and Microsoft Edge. The browser picker offers tab, window, or screen capture and a private local preview. Tab audio supports web videos and local media opened in a browser tab. Add an Enable sound control for the projector. Stream native WebM with VP8 video and Opus tab audio, targeting up to 1080p at 30 fps. Use short, ordered chunks, bounded in-memory buffers, and Media Source Extensions for playback. Provide a current-turn media boundary so queued presenters cannot publish captured media. Keep media in memory only, use the repository's BSD-3-Clause license, and add no third-party libraries.

Acceptance
- Demonstrate view-only tab video with synchronized tab audio on a direct LAN mesh link using both desktop browsers.
- Verify window and screen capture, invite rejection, the local-only projector/control boundary, and cleanup of temporarily exposed services and forwards.
