// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

// The peer channel uses yoke's shared SSH transport and Bashy's existing
// shell session runner. It does not link the outpost service implementation.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/sshclient"
	"github.com/qiangli/yoke/pkg/sshserver"
	"golang.org/x/crypto/ssh"
)

// peerChannelPort is intentionally not the system SSH port. Remote install
// starts outpost sshd on this port, leaving a user's system sshd untouched.
const peerChannelPort = 2223

// PeerSSHServerConfig describes the user-space outpost SSH listener that a
// remote bashy starts for peer connections. HostKeyPath is the install-time
// host identity; AuthorizedKeysPath is the install-time client trust file.
// Address defaults to the LAN-reachable alternate port when empty.
type PeerSSHServerConfig struct {
	Address            string
	AuthorizedKeysPath string
	HostKeyPath        string
}

// PeerSSHServer owns an embedded outpost SSH server and its listener.
// Closing it only stops this user-space listener; it never touches system
// sshd or any other listener owned by the user.
type PeerSSHServer struct {
	listener net.Listener
	err      chan error
	once     sync.Once
}

// StartPeerSSHServer starts outpost's SSH server on the configured alternate
// port. The caller must provide install-time authorized keys and a persisted
// host key; no key is generated or silently trusted at runtime.
func StartPeerSSHServer(ctx context.Context, cfg PeerSSHServerConfig) (*PeerSSHServer, error) {
	if ctx == nil {
		return nil, errors.New("peer ssh server: nil context")
	}
	if strings.TrimSpace(cfg.Address) == "" {
		cfg.Address = fmt.Sprintf(":%d", peerChannelPort)
	}
	if strings.TrimSpace(cfg.AuthorizedKeysPath) == "" {
		return nil, errors.New("peer ssh server: authorized keys path is required")
	}
	if strings.TrimSpace(cfg.HostKeyPath) == "" {
		return nil, errors.New("peer ssh server: host key path is required")
	}
	hostKey, err := readSSHSigner(cfg.HostKeyPath)
	if err != nil {
		return nil, fmt.Errorf("peer ssh server: read host key: %w", err)
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("peer ssh server: listen %s: %w", cfg.Address, err)
	}
	server := &PeerSSHServer{listener: listener, err: make(chan error, 1)}
	go func() {
		server.err <- sshserver.Serve(ctx, listener, sshserver.Config{
			AuthorizedKeysPath: cfg.AuthorizedKeysPath,
			HostKey:            hostKey,
			Execute: func(ctx context.Context, command string, in io.Reader, out, errOut io.Writer) uint32 {
				return uint32(cli.RunSessionCommandWithConfig(ctx, cli.SessionIO{Command: command, Env: os.Environ(), Stdin: in, Stdout: out, Stderr: errOut}, cli.SessionConfig{WireExec: WireExec, Preamble: Preamble}))
			},
		})
	}()
	return server, nil
}

func readSSHSigner(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, err
	}
	return signer, nil
}

// Addr returns the actual listener address, including an OS-assigned port.
func (s *PeerSSHServer) Addr() net.Addr {
	if s == nil || s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

// Wait waits for the embedded server to stop.
func (s *PeerSSHServer) Wait() error {
	if s == nil {
		return nil
	}
	return <-s.err
}

// Close stops the embedded server and is safe to call more than once.
func (s *PeerSSHServer) Close() error {
	if s == nil || s.listener == nil {
		return nil
	}
	s.once.Do(func() { _ = s.listener.Close() })
	return nil
}

// PeerChannelConfig is the install-time identity needed to dial a remote
// bashy's outpost listener. HostKeyCallback is supplied by the caller's
// existing outpost trust store; Bashy neither invents nor weakens that model.
type PeerChannelConfig struct {
	Address         string
	User            string
	PrivateKeyPath  string
	HostKeyCallback ssh.HostKeyCallback
}

func (c PeerChannelConfig) validate() error {
	if strings.TrimSpace(c.Address) == "" {
		return errors.New("peer channel: remote address is required")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("peer channel: remote user is required")
	}
	if strings.TrimSpace(c.PrivateKeyPath) == "" {
		return errors.New("peer channel: install-time private key is required")
	}
	if c.HostKeyCallback == nil {
		return errors.New("peer channel: outpost host-key callback is required")
	}
	return nil
}

// PeerChannel is an authenticated outpost SSH connection. It exposes only
// the operations the remote DAG lane needs: exec, SFTP and TCP forwarding.
type PeerChannel struct{ client *sshclient.Client }

// DialPeerChannel dials the remote's alternate outpost SSH port using the
// install-time private key. It never invokes system ssh or sshd.
func DialPeerChannel(ctx context.Context, cfg PeerChannelConfig) (*PeerChannel, error) {
	if ctx == nil {
		return nil, errors.New("peer channel: nil context")
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	key, err := os.ReadFile(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("peer channel: read private key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("peer channel: parse private key: %w", err)
	}
	transport, err := (&net.Dialer{}).DialContext(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("peer channel: dial %s: %w", cfg.Address, err)
	}
	client, err := sshclient.Dial(ctx, sshclient.Config{
		Transport:       transport,
		HostAlias:       cfg.Address,
		User:            cfg.User,
		HostKeyCallback: cfg.HostKeyCallback,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
	})
	if err != nil {
		_ = transport.Close()
		return nil, fmt.Errorf("peer channel: authenticate %s: %w", cfg.Address, err)
	}
	return &PeerChannel{client: client}, nil
}

// DialInstalledPeerChannel loads Bashy's persisted install identity and pinned
// host key for host. It fails closed when either file is absent or malformed.
func DialInstalledPeerChannel(ctx context.Context, host, user string) (*PeerChannel, error) {
	name := safePeerName(host)
	dir := filepath.Join(os.Getenv("HOME"), ".bashy", "remote", "peers", name)
	privatePath := filepath.Join(dir, "id_ed25519")
	address := host
	if at := strings.LastIndex(host, "@"); at >= 0 {
		if user == "" {
			user = host[:at]
		}
		address = host[at+1:]
	}
	if user == "" {
		user = os.Getenv("USER")
	}
	if !strings.Contains(address, ":") {
		address = net.JoinHostPort(address, "2223")
	}
	known, err := os.ReadFile(filepath.Join(dir, "host_key.pub"))
	if err != nil {
		return nil, fmt.Errorf("peer channel: pinned host key unavailable: %w", err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey(known)
	if err != nil {
		return nil, fmt.Errorf("peer channel: parse pinned host key: %w", err)
	}
	return DialPeerChannel(ctx, PeerChannelConfig{Address: address, User: user, PrivateKeyPath: privatePath, HostKeyCallback: ssh.FixedHostKey(pub)})
}

func (c *PeerChannel) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *PeerChannel) Exec(ctx context.Context, opts sshclient.ExecOptions) (*sshclient.ExecResult, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("peer channel: nil client")
	}
	return c.client.Exec(ctx, opts)
}

func (c *PeerChannel) SFTP() (*sftp.Client, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("peer channel: nil client")
	}
	return c.client.SFTP()
}

func (c *PeerChannel) DirectTCPIP(ctx context.Context, host string, port int) (net.Conn, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("peer channel: nil client")
	}
	return c.client.DirectTCPIP(ctx, host, port)
}

func (c *PeerChannel) LocalForward(ctx context.Context, listener net.Listener, host string, port int) error {
	if c == nil || c.client == nil {
		return errors.New("peer channel: nil client")
	}
	return c.client.LocalForward(ctx, listener, host, port)
}

// RemoteForward exposes outpost's loopback-only remote TCP forward. Cancel ctx
// to remove the remote listener and close its active connections.
func (c *PeerChannel) RemoteForward(ctx context.Context, remoteAddr, localTarget string) (net.Addr, error) {
	if c == nil || c.client == nil {
		return nil, errors.New("peer channel: nil client")
	}
	return c.client.RemoteForward(ctx, remoteAddr, localTarget)
}
