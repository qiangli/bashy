// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

// The peer channel deliberately contains no SSH protocol code. Outpost owns
// both the server and client implementation; this is only Bashy's small,
// key-authenticated adapter around its public client API.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/pkg/sftp"
	"github.com/qiangli/outpost/pkg/sshclient"
	"golang.org/x/crypto/ssh"
)

// peerChannelPort is intentionally not the system SSH port. Remote install
// starts outpost sshd on this port, leaving a user's system sshd untouched.
const peerChannelPort = 2223

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

func (c *PeerChannel) Close() error {
	if c == nil {
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
