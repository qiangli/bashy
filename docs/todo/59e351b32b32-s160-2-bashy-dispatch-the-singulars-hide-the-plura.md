---
id: 59e351b32b32
kind: task
title: 'S160.2 bashy: dispatch the singulars, hide the plurals, fold agent whoami, issue->todo, messages hidden, e2e covers hidden verbs'
seq: 259
status: assigned
priority: p0
created: 2026-09-12T21:20:51.676962Z
assignee: mortise
sprint: 160
---

agentos.go: delete case agent (agentcmd) and mount agentcmd.NewWhoamiCmd under the roster (agents.go newAgentsRosterCmd) so bashy agent whoami works and bashy agent list is the registry; fleet arm + runFleet accept agent/model/tool/person and the plurals; apps/secrets/skills/todo arms accept the singular (+issue); cmd.Use = os.Args[1] on alias spellings (bootstrap/upgrade precedent). alwaysShimVerbs: plural -> singular. hiddenFrontDoorVerbs += agents models tools skills secrets apps people messages issue; messages removed from directFrontDoorVerbs. verbSynopsis: singular gets the synopsis, plural = hidden alias line. Tests: commands_test.go want-list, atlas_test.go messages assertion, localfirst theLoop skills->skill, commands_e2e_test.go native set + iterate hidden_verbs from commands --json --all. inspect.go paths: store name stays plural, ReadBy becomes singular. meta.go: resolve AliasOf so bashy apps meta still answers. Gate: go test ./internal/agentos/...; go test -tags e2e -run TestE2EAllListedCommandsDispatch ./internal/agentos; CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/bashy; installed-binary smoke (make install).
