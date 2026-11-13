# demo/series-index — the shaping demo wish

The base journal links posts that share a `series:` front-matter name, but
the home page has no series overview. This directory is a hand-made shaping
candidate that adds one: a **Series** section above the post list, each
series with its posts in reading order (by `part`, then date).

The whole change is `series.go`: a single extra file dropped into the
candidate's `generator/` directory. The base generator exposes the seam
(`seriesIndexHook` in `generator/main.go`) precisely so a shaped copy can
add this without editing any base file — the diff of the wish is one new
file plus the rebuilt `public/`.

The candidate deliberately keeps the set of published routes identical
(the section lives in the existing `public/index.html`): adding or removing
routes needs an approved contract step, and undo must stay one command.

Run the full lifecycle against a registered `blog` app:

```bash
bashy blog shape-demo            # wish -> candidate -> verify -> preview ->
                                  # approve -> isolation proof -> undo
```

Or drive the verbs yourself:

```bash
work=$(mktemp -d)
cp -R bashy/examples/blog/sample/. "$work/"
cp bashy/examples/blog/demo/series-index/series.go "$work/generator/"
(cd "$work" && go run ./generator/*.go . public)
grep -q 'series-list' "$work/public/index.html"

bashy shape wish blog "Add a series index section to the home page" \
  --from "$work" --user device:demo
bashy shape verify blog <ID> --user device:demo
bashy shape preview blog <ID> --user device:demo
bashy shape approve blog <ID> --user device:demo
bashy src history blog --user device:demo --json      # active = candidate
bashy src history blog --user device:other --json     # still the base
bashy shape undo blog --user device:demo
```

The declared checks in `sample/checks/smoke.bsh` are frozen from the base
commit, so the candidate is judged by the base's rules: it must still build
a complete journal with all its rich content.

Note: a candidate that ADDS or REMOVES files under `public/` changes the
declared route surface; the shaper's compatibility gate then refuses undo
(rolling back would drop routes, which needs an approved contract step —
see the filed todo). Content changes inside existing pages shape, verify,
approve and undo cleanly, which is what this demo wish does.
