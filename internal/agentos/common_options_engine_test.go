package agentos

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/weavecli"
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

func TestUnsupportedFieldsNeverRunsBody(t *testing.T) {
	// sub has a native --json but no --fields: engine projection is refused.
	out, ran, err := run(t, "sub", "--json", "--fields", "a")
	if err == nil || ran || out != "" {
		t.Fatalf("ran=%v out=%q err=%v", ran, out, err)
	}
	if !strings.Contains(err.Error(), "--fields unsupported for root sub") {
		t.Fatalf("unclear reason: %v", err)
	}
}

func TestOwnFieldsFlagTakesPrecedence(t *testing.T) {
	var got string
	root := &cobra.Command{Use: "root"}
	sub := &cobra.Command{Use: "sub", RunE: func(c *cobra.Command, _ []string) error {
		got, _ = c.Flags().GetString("fields")
		return nil
	}}
	sub.Flags().String("fields", "", "own fields")
	root.AddCommand(sub)
	out, err := applyCommonOptions(root, []string{"sub", "--fields", "x,y"})
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs(out)
	if err := root.Execute(); err != nil || got != "x,y" {
		t.Fatalf("own --fields not honored: got=%q err=%v", got, err)
	}
}

func TestNoInputIsCommandScoped(t *testing.T) {
	t.Setenv(noInputEnv, "")
	var inside []bool
	var stdin string
	build := func() *cobra.Command {
		root := &cobra.Command{Use: "root"}
		root.AddCommand(&cobra.Command{Use: "sub", RunE: func(c *cobra.Command, a []string) error {
			inside = append(inside, noInputRequested())
			_, askErr := askHumanToConfirm("f", "op", "e")
			if noInputRequested() && askErr == nil {
				t.Error("prompt must fail under --no-input")
			}
			b, _ := io.ReadAll(c.InOrStdin())
			stdin = string(b)
			if len(a) > 0 && a[0] == "fail" {
				return errors.New("boom")
			}
			return nil
		}})
		return root
	}
	exec := func(in string, args ...string) {
		root := build()
		root.SetIn(strings.NewReader(in))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		out, err := applyCommonOptions(root, args)
		if err != nil {
			t.Fatal(err)
		}
		root.SetArgs(out)
		_ = root.Execute()
	}
	exec("payload", "sub", "--no-input")
	if stdin != "payload" {
		t.Fatalf("piped stdin lost: %q", stdin)
	}
	exec("", "sub")
	exec("", "sub", "--no-input", "fail")
	exec("", "sub")
	exec("", "sub", "--no-input=false")
	want := []bool{true, false, true, false, false}
	if fmt.Sprint(inside) != fmt.Sprint(want) {
		t.Fatalf("inside = %v, want %v", inside, want)
	}
	if noInputRequested() || os.Getenv(noInputEnv) != "" {
		t.Fatal("no-input leaked past the command")
	}
}

func TestOptionKeyNormalization(t *testing.T) {
	for _, k := range []string{"dry_run", "DRY_RUN", "dryrun", "dry-run", " Dry_Run "} {
		if got := NormalizeOptionKey(k); got != "dry-run" {
			t.Errorf("%q -> %q", k, got)
		}
	}
	if OptionKey("dry-run") != "dry_run" || OptionKey("dryrun") != "dry_run" {
		t.Fatal("official key spelling must be dry_run")
	}
}

func TestBindOutputFlags(t *testing.T) {
	c := &cobra.Command{Use: "c"}
	c.Flags().Bool("json", false, "own")
	b := BindOutputFlags(c)
	for _, n := range []string{"json", "plain", "quiet"} {
		if c.Flags().Lookup(n) == nil {
			t.Fatalf("missing --%s", n)
		}
	}
	if err := c.ParseFlags([]string{"--plain"}); err != nil {
		t.Fatal(err)
	}
	if got := b.Mode(); got != weavecli.OutputPlain {
		t.Fatalf("mode %v", got)
	}
}

func TestExplainValidatesAndOwnFlagsWin(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	ran := false
	sub := &cobra.Command{Use: "sub", Args: cobra.ExactArgs(1), RunE: func(*cobra.Command, []string) error { ran = true; return nil }}
	sub.Flags().String("fields", "", "own fields")
	root.AddCommand(sub)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	out, err := applyCommonOptions(root, []string{"sub", "--fields", "mine", "--explain"})
	if err != nil {
		t.Fatal(err)
	}
	root.SetArgs(out)
	if err := root.Execute(); err == nil || ran {
		t.Fatalf("explain must still validate operands: err=%v ran=%v", err, ran)
	}
	if strings.Join(out, " ") != "sub --fields mine" {
		t.Fatalf("own --fields must pass through: %v", out)
	}
}
