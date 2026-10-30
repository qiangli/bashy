package main

import (
	"bytes"
	"encoding/json"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportPDF(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "new", "--id", "pdf-note", "--title", "PDF Note Title", "--body", "This is note content for PDF generation."}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}

	out.Reset()
	outFile := filepath.Join(t.TempDir(), "exported.pdf")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "pdf-note", "--to", "pdf", "-o", outFile}, &out); code != 0 {
		t.Fatalf("export pdf: exit %d: %s", code, out.String())
	}

	exportedPath := strings.TrimSpace(out.String())
	if exportedPath != outFile {
		t.Errorf("expected output path %s, got %s", outFile, exportedPath)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading exported pdf: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF")) {
		prefixLen := len(data)
		if prefixLen > 20 {
			prefixLen = 20
		}
		t.Errorf("PDF output does not start with %%PDF: %q", string(data[:prefixLen]))
	}
}

func TestExportPNG(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "new", "--id", "png-note", "--title", "PNG Note Title", "--body", "Rendering text into PNG via golang.org/x/image."}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}

	out.Reset()
	outFile := filepath.Join(t.TempDir(), "exported.png")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "png-note", "--to", "png", "-o", outFile}, &out); code != 0 {
		t.Fatalf("export png: exit %d: %s", code, out.String())
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading exported png: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("PNG failed to decode with image/png: %v", err)
	}
	if img == nil {
		t.Fatalf("decoded PNG is nil")
	}
}

func TestExportHTML(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	heading := "# Exported HTML Heading"
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "new", "--id", "html-note", "--title", "HTML Note", "--body", heading + "\n\nParagraph text in markdown."}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}

	out.Reset()
	outFile := filepath.Join(t.TempDir(), "exported.html")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "html-note", "--to", "html", "-o", outFile}, &out); code != 0 {
		t.Fatalf("export html: exit %d: %s", code, out.String())
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading exported html: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Exported HTML Heading") || !strings.Contains(content, "<h1>") {
		t.Errorf("HTML output does not contain heading: %s", content)
	}
}

func TestExportTXT(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	bodyText := "Plain text body\nSecond line"
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "new", "--id", "txt-note", "--title", "TXT Note", "--body", bodyText}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}

	out.Reset()
	outFile := filepath.Join(t.TempDir(), "exported.txt")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "txt-note", "--to", "txt", "-o", outFile}, &out); code != 0 {
		t.Fatalf("export txt: exit %d: %s", code, out.String())
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading exported txt: %v", err)
	}
	if string(data) != bodyText {
		t.Errorf("TXT output: got %q, want %q", string(data), bodyText)
	}
}

func TestExportViaAPI(t *testing.T) {
	srvURL, _ := startServer(t, "")

	// 1. Create note
	newReq := `{"action":"new","id":"api-export-note","title":"API Note","body":"# API Note Heading\n\nContent"}`
	res, err := http.Post(srvURL+"/api/action", "application/json", strings.NewReader(newReq))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("new note via API failed: %v, status: %v", err, res.StatusCode)
	}
	res.Body.Close()

	// 2. Export via POST /api/action
	exportReq := `{"action":"export","id":"api-export-note","to":"html"}`
	res, err = http.Post(srvURL+"/api/action", "application/json", strings.NewReader(exportReq))
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("export via POST /api/action failed: %v, status: %v", err, res.StatusCode)
	}
	var exportData map[string]any
	json.NewDecoder(res.Body).Decode(&exportData)
	res.Body.Close()

	pathVal, ok := exportData["path"].(string)
	if !ok || pathVal == "" {
		t.Fatalf("expected path in export response, got: %v", exportData)
	}
	if _, err := os.Stat(pathVal); err != nil {
		t.Fatalf("exported file does not exist at %s: %v", pathVal, err)
	}

	// 3. Export via GET /api/export
	getExportURL := srvURL + "/api/export?id=api-export-note&to=pdf"
	res, err = http.Get(getExportURL)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("export via GET /api/export failed: %v, status: %v", err, res.StatusCode)
	}
	exportData = nil
	json.NewDecoder(res.Body).Decode(&exportData)
	res.Body.Close()
	pathVal, ok = exportData["path"].(string)
	if !ok || pathVal == "" {
		t.Fatalf("expected path in GET /api/export response, got: %v", exportData)
	}
	pdfData, err := os.ReadFile(pathVal)
	if err != nil || !bytes.HasPrefix(pdfData, []byte("%PDF")) {
		prefixLen := len(pdfData)
		if prefixLen > 10 {
			prefixLen = 10
		}
		t.Fatalf("GET /api/export pdf invalid: err=%v, prefix=%q", err, string(pdfData[:prefixLen]))
	}
}
