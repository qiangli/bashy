package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// printPDFBrowser prints self-contained export HTML to a PDF with headless
// Chrome, matching examples/desk/viewer/pdf.go. CHROME_PATH (or
// CHROMIUM_PATH) overrides discovery via findChrome.
func printPDFBrowser(htmlBytes []byte, out string) error {
	chrome := findChrome()
	if chrome == "" {
		return fmt.Errorf("no Chrome/Chromium/Edge found for PDF")
	}
	tmp, err := os.MkdirTemp("", "page-pdf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "export.html")
	if err := os.WriteFile(src, htmlBytes, 0600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, chrome, chromePDFArgs(filepath.Join(tmp, "profile"), out, src)...)
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chrome print-to-pdf: %v: %s", err, msg)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return fmt.Errorf("chrome produced no PDF")
	}
	return nil
}

func chromePDFArgs(profileDir, outPath, htmlPath string) []string {
	return []string{
		"--headless=new",
		"--disable-gpu",
		"--no-sandbox",
		"--no-first-run",
		"--no-default-browser-check",
		"--use-mock-keychain",
		"--password-store=basic",
		"--user-data-dir=" + profileDir,
		"--no-pdf-header-footer",
		"--print-to-pdf=" + outPath,
		"file://" + filepath.ToSlash(htmlPath),
	}
}

func findChrome() string {
	for _, env := range []string{"CHROMIUM_PATH", "CHROME_PATH"} {
		if p := os.Getenv(env); p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
			if lp, err := exec.LookPath(p); err == nil {
				return lp
			}
			return ""
		}
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if base != "" {
				candidates = append(candidates,
					filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
					filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "chrome", "msedge"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}
