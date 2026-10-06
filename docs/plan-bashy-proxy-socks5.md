# SOCKS5 proxy (Sprint #343, Story #368)

`bashy proxy socks` runs a standalone TCP SOCKS5 CONNECT server. It defaults to
`127.0.0.1:1080`; `--listen` changes the address. Passing both `--username` and
`--password` requires RFC 1929 username/password authentication. Without them,
the server accepts the no-auth method. Logs are JSON on stderr and never include
the configured password.

SOCKS5 domain-name requests pass the hostname directly to `net.Dialer` on the
proxy host, providing socks5h-style remote DNS. IPv4 and IPv6 requests also
work. The server rejects BIND and UDP ASSOCIATE. Signal cancellation closes
the listener and active clients; the dialer receives the cancellation context.

The server uses only the Go standard library and the already linked Cobra CLI.
The interoperability tests use `golang.org/x/net/proxy` v0.56.0, whose local
LICENSE is BSD-3-Clause. `go list -deps golang.org/x/net/proxy` shows no other
third-party modules. This test-only import does not add a release dependency.

Acceptance is exercised in `internal/agentos/proxy_socks_test.go`: no-auth,
username/password and a hostname target through the public pure-Go client,
plus explicit rejection of unsupported commands and bad credentials. The
hostname test observes the hostname received by the proxy-side dialer.
