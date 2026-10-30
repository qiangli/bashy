module notepad

go 1.27

replace desk-kit => ../kit

require (
	desk-kit v0.0.0-00010101000000-000000000000
	github.com/go-pdf/fpdf v0.9.0
	github.com/russross/blackfriday/v2 v2.1.0
	golang.org/x/image v0.47.0
)
