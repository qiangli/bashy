# Preserve the Go caller's process environment

Sprint: #118; Story: #253; Story-ID: `b9aaa2b3a0fa`.

Cold Bashy startup enriches the shell environment and agentos initialization
installs BASHY_AGENT_MANIFEST. Ordinary Go input must observe the environment
its caller supplied, rather than that shell state.

The cli package captures `os.Environ()` during package initialization. Agentos
imports cli, so the capture precedes agentos initialization and its manifest
overwrite. Cold runners pass that ordered snapshot through
`interp.GoSourceEnv`; warm runners pass their request's own `SessionIO.Env`.
No shell/agent variable names are filtered. Explicitly supplied values and
original ordering survive, and normal shell startup remains unchanged.

This requires the companion sh `interp.GoSourceEnv` implementation. The manager
must integrate its sh revision before building this Bashy change. The option
configures only GoSource dependency processes; interpreter shell variables and
normal Bash subprocess environments retain their usual behavior.

`TestGoSourceExecutableEnvironment` builds the real default cmd/bashy executable
without optional tags, then compares raw stdout/stderr against native Go for
the unchanged Go by Example environment program. It covers both absent and
explicitly supplied BASH, SHELL, SHLVL, UID, EUID, IFS, OPTIND, BASH_VERSION and
BASHY_AGENT_MANIFEST, including their order. Both binaries receive identical
HOME/build-cache settings; the source importer requires a usable Go build cache.
No source or output normalization is used.

Measured: actual executable differential PASS (13.1 s); warm GoSource session,
shell startup matrix, unexported Bash version variables and OLDPWD regressions
PASS (2.3 s).

```sh
GOMAXPROCS=2 go test -p 2 ./internal/cli -run '^TestGoSourceExecutableEnvironment$' -count=1
GOMAXPROCS=2 go test -p 2 ./internal/cli -run 'TestGoSourceWarmSessionEnvironment|TestNewRunnerShellStartupMatrix|TestNewRunnerKeepsBashVersionVarsUnexported|TestNewRunnerInheritsOLDPWD' -count=1
```

Fixture SHA-256:
`1c8d6019811e77ed15c09a95913e3e60361382c6a8255af2c7a7c4ce3c522f21`.
Full Tour/GbE acceptance still requires the next authenticated candidate replay.
