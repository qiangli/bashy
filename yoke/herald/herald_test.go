package main

import (
	"context"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/herald"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	if code := herald.Run(context.Background(), []string{"--help"}); code != 0 {
		t.Fatalf("herald --help exited %d", code)
	}
	adapter.AssertStandaloneDeps(t, "herald")
}
