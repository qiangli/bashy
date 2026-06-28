// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

func TestPreambleDefinesDocker(t *testing.T) {
	src := Preamble()
	f, err := syntax.NewParser().Parse(strings.NewReader(src), "")
	if err != nil {
		t.Fatalf("preamble is not valid shell: %v\n%s", err, src)
	}
	var funcs []string
	for _, st := range f.Stmts {
		if fn, ok := st.Cmd.(*syntax.FuncDecl); ok && fn.Name != nil {
			funcs = append(funcs, fn.Name.Value)
		}
	}
	if len(funcs) == 0 || funcs[0] != "docker" {
		t.Fatalf("preamble should define a docker function, got %v", funcs)
	}
	if !strings.Contains(src, "bashy podman") {
		t.Fatalf("docker should route to `bashy podman`: %q", src)
	}
}
