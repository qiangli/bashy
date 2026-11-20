package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Story e1b0b5ae: assert viewer PDF export passes --use-mock-keychain,
// --password-store=basic, and a throwaway --user-data-dir without launching a
// real browser.
func TestChromePDFArgsIsolateKeychainAndProfile(t *testing.T) {
	args := chromePDFArgs("/tmp/viewer-pdf-123/profile", "/tmp/out.pdf", "/tmp/viewer-pdf-123/export.html")
	for _, want := range []string{
		"--headless=new",
		"--use-mock-keychain",
		"--password-store=basic",
		"--user-data-dir=/tmp/viewer-pdf-123/profile",
		"--print-to-pdf=/tmp/out.pdf",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("chromePDFArgs missing %q: %v", want, args)
		}
	}

	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	argsLog := filepath.Join(dir, "argv.txt")
	fakeChrome := filepath.Join(dir, "fake-chrome.sh")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + argsLog + "'\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in --print-to-pdf=*) printf '%%PDF-1.4\\n' > \"${a#--print-to-pdf=}\" ;; esac\n" +
		"done\n"
	if err := os.WriteFile(fakeChrome, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROMIUM_PATH", fakeChrome)

	outPDF := filepath.Join(dir, "rendered.pdf")
	if err := printPDFBrowser([]byte("<!doctype html><p>hi</p>"), outPDF); err != nil {
		t.Fatalf("printPDFBrowser with fake chrome: %v", err)
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, want := range []string{"--use-mock-keychain", "--password-store=basic"} {
		if !slices.Contains(got, want) {
			t.Fatalf("recorded viewer PDF argv missing %q: %v", want, got)
		}
	}
	foundThrowaway := false
	for _, a := range got {
		if strings.HasPrefix(a, "--user-data-dir=") && strings.Contains(a, "viewer-pdf-") {
			foundThrowaway = true
		}
	}
	if !foundThrowaway {
		t.Fatalf("recorded viewer PDF argv missing throwaway viewer-pdf-* user-data-dir: %v", got)
	}
}
