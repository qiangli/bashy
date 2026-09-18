package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/pkg/policy/advice"
	"github.com/qiangli/yoke/pkg/policy/audit"
)

// Helper to run bashy with given env, cap and a script.
func runWithGuard(t *testing.T, script string, env map[string]string, capStr string) (error, *bytes.Buffer) {
	out := new(bytes.Buffer)
	parser := syntax.NewParser()
	prog, err := parser.Parse(bytes.NewBufferString(script), "")
	if err != nil {
		t.Fatal(err)
	}

	environ := os.Environ()
	for k, v := range env {
		t.Setenv(k, v)
		environ = append(environ, k+"="+v)
	}

	runner, err := interp.New(
		interp.Env(nil), // uses os.Environ
		interp.StdIO(nil, out, out),
	)
	if err != nil {
		t.Fatal(err)
	}

	opts := wireExec(nil, false, environ, nil, out, out, false)
	for _, opt := range opts {
		opt(runner)
	}

	ctx := context.Background()
	if capStr != "" {
		if c, err := advice.ParseCap(capStr); err == nil {
			ctx = advice.WithCap(ctx, c)
		} else {
			t.Fatal(err)
		}
	}

	err = runner.Run(ctx, prog)
	return err, out
}

func TestGuardAllowedRun(t *testing.T) {
	// e.g. echo requires no effects
	err, out := runWithGuard(t, "echo hello", nil, "read,write")
	if err != nil {
		t.Fatalf("expected allowed run to succeed, got %v", err)
	}
	if out.String() != "hello\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestGuardBodyNotRun(t *testing.T) {
	cap, err := advice.ParseCap("read")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	handler := auditHandler(nil, audit.Actor{}, "test")(func(context.Context, []string) error {
		called = true
		return nil
	})
	err = handler(advice.WithCap(context.Background(), cap), []string{"curl", "https://example.invalid"})
	if called {
		t.Fatal("denied command executed")
	}
	if err == nil {
		t.Fatal("expected denied run to fail, got success")
	}
	status, ok := exitStatusOf(err)
	if !ok || status == 0 {
		t.Fatalf("expected nonzero exit status, got %d", status)
	}
}

func TestGuardNestedCap(t *testing.T) {
	// Nested cap should intersect
	ctx := context.Background()
	c1, _ := advice.ParseCap("read,write,net")
	ctx = advice.WithCap(ctx, c1)
	c2, _ := advice.ParseCap("read")
	ctx = advice.WithCap(ctx, c2)

	if final, ok := advice.CapFrom(ctx); !ok || len(final.Effects()) != 1 || final.Effects()[0] != "read" {
		t.Fatalf("expected nested cap to intersect, got %v", final.Effects())
	}
}

func TestAuditDenyRecordHashChain(t *testing.T) {
	tmp := t.TempDir()
	auditLog := filepath.Join(tmp, "audit.jsonl")
	env := map[string]string{
		"BASHY_AUDIT": auditLog,
		"BASHY_HOME":  tmp, // Fallback
	}

	// Command requires net, cap is read
	err, _ := runWithGuard(t, "curl http://example.com", env, "read")
	if err == nil {
		t.Fatal("expected deny error")
	}

	f, err := os.Open(auditLog)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Verify hash-chain verification
	res := audit.Verify(f)
	if !res.OK {
		t.Fatalf("audit log verification failed: %s", res.Reason)
	}
	if res.Records != 1 {
		t.Fatalf("expected 1 audit record, got %d", res.Records)
	}

	f.Seek(0, 0)
	var rec audit.Record
	importJson := json.NewDecoder(f)
	if err := importJson.Decode(&rec); err != nil {
		t.Fatal(err)
	}

	if rec.Decision != "deny" {
		t.Fatalf("expected decision 'deny', got %q", rec.Decision)
	}
}
