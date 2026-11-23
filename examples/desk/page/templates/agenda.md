# Agenda

Purpose: _what the group should leave with._

Each item has a timebox, an owner and a visible outcome. Edit this page
with `page set PAGE -f agenda.md`; everyone's view updates live.

## Check-in (5 min)

```block here
```

```block timer:check-in duration=5m
```

## Items

| # | Item | Owner | Minutes | Outcome |
|---|------|-------|---------|---------|
| 1 | _item_ | _owner_ | 10 | _decision or artifact_ |
| 2 | _item_ | _owner_ | 15 | _decision or artifact_ |
| 3 | _item_ | _owner_ | 10 | _decision or artifact_ |

```block timer:item duration=10m
```

Reset and restart the item timer from the organizer view as each item
begins.

```block hands
```

Add material as it is needed — a poll for a quick decision, a shared note
or reading, an embedded tool:

```bash
page block add poll PAGE --options "Yes,No,Not yet" --title "Agree?"
page set PAGE -f agenda.md    # after adding note:ID, book:ID or embed:/PATH blocks
```

## Wrap-up (5 min)

```block timer:wrap-up duration=5m
```

Read back the outcomes and owners, then:

```bash
page close PAGE
page export PAGE --to md -o agenda-record.md
```
