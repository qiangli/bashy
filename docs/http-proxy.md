# HTTP forward proxy

Start a local proxy:

```sh
bashy proxy http --listen 127.0.0.1:8080
bashy proxy http --listen 127.0.0.1:8080 --auth user:pass
curl --proxy http://127.0.0.1:8080 --proxy-user user:pass http://example.com/
curl --proxy http://127.0.0.1:8080 --proxy-user user:pass https://example.com/
```

The default listen address is `127.0.0.1:8080`. HTTP requests use absolute-form
HTTP/1.1 forwarding. HTTPS uses CONNECT with opaque byte forwarding: the client
validates the origin certificate, and the proxy does no TLS interception.
Basic proxy authentication is optional; missing or incorrect credentials receive
407 with a `Proxy-Authenticate` challenge. The password may contain colons.
Basic authentication does not encrypt credentials on the client-to-proxy hop.
Structured JSON lifecycle and request events go to stderr, without credentials,
URL paths, or query strings. Ctrl-C stops the listener and active connections.

`internal/httpproxy` is standard-library-only. `New(Config)` returns an
`http.Handler`; call `Close` when finished to close tunnels and idle connections.
`Serve(ctx, listener, Config)` owns a caller-supplied listener and stops on context
cancellation. `Config.DialContext` routes outgoing connections through a custom
transport (for example a peer channel); the default dials directly without
consulting proxy environment variables. `Config.Logger` accepts a `slog.Logger`.
