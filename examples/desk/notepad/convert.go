package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/ledongthuc/pdf"
)

// Supported content formats
const (
	FormatText = "txt"
	FormatMD   = "md"
	FormatHTML = "html"
	FormatJSON = "json"
	FormatPDF  = "pdf"
)

// DetectFormat detects format from filename extension or magic bytes/content.
func DetectFormat(data []byte, filenameOrExt string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filenameOrExt), "."))
	if ext == "" {
		ext = strings.ToLower(strings.TrimSpace(filenameOrExt))
	}
	switch ext {
	case "pdf":
		return FormatPDF
	case "html", "htm":
		return FormatHTML
	case "json":
		return FormatJSON
	case "md", "markdown":
		return FormatMD
	case "txt", "text":
		return FormatText
	}

	// Content sniffing / magic bytes
	trimmed := bytes.TrimSpace(data)
	if bytes.HasPrefix(trimmed, []byte("%PDF-")) {
		return FormatPDF
	}
	if (bytes.HasPrefix(trimmed, []byte("{")) && bytes.HasSuffix(trimmed, []byte("}"))) ||
		(bytes.HasPrefix(trimmed, []byte("[")) && bytes.HasSuffix(trimmed, []byte("]"))) {
		var js any
		if json.Unmarshal(trimmed, &js) == nil {
			return FormatJSON
		}
	}
	lower := strings.ToLower(string(trimmed))
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") ||
		strings.Contains(lower, "<p>") || strings.Contains(lower, "</div>") || strings.Contains(lower, "<body") {
		return FormatHTML
	}
	return FormatText
}

// ConvertContent converts raw data in the specified format (or auto-detected) into text/Markdown.
func ConvertContent(data []byte, formatOrFilename string) (string, string, error) {
	format := DetectFormat(data, formatOrFilename)
	switch format {
	case FormatPDF:
		r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", format, fmt.Errorf("reading pdf: %w", err)
		}
		pt, err := r.GetPlainText()
		if err != nil {
			return "", format, fmt.Errorf("extracting text from pdf: %w", err)
		}
		b, err := io.ReadAll(pt)
		if err != nil {
			return "", format, fmt.Errorf("reading extracted text: %w", err)
		}
		return strings.TrimSpace(string(b)), format, nil

	case FormatHTML:
		md, err := htmltomarkdown.ConvertString(string(data))
		if err != nil {
			return "", format, fmt.Errorf("converting html to markdown: %w", err)
		}
		return strings.TrimSpace(md), format, nil

	case FormatJSON:
		var buf bytes.Buffer
		if err := json.Indent(&buf, data, "", "  "); err != nil {
			var v any
			if err := json.Unmarshal(data, &v); err != nil {
				return "", format, fmt.Errorf("invalid json: %w", err)
			}
			pretty, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "", format, fmt.Errorf("formatting json: %w", err)
			}
			return string(pretty), format, nil
		}
		return strings.TrimSpace(buf.String()), format, nil

	case FormatMD, FormatText:
		fallthrough
	default:
		return string(data), format, nil
	}
}
