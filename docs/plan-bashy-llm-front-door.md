# `bashy llm` front-door plan

Sprint #305, story #1014 (`995d906f4fa2`), part b.

1. Resolve yoke's nested `github.com/qiangli/yoke/pkg/llmgw` module through
   the flat sibling checkout, using the same zero pseudo-version pattern as
   `pkg/oci`, without changing unrelated dependency versions.
2. Mount `cligw.NewCmd()` as the `bashy llm` dispatcher, catalog `llm` with
   the design synopsis, and keep it direct-only so no bare `llm` shell shim
   shadows the unrelated popular CLI.
3. Ratchet the catalog/direct-only behavior and exercise the real binary's
   `llm --help` and serverless `llm env` behavior with state isolated under
   `BASHY_HOME`.
4. Run the focused tests, full Go gate, and Windows cross-build. Do not run
   `make test-bash`, because this change does not touch shell semantics.
