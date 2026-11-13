---
id: eeaeb68a3260
kind: bug
title: 'media subs: English line rendered in the CJK font has uneven letter spacing (''Hol d the l ight'')'
seq: 3233
status: todo
priority: p2
labels:
    - media
    - subtitles
created: 2026-11-13T19:24:27.244295Z
sprint: 634
sprint_id: f57b15f7-34b1-5edb-80df-fa3588507b7c
sprint_title: 'Film pipeline follow-ups: local, recorded translation (and text) stages; subtitle font fix'
---

Seen in S627 draft04-en-zh (spot20.png): primary English line uses the secondary style's CJK font (Noto Sans SC) and kerns badly. Fix: primary .ass style uses a Latin font, CJK font only on the secondary line; keep the pinned/verified font-cache path. Verify: spot frame shows normal English spacing and correct Chinese glyphs; media gate green.
