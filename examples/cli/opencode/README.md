# OpenCode YAML CLI adapter

This profile captures the common OpenCode session lifecycle from the pinned
source: init/start, model, plan, resume and exit, plus help, version,
validation and completion. Hosted services, plugins and marketplace behavior
are explicit unsupported cases.

Behavioral source pin: `ycode/priorart/opencode` at
`e03db9bc6908f75c9334d8aa997deeaac81c0298`.

The YAML is interpreted by ycode. `main.bpp` forwards substantive behavior to
an installed upstream executable selected by `OPENCODE_BIN`.
