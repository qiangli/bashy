package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// printPDF prints self-contained export HTML to a PDF with headless Chrome
// (the agent's "browser print"), falling back to a minimal PDF stream if no
// browser is available. Humans use the page's Print / PDF button.
// CHROMIUM_PATH / CHROME_PATH overrides discovery.
func printPDF(htmlBytes []byte, out string) error {
	if err := printPDFBrowser(htmlBytes, out); err == nil {
		return nil
	}
	return writeFallbackPDF(htmlBytes, out)
}

func printPDFBrowser(htmlBytes []byte, out string) error {
	chrome := findChrome()
	if chrome == "" {
		return fmt.Errorf("no Chrome/Chromium/Edge found for PDF")
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
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox", "--no-first-run",
		"--no-default-browser-check", "--user-data-dir="+filepath.Join(tmp, "profile"),
		"--no-pdf-header-footer", "--print-to-pdf="+out, "file://"+filepath.ToSlash(src))
	if msg, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chrome print-to-pdf: %v: %s", err, msg)
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return fmt.Errorf("chrome produced no PDF")
	}
	return nil
}

var htmlStripRe = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>|<script[^>]*>.*?</script>|<[^>]+>`)

func writeFallbackPDF(htmlBytes []byte, out string) error {
	plain := htmlStripRe.ReplaceAllString(string(htmlBytes), " ")
	plain = strings.Join(strings.Fields(plain), " ")
	if len(plain) > 200 {
		plain = plain[:200]
	}
	esc := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`).Replace(plain)
	stream := fmt.Sprintf("BT /F1 12 Tf 50 750 Td (%s) Tj ET\n", esc)
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	off1 := b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	off2 := b.Len()
	b.WriteString("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	off3 := b.Len()
	b.WriteString("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n")
	off4 := b.Len()
	fmt.Fprintf(&b, "4 0 obj\n<< /Length %d >>\nstream\n%sendstream\nendobj\n", len(stream), stream)
	off5 := b.Len()
	b.WriteString("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \ntrailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		off1, off2, off3, off4, off5, xref)
	return os.WriteFile(out, b.Bytes(), 0644)
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
