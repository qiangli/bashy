---
id: 01a1d53d-ca01-7914-8d97-f38a497b5c27
seq: 28
form: page
type: runbook
title: 'Mock up a video before final production: animatic plus one proof slice'
description: 'How to make a cheap, fast video draft (a mockup or animatic) that lets the owner approve the plan before paid or slow final production: full-length draft soundtrack, one small still per distinct shot with beat-locked cuts, subtitles and shot numbers, still reuse for repeats, a look sheet, and ONE short proof slice at target fidelity; locked storyboard re-rendered by the final — WHEN planning or making a music video, demo, pitch or any generated video, or asked for a draft, preview, mockup, animatic or concept (keywords: film, draft, mockup, mock-up, animatic, storyboard, proof slice, contact sheet, shot list, song, image, media, oss, final, commercial, budget)'
tags:
    - media
    - film
    - video
    - draft
    - mockup
    - storyboard
status: candidate
evidence: '2026-11-13: operator-approved strategy for Sprint 627 M8 (music-video showcase); stage times measured on a Windows APU host (song 75 s in ~240 s, FLUX.2 klein 512px ~269 s first image / ~158 s edit, decoder-shaped 1280x720 convs ~8 s after the driver/ROCm fix); budgets are estimates derived from those'
source:
    tool: claude
created: "2026-11-13T14:09:18Z"
---

A video draft is a **mockup**, like the page mockups built before a real
website. It exists so the owner can approve or change the PLAN (story,
shots, timing, words, characters) before money and hours go into final
production. Optimise it for decisions, not looks.

## The rule

**Draft cost scales with the number of distinct shots, not with the video's
duration, plus ONE fixed-size proof slice.** Render the whole length
cheaply, and render only a few seconds expensively.

## Fidelity ladder (free, local routes)

| Layer | Covers | Fidelity | Answers |
|---|---|---|---|
| Soundtrack | full length | draft quality, one take (`bashy song --route oss`) | Is this the song? Everything is timed to it. |
| Animatic | full length | one small still per shot (about 512 wide, few diffusion steps; `bashy image --route oss`), slow pan or zoom, cuts on the beat, lyric or narration subtitles, a burned-in **shot number** on every shot (`bashy media cards/subs/cut`) | Pacing, story, coverage, words. |
| Reuse | full length | repeated sections (chorus, refrain, recurring set-ups) reuse the same stills | A 5-minute song is about 25-35 distinct shots, not 60+. |
| Look sheet | one frame per character + each set | medium-resolution stills | Consistency and style. |
| Proof slice | ONE shot, about 5 s | the best local fidelity available (full-resolution still with motion, or real local animation once it works) | Can we hit the target look? |

Optional: make the proof slice one **commercial** clip (a few seconds, about
USD 1). It spends money, so it needs the owner's explicit `--yes`, and
`--dry-run` shows the cost class first.

## Lock the storyboard, then produce

The draft's storyboard is the contract for the final: shot number, start and
end time, lyric or line, prompt, seed, and character references. Feedback
names shots ("change shot 7"), and the storyboard changes first. The final
(`film --final`, on a commercial or high-fidelity route) re-renders exactly
those shots from the locked storyboard. Nothing the owner already approved
is regenerated.

## Deliverables of a draft

1. The animatic MP4.
2. A contact sheet: every shot as a thumbnail with its number.
3. The storyboard table: shot#, time, lyric, prompt, seed.
4. Wall time per stage and the total spend (zero for an oss draft).

## Rough budgets (one consumer APU or GPU host, draft settings)

These are estimates from measured stage times; use them to plan, and record
your own.

- 1-minute video: about 15-20 min of compute.
- 5-minute video: about 45-55 min (song about 10, about 30 small stills
  10-15, look sheet about 5, proof slice 5-10, assembly and QA about 10).
- Rendering every shot at full fidelity instead costs hours, and real local
  lip-synced performance video can cost 10-30 min per 5 s clip on an
  integrated GPU. That is exactly what the mockup avoids.

## Practical notes

- Do one full-resolution safety decode on a new GPU or driver stack before a
  batch. Run one GPU job at a time; after a GPU timeout or driver reset
  (Windows Event 141), reboot before retrying.
- Generate the soundtrack first. Cut timing, subtitles and shot lengths all
  derive from it (transcribe or align the lyrics for timing).
- Prefer native Windows over WSL on Windows hosts
  ([[kb:windows-native-first-wsl-only-with-a-recorded-reason]]).
- Original or fictional characters plus an "AI-generated" disclaimer card
  for anything published.

Related: [[kb:make-a-narrated-video]],
[[kb:work-with-cloud-providers-safely-do-aws-gcloud-azure]].
