package main

import (
	"bytes"
	"testing"
)

func TestHello(t *testing.T) {
	var buf bytes.Buffer
	Hello(&buf)

	got := buf.String()
	want := "Hello, World!\n"

	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
