// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"reflect"
	"testing"

	"github.com/qiangli/bashy/internal/agentos"
	"github.com/qiangli/bashy/internal/cli"
)

func TestDefaultBuildWiresCompleteAgentOS(t *testing.T) {
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"dispatch", cli.AgentOSDispatch, agentos.Dispatch},
		{"wire exec", cli.AgentOSWireExec, agentos.WireExec},
		{"preamble", cli.AgentOSPreamble, agentos.Preamble},
		{"usage", cli.AgentOSUsage, agentos.Usage},
	}
	for _, tt := range tests {
		if reflect.ValueOf(tt.got).Pointer() != reflect.ValueOf(tt.want).Pointer() {
			t.Errorf("default cmd/bashy %s hook is not the AgentOS implementation", tt.name)
		}
	}
}
