package main

import (
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
)

func TestStandaloneDeps(t *testing.T) {
	adapter.AssertStandaloneDeps(t, "outpost", "github.com/qiangli/yoke/external/meshagent")
}
