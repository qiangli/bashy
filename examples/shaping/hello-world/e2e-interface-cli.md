# Selected interface: CLI

Use the candidate executable in BASHY_BIN, also available as `bashy` on PATH.
Invoke `bashy shape` directly for each shaping operation. Discover its help.
Do not use `--remote`, MCP or raw REST as substitutes. You may prepare the
requested change in a separate disposable candidate workspace and import it
with `bashy shape wish APP REQUEST --from WORKSPACE`. Then use the CLI for
show/diff, verify, preview, explicit approval, rollback and submission. Never
edit the deployed app, managed active store or shared source checkout directly.
Retain actual invocations and sanitized structured results.

