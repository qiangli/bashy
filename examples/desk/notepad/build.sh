#!/bin/sh
# Build the notepad reference: compile the Go server/CLI to ./notepad-bin,
# which the ./notepad launcher runs. Needs Go 1.27.4 on the PATH, writes
# only inside this folder, needs no network beyond the module cache, and
# can be run again safely.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$here"
GOWORK=off CGO_ENABLED=0 go build -trimpath -o notepad-bin .
echo "built: $here/notepad-bin"
