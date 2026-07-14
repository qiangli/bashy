package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	Hello(os.Stdout)
}

func Hello(w io.Writer) {
	fmt.Fprintln(w, "Hello, World!")
}
