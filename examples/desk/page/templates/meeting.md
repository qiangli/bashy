# Team meeting

Goal: _one sentence on what this meeting decides._

Explore the goal, decide together, and record commitments with visible
outcomes. Every phase ends with something written on this page.

## Who is here

```block here
```

```block hands
```

Raise your hand to speak; the facilitator takes the next hand off the queue.

## Explore (15 min)

```block timer:explore duration=15m
```

Ask focused questions and make room for each perspective. Collect ideas on a
board when the room is big:

```bash
page block add board PAGE --session ideas --title "Open questions"
```

## Decide (20 min)

```block timer:decide duration=20m
```

Put each open choice to a vote:

```bash
page block add poll PAGE --options "Option A,Option B,Defer" --title "Which way?"
page block close PAGE poll-1
page block show-results PAGE poll-1
```

## Commit (10 min)

```block timer:commit duration=10m
```

Record each commitment as an owner, a next step and the evidence that shows
it is done. A form collects them from everyone at once:

```bash
page block add form PAGE --def commitments.json --title "My next step"
```

## Record

Close the page and export the record with every block's final state:

```bash
page close PAGE
page export PAGE --to md -o meeting-record.md
```
