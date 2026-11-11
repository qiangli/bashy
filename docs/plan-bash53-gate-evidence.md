# Bash 5.3 sealed gate evidence

The sealed script runs the existing `bash53suite -json` runner and saves its
single report. `tools/bash53evidence` validates the runner's verified mode,
infrastructure status, verdict inventory and totals, then records provenance
and the failing, timed out and skipped fixture names. The fixed `--user
1000:1000` container selection is recorded as testee UID 1000; the OCI image
ID is a content-addressed sha256 digest.

When a baseline is given, compare fixture names within the same mode and
inventory. Record NEW, RECOVERED and UNCHANGED failure sets. Fail on NEW names,
and require 86 passing with no failure, timeout or skip in the combined release
gate. Native `make test-bash` remains a local diagnostic.

Verification on this host: `go test ./tools/bash53evidence` and
`/bin/sh scripts/test-bash-container-mode.sh`. The sealed container gate must
run on the designated Linux venue, not this host.
