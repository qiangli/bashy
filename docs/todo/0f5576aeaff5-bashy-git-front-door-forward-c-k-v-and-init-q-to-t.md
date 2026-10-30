---
id: 0f5576aeaff5
kind: task
title: 'bashy git front door: forward -c k=v and ''init -q'' to the native engine (yoke f8cacdb3 supports them)'
seq: 2842
status: done
priority: p1
created: 2026-10-29T23:26:34.193483Z
assignee: claude-sonnet5.5
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T23:55:14.676647Z
closed_by: s531-manager
---

Live check 10-29 23:20Z on elephant native Windows, candidate bashy 768fc01d (yoke 35d5cb56 incl. f8cacdb3 from 557d8c19). 'bashy git init -q .' -> 'unknown shorthand flag: q in -q' (bashy's typed cobra 'git init'); 'bashy git -c user.name=d3 -c user.email=d3@local commit -q -m first' -> 'git -c is not applied by bashy native git engine' (bashy/internal/agentos/git_globals.go splitGitGlobals: -c is in gitValueGlobals and refused before the engine). 'git log --oneline' then failed only because init had failed. Fix: splitGitGlobals forwards '-c k=v' (and '-ck=v') to the engine via git.WithConfigOverrides / the Exec argv (keep refusing --git-dir/--work-tree etc. until supported); the typed 'git init' accepts -q/--quiet (or routes to Exec). Red/green unit tests (no re-exec of the binary); focused go test ./internal/agentos only; GOOS=windows go vet.
