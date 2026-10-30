# desk family gallery

Light + dark screenshots of every desk tool on the kit tokens and app shell,
side by side in `index.html` (styled by `kit.css` itself). Regenerate:

```bash
B=$(mktemp -d)
for t in notepad:desk/notepad form:desk/form/server chart:desk/chart \
         whiteboard:whiteboard/server brainstorm:brainstorm/server; do
  (cd "examples/${t#*:}" && GOWORK=off go build -o "$B/${t%%:*}" .)
done
PLAYWRIGHT_MODULE=/path/to/playwright/index.mjs DESK_BIN=$B \
  node examples/desk/gallery/capture.mjs
```

The script fails on any page error. See `../kit/README.md` §One look.
