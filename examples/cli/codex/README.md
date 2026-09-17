# Codex YAML CLI adapter

This profile captures the common Codex session lifecycle from the pinned
source: init/start, model, plan, resume and exit, plus help, version,
validation and completion. Hosted authentication, marketplace and
remote-control behavior are explicit unsupported cases.

Behavioral source pin: `ycode/priorart/codex` at
`a8964cb1bad67bc26a826fb07d1bef99c6a3f008`.

The YAML is interpreted by ycode. `main.bpp` forwards substantive behavior to
an installed upstream executable selected by `CODEX_BIN`.
