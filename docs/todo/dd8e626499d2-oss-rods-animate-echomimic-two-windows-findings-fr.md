---
id: dd8e626499d2
kind: bug
title: 'oss-rods/animate-echomimic: two Windows findings from the Sprint 635 full-length run'
seq: 3238
status: todo
priority: p2
labels:
    - windows
created: 2026-11-14T17:34:33.634218Z
---

1) infer_flash.py ends with os.remove(<image>_tmp.mp4) after moviepy muxes the audio; on Windows the handle is still open and the script exits 1 with PermissionError WinError 32 although <image>_output.mp4 is complete. The rod's patch-on-copy should wrap that remove (ignore PermissionError), or the rod must not treat exit 1 with a complete output as failure. 2) video_length_actual = min(audio_s*25, video_length) rounded DOWN to 4n+1, so a take can be up to 0.16 s shorter than its shot (shot 09: 1.304 s audio -> 29 frames = 1.16 s); film's media cut must hold the last frame, or the rod should pad the audio slice by 4 frames before calling the model. Evidence: umbrella .agents/s635-review/takes-local/09.txt, host C:\s635\out\takes\09\gen.log.
