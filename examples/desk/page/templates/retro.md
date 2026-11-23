# Retrospective

Period: _the sprint, project or session under review._

Look back together, name what to carry forward, and leave with a short list
of changes someone owns.

## Who is here

```block here
```

## Warm-up (5 min)

```block timer:warm-up duration=5m
```

One word each on how the period went. Raise a hand to speak:

```block hands
```

## Gather (15 min)

```block timer:gather duration=15m
```

Collect cards for what went well, what did not, and what to try:

```bash
page block add board PAGE --session retro --title "Went well / Not well / Try"
```

## Choose (10 min)

```block timer:choose duration=10m
```

Vote on the cards to keep, or run a poll on the top candidates:

```bash
page block add poll PAGE --options "Try A,Try B,Try C" --votes 2 --title "What do we change?"
```

## Commit (5 min)

```block timer:commit duration=5m
```

Each change gets one owner and one check that shows it happened by the next
retro.

## Record

```bash
page close PAGE
page export PAGE --to md -o retro-record.md
```
