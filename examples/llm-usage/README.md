# examples/llm-usage — share an LLM with metering

One skill-script alarm plus one viewer template over
`bashy llm usage --json`, and the [runbook](../docs/kb/pages/) for sharing
the model door with a peer. Requires the yoke A3/A4 verbs
(`bashy llm usage [--peer NAME] [--since 7d|24h|0] --json`,
`bashy llm peer limit NAME --tokens-day N --usd-month N --rpm N`;
over-limit answers are HTTP 429 with a reason and reset).

| File | What it is |
|---|---|
| `usage-alarm.bsh` | Skill-script: compares each principal against `thresholds.json` (tokens for unpriced principals such as ollama, USD where priced), calls `bashy notify` once on breach. Run by `bashy schedule` or by hand. |
| `usage-viewer.html` | The shaping hook: one dependency-free page rendering a `bashy-llm-usage-v1` document (paste, file picker, or a sibling `usage.json`). Edit it to shape the view. |
| `thresholds.example.json` | Copy to `thresholds.json` and set caps for one fetched window. |
| `fixtures/usage-over.json`, `fixtures/usage-ok.json` | Alarm unit fixtures (no live door). |

## Try it (no live door)

```sh
cp thresholds.example.json thresholds.json   # never commit real caps if they are sensitive
bashy --bashsharp usage-alarm.bsh --limits thresholds.json --usage-file fixtures/usage-ok.json --no-notify
bashy --bashsharp usage-alarm.bsh --limits thresholds.json --usage-file fixtures/usage-over.json --no-notify; echo "exit=$?"
bashy dag examples/llm-usage test            # the fixture unit checks
```

## Share and watch (live door)

```sh
bashy llm peer add FIELD_LAPTOP --share allow
bashy llm peer limit FIELD_LAPTOP --tokens-day 2000000 --usd-month 25 --rpm 60
bashy llm usage --since 24h --json | tee usage.json
bashy schedule add --every 1h --name llm-usage-alarm -- \
  bashy --bashsharp "$PWD/usage-alarm.bsh" --limits "$PWD/thresholds.json" --since 24h
```

Open `usage-viewer.html` in a browser next to `usage.json`.

Provider budgets (e.g. AWS Budgets) are host configuration, not product code:
keep them in the host setup, out of this example.
