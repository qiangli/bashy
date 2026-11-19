---
id: 317e0d7e-123e-4469-9c4e-508ce29e8e54
seq: 29
form: page
type: runbook
title: 'Publish a video to YouTube and link it from an app'
description: Upload a video to the user's own YouTube channel (unlisted by default) with the Google example command, then link or embed it from a shaped app, a site or a doc — WHEN an agent publishes a demo, tutorial or recording for a bashy user (keywords youtube, upload, unlisted, embed, tutorial, demo video, youtube-nocookie, youtube.upload scope).
tags:
    - youtube
    - google
    - video
    - publish
status: candidate
evidence: 'Live run: API enabled with gcloud, YouTube-only consent in a separate slot (the combined request was refused: drive.file and youtube.upload cannot be requested together; the first consent silently omitted the unticked YouTube checkbox), unlisted upload returned watch and embed URLs, linked from a shaped practice app. Provider UI may change.'
created: "2026-11-19T04:33:22Z"
updated: "2026-11-19T04:33:22Z"
---

Any agentic tool with shell access can publish a video for a bashy user to **that user's own** YouTube channel. The account is always the user's (`account=ADDR`, e.g. `alice@example.com`); never reuse another person's credentials or channel.

1. **Set up the Google example command once** (see the Gmail runbook): `bashy src fetch bashy`, run `examples/google/register.sh`, then `bashy google auth-status account=ADDR`. If it is not ready, run the BYOP onboarding it prints (`auth-guide`, `auth-import`).
2. **Plan first:** `bashy google youtube-upload VIDEO title='…' privacy=unlisted --dry-run account=ADDR`. It prints the metadata and the two one-time owner steps under `needs`.
3. **One-time owner steps** (only the user can do these; hand them over, do not work around them):
   - grant the upload permission in its **own credential slot**: `bashy google auth-login account=ADDR slot=youtube scopes=https://www.googleapis.com/auth/youtube.upload`. Google will not grant YouTube together with Drive, so it is never added to the main grant. The consent page shows "Manage your YouTube videos" as an **unticked checkbox** (granular consent): the user must tick it; otherwise auth-login stops with `SCOPE_NOT_GRANTED` and says so;
   - enable **YouTube Data API v3** in the OAuth client's Cloud project (`gcloud services enable youtube.googleapis.com --project PROJECT`, or the Cloud Console).
   If either is missing, the upload fails with `YOUTUBE_REFUSED` and `reason` `insufficient_scope` or `api_disabled`, plus the exact fix.
4. **Upload:** `bashy google youtube-upload VIDEO title='…' description='…' privacy=unlisted tags=a,b account=ADDR`. It returns `url` (https://youtu.be/ID) and `embed` (https://www.youtube-nocookie.com/embed/ID). Default to **unlisted** unless the user asks for public.
5. **Link it from the app:** prefer a lazy embed plus a plain link, so the page loads nothing until someone presses play:

```html
<iframe src="EMBED_URL" title="TITLE" loading="lazy" allow="encrypted-media; picture-in-picture; fullscreen" allowfullscreen></iframe>
<a href="WATCH_URL">Watch on YouTube</a>
```

For a shaped app, change the page by wish (`bashy shape wish APP "add the tutorial video WATCH_URL"`) or commit it to the app's source; a shaped app serves its committed revision, not loose files.

Never print or paste OAuth tokens, and never put the video URL of one user into another user's public repo or example; examples stay generic, user content stays in the user's own repos and apps.
