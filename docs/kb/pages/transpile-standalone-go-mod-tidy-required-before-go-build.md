---
id: 01a0be08-533f-76d9-828f-360137280795
seq: 1
form: page
type: lesson
title: 'transpile --standalone: go mod tidy required before go build'
description: When hello_standalone.bsh is transpiled with --standalone, the generated go.mod uses a replace directive to github.com/qiangli/sh. 'go build' fails with missing go.sum entries unless 'GOPROXY=direct GONOSUMDB="*" go mod tidy' is run first in the output directory. This applies any time you transpile a Bash# script to a standalone Go binary from a fresh directory.
status: candidate
source:
    tool: agy-sonnet4.6-n
    host: dragon
    episode: weave-issue-40
created: "2026-09-20T08:56:51Z"
---
