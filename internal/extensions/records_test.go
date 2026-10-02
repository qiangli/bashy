package extensions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDataOnlyCRUDAndOfflineVerify(t *testing.T) {
	store := Store{Root: t.TempDir(), Reserved: BuiltinReserved}
	exe := filepath.Join(t.TempDir(), "runner")
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(exe, body, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	platform := runtime.GOOS + "/" + runtime.GOARCH
	var out bytes.Buffer
	add := []string{"add", "acme", "--set", "aliases=acme-old", "--set", "fences=acme,acmex", "--set", "effects=pure", "--set", "payloads." + platform + ".version=1.2.3", "--set", "payloads." + platform + ".source=https://127.0.0.1:9/never", "--set", "payloads." + platform + ".sha256=" + digest, "--set", "payloads." + platform + ".executable=" + exe}
	if err := Run(store, "language", add, &out); err != nil {
		t.Fatalf("add: %v", err)
	}
	if err := Run(store, "language", []string{"verify", "acme-old"}, &out); err != nil {
		t.Fatalf("offline verify: %v", err)
	}
	out.Reset()
	if err := Run(store, "language", []string{"show", "acme-old", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"schema": "bashy.extension/v1"`) || !strings.Contains(out.String(), `"name": "acme"`) {
		t.Fatalf("show: %s", out.String())
	}
	path := filepath.Join(store.Root, "languages", "acme.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(store, "language", []string{"set", "acme", "--set", "protocol.major=2"}, &out); err == nil || !strings.Contains(err.Error(), "unsupported runner protocol") {
		t.Fatalf("major mismatch: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("rejected update changed the record")
	}
	if err := Run(store, "language", []string{"set", "acme", "--set", "protocol.minor=1"}, &out); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := Run(store, "language", []string{"add", "other", "--set", "aliases=acmex", "--set", "fences=other", "--set", "effects=pure", "--set", "payloads." + platform + ".version=1", "--set", "payloads." + platform + ".source=local", "--set", "payloads." + platform + ".sha256=" + digest, "--set", "payloads." + platform + ".executable=" + exe}, &out); err == nil || !strings.Contains(err.Error(), "belongs to") {
		t.Fatalf("alias collision: %v", err)
	}
	if err := os.WriteFile(exe, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify("language", "acme"); err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("digest mismatch: %v", err)
	}
	if err := Run(store, "language", []string{"rm", "acme-old"}, &out); err != nil {
		t.Fatalf("rm: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("record remains after rm: %v", err)
	}
}

func TestReservedNamesAndMalformedRecords(t *testing.T) {
	for _, tc := range []struct{ kind, name string }{{"language", "python"}, {"language", "py"}, {"toolchain", "go"}, {"toolchain", "clang"}} {
		if !BuiltinReserved(tc.kind, tc.name) {
			t.Errorf("%s %s is not reserved", tc.kind, tc.name)
		}
	}
	store := Store{Root: t.TempDir(), Reserved: BuiltinReserved}
	r := Record{Schema: Schema, Kind: "toolchain", Name: "acme", Stage: "optional", Protocol: Protocol{Major: 1}, Effects: []string{"pure"}, Payloads: map[string]Payload{runtime.GOOS + "/" + runtime.GOARCH: {Version: "latest", Source: "local", SHA256: strings.Repeat("0", 64), Executable: "/bin/true"}}}
	if err := store.Save(r, false); err == nil || !strings.Contains(err.Error(), "immutable version") {
		t.Fatalf("floating version: %v", err)
	}
	r.Payloads[runtime.GOOS+"/"+runtime.GOARCH] = Payload{Version: "1", Source: "local", SHA256: "bogus", Executable: "/bin/true"}
	if err := store.Save(r, false); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("bad digest: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.Root, "toolchains", "acme.yaml")); !os.IsNotExist(err) {
		t.Fatalf("bad record persisted: %v", err)
	}
}
