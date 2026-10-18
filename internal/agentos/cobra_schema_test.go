package agentos

import (
	"reflect"
	"testing"

	"github.com/qiangli/coreutils/tool"
	"github.com/spf13/cobra"
)

func TestWalkCobraSchemas(t *testing.T) {
	root := &cobra.Command{Use: "root", Run: func(*cobra.Command, []string) { t.Fatal("executed root") }}
	root.PersistentFlags().Bool("quiet", false, "quiet")
	root.PersistentFlags().String("scope", "root", "scope")
	child := &cobra.Command{Use: "child ARG", Aliases: []string{"alias"}, Hidden: true}
	child.Flags().Int("scope", 7, "shadow")
	child.Flags().StringSlice("fields", []string{"name"}, "fields")
	child.Flags().Bool("hidden", false, "hidden")
	_ = child.Flags().MarkHidden("hidden")
	root.AddCommand(child)
	child.AddCommand(&cobra.Command{Use: "leaf"})
	got := map[string]tool.ArgSchema{}
	walkCobraSchemas(root, "root", func(path string, schema tool.ArgSchema) { got[path] = schema })
	want := []tool.ArgFlag{{Name: "fields", Type: "stringSlice", Default: "[name]"}, {Name: "quiet", Type: "bool", Default: "false"}, {Name: "scope", Type: "int", Default: "7"}}
	if !reflect.DeepEqual(got["root child"].Flags, want) {
		t.Fatalf("child: %#v", got["root child"])
	}
	if len(got) != 3 {
		t.Fatalf("paths: %v", got)
	}
	if root.Flags().Changed("quiet") || child.Flags().Changed("scope") {
		t.Fatal("changed parsing state")
	}
}

func TestCobraVerbSchemas(t *testing.T) {
	ringDir(t)
	for _, name := range []string{"sprint", "weave", "kb", "tool", "model", "agent", "service", "steward"} {
		schemas := cobraSchemas(name)
		if len(schemas) < 2 {
			t.Errorf("%s has no child schemas", name)
		}
		if _, ok := schemas[name]; !ok {
			t.Errorf("%s has no root schema", name)
		}
	}
	if cobraSchemas("not-a-command") != nil {
		t.Fatal("invented a schema")
	}
}
