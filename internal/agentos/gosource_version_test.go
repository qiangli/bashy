package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/internal/cli"
)

func TestGoSourceVersionWiring(t *testing.T) {
	bytes := []byte("package p\nvar _ = 0b101\n")
	files := []cli.GoSourceFile{{Name: "original.go", Data: bytes}}
	for _, v := range []string{"go1.12", "go1.13", "go1.12"} {
		_, err := loadGoSource(files, cli.GoSourceOptions{GoVersion: v, Dir: t.TempDir()})
		if v == "go1.12" {
			if err == nil || !strings.Contains(err.Error(), "binary literal requires go1.13 or later") {
				t.Fatalf("old language accepted: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
}

func TestTranspileGoSourceVersion(t *testing.T) {
	dir := t.TempDir()
	source, output := filepath.Join(dir, "original.go"), filepath.Join(dir, "generated.go")
	bytes := []byte("package p\nvar _ = 0b101\n")
	if err := os.WriteFile(source, bytes, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{{"--go-version=go1.12"}, {"--go-version", "go1.12"}} {
		args := append([]string{"--bashpp", "--source=go"}, flags...)
		args = append(args, source, "-o", output)
		exit, stderr := captureTranspileStderr(t, args)
		if exit != 2 || !strings.Contains(stderr, "binary literal requires go1.13 or later") {
			t.Fatalf("version dropped: exit=%d stderr=%q", exit, stderr)
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("rejected source produced an artifact")
		}
	}
	for _, flags := range [][]string{{"--source=sh", "--go-version=go1.12"}, {"--source=go", "--go-version="}} {
		args := append([]string{"--bashpp"}, flags...)
		exit, _ := captureTranspileStderr(t, append(args, source, "-o", output))
		if exit != 2 {
			t.Fatal("invalid version invocation accepted")
		}
	}
	got, err := os.ReadFile(source)
	if err != nil || string(got) != string(bytes) {
		t.Fatalf("source changed: %v", err)
	}
}
