package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	"desk-kit/render"
	"github.com/go-pdf/fpdf"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Supported export formats:
//   - pdf: via github.com/go-pdf/fpdf (MIT License)
//   - png: via golang.org/x/image (BSD-3-Clause License)
//   - html: the shared kit renderer (desk-kit/render, goldmark MIT) with
//     assets inlined, so export matches the live preview exactly
//   - txt: standard library
const (
	ExportPDF  = "pdf"
	ExportPNG  = "png"
	ExportHTML = "html"
	ExportTXT  = "txt"
)

// ExportContent converts human-edited text/Markdown into the requested format (pdf, png, html, txt).
func ExportContent(title, body, format string) ([]byte, error) {
	fmtNorm := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
	switch fmtNorm {
	case ExportPDF:
		return renderPDF(title, body)
	case ExportPNG:
		return renderPNG(title, body)
	case ExportHTML, "htm":
		return renderHTML(title, body)
	case ExportTXT, "text", "md", "markdown":
		return renderTXT(title, body)
	default:
		return nil, fmt.Errorf("unsupported export format: %q (supported: pdf, png, html, txt)", format)
	}
}

func renderTXT(title, body string) ([]byte, error) {
	out := body
	if strings.TrimSpace(out) == "" && title != "" {
		out = title
	}
	return []byte(out), nil
}

func renderHTML(title, body string) ([]byte, error) {
	// The same rendered view as the live preview, self-contained:
	// Inline assets become data: URIs and the page carries the strict
	// view CSP with kit CSS/JS only.
	frag, err := render.Markdown([]byte(body), render.Options{Inline: true})
	if err != nil {
		return nil, err
	}
	docTitle := title
	if docTitle == "" {
		docTitle = "Notepad Export"
	}
	return render.Page(docTitle, frag), nil
}

func renderPDF(title, body string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(title, false)
	pdf.AddPage()
	if title != "" {
		pdf.SetFont("Arial", "B", 16)
		pdf.Cell(0, 10, title)
		pdf.Ln(12)
	}
	pdf.SetFont("Arial", "", 12)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		pdf.MultiCell(0, 6, line, "", "", false)
	}
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("generating pdf: %w", err)
	}
	return buf.Bytes(), nil
}

func renderPNG(title, body string) ([]byte, error) {
	lines := strings.Split(body, "\n")
	if title != "" {
		if len(lines) == 0 || lines[0] != title {
			lines = append([]string{title, ""}, lines...)
		}
	}

	lineHeight := 18
	charWidth := 7
	maxLineLen := 0
	for _, l := range lines {
		if len(l) > maxLineLen {
			maxLineLen = len(l)
		}
	}
	if maxLineLen < 40 {
		maxLineLen = 40
	}
	width := maxLineLen*charWidth + 40
	height := len(lines)*lineHeight + 40
	if width < 300 {
		width = 300
	}
	if height < 100 {
		height = 100
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{color.RGBA{R: 30, G: 30, B: 30, A: 255}},
		Face: basicfont.Face7x13,
	}

	y := 24
	for _, line := range lines {
		d.Dot = fixed.P(20, y)
		d.DrawString(line)
		y += lineHeight
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encoding png: %w", err)
	}
	return buf.Bytes(), nil
}
