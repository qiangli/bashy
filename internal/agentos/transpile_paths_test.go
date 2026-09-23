package agentos

import (
	"strings"
	"testing"
)

func TestTranspilePathArgsClassifiesPathValues(t *testing.T) {
	args := []string{
		"--bashsharp", "--source", "go", "--go-version=1.27",
		"--go-import-base", "example", "--go-import-path=example/app",
		"--go-file", "/c/src/main.go", "--go-test-file=/c/src/main_test.go",
		"--go-package", "example/lib=/c/src/lib.go,/d/shared/more.go",
		"/c/src/app.go", "-o/c/out/app.go", "--map=/d/maps/app.map",
	}
	got := transpilePathArgsMode(args, `C:\work`, true)
	joined := strings.Join(got, "\n")
	for _, unchanged := range []string{"go", "1.27", "example", "example/app"} {
		if !strings.Contains(joined, unchanged) {
			t.Fatalf("identifier %q disappeared from %#v", unchanged, got)
		}
	}
	for _, shellPath := range []string{
		"/c/src/main.go", "/c/src/main_test.go", "/c/src/lib.go",
		"/d/shared/more.go", "/c/src/app.go", "/c/out/app.go", "/d/maps/app.map",
	} {
		if strings.Contains(joined, shellPath) {
			t.Fatalf("shell path %q was not converted in %#v", shellPath, got)
		}
	}
	if args[8] != "/c/src/main.go" {
		t.Fatalf("caller arguments were mutated: %#v", args)
	}
}

func TestTranspilePathArgsIsIdentityOffWindows(t *testing.T) {
	args := []string{"--bashsharp", "/c/src/app.bsh", "-o", "/tmp/app.go"}
	got := transpilePathArgsMode(args, "/work", false)
	if &got[0] != &args[0] {
		t.Fatal("non-Windows conversion copied an unchanged argument slice")
	}
}
