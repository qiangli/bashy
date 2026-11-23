# examples/shaping/discover — the Sprint 693 discovery matrix runner

One command produces the live discovery evidence for a people-and-agents
site (plan: `docs/sprint-693-master-execution-plan.md`): every client is
handed **only** the page URL (`ORIGIN/about`) and the matrix records what
it actually sent and received — never what it was assumed to do.

```bash
# 1. Local fixture first (real facts before the live release):
bashy --bashsharp discover.bsh serve --log /tmp/req.log --port 18931 &
bashy --bashsharp discover.bsh matrix http://127.0.0.1:18931 --log /tmp/req.log

# 2. The live run after S1 (the S2 evidence command):
bashy --bashsharp discover.bsh matrix https://ai.dhnt.io
#    → paste the Markdown table into docs/evidence/sprint-693-dhnt-io-navigation.md
bashy --bashsharp discover.bsh matrix https://ai.dhnt.io --json
```

## Verbs

- `matrix ORIGIN [--clients LIST] [--json|--md] [--log FILE]` — clients:
  `curl` (default `*/*`), `curl-cc` (Claude Code's
  `Accept: text/markdown, text/html, */*`), `bashy-fetch`,
  `bashy-fetch-md`, `browser` (`bashy browser --mode solo`: rendered text
  plus links), and the agent CLIs `claude`, `codex`, `gemini`, `agy`,
  `opencode` — each asked non-interactively to read the URL with its own
  fetch tool and report the content-type, section ids and links it saw.
  An absent CLI is a `skipped` row, never a failure. With `--log`, each
  client's request line (method, Accept, User-Agent), any `/llms.txt`
  fetch, and any followed `rel=alternate` twin are read from the server's
  own request log; without a log those cells read `not-observed`.
- `serve [DIR] --log FILE [--port PORT] [--proxy ORIGIN]` — a local
  request-logging server (stdlib python3). Without `--proxy` it serves
  DIR, or the bundled `testdata/` fixture when DIR is omitted: HTML with
  `<section id>`, `PATH.md`, `PATH.json`, `Accept` negotiation,
  `Vary: Accept`, `ETag` = the `REVISION` file, `Link` rel=alternate,
  `robots.txt` and `llms.txt`. With `--proxy ORIGIN` it reverse-proxies a
  site server so a real site's clients can be observed the same way.

Per client the matrix records: status, the request sent (method, Accept,
User-Agent when observable), the representation received (content-type,
ETag, Vary), the section ids and links extracted, discovery
(`self` = structured format received directly, `self-md` = local `--md`
conversion, `twin` = HTML advertising the direct `.md`/`.json` twin,
`skill` = neither), and whether `/llms.txt` was fetched or a
`rel=alternate` twin followed — from the log, never assumed.

The embedded skill (the `~~~skill` block in `discover.bsh`) doubles as
the **dhnt-site reading skill**: negotiate with
`Accept: text/markdown`, else follow the advertised `.md` twin, cite
`page#section @ revision` from the `.json` twin.

## Tests

`GOWORK=off go test ./...` runs the script tests: they start the fixture
server on a free port, run the matrix against it (including a fake agent
CLI whose request headers must appear from the log), and check the
Markdown and JSON shapes. Browser and real agent legs stay manual and
small — a few headless navigations, no gate suites.
