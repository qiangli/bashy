---
id: 854fdeb5258b
kind: bug
title: 'media cards: vertical sizes are letterboxed (fixed 60x14 landscape grid)'
seq: 3329
status: todo
priority: p2
labels:
    - media
    - film
reporter: claude-opus5.5
created: 2026-11-27T18:27:40.333696Z
---

media cards (examples/media/media.bsh card_gif) always rasterises the card text on a fixed 60x14 terminal grid (cols=60 rows=14, agg font-size 48), then cmd_cards scales with force_original_aspect_ratio=decrease and pads to --size. With a vertical size such as 1080x1920 every card becomes a 16:9 strip centred in black bars. film inherits it: a storyboard with size 1080x1920 gets letterboxed title/end cards. Repro: printf 'Title\nsecond line\n' > c.txt; media cards c.txt -o c.mp4 --size 1080x1920; grab a frame -> text only in a 1080x608 band. Fix: derive cols/rows from the target aspect (e.g. keep ~60 cols for landscape, ~30 cols x ~28 rows for 9:16), wrap long lines to cols, and cover it with a media test at 1080x1920. Found while drafting vertical short-form reels; not urgent during the outpost/bashy/cloudbox refactor.

Also for vertical output: `media subs` burns captions with a fixed bottom margin (MarginV 50), which sits under the Shorts, Reels and TikTok buttons on 1080x1920. Short-form needs captions in the safe area, roughly above the bottom 25%. Make the margin (or a safe-area preset) configurable.
