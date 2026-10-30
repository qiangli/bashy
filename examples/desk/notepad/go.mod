module notepad

go 1.27

replace desk-kit => ../kit

require (
	desk-kit v0.0.0-00010101000000-000000000000
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2
	github.com/go-pdf/fpdf v0.9.0
	github.com/ledongthuc/pdf v0.0.0-20261020125853-5e46805f718e
	github.com/russross/blackfriday/v2 v2.1.0
	golang.org/x/image v0.47.0
)

require (
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	golang.org/x/net v0.55.0 // indirect
)
