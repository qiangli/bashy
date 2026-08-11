// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestDispatchCoreutilsToolFetchHelp(t *testing.T) {
	var out, err bytes.Buffer
	code := dispatchCoreutilsTool("fetch", []string{"--help"}, tool.Stdio{
		Out: &out,
		Err: &err,
	})
	if code != 0 {
		t.Fatalf("fetch --help exit = %d, stderr = %q", code, err.String())
	}
	if !strings.Contains(out.String(), "Usage: fetch") {
		t.Fatalf("fetch help missing usage:\n%s", out.String())
	}
}

func TestDispatchCoreutilsAwkPOSIXFloatFormatter(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var out, err bytes.Buffer
	code := dispatchCoreutilsTool("awk", []string{
		`BEGIN {
			printf "<%.*a><%.*F><%.*f>\n", -0.5, 0.1, -0.5, 0.125, -0.5, 0.125
			printf "<%g><%G><%.*g>\n", 4.323232245, 0.00004323232245, -1, 4.323232245
		}`,
	}, tool.Stdio{Out: &out, Err: &err})
	if code != 0 || err.Len() != 0 {
		t.Fatalf("awk exit = %d, stderr = %q", code, err.String())
	}
	if got, want := out.String(), "<0x2p-4><0><0>\n<4.32323><4.32323E-05><4.32323>\n"; got != want {
		t.Fatalf("awk stdout = %q, want %q", got, want)
	}
}

func TestDispatchCoreutilsAwkPOSIXOctalAlternateForm(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var out, err bytes.Buffer
	code := dispatchCoreutilsTool("awk", []string{
		`BEGIN { printf "<%#.o><%#.00o><%#.*o><%.0o>\n", 0, 0, 0, 0, 0 }`,
	}, tool.Stdio{Out: &out, Err: &err})
	if code != 0 || err.Len() != 0 {
		t.Fatalf("awk exit = %d, stderr = %q", code, err.String())
	}
	if got, want := out.String(), "<0><0><0><>\n"; got != want {
		t.Fatalf("awk stdout = %q, want %q", got, want)
	}
}

func TestDispatchCoreutilsAwkPOSIXEREBackend(t *testing.T) {
	t.Setenv("LC_ALL", "C")
	var out, err bytes.Buffer
	input := strings.Repeat("a", 1001) + "\n"
	code := dispatchCoreutilsTool("awk", []string{
		`/^a{1001}$/ { print "literal" }`,
	}, tool.Stdio{In: strings.NewReader(input), Out: &out, Err: &err})
	if code != 0 || err.Len() != 0 {
		t.Fatalf("awk exit = %d, stderr = %q", code, err.String())
	}
	if got, want := out.String(), "literal\n"; got != want {
		t.Fatalf("awk stdout = %q, want %q", got, want)
	}
}

func TestDispatchCoreutilsToolUnknown(t *testing.T) {
	var err bytes.Buffer
	code := dispatchCoreutilsTool("__missing__", nil, tool.Stdio{Err: &err})
	if code != 127 {
		t.Fatalf("missing tool exit = %d, stderr = %q", code, err.String())
	}
	if !strings.Contains(err.String(), "No such command") {
		t.Fatalf("missing tool stderr should explain failure, got %q", err.String())
	}
}
