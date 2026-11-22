# Sprint 692 / Story 3303: DAG capacity adapter parity

Root cause: runDagDispatch mounted sprintCapacityServices(), while the adapter
passed an empty dag.CapacityServices{}. Missing callbacks: Identity (process
start ID), Probe (fresh host headroom and destination disk), Observe (bounded
sprint monitor snapshot). Restore all three in yoke/dag, duplicating only the
read-only collector closure and no sprint command imports. Move unused old
production dispatch/services into test-only helpers for existing legacy tests.

Validate a real capacity receive/observe Cobra invocation against isolated policy,
process identity, destination disk regressions, standalone dependencies and build.
Full integration remains the rc.40 candidate gate 458628c0.
