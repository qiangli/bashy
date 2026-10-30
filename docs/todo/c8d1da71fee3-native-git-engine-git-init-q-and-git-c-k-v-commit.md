---
id: c8d1da71fee3
kind: task
title: 'native git engine: ''git init -q'' and ''git -c k=v commit'' unsupported (hosts without host git, e.g. elephant WSL)'
seq: 2836
status: wontfix
priority: p1
created: 2026-10-29T18:11:04.275975Z
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T22:29:35.207548Z
---

Found in the D3 live run 10-29 on elephant WSL, where host git is absent: 'bashy git init -q .' -> unknown shorthand flag 'q'; 'bashy git -c user.email=.. -c user.name=.. commit -qm init' -> '-c is not applied by bashy native git engine'; 'git log --oneline' -> not served by the native engine. Agents (genie) then cannot commit in a fresh repo. Same-on-every-OS, zero setup: support -q on init/commit, -c config overrides, log --oneline in the native engine. Red/green tests; focused go test.
