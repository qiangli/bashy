// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/qiangli/outpost/pkg/sshclient"
	"golang.org/x/crypto/ssh"
)

func TestPeerChannelUsesAlternatePort(t *testing.T) {
	if peerChannelPort == 22 {
		t.Fatal("peer channel must not use the system ssh port")
	}
}

func TestPeerChannelConfigRequiresInstallIdentityAndTrust(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	good := PeerChannelConfig{
		Address:         "novidesign.local:2223",
		User:            "operator",
		PrivateKeyPath:  "/tmp/bashy-peer-key",
		HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()),
	}
	if err := good.validate(); err != nil {
		t.Fatalf("valid config: %v", err)
	}
	for name, cfg := range map[string]PeerChannelConfig{
		"address": {User: good.User, PrivateKeyPath: good.PrivateKeyPath, HostKeyCallback: good.HostKeyCallback},
		"user":    {Address: good.Address, PrivateKeyPath: good.PrivateKeyPath, HostKeyCallback: good.HostKeyCallback},
		"key":     {Address: good.Address, User: good.User, HostKeyCallback: good.HostKeyCallback},
		"trust":   {Address: good.Address, User: good.User, PrivateKeyPath: good.PrivateKeyPath},
	} {
		t.Run(name, func(t *testing.T) {
			if err := cfg.validate(); err == nil {
				t.Fatal("validate unexpectedly accepted incomplete config")
			}
		})
	}
}

func TestDialPeerChannelRejectsNilContext(t *testing.T) {
	_, err := DialPeerChannel(nil, PeerChannelConfig{})
	if err == nil {
		t.Fatal("nil context unexpectedly accepted")
	}
}

func TestPeerChannelCloseNilClientIsSafe(t *testing.T) {
	if err := (&PeerChannel{}).Close(); err != nil {
		t.Fatalf("close nil client: %v", err)
	}
}

func TestPeerChannelEmbeddedServerExecSFTPAndForward(t *testing.T) {
	me, err := user.Current()
	if err != nil || me.Username == "" {
		t.Skipf("current user unavailable: %v", err)
	}
	hostKey, hostPrivate := testSSHKey(t)
	clientKey, clientPrivate := testSSHKey(t)
	keysDir := t.TempDir()
	authorizedKeys := filepath.Join(keysDir, "authorized_keys")
	if err := os.WriteFile(authorizedKeys, ssh.MarshalAuthorizedKey(clientKey.PublicKey()), 0o600); err != nil {
		t.Fatal(err)
	}
	hostKeyPath := filepath.Join(keysDir, "host_key")
	privatePEM, err := ssh.MarshalPrivateKey(hostPrivate, "bashy-peer-host")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hostKeyPath, pemBytes(privatePEM), 0o600); err != nil {
		t.Fatal(err)
	}
	clientKeyPath := filepath.Join(keysDir, "client_key")
	clientPEM, err := ssh.MarshalPrivateKey(clientPrivate, "bashy-peer-client")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(clientKeyPath, pemBytes(clientPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	serverCtx, serverCancel := context.WithCancel(context.Background())
	defer serverCancel()
	server, err := StartPeerSSHServer(serverCtx, PeerSSHServerConfig{
		Address:            "127.0.0.1:0",
		AuthorizedKeysPath: authorizedKeys,
		HostKeyPath:        hostKeyPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		serverCancel()
		_ = server.Close()
		if err := server.Wait(); err != nil && err != context.Canceled && err != net.ErrClosed {
			t.Errorf("server shutdown: %v", err)
		}
	}()

	channel, err := DialPeerChannel(context.Background(), PeerChannelConfig{
		Address:         server.Addr().String(),
		User:            me.Username,
		PrivateKeyPath:  clientKeyPath,
		HostKeyCallback: ssh.FixedHostKey(hostKey.PublicKey()),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()

	result, err := channel.Exec(context.Background(), sshclient.ExecOptions{Command: "printf peer-exec"})
	if err != nil || result.ExitCode != 0 || string(result.Stdout) != "peer-exec" {
		t.Fatalf("exec: result=%+v err=%v", result, err)
	}

	sftpClient, err := channel.SFTP()
	if err != nil {
		t.Fatal(err)
	}
	defer sftpClient.Close()
	remotePath := filepath.Join(t.TempDir(), "peer-roundtrip.txt")
	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := remoteFile.Write([]byte("peer-sftp")); err != nil {
		t.Fatal(err)
	}
	_ = remoteFile.Close()
	remoteFile, err = sftpClient.Open(remotePath)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(remoteFile)
	_ = remoteFile.Close()
	if err != nil || string(got) != "peer-sftp" {
		t.Fatalf("sftp: got %q err=%v", got, err)
	}

	target := startEchoListener(t)
	direct, err := channel.DirectTCPIP(context.Background(), "127.0.0.1", target.Port)
	if err != nil {
		t.Fatal(err)
	}
	assertEcho(t, direct, "peer-direct")

	forwardCtx, forwardCancel := context.WithCancel(context.Background())
	defer forwardCancel()
	local, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	forwardDone := make(chan error, 1)
	go func() { forwardDone <- channel.LocalForward(forwardCtx, local, "127.0.0.1", target.Port) }()
	forwardConn := dialEventually(t, local.Addr().String())
	assertEcho(t, forwardConn, "peer-forward")
	forwardCancel()
	select {
	case <-forwardDone:
	case <-time.After(2 * time.Second):
		t.Fatal("local forward did not stop")
	}
}

type echoTarget struct{ Port int }

func startEchoListener(t *testing.T) echoTarget {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return echoTarget{Port: ln.Addr().(*net.TCPAddr).Port}
}

func assertEcho(t *testing.T, conn net.Conn, message string) {
	t.Helper()
	defer conn.Close()
	if _, err := conn.Write([]byte(message)); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(message))
	if _, err := io.ReadFull(conn, got); err != nil || string(got) != message {
		t.Fatalf("echo: got %q err=%v", got, err)
	}
}

func dialEventually(t *testing.T, address string) net.Conn {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", address)
		if err == nil {
			return conn
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("could not dial %s", address)
	return nil
}

func testSSHKey(t *testing.T) (ssh.Signer, ed25519.PrivateKey) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer, privateKey
}

func pemBytes(block *pem.Block) []byte { return pem.EncodeToMemory(block) }
