---
name: lens
description: Representation filter — convert data (json↔yaml, csv↔json, html→md, pdf/docx→md), list archives/containers as pipe filter, and render command lines across bash/bsh/cmd/pwsh/json/yaml/mcp/skill.
---

# lens

`lens` is the representation filter (iconv for formats). One job: change how bytes are represented. It never runs a command, only converts.

```
lens [-f FROM] -t TO [FILE]           # read stdin or FILE, write stdout
lens -l                               # list declared rows
lens -f json -t yaml < cfg.json       # json↔yaml, toml, jsonl
lens -t md report.pdf                 # pdf/docx/xlsx/html → markdown (recipe + Go fallback)
lens -t listing archive.zip           # uniform listing {path,type,size,mode,mtime} for tar/zip/pax/7z/git tree
lens -f bash -t pwsh --switch=pascal --drive=C   # render command lines across dialects
lens -f cmd:ps -t json < ps.txt       # classic text → JSON via declared cmd: rows
```

Trigger words: `lens -t md`, `lens -f json -t yaml`, `lens -t listing`, `lens -f bash -t pwsh`, `lens -t md git:REV:PATH`. Paired with common options: `--json --fields`, `--null` via `adapt`.

See `bashy help lens`, `bashy help common-options`, and `bashy adapt --help` for composition.
