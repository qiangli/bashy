# Bash++ Transpilation & Standalone Build Recipe

`bashy transpile` transpiles Bash++ source scripts into Go code backed by `mvdan.cc/sh/v3/lower` compiler definitions and `shellrt` runtime primitives.

## Compiler Build Toolchain

The Bashy and shell-engine modules retain a Go 1.26.5 compatibility floor and
select `toolchain go1.27.0` for default automatic builds. Build Bashy and the
generated artifacts with the reviewed Go 1.27 toolchain for the complete source
profile. An older explicit `GOTOOLCHAIN=local` build cannot check independent
method type parameters and reports `LOWER-ETOOLCHAIN` before emitting output.
Changing the toolchain environment when running an existing Bashy binary does
not change its compiled-in Go checker.

## Workspace Directory Structure

Organize the build environment into a dedicated workspace containing the dependency clone in `deps/sh` and the application source in `app`:

```text
workspace/
├── deps/
│   └── sh/          # Cloned dependency repository pinned to published commit
└── app/
    ├── input.bpp    # Bash++ source script
    ├── output.go    # Transpiled Go output
    └── go.mod       # Standalone module configuration with replace directive
```

## Standalone Build Recipe

Because upstream `mvdan.cc/sh/v3` does not include the `lower` compiler package or `shellrt` runtime, running `go mod tidy` in an unconfigured module environment would attempt to fetch upstream `mvdan.cc/sh/v3` and fail.

To build standalone Go binaries transpiled from Bash++:

### 1. Clone & Pin the Published Dependency Repo

Clone the compiler/runtime repository to `workspace/deps/sh` and checkout the published commit SHA (`ed9358e94803fd2de8b8f9e980736a9f36f93c9c`):

```bash
mkdir -p workspace/deps
git clone https://github.com/qiangli/sh workspace/deps/sh
git -C workspace/deps/sh checkout ed9358e94803fd2de8b8f9e980736a9f36f93c9c
```

### 2. Create Application Directory & Transpile

Create the `workspace/app` directory, write the Bash++ source script, change into `workspace/app`, and invoke `bashy transpile`:

```bash
mkdir -p workspace/app
cd workspace/app

cat << 'EOF' > input.bpp
var x int = 42
println("hello from transpiled standalone:", x)
EOF

bashy transpile --bashpp input.bpp -o output.go
```

### 3. Setup `go.mod` with Replace Directive

Initialize the Go module in `workspace/app` and configure the `replace` directive pointing to the relative path `../deps/sh`:

```bash
go mod init app
go mod edit -require=mvdan.cc/sh/v3@v3.0.0
go mod edit -replace=mvdan.cc/sh/v3=../deps/sh
go mod tidy
```

For pure standard-library emitted scripts, `go mod tidy` prunes unused `require` entries while keeping the module `replace` directive intact:

```go
module app

go 1.27

replace mvdan.cc/sh/v3 => ../deps/sh
```

### 4. Build Standalone Binary

Compile the transpiled Go code into a standalone binary:

```bash
go build -mod=mod -o myapp output.go
```

### 5. Remove Sources & Execute Binary

Remove the input Bash++ script (`input.bpp`) and transpiled Go file (`output.go`) to prove standalone binary execution:

```bash
rm input.bpp output.go
PATH="" ./myapp
```

Output:
```text
hello from transpiled standalone: 42
```

## Runtime Dependencies & Semantics

- **Standard Library Base & `shellrt`**: Emitted Go code imports `mvdan.cc/sh/v3/lower/shellrt` for shell runtime helpers. Standard typed constructs use pure Go stdlib primitives (`fmt`, `os`, `strconv`).
- **Typed-Only Standalone Execution with Empty PATH**: Typed-only programs (using typed variables, arithmetic, functions, and standard `println`/`printf`) execute independently with `PATH=""` when source files and host shell binaries are absent.
- **Dynamic External Commands**: Dynamic scripts invoking external shell commands (such as `ls`, `grep`, or `curl`) require their respective external binaries to be available on `PATH`.
- **Parity & Scope**: No unsupported parity claims are made; dynamic subshell features or unmapped shell constructs rely on explicit `shellrt` bridge invocations.
