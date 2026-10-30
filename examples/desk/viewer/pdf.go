package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// printPDF prints self-contained export HTML to a PDF with headless Chrome
// (the agent's "browser print"). Humans use the page's Print / PDF button.
// CHROME_PATH overrides discovery.
func printPDF(htmlBytes []byte, out string) error {
	chrome := findChrome()
	if chrome == "" {
		return errors.New("no Chrome/Chromium/Edge found for PDF; set CHROME_PATH, or open the page and use Print / PDF")
	}
	tmp, err := os.MkdirTemp("", "viewer-pdf-")
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
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-first-run",
		"--no-default-browser-check", "--user-data-dir="+filepath.Join(tmp, "profile"),
		"--no-pdf-header-footer", "--print-to-pdf="+out, "file://"+filepath.ToSlash(src))
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chrome print-to-pdf: %v: %s", err, msg)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return errors.New("chrome produced no PDF")
	}
	return nil
}

func findChrome() string {
	if p := os.Getenv("CHROME_PATH"); p != "" {
		return p
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
