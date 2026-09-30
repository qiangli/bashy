// Copyright (c) 2026 qiangli
// See LICENSE for licensing information.

package agentos

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

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
