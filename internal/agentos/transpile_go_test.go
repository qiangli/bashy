package agentos

import (
	"context"
	"os"
	"testing"
)

func TestGoVersionMeetsBaseline(t *testing.T) {
	for reported, want := range map[string]bool{
		"go1.27.0":            true,
		"go1.27.1":            true,
		"go1.28":              true,
		"go1.26.5":            false,
		"go1.24.0":            false,
		"devel +abc123":       false,
		"":                    false,
		"go version go1.27.1": false, // GOVERSION is the bare token, never the sentence
	} {
		if goVersionMeetsBaseline(reported) != want {
			t.Errorf("goVersionMeetsBaseline(%q) != %v", reported, want)
		}
	}
}

// An explicit BASHPP_GO is the operator's choice and is never overwritten,
// even by a provisioned toolchain.
func TestEnsureGoForTranspileRespectsInjection(t *testing.T) {
	t.Setenv("BASHPP_GO", "operator-choice")
	ensureGoForTranspile(context.Background())
	if got := os.Getenv("BASHPP_GO"); got != "operator-choice" {
		t.Fatalf("BASHPP_GO rewritten to %q", got)
	}
}
