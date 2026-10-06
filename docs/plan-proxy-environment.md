# Standard proxy environment for Bashy network use

Sprint #343, Story #371 makes Bashy's own network paths obey one standard
proxy policy.

## Scope

- Resolve `HTTP_PROXY`/`http_proxy` and `HTTPS_PROXY`/`https_proxy` first, with
  `ALL_PROXY`/`all_proxy` as the fallback for either scheme.
- Accept HTTP, `socks5://`, and `socks5h://` proxy URLs.
- Apply `NO_PROXY`/`no_proxy` matching before connecting.
- Install the policy for the default HTTP transport used by binmgr and for the
  go-git HTTP/HTTPS protocols used by `bashy git`.
- Derive missing HTTP(S) proxy variables before launching the provisioned Go
  command, so `bashy go mod download` has the same `ALL_PROXY` behavior. The Go
  command already consumes scheme-specific proxy variables and `NO_PROXY`.

## Verification

Integration tests download a checksummed binmgr artifact through the Sprint
#343 HTTP, SOCKS5, and SOCKS5H proxy servers. A native go-git clone probe proves
that its separately captured transport crosses the SOCKS5H proxy. Focused
resolver tests cover scheme-specific precedence and `NO_PROXY`, and an
environment test covers the Go subprocess fallback.
