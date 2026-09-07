# Bash++ Transpilation & Standalone Build Recipe

`bashy transpile` transpiles Bash++ source scripts into Go code backed by `mvdan.cc/sh/v3/lower` compiler definitions and `shellrt` runtime primitives.

## Standalone Build Recipe

Because upstream `mvdan.cc/sh/v3` does not include the `lower` compiler package or `shellrt` runtime, running `go mod tidy` in an unconfigured module environment would fetch upstream `mvdan.cc/sh/v3` and fail to resolve dependencies.

To build standalone Go binaries transpiled from Bash++:

### 1. Clone & Pin the Dependency Repo

Clone the compiler/runtime repository to a local path (e.g. `../deps/sh` or `./deps/sh`) and pin to the published commit:

```bash
git clone https://github.com/qiangli/sh deps/sh
git -C deps/sh checkout aeecec06dde29255ed581ad61982246e9a52e617
```

### 2. Transpile the Source Script

Transpile the Bash++ input file to Go output:

```bash
bashy transpile --bashpp input.bpp -o output.go
```

### 3. Setup `go.mod` with Replace Directive

Initialize the standalone module and set up the replace directive mapping `mvdan.cc/sh/v3` to the local clone relative path:

```bash
go mod init standalone
go mod edit -require=mvdan.cc/sh/v3@v3.0.0
go mod edit -replace=mvdan.cc/sh/v3=../deps/sh
go mod tidy
```

The resulting `go.mod` file should resemble:

```go
module standalone

go 1.27

require mvdan.cc/sh/v3 v3.0.0

replace mvdan.cc/sh/v3 => ../deps/sh
```

### 4. Build Standalone Binary

Compile the Go output into a standalone executable:

```bash
go build -mod=mod -o myapp output.go
```

## Runtime Dependencies & Semantics

- **Standard Library Base & `shellrt`**: Emitted Go code imports `mvdan.cc/sh/v3/lower/shellrt` for shell runtime helpers. Standard primitives use pure Go stdlib constructs (`fmt`, `os`, `strconv`) where applicable.
- **Standalone Execution**: Compiled binaries run independently without needing the original `.bpp` source file or host shell binaries (`/bin/sh`, `/bin/bash`). Binaries execute cleanly even with `PATH=""`.
- **Parity & Scope**: No unsupported parity claims are made; dynamic subshell features or unmapped shell constructs rely on explicit `shellrt` bridge invocations rather than unanalyzed interpreter fallbacks.
