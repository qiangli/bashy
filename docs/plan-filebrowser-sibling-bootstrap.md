# Plan: pin the Filebrowser sibling

Status: implemented and gated.

## Objective

Make a standalone Bashy checkout reproducible after the direct
`../filebrowser` module replacement was added. Every direct flat sibling must
be cloned at an exact, origin-reachable commit before any build begins.

## Changes and gates

1. Record the full Filebrowser commit in `.sibling-pins` and teach both
   bootstrap paths its canonical repository URL.
2. Keep clean-container and reusable-OCI staging aligned with the complete
   five-repository build input set.
3. Validate pin syntax and coverage against every direct flat `go.mod`
   replacement.
4. Reproduce bootstrap and `make build-bashy` from a sibling-less temporary
   checkout using only the committed pins.
