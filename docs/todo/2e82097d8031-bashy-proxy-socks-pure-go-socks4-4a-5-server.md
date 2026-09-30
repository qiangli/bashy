---
id: 2e82097d8031
kind: feature
title: 'bashy proxy socks: pure-Go SOCKS4/4a/5 server'
seq: 368
status: todo
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:15.32509Z
sprint: 343
sprint_id: ed2cbe05-a807-56d6-81dc-a9a04e7a11f5
sprint_title: 'Built-in tunnelling for air-gapped remotes: SOCKS4/4a/5 and HTTP/HTTPS proxy over the bashy peer channel'
---

Owner 2026-09-30. Scope (5 pt): SOCKS4, SOCKS4a (remote DNS) and SOCKS5 CONNECT; auth none and username/password; listen address flag; structured logging. No UDP ASSOCIATE/BIND in v1. Acceptance: red/green tests with a pure-Go client for each version.

Owner references 2026-09-30 and review: https://pkg.go.dev/golang.org/x/net/internal/socks is a SOCKS5 client in an internal package, so Bashy cannot import it. Public https://pkg.go.dev/golang.org/x/net/proxy supplies the client for interoperability tests and environment-based dialing. https://github.com/things-go/go-socks5 is an MIT-licensed SOCKS5 server with no-auth, user/pass, CONNECT, custom DNS and filtering, but no SOCKS4/4a claim. Also compare https://github.com/0990/socks5, Apache-2.0, which claims SOCKS4/4a/5 server support. Verify actual tests/API, cancellation/listener control, dependency footprint and peer-channel fit before selecting; preserve all required versions and scope.
