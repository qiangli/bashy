---
id: a93e0023c1ee
kind: task
title: reconsider sprint inbox-ack now that the fuse is gone
seq: 19
status: todo
priority: p2
created: 2026-09-02T18:51:19.227634Z
---

Raised while simplifying the coordination surface on dhnt sprint 99. Not a defect — a question worth answering deliberately rather than leaving the verb to drift.

WHAT CHANGED. internal/agentos/sprint_watch.go used to end the attached watch after three unacknowledged reminders. It no longer does: the guard that prevents message loss is that a cursor never advances until the manager proves it read, and that holds whether or not the watch is alive, so exiting protected nothing while destroying the seat's delivery path over one unread message. Separately, `bashy inbox --as X` now refreshes the lease of every sprint X owns, so reading your mail is what keeps a seat live.

THE QUESTION. With no fuse and with reading-as-heartbeat, what is `sprint inbox-ack` still for? It remains the only EXPLICIT proof-of-consumption in the system, which is a real thing to have — an ack is evidence a batch was handled, where a cursor only shows it was rendered. But nothing now depends on it, and a verb nothing depends on is one agents stop running.

OPTIONS, in the order they should be considered:
 1. Keep it and give it a consumer — the honest delivery ladder needs a `handled` state that is not merely `turn-ended`, and this is the only signal that could carry one. See dhnt story 70211f1503a2.
 2. Fold it into the inbox read: reading IS handling for most traffic, and a separate command is ceremony.
 3. Retire it.

Do not simply delete it before deciding 1: dropping the only explicit handled signal would make the delivery ladder unimplementable, and that ladder is the point of the surrounding work.
