# Subtitle fonts

`media subs` obtains the Noto Sans SC variable font from a pinned revision of
the [Google Fonts repository](https://github.com/google/fonts/tree/main/ofl/notosanssc)
on first use and verifies SHA-256
`a3041811a78c361b1de50f953c805e0244951c21c5bd412f7232ef0d899af0da`.
It caches the font under `MEDIA_CACHE` (or the platform cache directory).
Set `MEDIA_FONT_DIR` to an existing copy for offline rendering. The license
is the SIL Open Font License in `NotoSansSC-OFL.txt`.

`media subs` uses this face for burned bilingual captions and declares it for
the secondary soft subtitle style so Simplified Chinese glyphs are available
on each supported OS. `MEDIA_OFFLINE=1` fails with the prefetch command if
the font is missing.
