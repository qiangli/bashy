package agentos

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/qiangli/yoke/pkg/accounts"
	"github.com/qiangli/yoke/pkg/accounts/accountstest"
)

type connectCall struct {
	env  []string
	argv []string
}

func hostAccount(t *testing.T, st *accounts.Store, alias, user, endpoint, ref string, cat accounts.Category) {
	t.Helper()
	_, err := st.Add(accounts.Account{
		Alias: alias, Category: cat, Provider: "ssh", Username: user, Status: accounts.Resolved,
		Connections: []accounts.Connection{{ID: "c1", Endpoint: endpoint, CredentialRef: ref}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func captureStd(t *testing.T, fn func()) string {
	t.Helper()
	r, w, _ := os.Pipe()
	oo, oe := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	done := make(chan string)
	go func() { var b bytes.Buffer; io.Copy(&b, r); done <- b.String() }()
	fn()
	w.Close()
	os.Stdout, os.Stderr = oo, oe
	return <-done
}

func testDeps(st *accounts.Store, calls *[]connectCall, mu *sync.Mutex) connectDeps {
	return connectDeps{
		store: func() *accounts.Store { return st },
		secretGet: func(name string) (string, error) {
			if name == "owl-pw" {
				return accountstest.CanarySecret, nil
			}
			if name == "otter-pw" {
				return "otter-" + accountstest.CanarySecret, nil
			}
			return "", errors.New("boom " + accountstest.CanarySecret)
		},
		transport: func(_ context.Context, env []string, argv ...string) error {
			mu.Lock()
			*calls = append(*calls, connectCall{env, argv})
			mu.Unlock()
			return nil
		},
	}
}

func TestConnectAccount(t *testing.T) {
	st := accounts.Open(filepath.Join(t.TempDir(), "accounts.json"))
	hostAccount(t, st, "owl", "alice", "owl.example:2222", "secret:owl-pw", accounts.HostLogin)
	hostAccount(t, st, "otter", "bob", "otter.example", "secret:otter-pw", accounts.HostLogin)
	hostAccount(t, st, "keyed", "carol", "owl.example", "ssh-key:SHA256:abc", accounts.HostLogin)
	hostAccount(t, st, "nocred", "dave", "owl.example", "", accounts.HostLogin)
	hostAccount(t, st, "badpw", "erin", "owl.example", "secret:missing", accounts.HostLogin)
	hostAccount(t, st, "llm", "alice", "owl.example", "secret:owl-pw", accounts.LLM)

	// a host-login identity is (endpoint, user): a second alias for it is refused
	if _, err := st.Add(accounts.Account{
		Alias: "owl2", Category: accounts.HostLogin, Provider: "ssh", Username: "alice", Status: accounts.Resolved,
		Connections: []accounts.Connection{{ID: "c1", Endpoint: "owl.example:2222", CredentialRef: "secret:owl-pw"}},
	}); err == nil {
		t.Fatal("duplicate endpoint+user alias accepted")
	}
	var calls []connectCall
	var mu sync.Mutex
	deps := testDeps(st, &calls, &mu)

	run := func(args ...string) (int, string) {
		var code int
		out := captureStd(t, func() { code = runConnectWith(context.Background(), args, deps) })
		return code, out
	}

	code, out := run("--account", "owl", "--", "uptime", "-p")
	if code != 0 || len(calls) != 1 {
		t.Fatalf("code %d calls %d out %s", code, len(calls), out)
	}
	if want := []string{"ssh", "--", "alice@owl.example:2222", "uptime", "-p"}; !slices.Equal(calls[0].argv, want) {
		t.Fatalf("argv %q", calls[0].argv)
	}
	if want := []string{"OUTPOST_SSH_PASSWORD=" + accountstest.CanarySecret}; !slices.Equal(calls[0].env, want) {
		t.Fatalf("env %q", calls[0].env)
	}
	accountstest.AssertNoSecret(t, out, calls[0].argv)
	if os.Getenv("OUTPOST_SSH_PASSWORD") != "" {
		t.Fatal("secret leaked into parent env")
	}

	// matching explicit host (with and without user) is allowed
	calls = nil
	for _, h := range []string{"owl.example:2222", "alice@owl.example:2222"} {
		if code, _ := run("--account=owl", h); code != 0 {
			t.Fatalf("matching host %s rejected", h)
		}
	}
	// key credential: no env
	calls = nil
	if code, _ := run("--account", "keyed"); code != 0 || len(calls[0].env) != 0 {
		t.Fatalf("keyed: %d %v", code, calls)
	}

	// failures exit 2 before any transport call
	calls = nil
	for _, args := range [][]string{
		{"--account", "owl", "other.example"},
		{"--account", "owl", "bob@owl.example:2222"},
		{"--account", "nocred"},
		{"--account", "badpw"},
		{"--account", "llm"},
		{"--account", "nosuch"},
	} {
		code, out := run(args...)
		if code != 2 || !strings.Contains(out, "error=") {
			t.Fatalf("%v: code %d out %q", args, code, out)
		}
		accountstest.AssertNoSecret(t, out)
	}
	if len(calls) != 0 {
		t.Fatalf("transport called on failure: %v", calls)
	}

	// no --account: unchanged, no env
	if code, _ := run("owl.example", "--", "id"); code != 0 || len(calls[0].env) != 0 || !slices.Equal(calls[0].argv, []string{"ssh", "--", "owl.example", "id"}) {
		t.Fatalf("plain: %v", calls)
	}
}

func TestConnectAccountConcurrentEnvIsolation(t *testing.T) {
	st := accounts.Open(filepath.Join(t.TempDir(), "accounts.json"))
	hostAccount(t, st, "owl", "alice", "owl.example", "secret:owl-pw", accounts.HostLogin)
	hostAccount(t, st, "otter", "bob", "otter.example", "secret:otter-pw", accounts.HostLogin)
	var calls []connectCall
	var mu sync.Mutex
	deps := testDeps(st, &calls, &mu)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		for _, a := range []string{"owl", "otter"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runConnectWith(context.Background(), []string{"--account", a}, deps)
			}()
		}
	}
	wg.Wait()
	for _, c := range calls {
		owl := c.argv[2] == "alice@owl.example"
		pw := c.env[0] == "OUTPOST_SSH_PASSWORD="+accountstest.CanarySecret
		if owl != pw || len(c.env) != 1 {
			t.Fatalf("crossed env: %v", c)
		}
	}
	if len(calls) != 40 {
		t.Fatalf("calls %d", len(calls))
	}
}

func TestChildEnvReplacesPassword(t *testing.T) {
	got := childEnv([]string{"A=1", "OUTPOST_SSH_PASSWORD=old"}, []string{"OUTPOST_SSH_PASSWORD=new"})
	if !slices.Equal(got, []string{"A=1", "OUTPOST_SSH_PASSWORD=new"}) {
		t.Fatal(got)
	}
}

func TestAccountResolveForConnectIsSafe(t *testing.T) {
	st := accounts.Open(filepath.Join(t.TempDir(), "accounts.json"))
	hostAccount(t, st, "owl", "alice", "owl.example", "secret:owl-pw", accounts.HostLogin)
	cmd := accounts.NewAccountCmd(accounts.WithStore(st))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"resolve", "owl", "--for", "connect"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "owl.example") || strings.Contains(s, "OUTPOST_SSH_PASSWORD") {
		t.Fatalf("unexpected output %q", s)
	}
	accountstest.AssertNoSecret(t, s)
}
