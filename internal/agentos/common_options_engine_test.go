package agentos

import (
	"bytes"
	"strings"
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

func TestNormalizerAcceptsUnderscore(t *testing.T) {
	_, ran, err := run(t, "sub", "--dry_run")
	if err != nil || !ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
}

func TestExplainNeverRuns(t *testing.T) {
	out, ran, err := run(t, "sub", "--dry-run", "--explain", "x")
	if err != nil || ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
	if !strings.Contains(out, "root sub (not executed)") || !strings.Contains(out, "--dry-run=true") || !strings.Contains(out, "operands: x") {
		t.Fatalf("bad explain: %q", out)
	}
}

func TestNullMapsToZero(t *testing.T) {
	out, err := applyCommonOptions(testTree(new(bool)), []string{"sub", "--null"})
	if err != nil || strings.Join(out, " ") != "sub --zero" {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestUnsupportedExits(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	root.AddCommand(&cobra.Command{Use: "plain", Run: func(*cobra.Command, []string) {}})
	for _, a := range []string{"--json", "--null", "--fields=a"} {
		if _, err := applyCommonOptions(root, []string{"plain", a}); err == nil {
			t.Fatalf("%s should be unsupported", a)
		}
	}
}

func TestOwnFlagWins(t *testing.T) {
	out, err := applyCommonOptions(testTree(new(bool)), []string{"sub", "--json"})
	if err != nil || strings.Join(out, " ") != "sub --json" {
		t.Fatalf("out=%v err=%v", out, err)
	}
}

func TestFieldsProjects(t *testing.T) {
	out, ran, err := run(t, "sub", "--json", "--fields", "a")
	if err != nil || !ran {
		t.Fatalf("ran=%v err=%v", ran, err)
	}
	if strings.Contains(out, `"b"`) || !strings.Contains(out, `"a": 1`) {
		t.Fatalf("bad projection: %q", out)
	}
}

func TestNoInputBlocksConfirmPrompt(t *testing.T) {
	t.Setenv(noInputEnv, "")
	root := testTree(new(bool))
	if _, err := applyCommonOptions(root, []string{"sub", "--no-input"}); err != nil {
		t.Fatal(err)
	}
	defer t.Setenv(noInputEnv, "")
	if _, err := askHumanToConfirm("f", "op", "e"); err == nil {
		t.Fatal("prompt must fail under --no-input")
	}
}
