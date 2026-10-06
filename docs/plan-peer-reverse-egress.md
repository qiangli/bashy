# Peer reverse egress — Sprint 343, story 370

Use outpost RemoteForward through PeerChannel with a remote loopback listener.
Pin the landed outpost dependency, then prove a failing local regression before
implementing proxy URL validation, tunnel lifecycle, and command environment.
Expose remote exec --proxy for an already running operator HTTP or SOCKS proxy.
Export uppercase and lowercase proxy variables to avoid inherited bypasses.
Use the in-process peer server for forwarding, environment, rejection and cleanup
checks. Run the story gate and broader repository checks; commit on this branch.
The conductor owns the reserved-host no-default-route URL and Go module e2e.

## Usage

After `bashy remote install host` has provisioned the peer identity, start the
operator proxy in one terminal:

```sh
bashy proxy http --listen 127.0.0.1:8080
```

Run commands in another terminal:

```sh
bashy remote exec --proxy http://127.0.0.1:8080 host 'curl https://example.com'
bashy remote exec --proxy http://127.0.0.1:8080 host 'cd project && go mod download'
```

For SOCKS use `bashy proxy socks --listen 127.0.0.1:1080` and
`--proxy socks5h://127.0.0.1:1080`. A `socks5://` URL is normalized to `socks5h://`
so clients that distinguish them resolve destination names through the operator.
Proxy authentication can be supplied as URL userinfo. The operator target must
be loopback with an explicit port. `--proxy-listen 127.0.0.1:0` is the default;
outpost rejects non-loopback remote binds. `--no-proxy` defaults to
`localhost,127.0.0.1,::1`; use an empty value to disable bypasses. Both upper and
lowercase variables are exported, replacing inherited proxy/bypass settings.
Commands are supplied as one quoted shell string. Output uses outpost's bounded
capture (1 MiB stdout, 256 KiB stderr); truncation returns an error.

`PeerChannel.ReverseProxy` and `PeerReverseProxy.Exec` provide the reusable API.
Cancellation or Close removes the listener and active forwarding connections;
commands through a closed proxy fail. The existing DAG transport is not changed
by this story; callers can run a remote DAG using remote exec.

## Evidence

The initial local test failed to compile because RemoteForward and ReverseProxy
were absent. After implementation the embedded-peer test passes HTTP and SOCKS5h
authenticated requests to an operator-only DNS name, including a fetch from an
actual SSH-executed subprocess using net/http environment discovery. It checks
command environment exports,
listener cleanup and rejection of wildcard binds. The reserved-host test with
no default route and a real Go module download remains conductor-gated.

Validation on this workspace (2026-10-05):

- Focused `bashy gate --command "go build ./internal/agentos/ && go test ./internal/agentos/ -run 'PeerChannel|ReverseProxy|SOCKS5|HTTPProxy' -count=1"`: passed; verbose tests independently inspected.
- `go build ./...` and `go test ./...`: passed.
- Default `bashy gate`: build and internal/tools tests passed, then abstained
  because its in-process make rejected the GNU Makefile (`invalid inference rule`).
- Explicit host `env make test-bash`: 83 passed, 3 failed, 0 skipped, 0 timed out.
  Failures: exp-tests, extglob, new-exp, matching the differences already recorded
  in story #357 (`9e8271251090`). No shell engine code is changed by this story.
- Reserved-host no-default-route URL and Go module acceptance: not run here;
  pending the conductor's reserved-host gate.
