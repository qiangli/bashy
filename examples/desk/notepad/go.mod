module notepad

go 1.27

replace desk-kit => ../kit

require (
	desk-kit v0.0.0-00010101000000-000000000000
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2
	github.com/ledongthuc/pdf v0.0.0-20261020125853-5e46805f718e
)

require (
	github.com/JohannesKaufmann/dom v0.3.1 // indirect
	golang.org/x/net v0.55.0 // indirect
)
