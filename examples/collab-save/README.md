# collab-save

`collab-save.bsh` composes file exports with local Git. Each `save` copies the
requested files into a repository, tracks configured media with Git LFS when
available, stages and commits them. `publish` is a separate, explicit push;
there is no automatic synchronization. Both verbs accept `--dry-run` and
`--json`.

The shaping hook is [`collab-save.json`](collab-save.json): edit `lfs_patterns`
for large media and `refuse_patterns` for paths that must never enter an
exported repository. The defaults cover video, audio, PNG and PDF files, and
refuse secret, token, credential, password, database and SQLite looking paths.
Never save credentials, private share tokens or application databases.

Run the example directly with Bashy:

```bash
bashy --bashsharp examples/collab-save/collab-save.bsh save report.md recording.wav --repo ./archive --message 'Save report and recording' --json
bashy --bashsharp examples/collab-save/collab-save.bsh publish --repo ./archive --remote origin --json
```

Examples of existing exports that feed it:

```bash
bashy notepad export NOTE_ID --to pdf -o report.pdf
bashy viewer export BOOK_ID -o recap.html
record video --out interview.mp4
bashy transcribe-pipeline --format srt --output captions.srt recording.wav
bashy dag examples/video sample  # output: examples/video/sample/build/demo.mp4
```

Pass those output paths to `save`; the source tools retain their own contracts
and remain usable on their own. `record` is the existing path-returning media
capture command, and `examples/video/video.sh build PROJECT` creates
`PROJECT/build/demo.mp4` with its build inputs in the project directory.
