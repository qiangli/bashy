package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// Story e1b0b5ae: assert notepad PDF export passes --use-mock-keychain,
// --password-store=basic, and a throwaway --user-data-dir without launching a
// real browser.
func TestBrowserPDFArgsIsolateKeychainAndProfile(t *testing.T) {
	args := browserPDFArgs("/tmp/notepad-pdf-123/profile", "/tmp/out.pdf", "/tmp/notepad-pdf-123/export.html")
	for _, want := range []string{
		"--headless=new",
		"--use-mock-keychain",
		"--password-store=basic",
		"--user-data-dir=/tmp/notepad-pdf-123/profile",
		"--print-to-pdf=/tmp/out.pdf",
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("browserPDFArgs missing %q: %v", want, args)
		}
	}

	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	argsLog := filepath.Join(dir, "argv.txt")
	fakeBrowser := filepath.Join(dir, "fake-chrome.sh")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > '" + argsLog + "'\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in --print-to-pdf=*) printf '%%PDF-1.4\\n' > \"${a#--print-to-pdf=}\" ;; esac\n" +
		"done\n"
	if err := os.WriteFile(fakeBrowser, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CHROMIUM_PATH", fakeBrowser)

	pdfBytes, err := printHTMLToPDF("Title", []byte("<!doctype html><html><body><p>hi</p></body></html>"))
	if err != nil || !strings.HasPrefix(string(pdfBytes), "%PDF-") {
		t.Fatalf("printHTMLToPDF with fake browser: err=%v, bytes=%q", err, string(pdfBytes))
	}
	raw, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	for _, want := range []string{"--use-mock-keychain", "--password-store=basic"} {
		if !slices.Contains(got, want) {
			t.Fatalf("recorded notepad PDF argv missing %q: %v", want, got)
		}
	}
	foundThrowaway := false
	for _, a := range got {
		if strings.HasPrefix(a, "--user-data-dir=") && strings.Contains(a, "notepad-pdf-") {
			foundThrowaway = true
		}
	}
	if !foundThrowaway {
		t.Fatalf("recorded notepad PDF argv missing throwaway notepad-pdf-* user-data-dir: %v", got)
	}
}
