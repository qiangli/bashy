package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/todo"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := todo.NewTodoCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy todo:", nil)
	if code != 0 {
		t.Fatalf("todo --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("todo --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "todo")
}

func TestStandaloneSprintAndCloseLifecycleHooksWired(t *testing.T) {
	if todo.SprintHandles == nil || todo.SprintChanged == nil || todo.SprintPreflight == nil {
		t.Fatalf("standalone todo adapter missing sprint/weave lifecycle hooks: SprintHandles=%v SprintChanged=%v SprintPreflight=%v",
			todo.SprintHandles != nil, todo.SprintChanged != nil, todo.SprintPreflight != nil)
	}
}
