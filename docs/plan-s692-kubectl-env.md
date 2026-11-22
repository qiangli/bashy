# Sprint 692 / Story 3304: kubectl Windows environment parity

At baseline 25b60ada the reported missing conversion is already present:
custom/kubectl/main.go calls adapter.AdaptKubeEnvironment on Windows, and
custom/internal/adapter/helpers.go converts all four Kubernetes path variables.
Do not add a duplicate conversion. Expose the existing platform branch to
host-independent tests; move the former dead-copy regression into kubectl and
cover Windows path lists and non-Windows preservation. Remove the unused
internal/agentos copy. Helm continues using the existing adapter helper.

Validate focused kubectl unit tests, standalone build, Windows cross-build,
and root cmd/bashy build. Native Windows execution remains the candidate gate.
