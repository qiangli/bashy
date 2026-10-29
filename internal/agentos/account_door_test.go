package agentos

import (
	"io"
	"os"
	"strings"
	"testing"
)

func runAccountDoor(t *testing.T, args ...string) (code int) {
	t.Helper()
	oldArgs := os.Args
	os.Args = append([]string{"bashy", "account"}, args...)
	frontDoorObserving.Store(true)
	defer func() {
		frontDoorObserving.Store(false)
		os.Args = oldArgs
	}()
	func() {
		defer func() {
			v := recover()
			if v == nil {
				t.Error("account door returned without exiting")
				return
			}
			c, ok := v.(frontDoorExit)
			if !ok {
				panic(v)
			}
			code = int(c)
		}()
		dispatch()
	}()
	return code
}

func TestDispatchAccountInProcess(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	for _, args := range [][]string{{"schema", "--json"}, {"list", "--json"}} {
		if code := runAccountDoor(t, args...); code != 0 {
			t.Errorf("bashy account %v exited %d, want 0", args, code)
		}
	}
}

func TestAccountIsAlwaysShimmedAndSchemaed(t *testing.T) {
	if cobraVerb("account") == nil {
		t.Fatal("cobraVerb(account) is nil")
	}
	if !isFrontDoorInvocation("account") {
		t.Fatal("account is not a front-door verb")
	}
}

func captureAccountDoor(t *testing.T, args ...string) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	code := runAccountDoor(t, args...)
	os.Stdout = old
	w.Close()
	return <-done, code
}

func TestAccountBuiltinAdaptersLinked(t *testing.T) {
	const sentinel = "S524-SECRET-SENTINEL-do-not-leak"
	t.Setenv("BASHY_HOME", t.TempDir())
	t.Setenv("AWS_SECRET_ACCESS_KEY", sentinel)
	t.Setenv("DIGITALOCEAN_TOKEN", sentinel)

	disc, code := captureAccountDoor(t, "discover", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("account discover --dry-run --json exited %d", code)
	}
	schema, code := captureAccountDoor(t, "schema", "--json")
	if code != 0 {
		t.Fatalf("account schema --json exited %d", code)
	}
	for _, name := range []string{"cloudbox", "llm", "digitalocean", "aws"} {
		if !strings.Contains(schema, `"`+name+`"`) {
			t.Errorf("schema does not list adapter %q", name)
		}
	}
	if strings.Contains(disc+schema, sentinel) {
		t.Error("secret sentinel leaked into account output")
	}
}
