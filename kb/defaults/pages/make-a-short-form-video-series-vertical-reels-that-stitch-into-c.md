---
id: 01a21e4e-a0c2-7d71-b73e-84402f98a885
seq: 42
form: page
type: runbook
title: 'Make a short-form video series: vertical reels that stitch into chapters'
description: 'Plan and produce a serial short-form video series: 60–90 s vertical reels (Shorts, Reels, TikTok) that each stand alone and stitch into long-form chapters; constants vs per-reel form (genre or format); the form picks the route (sung → film, told/silent → media); vertical framing for muted phone viewers; board-animatic → draft → final — WHEN an owner wants a recurring video series, shorts, reels, episodes, a channel, or a demo told as a story (keywords: series, shorts, reels, tiktok, vertical, episode, chapter, season, genre, format, musical, storyboard, film, media)'
tags:
    - media
    - film
    - video
    - series
    - shorts
    - vertical
status: candidate
evidence: '2026-11-27: drafted from the film, media, narrated-video and mock-up runbooks plus a vertical board animatic (media cards at 1080x1920, four 81 s reels stitched into a 5:25 chapter on a blurred 1920x1080 backdrop). It surfaced the vertical letterbox gap (todo 854fdeb5). A sung vertical film draft is not yet run: still a candidate.'
source:
    tool: claude
created: "2026-11-27T18:39:59Z"
---

A short-form series is many small videos with one storyline: vertical **reels** of 60–90 s for
Shorts, Reels and TikTok, which stitch into longer **chapters** for a channel's long-form shelf. Build
it so every reel stands alone and the series still reads as one thing.

## Units

| Unit | Length and frame | Rule |
|---|---|---|
| Reel | 60–90 s, vertical 1080x1920 | one beat; a hook in the first second; one gag or turn; a button (a cliffhanger); an end card. It must make sense to a viewer who starts here. |
| Chapter | about 4 reels, 5–6 min | the reels joined in order, each vertical frame centred on a 1920x1080 blurred backdrop; the reels' own cards act as chapter markers |
| Season | about 6 chapters | a playlist |

Continuity devices that cost nothing:
- a series bug and reel number on every title card;
- a one-line "Previously…" recap;
- a recurring melody (see below).

## Constants and variables

Hold a few things constant: the protagonist, the world's written rules, one recurring melody, a
narrator's sign-off, and the visual style. Vary the **form** per reel, so a long series stays fresh:

- **Genres**: musical number, country, blues, noir, Western, horror, documentary, game show,
  trailer, slapstick.
- **Phone-native formats**: screen POV, notification storm, vlog to camera, fake tutorial, poll
  ending.
- **bashy-native**: a screen or terminal recording as the story (`record`, `cast`).

Use genre *conventions* only. Never imitate a specific artist, song, show, host, interface or
brand, and keep every UI and logo fictional. Public-domain compositions may carry new lyrics; record
the composition and its PD basis, and never copy a modern arrangement or recording.

## The form picks the route

| Audio mode | Use it for | Route |
|---|---|---|
| **Sung**: the whole beat is a song | story milestones, musical numbers | `film`: storyboard → `film draft` → review → `film lock` → `film final` (operator's go). One reel is one `film` project; `film` is built for 60–90 s songs. |
| **Told**: a narrator or characters speak over a music bed | most jokes; exact words matter and are easy to change | `media` narrated video (runbook *make a narrated video*): `media voice`, stills, `cards`, `subs`, `cut` |
| **Silent**: music, sound effects and captions | visual gags, muted phone viewers | `media`: stills or cards cut to a bed |

Make the recurring melody, its short stings, the narrator voice and a small sound-effects set
**once per season** and reuse them.

## Vertical framing

- Set `size: 1080x1920` and a small portrait `draft.still_size` (for example 432x768) in the
  storyboard.
- Frame the small protagonist low, with the big world above it.
- Keep captions to two lines, clear of the platform UI: the top ~15% and the bottom ~25% of the
  frame hold buttons and titles.
- Many viewers watch muted, so every shot's prompt must describe a visible *action*, not just a
  mood. Burned-in captions are part of the design.
- Known gap: `media cards` letterboxes vertical sizes (bashy todo 854fdeb5). Until it is fixed, title
  and end cards render as a 16:9 band.

## Draft ladder (see *mock-up a video before final production*)

1. **Board animatic**, the same day:
   - text cards per shot (shot number, picture, line) on a click track at the song's bpm;
   - with `media cards`, it costs nothing and runs anywhere light;
   - review words, order and pacing here, because a sung reel bakes its lyrics into the song.
2. **Draft**:
   - the generated song or narration, plus one small still per shot (`film draft` or `media`), on
     the gate host only;
   - iterate as often as wanted on oss routes.
3. **Final**: re-render only the locked shots, once, on the operator's explicit go.

## Every reel ships with

- an AI-generated disclosure, in the end card and in the platform's synthetic-content setting;
- the reel's storyboard and script in the project repo;
- the render's manifest (stages, routes, cost).
