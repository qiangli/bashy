# FIPS 140-3 mode

bashy can be built against the **Go Cryptographic Module v1.0.0**, which holds
CMVP certificate **#5247** (validated to FIPS 140-3). This matters for FedRAMP,
CMMC, and other US-government deployments that require validated cryptography —
and it is nearly free here, because bashy is already pure Go with
`CGO_ENABLED=0`: no BoringCrypto, no OpenSSL, no cgo, no forked toolchain. No
other cgo-free agentic shell can offer it.

## Building it

```sh
make build-fips          # GOFIPS140=v1.0.0 go build … for bin/bash and bin/bashy
```

A `GOFIPS140`-built binary defaults its runtime `GODEBUG` to `fips140=on`, so it
runs in FIPS mode with no extra environment. The mode can still be set
explicitly:

```sh
GODEBUG=fips140=on   bashy …   # validated module for approved algorithms
GODEBUG=fips140=only bashy …   # ALSO reject every non-approved algorithm
```

## `on` vs `only` — pick `on` for a general shell

- **`fips140=on`** (the build-fips default): approved algorithms (SHA-2, AES,
  ECDSA, …) run through the validated module; non-approved algorithms still
  work. This is the correct mode for a general-purpose shell, because the shell
  must keep providing tools like `md5sum` and `sha1sum`.
- **`fips140=only`**: additionally rejects every non-approved algorithm. Under it
  `md5sum` fails (`use of MD5 is not allowed in FIPS 140-only mode`), by design.
  Use it only for a hardened deployment that genuinely must forbid weak hashing
  and does not need those tools. It is **not** the recommended default.

## Verifying it

The active state is surfaced where an operator and an agent both look:

```sh
bashy doctor            # a "FIPS 140-3" check: ok when active, info when not
bashy context --json    # runtime.fips140: true | false
```

`bashy context --json` is the first call an agent makes, so a policy that
requires FIPS can gate on `runtime.fips140` directly.

## What is and isn't covered

The module covers Go standard-library cryptography (the `crypto/*` packages).
bashy's own hashing — the audit-log chain (SHA-256), binmgr's integrity checks
(SHA-256; MD5 is refused by default anyway) — therefore runs through the
validated module in FIPS mode. External binaries bashy execs (git, podman, a
provisioned toolchain) bring their own crypto and are outside this boundary, as
they are outside every other in-process control — see `docs/audit-log.md` on the
`execve` boundary.

A FIPS-built `bin/bash` still passes the full bash-5.3 conformance suite (86/86):
validated crypto and exact shell semantics coexist.

## Compliance framing

FIPS-validated crypto is a hard prerequisite for touching CUI in a FedRAMP /
CMMC boundary. Together with the tamper-evident audit log
(`docs/audit-log.md`), it is the pair that lets bashy enter government
procurement conversations that a cgo/OpenSSL-dependent shell cannot. Reference:
[go.dev/doc/security/fips140](https://go.dev/doc/security/fips140).
