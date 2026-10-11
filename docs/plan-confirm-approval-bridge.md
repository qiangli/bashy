# Plan — Bash# @confirm yield → shared approval queue (Sprint 418, S4 bridge)

Story-ID d1438073d581. Contract of record: `docs/effect-derived-confirmation.md`.

1. On a `@confirm` yield (`internal/agentos/confirm.go`), call `ask.Raise` with
   source `confirm`, the existing token, exact argv, declared effects and a
   stable session (`confirmSession`: `BASHY_SESSION_CLAIM` → `BASHY_EPISODE` →
   `BASHY_INSTANCE_UUID` → cwd). The yield line names the approval id.
2. On `--confirm=TOKEN:yes`, redeem with `ask.Resolve(source, token, session,
   argv)`. Allow is single-use, deny persists, no decision means ask/yield
   again. `:no` refuses without consuming. No self-supplied `yes` and no old
   format: the answer is the recorded decision.
3. `bashy approve` is mounted as a direct front-door verb (yoke
   `ask.NewApproveCmd`), atlas effect `priv` so a `@confirm` body cannot
   approve itself unconfirmed.
4. Evidence: unit tests in `confirm_test.go` (raise, idempotence, lifecycle,
   deny, session/argv scoping, unbacked yes), process-level
   `TestE2EConfirmYieldApproveResume` (`-tags e2e`), and `test/yield/run.sh`.
