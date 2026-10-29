# S531 / story 2840: relative script operands

Plan: inspect the CLI script open and Windows path conversion; reproduce with
a focused unit test; use the engine's directory-aware path resolver; run the
focused gate and commit with the assigned story provenance.

`runPath` used `interp.ShellPathToOS`, which leaves ordinary relative operands
relative. Consequently `os.Open` used the process cwd, not `Runner.Dir`.
Use `pathconv.JoinAbs` for the initial open and the existing PATH fallback.
Keep the original script spelling for execution, diagnostics and `$0`.

Validation on Darwin: `go test ./internal/cli -run '^TestRunPathScriptOperand$'
-count=1 -v`. Before the change, bare and `./` operands failed with status 127;
the absolute control passed. After the change all three passed. The fixture
uses a non-executable readable cwd script, a different process cwd and a
competing PATH script, and checks output, `$0` and `$1`.

This regression proves the cwd defect across platforms but does not reproduce
the reported bare-versus-`./` difference itself. The conductor retains the
elephant host claim: rerun both operands in the native Windows guarded D3
envelope before treating that incident as verified fixed. No native Windows
execution or full conformance gate is claimed here; the request limits this
worker to focused Go tests.
