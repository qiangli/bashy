module whiteboard-server

go 1.27

require (
	desk-chart v0.0.0
	desk-kit v0.0.0
)

require (
	github.com/alecthomas/chroma/v2 v2.27.0 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/dlclark/regexp2/v2 v2.2.1 // indirect
	github.com/gorilla/css v1.0.1 // indirect
	github.com/microcosm-cc/bluemonday v1.0.27 // indirect
	github.com/yuin/goldmark v1.8.2 // indirect
	golang.org/x/net v0.26.0 // indirect
)

replace desk-kit => ../../desk/kit

replace desk-chart => ../../desk/chart
