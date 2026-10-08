package installpair

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyOptionalLauncherPair(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "bash")
	mustWrite(t, base, "plain-go")
	run := func(path string, _ ...string) ([]byte, error) {
		return []byte("bash, version 5.3.0-bashy-v1.0.0\n"), nil
	}
	if err := VerifyOptional(base, run); err != nil {
		t.Fatalf("plain Go binary: %v", err)
	}

	mustWrite(t, base+".real", "plain-go")
	if err := VerifyOptional(base, run); err == nil || !strings.Contains(err.Error(), "byte copy") {
		t.Fatalf("identical launcher/payload error = %v", err)
	}

	mustWrite(t, base, "native-launcher")
	run = func(path string, _ ...string) ([]byte, error) {
		if strings.HasSuffix(path, ".real") {
			return []byte("bash, version 5.3.0-bashy-v1.0.1\n"), nil
		}
		return []byte("bash, version 5.3.0-bashy-v1.0.0\n"), nil
	}
	if err := VerifyOptional(base, run); err == nil || !strings.Contains(err.Error(), "build identity") {
		t.Fatalf("mismatched version error = %v", err)
	}

	run = func(string, ...string) ([]byte, error) {
		return []byte("bash, version 5.3.0-bashy-v1.0.0\n"), nil
	}
	if err := VerifyOptional(base, run); err != nil {
		t.Fatalf("valid launcher/payload pair: %v", err)
	}
}

func TestRefuseCompanion(t *testing.T) {
	base := filepath.Join(t.TempDir(), "bashy")
	mustWrite(t, base, "bashy")
	if err := RefuseCompanion(base); err != nil {
		t.Fatalf("one-file bashy: %v", err)
	}
	mustWrite(t, base+".real", "retired payload")
	if err := RefuseCompanion(base); err == nil || !strings.Contains(err.Error(), "one-file") {
		t.Fatalf("bashy.real refusal = %v", err)
	}
	if err := os.Remove(base + ".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", base+".real"); err != nil {
		t.Fatal(err)
	}
	if err := RefuseCompanion(base); err == nil {
		t.Fatal("dangling bashy.real symlink accepted")
	}
}

func TestVerifyOptionalPropagatesVersionProbeFailure(t *testing.T) {
	base := filepath.Join(t.TempDir(), "sh")
	mustWrite(t, base, "launcher")
	mustWrite(t, base+".real", "payload")
	want := errors.New("cannot execute")
	run := func(path string, _ ...string) ([]byte, error) {
		if strings.HasSuffix(path, ".real") {
			return nil, want
		}
		return []byte("same"), nil
	}
	if err := VerifyOptional(base, run); !errors.Is(err, want) {
		t.Fatalf("probe error = %v, want %v", err, want)
	}
}

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
