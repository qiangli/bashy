package agentos

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestExtractDagHost(t *testing.T) {
	gotHost, gotArgs, found, err := extractDagHost([]string{"--file", "dag.md", "-H", "novidesign.local", "test"})
	if err != nil || !found || gotHost != "novidesign.local" {
		t.Fatalf("extract host = %q, %v, %v", gotHost, found, err)
	}
	if want := []string{"--file", "dag.md", "test"}; !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("remaining args = %#v, want %#v", gotArgs, want)
	}
}

func TestStreamRemoteFileTailsOnlyNewBytes(t *testing.T) {
	me, err := user.Current()
	if err != nil || me.Username == "" {
		t.Skipf("current user unavailable: %v", err)
	}
	hostSigner, hostKey := testSSHKey(t)
	clientSigner, clientKey := testSSHKey(t)
	keys := t.TempDir()
	if err := os.WriteFile(filepath.Join(keys, "authorized"), ssh.MarshalAuthorizedKey(clientSigner.PublicKey()), 0o600); err != nil {
		t.Fatal(err)
	}
	keyPEM, err := ssh.MarshalPrivateKey(clientKey, "dag-stream-test")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(keys, "client")
	if err := os.WriteFile(keyPath, pemBytes(keyPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	hostPEM, err := ssh.MarshalPrivateKey(hostKey, "dag-stream-host")
	if err != nil {
		t.Fatal(err)
	}
	hostPath := filepath.Join(keys, "host")
	if err := os.WriteFile(hostPath, pemBytes(hostPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, err := StartPeerSSHServer(ctx, PeerSSHServerConfig{Address: "127.0.0.1:0", AuthorizedKeysPath: filepath.Join(keys, "authorized"), HostKeyPath: hostPath})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = server.Close(); _ = server.Wait() }()
	channel, err := DialPeerChannel(ctx, PeerChannelConfig{Address: server.Addr().String(), User: me.Username, PrivateKeyPath: keyPath, HostKeyCallback: ssh.FixedHostKey(hostSigner.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()
	sftp, err := channel.SFTP()
	if err != nil {
		t.Fatal(err)
	}
	defer sftp.Close()
	remote := filepath.Join(t.TempDir(), "output.log")
	f, err := sftp.Create(remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(f, "first"); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	offset, err := streamRemoteFile(sftp, remote, 0, &got)
	if err != nil || offset != 5 || got.String() != "first" {
		t.Fatalf("first tail: offset=%d data=%q err=%v", offset, got.String(), err)
	}
	f, err = sftp.Create(remote)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(f, "firstsecond"); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	offset, err = streamRemoteFile(sftp, remote, 5, &got)
	if err != nil || offset != 11 || got.String() != "firstsecond" {
		t.Fatalf("second tail: offset=%d data=%q err=%v", offset, got.String(), err)
	}
	local := filepath.Join(t.TempDir(), "source.txt")
	remoteSource := filepath.Join(filepath.Dir(remote), "source.txt")
	if err := os.WriteFile(local, []byte("version-one"), 0o644); err != nil {
		t.Fatal(err)
	}
	var transferLog bytes.Buffer
	if err := syncDagFile(sftp, local, remoteSource, "source.txt", &transferLog); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transferLog.String(), "transferred source.txt") {
		t.Fatalf("first sync log = %q", transferLog.String())
	}
	transferLog.Reset()
	if err := syncDagFile(sftp, local, remoteSource, "source.txt", &transferLog); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transferLog.String(), "unchanged source.txt") || strings.Contains(transferLog.String(), "transferred") {
		t.Fatalf("unchanged sync log = %q", transferLog.String())
	}
	if err := os.WriteFile(local, []byte("version-two"), 0o644); err != nil {
		t.Fatal(err)
	}
	transferLog.Reset()
	if err := syncDagFile(sftp, local, remoteSource, "source.txt", &transferLog); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transferLog.String(), "transferred source.txt") {
		t.Fatalf("changed sync log = %q", transferLog.String())
	}
}

func TestExtractDagHostRejectsDuplicateAndMissing(t *testing.T) {
	for _, args := range [][]string{{"-H"}, {"--host="}, {"-H", "one", "--host", "two"}} {
		if _, _, _, err := extractDagHost(args); err == nil {
			t.Errorf("extractDagHost(%q) unexpectedly succeeded", args)
		}
	}
}
