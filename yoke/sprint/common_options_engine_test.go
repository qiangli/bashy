package main

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func testTree(ran *bool) *cobra.Command {
	root := &cobra.Command{Use: "root"}
	var dry bool
	var zero bool
	sub := &cobra.Command{Use: "sub", Short: "a sub", RunE: func(c *cobra.Command, a []string) error {
		*ran = true
		c.Println(`[{"a":1,"b":2}]`)
		return nil
	}}
	sub.Flags().BoolVar(&dry, "dry-run", false, "preview")
	sub.Flags().BoolVar(&zero, "zero", false, "nul")
	sub.Flags().Bool("json", false, "json")
	root.AddCommand(sub)
	return root
}

func run(t *testing.T, args ...string) (string, bool, error) {
	t.Helper()
	ran := false
	root := testTree(&ran)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetGlobalNormalizationFunc(normalizeFlagName)
	out, err := applyCommonOptions(root, args)
	if err != nil {
		return "", ran, err
	}
	root.SetArgs(out)
	err = root.Execute()
	return buf.String(), ran, err
}
