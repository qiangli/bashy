package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/gomod"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := gomod.Run([]string{"--help"}, &out, &errBuf); code != 0 || out.Len() == 0 {
		t.Fatalf("mod --help code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "mod")
}
